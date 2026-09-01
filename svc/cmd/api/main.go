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
	apiOnce    sync.Once
	apiAdapter *chiadapter.ChiLambda
	apiInitErr error
	apiLog     *slog.Logger
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	apiOnce.Do(func() {
		apiLog = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		cfg, err := configuration.Load()
		if err != nil {
			apiLog.Error("api init failed", "err", err)
			apiInitErr = err
			return
		}
		runtime, err := composition.Build(ctx, apiLog, cfg, composition.Options{
			EnableJobStore: true,
			LeaseOwner:     "api",
		})
		if err != nil {
			apiLog.Error("api init failed", "err", err)
			apiInitErr = err
			return
		}
		apiAdapter = chiadapter.New(runtime.ChiRouter)
	})
	if apiInitErr != nil {
		// Init errors previously returned without CORS headers, so browsers reported a
		// CORS failure and hid the 500 body. Echo allowlisted Origin so the client can
		// surface {"error":"service unavailable"} (and check CloudWatch for the real err).
		if apiLog != nil {
			apiLog.Error("api unavailable", "method", req.HTTPMethod, "path", req.Path, "err", apiInitErr)
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
	return apiAdapter.ProxyWithContext(ctx, req)
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
