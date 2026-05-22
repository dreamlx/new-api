package happyhorse

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
	Sound      *bool  `json:"sound,omitempty"`
	Quality    string `json:"quality,omitempty"`
}

type GenerateResponse struct {
	Output    GenerateOutput `json:"output"`
	RequestID string         `json:"request_id,omitempty"`
}

type GenerateOutput struct {
	TaskID     string `json:"task_id,omitempty"`
	TaskStatus string `json:"task_status,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
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
	SR                  int     `json:"SR,omitempty"`
	Ratio               string  `json:"ratio,omitempty"`
}

type ErrorResponse struct {
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id,omitempty"`
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
