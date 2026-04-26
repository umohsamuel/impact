package video

type ProcessVideoRequest struct {
	SampleRate float64 `form:"sample_rate"` // frames/sec to sample, default 1.0
	MaxWidth   int     `form:"max_width"`   // default 1280
}

type FrameMetadata struct {
	FrameIndex  int    `json:"frame_index"`
	TimestampMs int64  `json:"timestamp_ms"`
	FilePath    string `json:"file_path"`
}

type ImpactAnalysisResponse struct {
	VideoAnalysis VideoAnalysis `json:"video_analysis"`
	ImpactFrames  []ImpactFrame `json:"impact_frames"`
}

type VideoAnalysis struct {
	TotalImpactsDetected int    `json:"total_impacts_detected"`
	OverallIntensity     string `json:"overall_intensity"`
	ContentType          string `json:"content_type"`
	ProcessingNotes      string `json:"processing_notes"`
}

type ImpactFrame struct {
	ImpactID     int            `json:"impact_id"`
	FrameIndex   int            `json:"frame_index"`
	TimestampMs  int64          `json:"timestamp_ms"`
	ImpactType   string         `json:"impact_type"`
	ImpactLabel  string         `json:"impact_label"`
	Confidence   float64        `json:"confidence"`
	Intensity    string         `json:"intensity"`
	FreezeFrame  FreezeConfig   `json:"freeze_frame"`
	InvertFilter InvertConfig   `json:"invert_filter"`
	BWFilter     BWConfig       `json:"bw_filter"`
	Zoom         ZoomConfig     `json:"zoom"`
	Vignette     VignetteConfig `json:"vignette"`
	Slowdown     SlowdownConfig `json:"slowdown"`
}

type FreezeConfig struct {
	Enabled          bool `json:"enabled"`
	FreezeDurationMs int  `json:"freeze_duration_ms"`
}

type InvertConfig struct {
	Enabled        bool    `json:"enabled"`
	InvertStrength float64 `json:"invert_strength"`
}

type BWConfig struct {
	Enabled       bool    `json:"enabled"`
	BWIntensity   float64 `json:"bw_intensity"`
	ContrastBoost float64 `json:"contrast_boost"`
}

type ZoomConfig struct {
	Enabled        bool    `json:"enabled"`
	ZoomFactor     float64 `json:"zoom_factor"`
	ZoomDurationMs int     `json:"zoom_duration_ms"`
}

type VignetteConfig struct {
	Enabled          bool    `json:"enabled"`
	VignetteStrength float64 `json:"vignette_strength"`
}

type SlowdownConfig struct {
	Enabled            bool     `json:"enabled"`
	PreSlowdownStartMs *int64   `json:"pre_slowdown_start_ms"`
	SlowdownFactor     *float64 `json:"slowdown_factor"`
}
