package seedance

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon/volcano"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

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

// BuildRequestURL constructs the upstream Volcano-compatible URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return volcano.BuildTaskURL(a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	volcano.SetCommonHeaders(req, a.apiKey)
	return nil
}

// EstimateBilling 计算预扣倍率：检测输入视频、分辨率，返回 OtherRatios。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	modelName := info.OriginModelName
	resolution := normalizeResolution(req.Size)
	// 视频输入检测：Volcano content[] 中的 video/video_url 条目 + OpenAI Video 风格的 req.Videos 字段
	hasVideoInput := volcano.HasVideoInMetadata(req.Metadata) || len(req.Videos) > 0

	// Calculate combined ratio based on model, resolution, and video input
	ratio := calculateCombinedRatio(modelName, resolution, hasVideoInput)

	if ratio != 1.0 {
		return map[string]float64{"seedance_condition": ratio}
	}
	return nil
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
	upstreamTaskID, responseBody, taskErr := volcano.ParseSubmitResponse(c, resp, info.PublicTaskID, info.OriginModelName)
	if taskErr != nil {
		return
	}

	// Write OpenAI Video response to client
	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return upstreamTaskID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	return volcano.FetchTask(baseUrl, key, body, proxy)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// convertToRequestPayload 将 TaskSubmitReq 转换为上游 Volcano-compatible 请求
func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*volcano.RequestPayload, error) {
	r := volcano.RequestPayload{
		Model:   req.Model,
		Content: []volcano.ContentItem{},
	}

	// Check if Metadata contains structured content items (from Volcano content[] format).
	// These preserve per-item fields like role that cannot be carried by the flat
	// Images/Videos/Audios string arrays.
	hasMetadataContent := false
	if req.Metadata != nil {
		if contentRaw, ok := req.Metadata["content"]; ok {
			if contentSlice, ok := contentRaw.([]interface{}); ok && len(contentSlice) > 0 {
				hasMetadataContent = true
			}
		}
	}

	if !hasMetadataContent {
		// OpenAI Video format: build content items from flat URL arrays
		if req.HasImage() {
			for _, imgURL := range req.Images {
				r.Content = append(r.Content, volcano.ContentItem{
					Type:     "image_url",
					ImageURL: &volcano.MediaURL{URL: imgURL},
				})
			}
		}

		if len(req.Videos) > 0 {
			for _, videoURL := range req.Videos {
				r.Content = append(r.Content, volcano.ContentItem{
					Type:     "video_url",
					VideoURL: &volcano.MediaURL{URL: videoURL},
				})
			}
		}

		if len(req.Audios) > 0 {
			for _, audioURL := range req.Audios {
				r.Content = append(r.Content, volcano.ContentItem{
					Type:     "audio_url",
					AudioURL: &volcano.MediaURL{URL: audioURL},
				})
			}
		}
	}

	// Unmarshal metadata into request payload (may override fields).
	// For Volcano content[] format, Metadata["content"] contains structured content
	// items with per-item fields like role preserved (e.g. "first_frame").
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
	} else if req.Duration > 0 {
		r.Duration = lo.ToPtr(dto.IntValue(req.Duration))
	}
	// Extract seed
	if req.Seed != nil {
		r.Seed = lo.ToPtr(dto.IntValue(*req.Seed))
	}

	// Remove existing text items and add prompt as text
	r.Content = lo.Reject(r.Content, func(c volcano.ContentItem, _ int) bool { return c.Type == "text" })
	r.Content = append(r.Content, volcano.ContentItem{
		Type: "text",
		Text: req.Prompt,
	})

	return &r, nil
}

// ParseTaskResult parses upstream task response and converts to TaskInfo
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	return volcano.ParseTaskResult(respBody)
}

// ConvertToOpenAIVideo converts internal task to OpenAI Video format response
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var sResp volcano.ResponseTask
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

	// Usage & metadata from upstream response
	if sResp.Usage.CompletionTokens > 0 || sResp.Usage.TotalTokens > 0 {
		openAIVideo.SetMetadata("usage", map[string]int{
			"completion_tokens": sResp.Usage.CompletionTokens,
			"total_tokens":      sResp.Usage.TotalTokens,
		})
	}
	if sResp.Duration > 0 {
		openAIVideo.SetMetadata("duration", sResp.Duration)
	}
	if sResp.Resolution != "" {
		openAIVideo.SetMetadata("resolution", sResp.Resolution)
	}
	if sResp.FramesPerSecond > 0 {
		openAIVideo.SetMetadata("framespersecond", sResp.FramesPerSecond)
	}

	if sResp.Status == volcano.StatusFailed {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: sResp.Error.Message,
			Code:    sResp.Error.Code,
		}
	}

	return common.Marshal(openAIVideo)
}
