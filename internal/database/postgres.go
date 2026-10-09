package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ pool *pgxpool.Pool }

func OpenPostgres(ctx context.Context, url string, maxConns int) (*Postgres, error) {
	if url == "" {
		return nil, errors.New("database: HS_DATABASE_URL is empty")
	}
	if maxConns < 1 {
		maxConns = 1
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = int32(maxConns)
	cfg.MinConns = 1
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.MaxConnLifetime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(connectCtx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	if p == nil || p.pool == nil {
		return errors.New("database: pool is nil")
	}
	return p.pool.Ping(ctx)
}

func (p *Postgres) Close() {
	if p != nil && p.pool != nil {
		p.pool.Close()
	}
}

// Pool 暴露底层连接池供 dbstore 等使用。
func (p *Postgres) Pool() *pgxpool.Pool {
	if p == nil {
		return nil
	}
	return p.pool
}
