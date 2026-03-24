package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/canonical"
	openaiapi "github.com/nikkofu/nexus-router/internal/httpapi/openai"
	"github.com/nikkofu/nexus-router/internal/providers"
	"github.com/nikkofu/nexus-router/internal/service"
)

type ExecuteService interface {
	Execute(ctx context.Context, policy auth.ClientPolicy, req canonical.Request) (providers.Result, []string, error)
}

type PolicyReader func(context.Context) (auth.ClientPolicy, bool)

func setStreamingHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if strings.HasPrefix(err.Error(), "unsupported_capability:") {
		openaiapi.WriteError(w, http.StatusBadRequest, "unsupported_capability", err.Error())
		return
	}

	openaiapi.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
}

func writeExecutionError(w http.ResponseWriter, err error) {
	var execErr *providers.ExecutionError

	switch {
	case errors.Is(err, service.ErrUnsupportedCapability):
		openaiapi.WriteError(w, http.StatusBadRequest, "unsupported_capability", err.Error())
	case strings.HasPrefix(err.Error(), "unsupported_capability:"):
		openaiapi.WriteError(w, http.StatusBadRequest, "unsupported_capability", err.Error())
	case errors.As(err, &execErr):
		openaiapi.WriteError(w, http.StatusBadGateway, "upstream_error", "upstream request failed")
	default:
		openaiapi.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	}
}
