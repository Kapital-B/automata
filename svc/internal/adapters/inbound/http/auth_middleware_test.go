package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	"github.com/google/uuid"
)

var testJWTSecret = []byte("abcdefghijklmnopqrstuvwxyz123456")

func testBearerToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	token, err := security.SignJWT(testJWTSecret, userID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func authedGet(t *testing.T, url string, userID uuid.UUID) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+testBearerToken(t, userID))
	return http.DefaultClient.Do(req)
}

func authedPost(t *testing.T, url, body string, userID uuid.UUID) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testBearerToken(t, userID))
	return http.DefaultClient.Do(req)
}

func TestAuthMiddlewareRejectsMissingAndInvalidTokens(t *testing.T) {
	userID := uuid.New()
	called := 0
	handler := authMiddleware(testJWTSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		uid, ok := UserIDFromContext(r.Context())
		if !ok || uid != userID {
			t.Fatalf("authenticated user = %s, present = %v", uid, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, header := range []string{"", "Bearer", "Bearer invalid", "Basic abc", "Bearer " + testBearerToken(t, uuid.Nil)} {
		req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized || called != 0 {
			t.Fatalf("header %q: status %d, handler calls %d", header, res.Code, called)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+testBearerToken(t, userID))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || called != 1 {
		t.Fatalf("valid token: status %d, handler calls %d", res.Code, called)
	}
}

func TestPublicRoutesStayAccessibleWithoutBearerToken(t *testing.T) {
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/health"},
		{http.MethodPost, "/api/auth/register"},
		{http.MethodPost, "/api/auth/login"},
		{http.MethodPost, "/api/auth/refresh"},
		{http.MethodGet, "/api/auth/microsoft"},
		{http.MethodGet, "/api/auth/microsoft/callback"},
		{http.MethodGet, "/api/auth/google"},
		{http.MethodGet, "/api/auth/google/callback"},
		{http.MethodGet, "/api/accounts/callback"},
		{http.MethodGet, "/api/accounts/google/callback"},
		{http.MethodGet, "/api/connectors/callback"},
	} {
		called := false
		handler := authMiddleware(testJWTSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			if _, ok := UserIDFromContext(r.Context()); ok {
				t.Fatal("public route injected a user")
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(route.method, route.path, nil))
		if !called || res.Code != http.StatusNoContent {
			t.Errorf("%s %s: status %d, called %v", route.method, route.path, res.Code, called)
		}
	}
}

func TestPublicRouteMatchIsMethodSpecific(t *testing.T) {
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/accounts"},
		{http.MethodPost, "/api/health"},
		{http.MethodGet, "/api/auth/login"},
		{http.MethodPost, "/api/accounts/callback"},
	} {
		res := httptest.NewRecorder()
		authMiddleware(testJWTSecret)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("protected route ran without a token")
		})).ServeHTTP(res, httptest.NewRequest(route.method, route.path, nil))
		if res.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d", route.method, route.path, res.Code)
		}
	}
}

func TestCORSPreflightDoesNotRequireBearerToken(t *testing.T) {
	h := &Handlers{JWTSecret: testJWTSecret, CORSOrigins: []string{"https://example.com"}}
	req := httptest.NewRequest(http.MethodOptions, "/api/accounts", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	res := httptest.NewRecorder()
	h.Routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Fatalf("preflight status %d, allow origin %q", res.Code, res.Header().Get("Access-Control-Allow-Origin"))
	}
}
