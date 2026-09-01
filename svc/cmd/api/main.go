package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/Kapital-B/automata/svc/internal/composition"
	"github.com/Kapital-B/automata/svc/internal/configuration"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	chiadapter "github.com/awslabs/aws-lambda-go-api-proxy/chi"
)

var (
	apiMu      sync.Mutex
	apiAdapter *chiadapter.ChiLambda
	apiLog     *slog.Logger
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	adapter, err := ensureAPI(ctx)
	if err != nil {
		if apiLog != nil {
			apiLog.Error("api unavailable", "method", req.HTTPMethod, "path", req.Path, "err", err)
		}
		headers := map[string]string{"Content-Type": "application/json"}
		applyCORSHeaders(headers, req)
		if strings.EqualFold(req.HTTPMethod, "OPTIONS") {
			return events.APIGatewayProxyResponse{StatusCode: 204, Headers: headers}, nil
		}
		return events.APIGatewayProxyResponse{
			StatusCode: 503,
			Headers:    headers,
			Body:       `{"error":"service unavailable"}`,
		}, nil
	}
	return adapter.ProxyWithContext(ctx, req)
}

// ensureAPI initializes once on success. Failures are not sticky so the next
// invoke retries — important when migrate IAM grants land after a warm start.
func ensureAPI(ctx context.Context) (*chiadapter.ChiLambda, error) {
	apiMu.Lock()
	defer apiMu.Unlock()
	if apiAdapter != nil {
		return apiAdapter, nil
	}

	apiLog = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := configuration.Load()
	if err != nil {
		apiLog.Error("api init failed", "err", err)
		return nil, err
	}
	runtime, err := composition.Build(ctx, apiLog, cfg, composition.Options{
		EnableJobStore: true,
		LeaseOwner:     "api",
	})
	if err != nil {
		apiLog.Error("api init failed", "err", err)
		return nil, err
	}
	apiAdapter = chiadapter.New(runtime.ChiRouter)
	return apiAdapter, nil
}

func applyCORSHeaders(headers map[string]string, req events.APIGatewayProxyRequest) {
	origin := headerValue(req.Headers, "Origin")
	if origin == "" {
		return
	}
	for _, candidate := range strings.Split(os.Getenv("CORS_ORIGINS"), ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), origin) {
			headers["Access-Control-Allow-Origin"] = origin
			headers["Access-Control-Allow-Methods"] = "GET,POST,PUT,PATCH,DELETE,OPTIONS"
			headers["Access-Control-Allow-Headers"] = "Accept,Content-Type,X-Request-ID,Authorization"
			headers["Access-Control-Expose-Headers"] = "X-Next-Cursor"
			headers["Access-Control-Max-Age"] = "300"
			headers["Vary"] = "Origin"
			return
		}
	}
}

func headerValue(headers map[string]string, key string) string {
	if headers == nil {
		return ""
	}
	if v, ok := headers[key]; ok && v != "" {
		return v
	}
	for k, v := range headers {
		if strings.EqualFold(k, key) && v != "" {
			return v
		}
	}
	return ""
}
