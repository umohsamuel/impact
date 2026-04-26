package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

const maxFileSize = 1000 << 20

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

	sampleRate := 1.0
	if sr := c.PostForm("sample_rate"); sr != "" {
		if parsed, err := strconv.ParseFloat(sr, 64); err == nil {
			sampleRate = parsed
		}
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

	// 3. Extract frames with FFmpeg
	framesDir := filepath.Join(tempDir, "frames")
	extracted, err := videoService.ExtractFrames(videoService.FrameExtractionConfig{
		InputPath:  inputPath,
		OutputDir:  framesDir,
		SampleRate: sampleRate,
		MaxWidth:   1280,
		MaxHeight:  720,
		Quality:    3,
	})
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(err)).Send(c)
		return
	}

	log.Printf("[pipeline] Extracted %d frames from video", extracted.FrameCount)

	// 4. Upload extracted frames to Gemini
	sort.Strings(extracted.Paths)
	var llmFiles []domainLLM.File
	for _, path := range extracted.Paths {
		llmFiles = append(llmFiles, domainLLM.File{
			Path:     path,
			MIMEType: "image/jpeg",
		})
	}

	log.Printf("[pipeline] Uploading %d frames to Gemini...", len(llmFiles))
	uploadedFiles, err := h.llm.UploadFiles(llmFiles)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to upload frames: %w", err))).Send(c)
		return
	}

	// 5. Build prompt with system instructions + video metadata
	systemPrompt := loadSystemPrompt()

	var frameInfo strings.Builder
	for i, path := range extracted.Paths {
		frameNum := extractFrameNumber(filepath.Base(path))
		timestampMs := int64(float64(frameNum-1) / sampleRate * 1000)
		fmt.Fprintf(&frameInfo, "- Image %d: frame_index=%d, timestamp=%dms\n", i+1, frameNum-1, timestampMs)
	}

	userPrompt := fmt.Sprintf(`Analyze these video frames for impact moments.

Video metadata:
- FPS: %.2f
- Total frames: %d
- Duration: %dms
- Sample rate: %.1f frames/sec
- Sampled frames: %d

Frame mapping (in order of provided images):
%s`, extracted.FPS, extracted.TotalFrames, extracted.DurationMs, sampleRate, extracted.FrameCount, frameInfo.String())

	fullPrompt := systemPrompt + "\n\n---\n\n" + userPrompt

	log.Printf("[pipeline] Calling Gemini for impact analysis...")
	llmResponse, err := h.llm.GenerateText(fullPrompt, false, uploadedFiles)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("LLM analysis failed: %w", err))).Send(c)
		return
	}

	log.Printf("[pipeline] LLM response received (cost: $%.6f)", llmResponse.Dollars)

	// 6. Parse LLM JSON response
	var impactAnalysis domainVideo.ImpactAnalysisResponse
	jsonStr := extractJSON(llmResponse.Response)
	if err := json.Unmarshal([]byte(jsonStr), &impactAnalysis); err != nil {
		log.Printf("[pipeline] Raw LLM response:\n%s", llmResponse.Response)
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to parse LLM response: %w", err))).Send(c)
		return
	}

	log.Printf("[pipeline] Detected %d impact moments", impactAnalysis.VideoAnalysis.TotalImpactsDetected)

	// 7. If no impacts, return original video
	if len(impactAnalysis.ImpactFrames) == 0 {
		response.NewSuccessResponse("No impact moments detected", gin.H{
			"download_url": fmt.Sprintf("/downloads/%s/%s", sessionID, file.Filename),
			"analysis":     impactAnalysis,
			"llm_cost":     llmResponse.Dollars,
		}, nil).Send(c)
		return
	}

	// 8. Apply FFmpeg effects at impact moments
	meta, err := videoService.GetVideoMetadata(inputPath)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to get video metadata: %w", err))).Send(c)
		return
	}

	log.Printf("[pipeline] Applying impact effects to video (%dx%d, %.1ffps)...", meta.Width, meta.Height, meta.FPS)
	outputPath, err := videoService.ApplyImpactEffects(inputPath, tempDir, impactAnalysis.ImpactFrames, meta)
	if err != nil {
		response.NewErrorResponse(errors.InternalServerError(fmt.Errorf("failed to apply effects: %w", err))).Send(c)
		return
	}

	outputFilename := filepath.Base(outputPath)
	downloadURL := fmt.Sprintf("/downloads/%s/%s", sessionID, outputFilename)

	log.Printf("[pipeline] Done! Download at: %s", downloadURL)

	response.NewSuccessResponse("Impact video generated successfully", gin.H{
		"download_url": downloadURL,
		"analysis":     impactAnalysis,
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

func extractFrameNumber(filename string) int {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	parts := strings.Split(name, "_")
	if len(parts) < 2 {
		return 1
	}
	num, _ := strconv.Atoi(parts[len(parts)-1])
	return num
}
