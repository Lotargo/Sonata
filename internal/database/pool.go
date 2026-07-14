package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Lotargo/Sonata/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RuntimeURL(cfg *config.RuntimeConfig) (string, error) {
	if cfg == nil {
		return "", errors.New("runtime config is required")
	}
	return resolveURL(cfg, cfg.Storage.Database.URLRef, "runtime database")
}

func DirectURL(cfg *config.RuntimeConfig) (string, error) {
	if cfg == nil {
		return "", errors.New("runtime config is required")
	}
	return resolveURL(cfg, cfg.Storage.Database.DirectURLRef, "direct database")
}

func resolveURL(cfg *config.RuntimeConfig, ref, label string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("%s secret reference is required", label)
	}
	value, ok := cfg.Secret(ref)
	if !ok || value.Empty() {
		return "", fmt.Errorf("%s secret %q is unresolved", label, ref)
	}
	url := strings.TrimSpace(value.Reveal())
	if url == "" {
		return "", fmt.Errorf("%s URL is empty", label)
	}
	return url, nil
}

// OpenRuntimePool opens the pooled application connection. DATABASE_URL may
// temporarily point to the same direct endpoint used by migrations in local
// development, but staging and production should use a Neon pooled endpoint.
func OpenRuntimePool(ctx context.Context, cfg *config.RuntimeConfig) (*pgxpool.Pool, error) {
	url, err := RuntimeURL(cfg)
	if err != nil {
		return nil, err
	}
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse runtime database URL: %w", err)
	}

	databaseConfig := cfg.Storage.Database
	poolConfig.MinConns = int32(databaseConfig.MinConnections)
	poolConfig.MaxConns = int32(databaseConfig.MaxConnections)
	poolConfig.MaxConnLifetime = databaseConfig.MaxConnLifetime.Value()
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	poolConfig.ConnConfig.RuntimeParams["application_name"] = cfg.App.ServiceName
	poolConfig.ConnConfig.RuntimeParams["search_path"] = "sonata,public"

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open runtime database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping runtime database: %w", err)
	}
	return pool, nil
}
