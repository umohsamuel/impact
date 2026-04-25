package adapters

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umohsamuel/impact/internals/configs/env"
	"github.com/umohsamuel/impact/internals/configs/logger"
	"github.com/umohsamuel/impact/internals/infrastructures/db/gen"
)

type AdapterDependencies struct {
	Logger               logger.Logger
	EnvironmentVariables *env.EnvironmentVariables
	DB                   *pgxpool.Pool
}

type Adapters struct {
	Logger               logger.Logger
	EnvironmentVariables *env.EnvironmentVariables
	Queries              *gen.Queries
}

func NewAdapters(dependencies AdapterDependencies) *Adapters {
	return &Adapters{
		Logger:               dependencies.Logger,
		EnvironmentVariables: dependencies.EnvironmentVariables,
		Queries:              gen.New(dependencies.DB),
	}
}
