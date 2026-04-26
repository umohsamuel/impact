package services

import (
	"github.com/umohsamuel/impact/internals/infrastructures/adapters"
	"github.com/umohsamuel/impact/internals/infrastructures/db/gen"
	domainLLM "github.com/umohsamuel/impact/internals/infrastructures/domain/llm"
)

type Services struct {
	Queries           *gen.Queries
	LLMImplementation domainLLM.Interface
}

func NewServices(adapters *adapters.Adapters) *Services {
	return &Services{
		Queries:           adapters.Queries,
		LLMImplementation: adapters.LLMImplementation,
	}
}
