package httpapi

import (
	"context"
	"net/http"

	"github.com/nikkofu/nexus-router/internal/auth"
)

type clientPolicyContextKey struct{}

func WithClientPolicy(ctx context.Context, policy auth.ClientPolicy) context.Context {
	return context.WithValue(ctx, clientPolicyContextKey{}, policy)
}

func ClientPolicyFromContext(ctx context.Context) (auth.ClientPolicy, bool) {
	policy, ok := ctx.Value(clientPolicyContextKey{}).(auth.ClientPolicy)
	return policy, ok
}

func RequireBearer(resolver auth.Resolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := auth.ParseBearer(r.Header.Get("Authorization"))
		if !ok {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		policy, ok := resolver.ResolveBearer(token)
		if !ok {
			http.Error(w, "invalid bearer token", http.StatusUnauthorized)
			return
		}

		r = r.WithContext(WithClientPolicy(r.Context(), policy))
		next.ServeHTTP(w, r)
	})
}
