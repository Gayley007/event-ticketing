// Package postgres opens a connection pool to a service's own database.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds the connection details, all sourced from environment variables.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

// URL builds a postgres:// connection string from the config.
func (c Config) URL() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     net.JoinHostPort(c.Host, c.Port),
		Path:     c.Name,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

// Connect opens a pool and waits (up to ~30s) for the database to accept
// connections, so a service that starts slightly before its database does
// not crash.
func Connect(ctx context.Context, cfg Config, log *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.URL())
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	const attempts = 15
	for i := 1; i <= attempts; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			log.Info("connected to database", "host", cfg.Host, "db", cfg.Name)
			return pool, nil
		}
		log.Warn("database not ready, retrying", "attempt", i, "err", err)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		}
	}
	pool.Close()
	return nil, fmt.Errorf("database %s unreachable: %w", cfg.Host, err)
}
