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

	video "github.com/umohsamuel/impact/internals/infrastructures/domain/video"
)

type FrameExtractionConfig struct {
	InputPath  string
	OutputDir  string
	SampleRate float64 // frames per second to extract (e.g. 1.0 = 1 frame/sec, 2.0 = 2 frames/sec)
	MaxWidth   int     // resize width, 0 = no resize
	MaxHeight  int     // resize height, 0 = no resize
	Quality    int     // JPEG quality 1-31 (lower = better quality in FFmpeg)
}

type ExtractedFrames struct {
	Paths       []string
	FrameCount  int
	FPS         float64
	DurationMs  int64
	TotalFrames int64
	OutputDir   string
}

type VideoMetadata struct {
	FPS         float64
	DurationMs  int64
	TotalFrames int64
	Width       int
	Height      int
}

// ExtractFrames extracts sampled frames from a video as JPEGs
func ExtractFrames(cfg FrameExtractionConfig) (*ExtractedFrames, error) {
	// Create output directory
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}

	// Build filter string
	filters := []string{
		fmt.Sprintf("fps=%g", cfg.SampleRate), // sample at desired rate
	}

	if cfg.MaxWidth > 0 || cfg.MaxHeight > 0 {
		w := cfg.MaxWidth
		h := cfg.MaxHeight
		if w == 0 {
			w = -1 // maintain aspect ratio
		}
		if h == 0 {
			h = -1
		}
		// scale down only, never upscale
		filters = append(filters, fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=decrease,scale=trunc(iw/2)*2:trunc(ih/2)*2",
			w, h,
		))
	}

	filterStr := strings.Join(filters, ",")
	outputPattern := filepath.Join(cfg.OutputDir, "frame_%05d.jpg")

	args := []string{
		"-i", cfg.InputPath,
		"-vf", filterStr,
		"-q:v", strconv.Itoa(cfg.Quality), // JPEG quality
		"-f", "image2",
		outputPattern,
	}

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg error: %w\noutput: %s", err, string(output))
	}

	// Collect output frame paths
	paths, err := filepath.Glob(filepath.Join(cfg.OutputDir, "frame_*.jpg"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob frames: %w", err)
	}

	// Get source video metadata
	meta, err := GetVideoMetadata(cfg.InputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	return &ExtractedFrames{
		Paths:       paths,
		FrameCount:  len(paths),
		FPS:         meta.FPS,
		DurationMs:  meta.DurationMs,
		TotalFrames: meta.TotalFrames,
		OutputDir:   cfg.OutputDir,
	}, nil
}

// GetVideoMetadata uses ffprobe to extract video info
func GetVideoMetadata(videoPath string) (*VideoMetadata, error) {
	// ffprobe outputs stream info as a flat key=value format
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

// parseFFprobeOutput parses key=value lines from ffprobe output
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
			// format is "30/1" or "30000/1001" (for 29.97)
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

	// Fallback: calculate total frames if nb_frames is missing
	if meta.TotalFrames == 0 && meta.FPS > 0 && meta.DurationMs > 0 {
		meta.TotalFrames = int64(float64(meta.DurationMs) / 1000.0 * meta.FPS)
	}

	return meta, nil
}

// parseFrameRate converts "30000/1001" or "30/1" to float64
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

// CleanupFrames removes the temp frame directory after use
func CleanupFrames(dir string) error {
	return os.RemoveAll(dir)
}

// ApplyImpactEffects applies freeze-frame + B&W + vignette + zoom at each detected impact and produces a new video
func ApplyImpactEffects(inputPath string, outputDir string, impacts []video.ImpactFrame, meta *VideoMetadata) (string, error) {
	workDir := filepath.Join(outputDir, "processing")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create processing dir: %w", err)
	}

	sort.Slice(impacts, func(i, j int) bool {
		return impacts[i].TimestampMs < impacts[j].TimestampMs
	})

	w := meta.Width
	h := meta.Height
	if w%2 != 0 {
		w--
	}
	if h%2 != 0 {
		h--
	}
	fps := meta.FPS
	if fps <= 0 {
		fps = 30
	}

	var segments []string
	lastEndSec := 0.0
	totalDurSec := float64(meta.DurationMs) / 1000.0

	for i, impact := range impacts {
		// Shift 100ms earlier so the freeze shows the moment just before contact
		impactSec := float64(impact.TimestampMs)/1000.0 - 1
		if impactSec < 0 {
			impactSec = 0
		}
		if impactSec >= totalDurSec {
			continue
		}

		// Normal segment before impact
		if impactSec > lastEndSec+0.05 {
			segPath := filepath.Join(workDir, fmt.Sprintf("seg_%d.mp4", i*2))
			duration := impactSec - lastEndSec
			if err := ffmpegExtractSegment(inputPath, lastEndSec, duration, segPath, w, h, fps); err != nil {
				log.Printf("Warning: segment extraction failed: %v", err)
			} else {
				segments = append(segments, segPath)
			}
		}

		// Extract impact frame as image
		framePath := filepath.Join(workDir, fmt.Sprintf("impact_%d.jpg", i))
		if err := ffmpegExtractFrame(inputPath, impactSec, framePath); err != nil {
			log.Printf("Warning: frame extraction failed: %v", err)
			continue
		}

		// Create freeze clip with effects
		freezePath := filepath.Join(workDir, fmt.Sprintf("freeze_%d.mp4", i))
		freezeDur := float64(impact.FreezeFrame.FreezeDurationMs) / 1000.0
		if freezeDur < 0.05 {
			freezeDur = 0.15 // default 150ms — short and punchy
		}
		if freezeDur > 0.15 {
			freezeDur = 0.15 // cap at 150ms
		}
		if err := ffmpegCreateFreezeClip(framePath, freezePath, freezeDur, impact, w, h, fps); err != nil {
			log.Printf("Warning: freeze clip creation failed: %v", err)
			continue
		}
		segments = append(segments, freezePath)

		lastEndSec = impactSec + (1.0 / fps)
	}

	// Final segment after last impact
	if lastEndSec < totalDurSec-0.05 {
		segPath := filepath.Join(workDir, "seg_final.mp4")
		duration := totalDurSec - lastEndSec
		if err := ffmpegExtractSegment(inputPath, lastEndSec, duration, segPath, w, h, fps); err != nil {
			log.Printf("Warning: final segment extraction failed: %v", err)
		} else {
			segments = append(segments, segPath)
		}
	}

	if len(segments) == 0 {
		return "", fmt.Errorf("no segments were created")
	}

	outputPath := filepath.Join(outputDir, "output.mp4")
	if err := ffmpegConcat(segments, outputPath, workDir); err != nil {
		return "", fmt.Errorf("concat failed: %w", err)
	}

	return outputPath, nil
}

