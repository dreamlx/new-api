package seedance

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Accept POST /v1/video/generations or POST /api/v3/contents/generations/tasks as "generate" action.
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream Volcano-compatible URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 计算预扣倍率：检测输入视频、分辨率，返回 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	otherRatios := make(map[string]float64)
	modelName := info.OriginModelName

	// Check if fast model tries to use 1080P (not supported)
	if IsFastModel(modelName) {
		resolution := strings.ToLower(req.Size)
		if resolution == Resolution1080P || resolution == "1080" {
			// Fast model + 1080P should be rejected by validation or upstream
			// For now, don't apply any resolution ratio
		}
	}

	// Check for video input from metadata
	hasVideoInput := hasVideoInMetadata(req.Metadata) || hasVideoInArrays(req.Metadata)

	// Check resolution
	resolution := normalizeResolution(req.Size)

	// Calculate combined ratio based on model, resolution, and video input
	ratio := calculateCombinedRatio(modelName, resolution, hasVideoInput)

	if ratio != 1.0 {
		otherRatios["seedance_condition"] = ratio
	}

	return otherRatios
}

// calculateCombinedRatio 根据模型、分辨率和是否输入视频计算组合倍率
func calculateCombinedRatio(modelName, resolution string, hasVideo bool) float64 {
	// Fast models: only support 480p/720p
	if IsFastModel(modelName) {
		if hasVideo {
			if r, ok := GetVideoInputRatio(modelName); ok {
				return r // ~0.5946
			}
		}
		return 1.0 // 基准：不含视频
	}

	// Main models: support 480p/720p/1080p
	// 基准：480p/720p，不含视频 = 1.0
	if resolution == Resolution1080P {
		if hasVideo {
			// 1080P + 输入视频
			if r, ok := GetResolution1080PWithVideoRatio(modelName); ok {
				return r // ~0.6739
			}
		} else {
			// 1080P，不含视频
			if r, ok := GetResolution1080PRatio(modelName); ok {
				return r // ~1.1087
			}
		}
	} else {
		// 480P/720P
		if hasVideo {
			if r, ok := GetVideoInputRatio(modelName); ok {
				return r // ~0.6087
			}
		}
		// 480P/720P，不含视频 = 基准 = 1.0
	}

	return 1.0
}

// normalizeResolution 规范化分辨率字符串
func normalizeResolution(size string) string {
	size = strings.ToLower(strings.TrimSpace(size))
	if size == "1080" || size == "1080p" {
		return Resolution1080P
	}
	if size == "720" || size == "720p" {
		return Resolution720P
	}
	if size == "480" || size == "480p" {
		return Resolution480P
	}
	return Resolution720P // default
}

// hasVideoInArrays 检查 metadata 中是否包含视频 URL
func hasVideoInArrays(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	// Check for videos in metadata
	if videos, ok := metadata["videos"]; ok {
		if videosSlice, ok := videos.([]interface{}); ok && len(videosSlice) > 0 {
			return true
		}
	}
	return false
}

// hasVideoInMetadata 检查 metadata 的 content 数组是否包含 video_url 条目
func hasVideoInMetadata(metadata map[string]interface{}) bool {
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

// BuildRequestBody converts request into Seedance/Volcano specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Seedance/Volcano response
	var sResp ResponsePayload
	if err := common.Unmarshal(responseBody, &sResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if sResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return sResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// convertToRequestPayload 将 TaskSubmitReq 转换为上游 Volcano-compatible 请求
func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*RequestPayload, error) {
	r := RequestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}

	// Add images if present
	if req.HasImage() {
		for _, imgURL := range req.Images {
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: imgURL,
				},
			})
		}
	}

	// Add videos if present (from metadata)
	if req.Metadata != nil {
		if videos, ok := req.Metadata["videos"]; ok {
			if videosSlice, ok := videos.([]interface{}); ok {
				for _, v := range videosSlice {
					if videoURL, ok := v.(string); ok {
						r.Content = append(r.Content, ContentItem{
							Type: "video_url",
							VideoURL: &MediaURL{
								URL: videoURL,
							},
						})
					}
				}
			}
		}

		// Add audios if present (from metadata)
		if audios, ok := req.Metadata["audios"]; ok {
			if audiosSlice, ok := audios.([]interface{}); ok {
				for _, a := range audiosSlice {
					if audioURL, ok := a.(string); ok {
						r.Content = append(r.Content, ContentItem{
							Type: "audio_url",
							AudioURL: &MediaURL{
								URL: audioURL,
							},
						})
					}
				}
			}
		}
	}

	// Unmarshal metadata into request payload (may override fields)
	metadata := req.Metadata
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	// Map OpenAI Video style fields to Volcano format
	if req.Size != "" {
		r.Resolution = req.Size
	}
	if sec, _ := strconv.Atoi(req.Seconds); sec > 0 {
		r.Duration = lo.ToPtr(dto.IntValue(sec))
	}
	// Extract seed from metadata if present
	if req.Metadata != nil {
		if seed, ok := req.Metadata["seed"]; ok {
			if seedInt, ok := seed.(float64); ok {
				r.Seed = lo.ToPtr(dto.IntValue(int(seedInt)))
			} else if seedInt, ok := seed.(int); ok {
				r.Seed = lo.ToPtr(dto.IntValue(seedInt))
			}
		}
	}

	// Remove existing text items and add prompt as text
	r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
	r.Content = append(r.Content, ContentItem{
		Type: "text",
		Text: req.Prompt,
	})

	return &r, nil
}

// ParseTaskResult parses upstream task response and converts to TaskInfo
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := ResponseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Seedance/Volcano status to internal status
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
		// 解析 usage 信息用于按 token 计费
		// 优先使用 completion_tokens，fallback 到 total_tokens
		if resTask.Usage.CompletionTokens > 0 {
			taskResult.CompletionTokens = resTask.Usage.CompletionTokens
			taskResult.TotalTokens = resTask.Usage.CompletionTokens
		} else if resTask.Usage.TotalTokens > 0 {
			taskResult.TotalTokens = resTask.Usage.TotalTokens
		}
	case StatusFailed:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

// ConvertToOpenAIVideo converts internal task to OpenAI Video format response
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var sResp ResponseTask
	if err := common.Unmarshal(originTask.Data, &sResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal seedance task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", sResp.Content.VideoURL)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if sResp.Status == StatusFailed {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: sResp.Error.Message,
			Code:    sResp.Error.Code,
		}
	}

	return common.Marshal(openAIVideo)
}
