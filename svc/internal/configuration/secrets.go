package configuration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

var (
	secretsMu     sync.Mutex
	secretsCache  = map[string]string{}
	secretsClient *secretsmanager.Client

	errEmptySecretString = errors.New("empty secret string")
)

// resolveSecret returns the plain env value when set; otherwise loads from
// Secrets Manager using the companion *_SECRET_ID env var.
func resolveSecret(plainEnv, secretIDEnv string) (string, error) {
	if v := os.Getenv(plainEnv); v != "" {
		return v, nil
	}
	secretID := os.Getenv(secretIDEnv)
	if secretID == "" {
		return "", nil
	}
	return getSecretValue(secretID)
}

// resolveOptionalSecret is like resolveSecret, but treats an empty Secrets Manager
// container (no AWSCURRENT value yet) as "" so optional providers can soft-disable.
func resolveOptionalSecret(plainEnv, secretIDEnv string) (string, error) {
	if v := os.Getenv(plainEnv); v != "" {
		return v, nil
	}
	secretID := os.Getenv(secretIDEnv)
	if secretID == "" {
		return "", nil
	}
	value, err := getSecretValue(secretID)
	if err == nil {
		return value, nil
	}
	if isUnpopulatedSecret(err) {
		slog.Warn("optional secret not populated; continuing without it",
			"env", plainEnv,
			"secret_id", secretID,
			"err", err,
		)
		return "", nil
	}
	return "", err
}

func isUnpopulatedSecret(err error) bool {
	var notFound *smtypes.ResourceNotFoundException
	if errors.As(err, &notFound) {
		return true
	}
	return errors.Is(err, errEmptySecretString) ||
		strings.Contains(err.Error(), "has no SecretString") ||
		strings.Contains(err.Error(), "AWSCURRENT")
}

func getSecretValue(secretID string) (string, error) {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	if v, ok := secretsCache[secretID]; ok {
		return v, nil
	}
	client, err := secretsManagerClient()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return "", fmt.Errorf("secretsmanager GetSecretValue %q: %w", secretID, err)
	}
	if out.SecretString == nil || *out.SecretString == "" {
		return "", fmt.Errorf("secretsmanager secret %q has no SecretString; populate it manually after terraform apply: %w", secretID, errEmptySecretString)
	}
	secretsCache[secretID] = *out.SecretString
	return *out.SecretString, nil
}

func secretsManagerClient() (*secretsmanager.Client, error) {
	if secretsClient != nil {
		return secretsClient, nil
	}
	region := getenv("AWS_REGION", "eu-west-2")
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}
	if endpoint := os.Getenv("AWS_ENDPOINT"); endpoint != "" {
		opts = append(opts,
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "test")),
		)
	}
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config for secretsmanager: %w", err)
	}
	clientOpts := []func(*secretsmanager.Options){}
	if endpoint := os.Getenv("AWS_ENDPOINT"); endpoint != "" {
		clientOpts = append(clientOpts, func(o *secretsmanager.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	secretsClient = secretsmanager.NewFromConfig(cfg, clientOpts...)
	return secretsClient, nil
}
