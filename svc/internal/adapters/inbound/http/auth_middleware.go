package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	"github.com/google/uuid"
)

type ctxKeyUserID int

const userIDKey ctxKeyUserID = 1

// UserIDFromContext returns the authenticated user id (set by optionalAuthMiddleware).
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(userIDKey).(uuid.UUID)
	return v, ok
}

func optionalAuthMiddleware(secret []byte, defaultUserID uuid.UUID) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			var uid uuid.UUID
			var ok bool
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				raw := strings.TrimSpace(auth[7:])
				if raw != "" {
					parsed, err := security.ParseJWT(secret, raw)
					if err == nil {
						uid = parsed
						ok = true
					}
				}
			}
			if !ok {
				uid = defaultUserID
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, userIDKey, uid)))
		})
	}
}
