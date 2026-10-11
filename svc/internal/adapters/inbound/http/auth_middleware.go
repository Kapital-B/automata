package http

import (
	"context"
	"net/http"
	"strings"

	domainai "github.com/Kapital-B/automata/svc/internal/domain/ai"
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	"github.com/google/uuid"
)

type ctxKeyUserID int

const userIDKey ctxKeyUserID = 1

// UserIDFromContext returns the authenticated user id set by authMiddleware.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(userIDKey).(uuid.UUID)
	return v, ok
}

func authMiddleware(verifier domainauth.AccessTokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if publicRoute(r.Method, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			auth := r.Header.Get("Authorization")
			parts := strings.Fields(auth)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			uid, err := verifyAccessToken(verifier, parts[1])
			if err != nil || uid == uuid.Nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, uid)
			// Model calls made while serving the request are metered against
			// the caller.
			ctx = domainai.WithUsageScope(ctx, &uid, nil)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func publicRoute(method, path string) bool {
	switch path {
	case "/api/health":
		return method == http.MethodGet
	case "/api/auth/register", "/api/auth/login", "/api/auth/refresh":
		return method == http.MethodPost
	case "/api/auth/microsoft", "/api/auth/microsoft/callback",
		"/api/auth/google", "/api/auth/google/callback",
		"/api/accounts/callback", "/api/accounts/google/callback", "/api/connectors/callback":
		return method == http.MethodGet
	default:
		return false
	}
}

func verifyAccessToken(verifier domainauth.AccessTokenVerifier, token string) (uuid.UUID, error) {
	if verifier == nil {
		return uuid.Nil, domainauth.ErrInvalidToken
	}
	return verifier.Verify(token)
}
