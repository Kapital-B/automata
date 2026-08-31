package factory

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dsql/auth"
)

// dsqlTokenLifetime is the default IAM auth token lifetime used by the AWS SDK.
const dsqlTokenLifetime = 15 * time.Minute

// withDSQLAuthToken injects a fresh Aurora DSQL IAM auth token as the password.
// Without this, pgx sends an empty/missing password and DSQL returns
// "invalid password packet size".
func withDSQLAuthToken(ctx context.Context, databaseURL string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database url: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("database url missing host")
	}
	username := "admin"
	if u.User != nil {
		if name := strings.TrimSpace(u.User.Username()); name != "" {
			username = name
		}
	}
	endpoint := strings.TrimSpace(os.Getenv("DSQL_CLUSTER_ENDPOINT"))
	if endpoint == "" {
		endpoint = u.Hostname()
	}
	region := firstNonEmpty(
		os.Getenv("DSQL_REGION"),
		os.Getenv("AWS_REGION"),
		os.Getenv("AWS_DEFAULT_REGION"),
	)
	if region == "" {
		return "", fmt.Errorf("DSQL_REGION or AWS_REGION is required for dsql auth")
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("load aws config for dsql auth: %w", err)
	}

	var token string
	if strings.EqualFold(username, "admin") {
		token, err = auth.GenerateDBConnectAdminAuthToken(ctx, endpoint, region, awsCfg.Credentials)
	} else {
		token, err = auth.GenerateDbConnectAuthToken(ctx, endpoint, region, awsCfg.Credentials)
	}
	if err != nil {
		return "", fmt.Errorf("generate dsql auth token: %w", err)
	}

	u.User = url.UserPassword(username, token)
	q := u.Query()
	if q.Get("sslmode") == "" {
		q.Set("sslmode", "require")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
