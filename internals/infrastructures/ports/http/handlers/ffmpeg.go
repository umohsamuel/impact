package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/umohsamuel/impact/internals/configs/env"
	"github.com/umohsamuel/impact/internals/configs/errors"
	configFile "github.com/umohsamuel/impact/internals/configs/file"
	"github.com/umohsamuel/impact/internals/configs/response"
	videoService "github.com/umohsamuel/impact/internals/infrastructures/adapters/video"
	domainLLM "github.com/umohsamuel/impact/internals/infrastructures/domain/llm"
	domainVideo "github.com/umohsamuel/impact/internals/infrastructures/domain/video"
)

type Handler struct {
	environmentVariables *env.EnvironmentVariables
	llm                  domainLLM.Interface
}

func NewFFMPEGHandler(environmentVariables *env.EnvironmentVariables, llm domainLLM.Interface) Handler {
	return Handler{
		environmentVariables: environmentVariables,
		llm:                  llm,
	}
}

const (
	maxFileSize = 1000 << 20
	sampleRate  = 10.0
)

func (h *Handler) ExtractFrames(c *gin.Context) {
	// 1. Receive the uploaded video
	file, err := c.FormFile("video")
	if err != nil {
		response.NewErrorResponse(errors.BadRequest(fmt.Errorf("no video file provided"))).Send(c)
		return
	}

	if file.Size > maxFileSize {
		response.NewErrorResponse(errors.BadRequest(fmt.Errorf(
			"file size %d bytes exceeds the %d byte limit",
			file.Size, maxFileSize,
		))).Send(c)
		return
	}

	// 2. Save uploaded file to temp location
	sessionID := fmt.Sprintf("%d", time.Now().UnixNano())
	tempDir := filepath.Join("tmp", sessionID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to create temp dir"))).Send(c)
		return
	}

	inputPath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, inputPath); err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to save upload"))).Send(c)
		return
	}

	// 3. Extract frames at 10fps with numbered labels burned in
	framesDir := filepath.Join(tempDir, "labeled_frames")
	extracted, err := videoService.ExtractAndLabelFrames(inputPath, framesDir, sampleRate)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(err)).Send(c)
		return
	}

	log.Printf("[pipeline] Extracted %d labeled frames at %.0ffps", extracted.FrameCount, sampleRate)

	// 4. Create 5x4 grids from labeled frames
	gridsDir := filepath.Join(tempDir, "grids")
	gridPaths, err := videoService.CreateGridImages(extracted.Paths, gridsDir)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to create grids: %w", err))).Send(c)
		return
	}

	log.Printf("[pipeline] Created %d grid images from %d frames", len(gridPaths), extracted.FrameCount)

	// 5. Upload grid images to Gemini
	var llmFiles []domainLLM.File
	for _, gp := range gridPaths {
		llmFiles = append(llmFiles, domainLLM.File{
			Path:     gp,
			MIMEType: "image/jpeg",
		})
	}

	log.Printf("[pipeline] Uploading %d grid images to Gemini...", len(llmFiles))
	uploadedFiles, err := h.llm.UploadFiles(llmFiles)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to upload grids: %w", err))).Send(c)
		return
	}

	// 6. Build prompt
	systemPrompt := loadSystemPrompt()

	userPrompt := fmt.Sprintf(`Analyze these video frame grids for key action moments.

Each grid is a 5x2 layout (5 columns, 2 rows) of sequential frames.
Each frame has a number label in the top-left corner -- that is the frame number.
Frames are extracted at %.0f frames per second from the original video.

Video info:
- Original FPS: %.2f
- Duration: %dms
- Total labeled frames: %d
- Grids provided: %d (10 frames per grid)

Return ONLY the JSON object with frame numbers where key action moments occur.`,
		sampleRate, extracted.FPS, extracted.DurationMs, extracted.FrameCount, len(gridPaths))

	fullPrompt := systemPrompt + "\n\n---\n\n" + userPrompt

	log.Printf("[pipeline] Calling Gemini for impact analysis...")
	llmResponse, err := h.llm.GenerateText(fullPrompt, false, uploadedFiles)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("LLM analysis failed: %w", err))).Send(c)
		return
	}

	log.Printf("[pipeline] LLM response received (cost: $%.6f)", llmResponse.Dollars)

	// 7. Parse LLM response -- just an array of frame numbers
	var impactResponse domainVideo.ImpactResponse
	jsonStr := extractJSON(llmResponse.Response)
	if err := json.Unmarshal([]byte(jsonStr), &impactResponse); err != nil {
		log.Printf("[pipeline] Raw LLM response:\n%s", llmResponse.Response)
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to parse LLM response: %w", err))).Send(c)
		return
	}

	log.Printf("[pipeline] Detected %d impact frames: %v", len(impactResponse.Impacts), impactResponse.Impacts)

	// 8. If no impacts, return original video
	if len(impactResponse.Impacts) == 0 {
		response.NewSuccessResponse("No impact moments detected", gin.H{
			"download_url": fmt.Sprintf("/downloads/%s/%s", sessionID, file.Filename),
			"impacts":      impactResponse.Impacts,
			"llm_cost":     llmResponse.Dollars,
		}, nil).Send(c)
		return
	}

	// 9. Build impact video: apply B&W+invert to impact frames +/- 1
	meta, err := videoService.GetVideoMetadata(inputPath)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to get video metadata: %w", err))).Send(c)
		return
	}

	log.Printf("[pipeline] Building impact video (%dx%d, %.1ffps)...", meta.Width, meta.Height, meta.FPS)
	outputPath, err := videoService.BuildImpactVideo(inputPath, tempDir, impactResponse.Impacts, sampleRate, meta)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to build impact video: %w", err))).Send(c)
		return
	}

	outputFilename := filepath.Base(outputPath)
	downloadURL := fmt.Sprintf("/downloads/%s/%s", sessionID, outputFilename)

	log.Printf("[pipeline] Done! Download at: %s", downloadURL)

	response.NewSuccessResponse("Impact video generated successfully", gin.H{
		"download_url": downloadURL,
		"impacts":      impactResponse.Impacts,
		"llm_cost":     llmResponse.Dollars,
	}, nil).Send(c)
}

func loadSystemPrompt() string {
	rootPath := configFile.GetRootPath()
	data, err := os.ReadFile(filepath.Join(rootPath, "internals", "configs", "prompts", "system.md"))
	if err != nil {
		log.Printf("Warning: could not load system prompt: %v", err)
		return ""
	}
	return string(data)
}

func extractJSON(s string) string {
	if start := strings.Index(s, "```json"); start != -1 {
		s = s[start+7:]
		if end := strings.Index(s, "```"); end != -1 {
			return strings.TrimSpace(s[:end])
		}
	}
	if start := strings.Index(s, "```"); start != -1 {
		s = s[start+3:]
		if end := strings.Index(s, "```"); end != -1 {
			return strings.TrimSpace(s[:end])
		}
	}

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start != -1 && end > start {
		return s[start : end+1]
	}
	return s
}
