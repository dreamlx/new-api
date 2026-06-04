package volcano

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

// ============================
// Volcano-compatible DTOs
// ============================

// ContentItem represents a single content entry in a Volcano-compatible request/response.
type ContentItem struct {
	Type     string    `json:"type,omitempty"`
	Text     string    `json:"text,omitempty"`
	ImageURL *MediaURL `json:"image_url,omitempty"`
	VideoURL *MediaURL `json:"video_url,omitempty"`
	AudioURL *MediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

// MediaURL is a URL wrapper used in ContentItem.
type MediaURL struct {
	URL string `json:"url,omitempty"`
}

// RequestPayload represents a Volcano-compatible task submission request.
type RequestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	Resolution  string         `json:"resolution,omitempty"`
	Ratio       string         `json:"ratio,omitempty"`
	Duration    *dto.IntValue  `json:"duration,omitempty"`
	Frames      *dto.IntValue  `json:"frames,omitempty"`
	Seed        *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark   *dto.BoolValue `json:"watermark,omitempty"`
}

// ResponsePayload represents the upstream task submission response.
type ResponsePayload struct {
	ID string `json:"id"` // upstream task_id
}

// ResponseTask represents the upstream task status response.
type ResponseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// ============================
// Status constants
// ============================

const (
	StatusPending    = "pending"
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusRunning    = "running"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
)

// TokenPriority defines which token field to use for billing
type TokenPriority int

const (
	// TokenPriorityCompletionFirst uses completion_tokens if available, else total_tokens.
	// This is the Seedance 2.0 official strategy per their documentation:
	// "准确 token 用量以接口返回的 completion tokens 为准"
	TokenPriorityCompletionFirst TokenPriority = iota

	// TokenPriorityBothFields sets both CompletionTokens and TotalTokens independently.
	// This preserves the original doubao behavior before volcano extraction.
	TokenPriorityBothFields
)

// ============================
// Shared adaptor helpers
// ============================

// BuildTaskURL returns the Volcano-compatible task submission URL.
func BuildTaskURL(baseURL string) string {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", baseURL)
}

// BuildFetchURL returns the Volcano-compatible task status query URL.
func BuildFetchURL(baseURL, taskID string) string {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseURL, taskID)
}

// SetCommonHeaders sets the three required headers on a request.
func SetCommonHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

// FetchTask performs a GET to the upstream Volcano task status endpoint.
func FetchTask(baseURL, apiKey string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := BuildFetchURL(baseURL, taskID)
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	SetCommonHeaders(req, apiKey)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// ParseSubmitResponse parses the upstream submit response and returns the upstream task ID.
// The caller is responsible for writing the response to the gin context.
func ParseSubmitResponse(resp *http.Response) (upstreamTaskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var sResp ResponsePayload
	if err := common.Unmarshal(responseBody, &sResp); err != nil {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if sResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// Return upstream task ID and raw body
	return sResp.ID, responseBody, nil
}

// ParseTaskResult maps the upstream Volcano task response to the internal TaskInfo.
// tokenPriority controls how usage tokens are extracted for billing:
//   - TokenPriorityCompletionFirst (Seedance): prefer completion_tokens, fallback to total_tokens
//   - TokenPriorityBothFields (Doubao): set both fields independently from upstream
func ParseTaskResult(respBody []byte, tokenPriority TokenPriority) (*relaycommon.TaskInfo, error) {
	var resTask ResponseTask
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, fmt.Errorf("unmarshal task result failed: %w", err)
	}

	taskResult := &relaycommon.TaskInfo{Code: 0}

	switch resTask.Status {
	case StatusPending, StatusQueued:
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case StatusProcessing, StatusRunning:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case StatusSucceeded:
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL

		// Apply token priority strategy
		switch tokenPriority {
		case TokenPriorityCompletionFirst:
			// Seedance strategy: prefer completion_tokens for billing accuracy
			if resTask.Usage.CompletionTokens > 0 {
				taskResult.CompletionTokens = resTask.Usage.CompletionTokens
				taskResult.TotalTokens = resTask.Usage.CompletionTokens
			} else if resTask.Usage.TotalTokens > 0 {
				taskResult.TotalTokens = resTask.Usage.TotalTokens
			}
		case TokenPriorityBothFields:
			// Doubao strategy: preserve both fields independently (original behavior)
			taskResult.CompletionTokens = resTask.Usage.CompletionTokens
			taskResult.TotalTokens = resTask.Usage.TotalTokens
		}
	case StatusFailed:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return taskResult, nil
}

// HasVideoInMetadata checks if the metadata content[] array contains a video entry.
func HasVideoInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" || itemMap["type"] == "video" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}