func ffmpegExtractSegment(inputPath string, startSec, duration float64, outputPath string, w, h int, fps float64) error {
	args := []string{
		"-y",
		"-i", inputPath,
		"-ss", fmt.Sprintf("%.3f", startSec),
		"-t", fmt.Sprintf("%.3f", duration),
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,fps=%g", w, h, w, h, fps),
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "128k",
		outputPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg segment error: %w\noutput: %s", err, string(output))
	}
	return nil
}

func ffmpegExtractFrame(inputPath string, timestampSec float64, outputPath string) error {
	args := []string{
		"-y",
		"-i", inputPath,
		"-ss", fmt.Sprintf("%.3f", timestampSec),
		"-frames:v", "1",
		"-q:v", "2",
		outputPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg frame extract error: %w\noutput: %s", err, string(output))
	}
	return nil
}

func ffmpegCreateFreezeClip(framePath, outputPath string, duration float64, impact video.ImpactFrame, w, h int, fps float64) error {
	var filters []string

	filters = append(filters, fmt.Sprintf("fps=%g", fps))

	// High-contrast invert: brightened background with darkened subjects
	// Gentle version — subjects remain clearly visible
	if impact.InvertFilter.Enabled && impact.InvertFilter.InvertStrength > 0 {
		strength := impact.InvertFilter.InvertStrength
		if strength > 1.0 {
			strength = 1.0
		}
		// Partial desaturation — keep some color at low strength
		saturation := 1.0 - (strength * 0.8) // 0.2 to 1.0 remaining
		filters = append(filters, fmt.Sprintf("hue=s=%.2f", saturation))
		// Moderate contrast boost — keep details visible
		contrastVal := 1.2 + (strength * 0.6)    // 1.2 to 1.8
		brightnessVal := 0.05 + (strength * 0.1) // slight brightness lift
		filters = append(filters, fmt.Sprintf("eq=contrast=%.2f:brightness=%.2f", contrastVal, brightnessVal))
		// Gentle curves — lighten highlights, slightly darken shadows
		filters = append(filters, "curves=all='0/0 0.25/0.15 0.5/0.55 0.75/0.9 1/1'")
	} else if impact.BWFilter.Enabled {
		// Standard B&W with high contrast
		saturation := 1.0 - impact.BWFilter.BWIntensity
		if saturation < 0 {
			saturation = 0
		}
		filters = append(filters, fmt.Sprintf("hue=s=%.2f", saturation))
		if impact.BWFilter.ContrastBoost > 1.0 {
			filters = append(filters, fmt.Sprintf("eq=contrast=%.2f", impact.BWFilter.ContrastBoost))
		}
	}

	if impact.Vignette.Enabled {
		filters = append(filters, "vignette")
	}

	if impact.Zoom.Enabled && impact.Zoom.ZoomFactor > 1.0 {
		zf := impact.Zoom.ZoomFactor
		filters = append(filters, fmt.Sprintf("scale=iw*%.2f:ih*%.2f", zf, zf))
		filters = append(filters, fmt.Sprintf("crop=%d:%d", w, h))
	}

	filters = append(filters, fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", w, h, w, h))
	filters = append(filters, "format=yuv420p")

	filterStr := strings.Join(filters, ",")

	args := []string{
		"-y",
		"-loop", "1",
		"-i", framePath,
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100",
		"-t", fmt.Sprintf("%.3f", duration),
		"-vf", filterStr,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "128k",
		"-shortest",
		outputPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg freeze clip error: %w\noutput: %s", err, string(output))
	}
	return nil
}

func ffmpegConcat(segments []string, outputPath, workDir string) error {
	listPath := filepath.Join(workDir, "concat_list.txt")
	var content strings.Builder
	for _, seg := range segments {
		absPath, _ := filepath.Abs(seg)
		content.WriteString(fmt.Sprintf("file '%s'\n", filepath.ToSlash(absPath)))
	}
	if err := os.WriteFile(listPath, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("failed to write concat list: %w", err)
	}

	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c", "copy",
		"-movflags", "+faststart",
		outputPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat error: %w\noutput: %s", err, string(output))
	}
	return nil
}
