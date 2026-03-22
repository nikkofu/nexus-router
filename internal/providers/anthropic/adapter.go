package anthropic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/providers"
)

type Adapter struct {
	client *http.Client
}

type ClassifiedError struct {
	StatusCode int
	Retryable  bool
	Message    string
}

func (e *ClassifiedError) Error() string {
	return fmt.Sprintf("anthropic upstream error: status=%d retryable=%t message=%s", e.StatusCode, e.Retryable, e.Message)
}

func NewAdapter(client *http.Client) *Adapter {
	if client == nil {
		client = http.DefaultClient
	}

	return &Adapter{client: client}
}

func (a *Adapter) Execute(ctx context.Context, upstream config.ProviderConfig, req canonical.Request) (providers.Result, error) {
	body, err := EncodeRequest(req)
	if err != nil {
		return providers.Result{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL(upstream.BaseURL), bytes.NewReader(body))
	if err != nil {
		return providers.Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	authHeaders, err := providers.ProviderAuthHeaders(upstream)
	if err != nil {
		return providers.Result{}, &providers.ExecutionError{
			Err:             err,
			Retryable:       false,
			OutputCommitted: false,
		}
	}
	for key, values := range authHeaders {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return providers.Result{}, &ClassifiedError{
			Retryable: true,
			Message:   err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(resp.Body)
		return providers.Result{}, &ClassifiedError{
			StatusCode: resp.StatusCode,
			Retryable:  resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout,
			Message:    string(payload),
		}
	}

	events, err := DecodeStream(req.EndpointKind, resp.Body)
	if err != nil {
		var streamErr *StreamDecodeError
		if errors.As(err, &streamErr) {
			return providers.Result{}, &providers.ExecutionError{
				Err:             fmt.Errorf("anthropic stream decode failed: %w", streamErr.Err),
				Retryable:       true,
				OutputCommitted: streamErr.OutputCommitted,
			}
		}

		return providers.Result{}, &providers.ExecutionError{
			Err:             err,
			Retryable:       true,
			OutputCommitted: false,
		}
	}

	return providers.Result{Events: events}, nil
}

func endpointURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/v1/messages"
}
