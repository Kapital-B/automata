package factory

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dsql/auth"
)

var dsqlIdentRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// dsqlAuthenticator mints an Aurora DSQL IAM auth token on demand.
//
// The token is deliberately not part of the DSN. It lives about 15 minutes,
// while a *sql.DB opens new physical connections for as long as the process
// runs — so a token frozen into the DSN authenticates the first connections
// and then fails every later one with "access denied (SQLSTATE 08006)". That
// surfaces wherever a container outlives the token, which for a once-a-minute
// scheduler is always, and for a low-traffic API only under load.
//
// awsCfg.Credentials is a credentials cache, so this also picks up rotated
// Lambda role credentials rather than pinning the ones present at startup.
type dsqlAuthenticator struct {
	username string
	endpoint string
	region   string
	creds    aws.CredentialsProvider
}

func newDSQLAuthenticator(ctx context.Context, databaseURL string) (*dsqlAuthenticator, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("database url missing host")
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
		return nil, fmt.Errorf("DSQL_REGION or AWS_REGION is required for dsql auth")
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config for dsql auth: %w", err)
	}
	return &dsqlAuthenticator{
		username: username,
		endpoint: endpoint,
		region:   region,
		creds:    awsCfg.Credentials,
	}, nil
}

// Token returns a freshly signed auth token. Called for every new connection.
func (a *dsqlAuthenticator) Token(ctx context.Context) (string, error) {
	var (
		token string
		err   error
	)
	if strings.EqualFold(a.username, "admin") {
		token, err = auth.GenerateDBConnectAdminAuthToken(ctx, a.endpoint, a.region, a.creds)
	} else {
		token, err = auth.GenerateDbConnectAuthToken(ctx, a.endpoint, a.region, a.creds)
	}
	if err != nil {
		return "", fmt.Errorf("generate dsql auth token: %w", err)
	}
	return token, nil
}

// dsqlDSN normalises the database URL for DSQL: TLS, and the app schema on the
// search path because custom roles cannot be granted usage on public.
//
// Any password in the URL is dropped — it is supplied per connection instead.
func dsqlDSN(databaseURL string) (string, error) {
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
	u.User = url.User(username)

	schema := strings.TrimSpace(os.Getenv("DSQL_SCHEMA"))
	if schema == "" {
		schema = "automata"
	}
	if !dsqlIdentRE.MatchString(schema) {
		return "", fmt.Errorf("invalid DSQL_SCHEMA %q", schema)
	}
	q := u.Query()
	if q.Get("sslmode") == "" {
		q.Set("sslmode", "require")
	}
	q.Set("search_path", schema+",public")
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
