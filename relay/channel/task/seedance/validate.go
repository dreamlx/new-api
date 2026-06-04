package seedance

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

// ============================
// Content[] format parsing (Volcano-compatible)
// ============================

// ContentItemInput represents a single content item from Volcano-compatible API request.
// This is distinct from volcano.ContentItem because the input format uses different field names
// for video/audio (e.g., "video" with nested "url" instead of "video_url").
type ContentItemInput struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url"`
	Video *struct {
		URL string `json:"url"`
	} `json:"video"`
	Audio *struct {
		URL string `json:"url"`
	} `json:"audio"`
	Role string `json:"role"`
}

// VolcanoRequestBody represents the raw Volcano-compatible request body.
// Uses ContentItemInput (input format) rather than volcano.ContentItem (upstream format).
// Only fields needed for billing logic (Duration, Seed, Resolution) are explicitly parsed;
// all other upstream fields are passed through via raw JSON → Metadata → UnmarshalMetadata.
type VolcanoRequestBody struct {
	Model      string             `json:"model"`
	Content    []ContentItemInput `json:"content"`
	Duration   *int               `json:"duration"`
	Seed       *int               `json:"seed"`
	Resolution string             `json:"resolution"`
}

// ValidateRequestAndSetAction overrides the default validation to support both:
// 1. OpenAI Video style: {model, prompt, images, videos, audios, duration, size}
// 2. Volcano content[] style: {model, content: [{type, text, image_url, video, audio}]}
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Read the raw body bytes
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return &dto.TaskError{Code: "read_body_failed", Message: err.Error(), StatusCode: http.StatusInternalServerError, LocalError: true, Error: err}
	}
	bodyBytes, err := storage.Bytes()
	if err != nil {
		return &dto.TaskError{Code: "read_body_failed", Message: err.Error(), StatusCode: http.StatusInternalServerError, LocalError: true, Error: err}
	}

	// Parse as generic map to detect format
	var raw map[string]json.RawMessage
	if err := common.Unmarshal(bodyBytes, &raw); err != nil {
		return &dto.TaskError{Code: "invalid_request", Message: err.Error(), StatusCode: http.StatusBadRequest, LocalError: true, Error: err}
	}

	var req relaycommon.TaskSubmitReq

	// Check if content[] exists (Volcano format)
	if _, hasContent := raw["content"]; hasContent {
		// Volcano content[] format
		var volcanoReq VolcanoRequestBody
		if err := common.Unmarshal(bodyBytes, &volcanoReq); err != nil {
			return &dto.TaskError{Code: "invalid_request", Message: err.Error(), StatusCode: http.StatusBadRequest, LocalError: true, Error: err}
		}
		req = convertVolcanoContentToTaskSubmit(volcanoReq, raw)
	} else {
		// Standard OpenAI Video format
		if err := common.Unmarshal(bodyBytes, &req); err != nil {
			return &dto.TaskError{Code: "invalid_request", Message: err.Error(), StatusCode: http.StatusBadRequest, LocalError: true, Error: err}
		}
	}

	// Validate prompt
	if strings.TrimSpace(req.Prompt) == "" {
		return &dto.TaskError{Code: "invalid_request", Message: "prompt is required", StatusCode: http.StatusBadRequest, LocalError: true, Error: errors.New("prompt is required")}
	}

	// Compat: single image
	if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		req.Images = []string{req.Image}
	}

	info.Action = constant.TaskActionGenerate
	c.Set("task_request", req)
	return nil
}

// convertVolcanoContentToTaskSubmit converts Volcano content[] format to TaskSubmitReq.
// raw contains the original JSON body fields; all keys except "content" and "model" are
// passed through to req.Metadata so that UnmarshalMetadata in convertToRequestPayload
// can automatically overlay them onto volcano.RequestPayload.
func convertVolcanoContentToTaskSubmit(volcanoReq VolcanoRequestBody, raw map[string]json.RawMessage) relaycommon.TaskSubmitReq {
	req := relaycommon.TaskSubmitReq{
		Model: volcanoReq.Model,
	}

	var promptParts []string
	var images []string
	var videos []string
	var audios []string

	// Build upstream-format content items preserving structured fields (e.g. role)
	var upstreamContentItems []map[string]interface{}

	for _, item := range volcanoReq.Content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				promptParts = append(promptParts, item.Text)
			}
		case "image_url":
			if item.ImageURL != nil && item.ImageURL.URL != "" {
				images = append(images, item.ImageURL.URL)
				// Build upstream content item preserving role
				imgItem := map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]string{"url": item.ImageURL.URL},
				}
				if item.Role != "" {
					imgItem["role"] = item.Role
				}
				upstreamContentItems = append(upstreamContentItems, imgItem)
			}
		case "video":
			if item.Video != nil && item.Video.URL != "" {
				videos = append(videos, item.Video.URL)
				// Convert input format "video" → upstream format "video_url"
				upstreamContentItems = append(upstreamContentItems, map[string]interface{}{
					"type":       "video_url",
					"video_url":  map[string]string{"url": item.Video.URL},
				})
			}
		case "audio":
			if item.Audio != nil && item.Audio.URL != "" {
				audios = append(audios, item.Audio.URL)
				// Convert input format "audio" → upstream format "audio_url"
				upstreamContentItems = append(upstreamContentItems, map[string]interface{}{
					"type":       "audio_url",
					"audio_url":  map[string]string{"url": item.Audio.URL},
				})
			}
		}
	}

	req.Prompt = strings.Join(promptParts, " ")
	req.Images = images
	req.Videos = videos
	req.Audios = audios

	if volcanoReq.Duration != nil {
		req.Duration = *volcanoReq.Duration
	}
	if volcanoReq.Seed != nil {
		req.Seed = volcanoReq.Seed
	}
	if volcanoReq.Resolution != "" {
		req.Size = volcanoReq.Resolution
	}

	// Pass through all raw top-level fields (except "content" and "model") to Metadata.
	// UnmarshalMetadata in convertToRequestPayload will overlay them onto volcano.RequestPayload.
	// This ensures upstream fields (ratio, callback_url, return_last_frame, generate_audio,
	// draft, tools, frames, camera_fixed, watermark, service_tier, execution_expires_after)
	// are forwarded without needing to add them to VolcanoRequestBody.
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	for key, rawValue := range raw {
		if key == "content" || key == "model" {
			continue
		}
		req.Metadata[key] = rawValue
	}

	// Preserve structured content items in Metadata so convertToRequestPayload
	// can restore role and other per-item fields via UnmarshalMetadata.
	// The "content" key from raw is excluded above because input format
	// (video/audio) differs from upstream format (video_url/audio_url).
	if len(upstreamContentItems) > 0 {
		req.Metadata["content"] = upstreamContentItems
	}

	return req
}
