package main

import (
	"context"
	"log/slog"
	"os"
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
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	apiOnce.Do(func() {
		log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		cfg, err := configuration.Load()
		if err != nil {
			log.Error("api init failed", "err", err)
			apiInitErr = err
			return
		}
		runtime, err := composition.Build(ctx, log, cfg, composition.Options{
			EnableJobStore: true,
			LeaseOwner:     "api",
		})
		if err != nil {
			log.Error("api init failed", "err", err)
			apiInitErr = err
			return
		}
		apiAdapter = chiadapter.New(runtime.ChiRouter)
	})
	if apiInitErr != nil {
		// Never echo init/config errors to clients — they may include secret names and AWS details.
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"service unavailable"}`,
		}, nil
	}
	return apiAdapter.ProxyWithContext(ctx, req)
}
