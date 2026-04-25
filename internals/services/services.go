package services

import (
	"github.com/umohsamuel/impact/internals/infrastructures/adapters"
	"github.com/umohsamuel/impact/internals/infrastructures/db/gen"
)

type Services struct {
	Queries *gen.Queries
}

func NewServices(adapters *adapters.Adapters) *Services {
	return &Services{
		Queries: adapters.Queries,
	}
}
