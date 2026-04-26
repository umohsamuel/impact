package llm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/umohsamuel/impact/internals/configs/env"
	"github.com/umohsamuel/impact/internals/infrastructures/domain/llm"
	"github.com/umohsamuel/impact/pkg/utils"
	"google.golang.org/genai"
)

type ModelPricing struct {
	TextInputPerMillion   float64
	AudioInputPerMillion  float64
	TextOutputPerMillion  float64
	AudioOutputPerMillion float64
}

var modelPricing = map[string]ModelPricing{
	"gemini-3.1-pro-preview":                        {TextInputPerMillion: 2.00, TextOutputPerMillion: 12.00},
	"gemini-3-pro-preview":                          {TextInputPerMillion: 2.00, TextOutputPerMillion: 12.00},
	"gemini-3-flash-preview":                        {TextInputPerMillion: 0.50, AudioInputPerMillion: 1.00, TextOutputPerMillion: 3.00},
	"gemini-2.5-pro":                                {TextInputPerMillion: 1.25, TextOutputPerMillion: 10.00},
	"gemini-2.5-flash":                              {TextInputPerMillion: 0.30, AudioInputPerMillion: 1.00, TextOutputPerMillion: 2.50},
	"gemini-2.5-flash-preview-tts":                  {TextInputPerMillion: 0.50, TextOutputPerMillion: 2.50, AudioOutputPerMillion: 10.00},
	"gemini-3.1-flash-live-preview":                 {TextInputPerMillion: 0.75, TextOutputPerMillion: 4.50, AudioOutputPerMillion: 12.00},
	"gemini-2.5-pro-preview-tts":                    {TextInputPerMillion: 1.00, TextOutputPerMillion: 10.00, AudioOutputPerMillion: 20.00},
	"gemini-2.5-flash-native-audio-preview-12-2025": {TextInputPerMillion: 0.50, AudioInputPerMillion: 3.00, TextOutputPerMillion: 2.00, AudioOutputPerMillion: 12.00},
	"gemini-2.0-flash":                              {TextInputPerMillion: 0.10, AudioInputPerMillion: 0.70, TextOutputPerMillion: 0.40},
	"gemini-2.0-flash-lite":                         {TextInputPerMillion: 0.075, TextOutputPerMillion: 0.30},
	"gemini-2.5-flash-lite":                         {TextInputPerMillion: 0.10, AudioInputPerMillion: 0.30, TextOutputPerMillion: 0.40},
	"gemini-2.5-flash-lite-preview-09-2025":         {TextInputPerMillion: 0.10, AudioInputPerMillion: 0.30, TextOutputPerMillion: 0.40},
	"gemini-2.5-flash-preview-09-2025":              {TextInputPerMillion: 0.30, AudioInputPerMillion: 1.00, TextOutputPerMillion: 2.50},
}

type GoogleAI struct {
	client *genai.Client
	config *env.GoogleGenerativeAI
}

func NewGoogleAI(client *genai.Client, config *env.GoogleGenerativeAI) llm.Interface {
	return &GoogleAI{client: client, config: config}
}

func (g *GoogleAI) UploadFiles(files []llm.File) ([]llm.UploadedFile, error) {
	ctx := context.Background()
	if len(files) == 0 {
		return nil, nil
	}

	type result struct {
		index int
		file  llm.UploadedFile
		err   error
	}

	// Limit concurrency to 5 to avoid rate-limiting / nil responses
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	ch := make(chan result, len(files))

	for i, f := range files {
		sem <- struct{}{}
		go func(i int, f llm.File) {
			defer func() { <-sem }()
			uf, err := g.uploadFile(ctx, f)
			ch <- result{index: i, file: uf, err: err}
		}(i, f)
	}

	results := make([]result, len(files))
	for range files {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		results[r.index] = r
	}

	uploaded := make([]llm.UploadedFile, len(files))
	for i, r := range results {
		uploaded[i] = r.file
	}
	return uploaded, nil
}

func (g *GoogleAI) uploadFile(ctx context.Context, f llm.File) (llm.UploadedFile, error) {
	if g.client == nil {
		return llm.UploadedFile{}, errors.New("genai client is nil")
	}
	uploaded, err := utils.WithRetry(func() (*genai.File, error) {
		return g.client.Files.UploadFromPath(ctx, f.Path, &genai.UploadFileConfig{
			MIMEType: f.MIMEType,
		})
	}, 10, 300*time.Millisecond)
	if err != nil {
		return llm.UploadedFile{}, fmt.Errorf("upload file: %w", err)
	}
	if uploaded == nil {
		return llm.UploadedFile{}, errors.New("uploaded file is nil")
	}
	if uploaded.URI == "" || uploaded.MIMEType == "" {
		return llm.UploadedFile{}, errors.New("uploaded file missing URI or MIME")
	}
	return llm.UploadedFile{URI: uploaded.URI, MIMEType: uploaded.MIMEType}, nil
}

func (g *GoogleAI) GenerateText(prompt string, useFastModel bool, uploadedFiles []llm.UploadedFile) (*llm.Response, error) {
	ctx := context.Background()

	var parts []*genai.Part

	for _, uf := range uploadedFiles {
		active, err := g.waitForFileActive(ctx, uf.URI, 30*time.Second)
		if err != nil {
			return nil, fmt.Errorf("wait for file active: %w", err)
		}
		if !active {
			return nil, errors.New("file failed to become active")
		}
		parts = append(parts, genai.NewPartFromURI(uf.URI, uf.MIMEType))
	}

	parts = append(parts, genai.NewPartFromText(prompt))

	model := g.config.Model
	if useFastModel {
		model = g.config.FastModel
	}

	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	genConfig := &genai.GenerateContentConfig{
		ThinkingConfig: &genai.ThinkingConfig{
			ThinkingLevel: genai.ThinkingLevelHigh,
		},
		Tools: []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		},
	}

	response, err := g.client.Models.GenerateContent(ctx, model, contents, genConfig)
	if err != nil {
		return nil, fmt.Errorf("generate content: %w", err)
	}

	text := response.Text()
	if text == "" {
		return nil, errors.New("failed to generate text: empty response")
	}

	dollars := g.calculateCost(model, response, "text")
	return &llm.Response{Response: text, Dollars: dollars}, nil
}

func (g *GoogleAI) waitForFileActive(ctx context.Context, fileURI string, maxWait time.Duration) (bool, error) {

	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		file, err := g.client.Files.Get(ctx, fileURI, nil)
		if err != nil {
			return false, fmt.Errorf("get file: %w", err)
		}
		switch file.State {
		case genai.FileStateActive:
			return true, nil
		case genai.FileStateFailed:
			return false, errors.New("file processing failed")
		}

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return false, nil
}

func (g *GoogleAI) calculateCost(model string, response *genai.GenerateContentResponse, costType string) float64 {
	pricing, ok := modelPricing[model]
	if !ok {
		return 0
	}
	if response.UsageMetadata == nil {
		return 0
	}

	inputTokens := float64(response.UsageMetadata.PromptTokenCount)
	outputTokens := float64(response.UsageMetadata.CandidatesTokenCount)

	inputCost := (inputTokens / 1_000_000) * pricing.TextInputPerMillion

	outputCostPerMillion := pricing.TextOutputPerMillion
	if costType == "audio" && pricing.AudioOutputPerMillion > 0 {
		outputCostPerMillion = pricing.AudioOutputPerMillion
	}
	outputCost := (outputTokens / 1_000_000) * outputCostPerMillion

	return inputCost + outputCost
}
