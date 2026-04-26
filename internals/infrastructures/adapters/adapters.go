package adapters

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umohsamuel/impact/internals/configs/env"
	"github.com/umohsamuel/impact/internals/infrastructures/adapters/llm"
	"github.com/umohsamuel/impact/internals/infrastructures/db/gen"
	domainLLM "github.com/umohsamuel/impact/internals/infrastructures/domain/llm"
	"google.golang.org/genai"
)

type AdapterDependencies struct {
	EnvironmentVariables *env.EnvironmentVariables
	DB                   *pgxpool.Pool
	GoogleGenAIClient    *genai.Client
}

type Adapters struct {
	EnvironmentVariables *env.EnvironmentVariables
	Queries              *gen.Queries
	LLMImplementation    domainLLM.Interface
}

func NewAdapters(dependencies AdapterDependencies) *Adapters {
	return &Adapters{
		EnvironmentVariables: dependencies.EnvironmentVariables,
		Queries:              gen.New(dependencies.DB),
		LLMImplementation:    llm.NewGoogleAI(dependencies.GoogleGenAIClient, dependencies.EnvironmentVariables.Gemini),
	}
}
