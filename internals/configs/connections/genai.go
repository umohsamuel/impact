package connections

import (
	"context"
	"log"

	"github.com/umohsamuel/impact/internals/configs/env"
	"google.golang.org/genai"
)

func NewGoogleGenAIClient(env *env.EnvironmentVariables) *genai.Client {
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: env.Gemini.APIKEY,
	})
	if err != nil {
		log.Fatalf("failed to create Google GenAI client: %v", err)
	}
	return client
}
