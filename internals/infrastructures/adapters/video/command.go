package handlers

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	gridCols      = 5
	gridRows      = 4
	framesPerGrid = gridCols * gridRows
)

type VideoMetadata struct {
	FPS         float64
	DurationMs  int64
	TotalFrames int64
	Width       int
	Height      int
}

type ExtractedFrames struct {
	Paths       []string
	FrameCount  int
	FPS         float64
	DurationMs  int64
	TotalFrames int64
	SampleRate  float64
	OutputDir   string
}

func ExtractAndLabelFrames(inputPath, outputDir string, sampleRate float64) (*ExtractedFrames, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}

	// drawtext burns the sequential frame number into the top-left corner.
	filterStr := fmt.Sprintf(
		"fps=%g,scale=640:-2,drawtext=text='%%{eif\\:n+1\\:d}':x=10:y=10:fontsize=28:fontcolor=white:borderw=2:bordercolor=black",
		sampleRate,
	)
	outputPattern := filepath.Join(outputDir, "frame_%05d.jpg")

	args := []string{
		"-y",
		"-i", inputPath,
		"-vf", filterStr,
		"-q:v", "3",
		"-f", "image2",
		outputPattern,
	}

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg extract+label error: %w\noutput: %s", err, string(output))
	}

	paths, err := filepath.Glob(filepath.Join(outputDir, "frame_*.jpg"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob frames: %w", err)
	}
	sort.Strings(paths)

	meta, err := GetVideoMetadata(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	return &ExtractedFrames{
		Paths:       paths,
		FrameCount:  len(paths),
		FPS:         meta.FPS,
		DurationMs:  meta.DurationMs,
		TotalFrames: meta.TotalFrames,
		SampleRate:  sampleRate,
		OutputDir:   outputDir,
	}, nil
}

func CreateGridImages(framePaths []string, outputDir string) ([]string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create grid dir: %w", err)
	}

	var gridPaths []string
	total := len(framePaths)

	for start := 0; start < total; start += framesPerGrid {
		end := start + framesPerGrid
		if end > total {
			end = total
		}
		batch := framePaths[start:end]
		gridIdx := start / framesPerGrid
		gridPath := filepath.Join(outputDir, fmt.Sprintf("grid_%03d.jpg", gridIdx+1))

		if err := createSingleGrid(batch, gridPath); err != nil {
			return nil, fmt.Errorf("grid %d failed: %w", gridIdx+1, err)
		}
		gridPaths = append(gridPaths, gridPath)
	}

	return gridPaths, nil
}

func createSingleGrid(framePaths []string, outputPath string) error {
	n := len(framePaths)
	if n == 0 {
		return fmt.Errorf("no frames to grid")
	}

	// Write a concat list so ffmpeg reads the frames as a sequence of single-frame videos.
	listPath := outputPath + ".txt"
	var listContent strings.Builder
	for _, p := range framePaths {
		abs, _ := filepath.Abs(p)
		listContent.WriteString(fmt.Sprintf("file '%s'\n", filepath.ToSlash(abs)))
		listContent.WriteString("duration 0.04\n")
	}
	if err := os.WriteFile(listPath, []byte(listContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write concat list: %w", err)
	}

	// For the last batch with fewer than whatever frames, tile still works and
	// fills missing cells with black.
	tileLayout := fmt.Sprintf("%dx%d", gridCols, gridRows)

	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-vf", fmt.Sprintf("scale=640:-2,tile=%s", tileLayout),
		"-frames:v", "1",
		"-q:v", "3",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg tile error: %w\noutput: %s", err, string(output))
	}

	os.Remove(listPath)
	return nil
}

func GetVideoMetadata(videoPath string) (*VideoMetadata, error) {
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=r_frame_rate,nb_frames,width,height",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1",
		videoPath,
	}

	cmd := exec.Command("ffprobe", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe error: %w", err)
	}

	return parseFFprobeOutput(string(output))
}

func parseFFprobeOutput(output string) (*VideoMetadata, error) {
	meta := &VideoMetadata{}
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]

		switch key {
		case "width":
			meta.Width, _ = strconv.Atoi(value)
		case "height":
			meta.Height, _ = strconv.Atoi(value)
		case "r_frame_rate":
			meta.FPS = parseFrameRate(value)
		case "nb_frames":
			meta.TotalFrames, _ = strconv.ParseInt(value, 10, 64)
		case "duration":
			dur, err := strconv.ParseFloat(value, 64)
			if err == nil {
				meta.DurationMs = int64(dur * 1000)
			}
		}
	}

	if meta.TotalFrames == 0 && meta.FPS > 0 && meta.DurationMs > 0 {
		meta.TotalFrames = int64(float64(meta.DurationMs) / 1000.0 * meta.FPS)
	}

	return meta, nil
}

func parseFrameRate(value string) float64 {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	return num / den
}

// BuildImpactVideo takes the original video, the impact frame numbers (1-based,
// at the sample rate), the sample rate, and produces a new video where those
// frames (plus 1 before and 1 after) have invert + B&W effects applied.
func BuildImpactVideo(inputPath, outputDir string, impactFrames []int, sampleRate float64, meta *VideoMetadata) (string, error) {
	if len(impactFrames) == 0 {
		return "", fmt.Errorf("no impact frames provided")
	}

	type timeWindow struct {
		start float64
		end   float64
	}

	var windows []timeWindow
	for _, f := range impactFrames {
		startFrame := f
		if startFrame < 1 {
			startFrame = 1
		}
		endFrame := f + 1

		startTime := float64(startFrame-1) / sampleRate
		endTime := float64(endFrame) / sampleRate
		windows = append(windows, timeWindow{startTime, endTime})
	}

	// Sort and merge overlapping windows
	sort.Slice(windows, func(i, j int) bool { return windows[i].start < windows[j].start })
	var merged []timeWindow
	for _, w := range windows {
		if len(merged) > 0 && w.start <= merged[len(merged)-1].end {
			if w.end > merged[len(merged)-1].end {
				merged[len(merged)-1].end = w.end
			}
		} else {
			merged = append(merged, w)
		}
	}

	// Build enable expression: between(t,s1,e1)+between(t,s2,e2)+...
	var parts []string
	for _, w := range merged {
		parts = append(parts, fmt.Sprintf("between(t\\,%.3f\\,%.3f)", w.start, w.end))
	}
	enableExpr := strings.Join(parts, "+")

	log.Printf("[effects] %d impact points -> %d merged time windows, enable=%s", len(impactFrames), len(merged), enableExpr)

	// Apply effect filters only during impact windows, preserving original framerate.
	filter := fmt.Sprintf(
		"hue=s=0:enable='%s',eq=contrast=3.5:brightness=0.9:enable='%s',curves=all='0/0 0.25/0.1 0.5/0.6 0.75/0.95 1/1':enable='%s'",
		enableExpr, enableExpr, enableExpr,
	)

	outputPath := filepath.Join(outputDir, "output.mp4")
	args := []string{
		"-y",
		"-i", inputPath,
		"-vf", filter,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "18",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg effect error: %w\noutput: %s", err, string(output))
	}

	return outputPath, nil
}
