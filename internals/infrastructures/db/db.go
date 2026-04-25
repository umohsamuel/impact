package db

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context) (*pgxpool.Pool, error) {
	connStr := os.Getenv("DB_URL")
	return pgxpool.New(ctx, connStr)
}
