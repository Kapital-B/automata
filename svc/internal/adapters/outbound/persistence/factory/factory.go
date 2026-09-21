package factory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Engine string

const (
	EngineSQLite   Engine = "sqlite"
	EnginePostgres Engine = "postgres"
	EngineDSQL     Engine = "dsql"
)

type Config struct {
	Engine           Engine
	DatabaseURL      string
	MaxOpenConns     int
	MaxIdleConns     int
	ConnMaxLifetime  time.Duration
	EnableForeignKey bool
}

func ParseEngine(raw string) (Engine, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", string(EngineSQLite):
		return EngineSQLite, nil
	case string(EnginePostgres):
		return EnginePostgres, nil
	case string(EngineDSQL):
		return EngineDSQL, nil
	default:
		return "", fmt.Errorf("unsupported database engine %q", raw)
	}
}

func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	engine, err := ParseEngine(string(cfg.Engine))
	if err != nil {
		return nil, err
	}
	driverName, err := driverFor(engine)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, fmt.Errorf("database url is required")
	}

	var db *sql.DB
	if engine == EngineDSQL {
		db, err = openDSQL(ctx, cfg.DatabaseURL)
	} else {
		db, err = sql.Open(driverName, cfg.DatabaseURL)
	}
	if err != nil {
		return nil, err
	}
	if cfg.MaxOpenConns <= 0 {
		switch engine {
		case EngineSQLite:
			cfg.MaxOpenConns = 1
		default:
			cfg.MaxOpenConns = 3
		}
	}
	if cfg.MaxIdleConns < 0 {
		cfg.MaxIdleConns = 0
	}
	if cfg.ConnMaxLifetime <= 0 && engine != EngineSQLite {
		cfg.ConnMaxLifetime = 55 * time.Minute
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if engine == EngineSQLite && cfg.EnableForeignKey {
		if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

// openDSQL builds a pool that signs a fresh IAM auth token for every physical
// connection. The token must not live in the DSN: it expires in ~15 minutes,
// and database/sql reuses one DSN for the life of the process, so a frozen
// token means every connection opened after it expires is refused.
func openDSQL(ctx context.Context, databaseURL string) (*sql.DB, error) {
	dsn, err := dsqlDSN(databaseURL)
	if err != nil {
		return nil, err
	}
	authn, err := newDSQLAuthenticator(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsql dsn: %w", err)
	}
	return stdlib.OpenDB(*connConfig, stdlib.OptionBeforeConnect(
		func(ctx context.Context, cc *pgx.ConnConfig) error {
			token, err := authn.Token(ctx)
			if err != nil {
				return err
			}
			cc.Password = token
			return nil
		},
	)), nil
}

func driverFor(engine Engine) (string, error) {
	switch engine {
	case EngineSQLite:
		return "sqlite", nil
	case EnginePostgres, EngineDSQL:
		return "pgx", nil
	default:
		return "", fmt.Errorf("unsupported database engine %q", engine)
	}
}
