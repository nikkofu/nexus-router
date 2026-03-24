package handlers

import (
	"net/http"

	openaiapi "github.com/nikkofu/nexus-router/internal/httpapi/openai"
	"github.com/nikkofu/nexus-router/internal/streaming"
)

func Responses(exec ExecuteService, policyReader PolicyReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy, ok := policyReader(r.Context())
		if !ok {
			openaiapi.WriteError(w, http.StatusUnauthorized, "auth_error", "missing client policy")
			return
		}

		req, err := openaiapi.DecodeResponsesRequest(r.Body)
		if err != nil {
			writeDecodeError(w, err)
			return
		}

		result, _, err := exec.Execute(r.Context(), policy, req)
		if err != nil {
			writeExecutionError(w, err)
			return
		}

		if req.Stream {
			setStreamingHeaders(w)
			if err := streaming.WriteResponsesSSE(w, result.Events); err != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}

		writeJSON(w, openaiapi.FinalizeResponse(result.Events, req.PublicModel))
	})
}
