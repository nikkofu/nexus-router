package httpapi

import (
	"net/http"

	"github.com/nikkofu/nexus-router/internal/auth"
)

func RequireBearer(resolver auth.Resolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := auth.ParseBearer(r.Header.Get("Authorization"))
		if !ok {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		if _, ok := resolver.ResolveBearer(token); !ok {
			http.Error(w, "invalid bearer token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
