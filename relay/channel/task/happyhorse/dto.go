package happyhorse

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

type GenerateRequest struct {
	Model      string      `json:"model"`
	Input      Input       `json:"input"`
	Parameters *Parameters `json:"parameters,omitempty"`
}

type Input struct {
	Prompt string      `json:"prompt,omitempty"`
	Media  []MediaItem `json:"media,omitempty"`
}

type MediaItem struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type Parameters struct {
	Resolution string `json:"resolution,omitempty"`
	Ratio      string `json:"ratio,omitempty"`
	Duration   *int   `json:"duration,omitempty"`
	Watermark  *bool  `json:"watermark,omitempty"`
	Seed       *int   `json:"seed,omitempty"`
}

type StatusResponse struct {
	Output    StatusOutput `json:"output"`
	Usage     *Usage       `json:"usage,omitempty"`
	RequestID string       `json:"request_id,omitempty"`
}

type StatusOutput struct {
	TaskID        string `json:"task_id,omitempty"`
	TaskStatus    string `json:"task_status,omitempty"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

type Usage struct {
	Duration            float64 `json:"duration,omitempty"`
	InputVideoDuration  float64 `json:"input_video_duration,omitempty"`
	OutputVideoDuration float64 `json:"output_video_duration,omitempty"`
	VideoCount          int     `json:"video_count,omitempty"`
	SR                  SRValue `json:"SR,omitempty"`
	Ratio               string  `json:"ratio,omitempty"`
}

// Native response DTOs for /happyhorse/api/generate and /happyhorse/api/status

type NativeSubmitResponse struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type NativeStatusResponse struct {
	TaskID  string            `json:"task_id"`
	Status  string            `json:"status"`
	Message string            `json:"message,omitempty"`
	Data    *NativeStatusData `json:"data,omitempty"`
}

type NativeStatusData struct {
	Model       string   `json:"model,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Duration    int      `json:"duration,omitempty"`
	AspectRatio string   `json:"aspect_ratio,omitempty"`
	VideoURL    string   `json:"video_url,omitempty"`
	ResultUrls  []string `json:"resultUrls,omitempty"`
}

// SRValue handles both int ("720") and string ("720") forms of the SR field.
type SRValue int

func (s *SRValue) UnmarshalJSON(data []byte) error {
	// Try int first
	var n int
	if err := common.Unmarshal(data, &n); err == nil {
		*s = SRValue(n)
		return nil
	}
	// Try string
	var str string
	if err := common.Unmarshal(data, &str); err == nil {
		v, err := strconv.Atoi(str)
		if err != nil {
			return fmt.Errorf("invalid SR value: %s", str)
		}
		*s = SRValue(v)
		return nil
	}
	return fmt.Errorf("SR must be int or string")
}

func (s SRValue) Int() int {
	return int(s)
}
