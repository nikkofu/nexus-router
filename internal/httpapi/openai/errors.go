package openai

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, code int, typ, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorBody{
			Type:    typ,
			Message: message,
		},
	})
}
