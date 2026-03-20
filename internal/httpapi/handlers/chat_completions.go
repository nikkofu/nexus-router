package handlers

import (
	"net/http"

	openaiapi "github.com/nikkofu/nexus-router/internal/httpapi/openai"
	"github.com/nikkofu/nexus-router/internal/streaming"
)

func ChatCompletions(exec ExecuteService, policyReader PolicyReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy, ok := policyReader(r.Context())
		if !ok {
			openaiapi.WriteError(w, http.StatusUnauthorized, "auth_error", "missing client policy")
			return
		}

		req, err := openaiapi.DecodeChatCompletionRequest(r.Body)
		if err != nil {
			openaiapi.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		result, _, err := exec.Execute(r.Context(), policy, req)
		if err != nil {
			writeExecutionError(w, err)
			return
		}

		if req.Stream {
			setStreamingHeaders(w)
			if err := streaming.WriteChatCompletionSSE(w, result.Events); err != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}

		writeJSON(w, openaiapi.FinalizeChatCompletion(result.Events, req.PublicModel))
	})
}
