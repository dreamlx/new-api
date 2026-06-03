package seedance

import (
	"encoding/json"
	"fmt"
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

// ContentItemInput represents a single content item from Volcano-compatible API request
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

// VolcanoRequestBody represents the raw Volcano-compatible request body
type VolcanoRequestBody struct {
	Model    string             `json:"model"`
	Content  []ContentItemInput `json:"content"`
	Duration *int               `json:"duration"`
	Seed     *int               `json:"seed"`
	// Extra fields that may be present
	Resolution  string `json:"resolution"`
	CallbackURL string `json:"callback_url"`
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
		req = convertVolcanoContentToTaskSubmit(volcanoReq)
	} else {
		// Standard OpenAI Video format
		if err := common.Unmarshal(bodyBytes, &req); err != nil {
			return &dto.TaskError{Code: "invalid_request", Message: err.Error(), StatusCode: http.StatusBadRequest, LocalError: true, Error: err}
		}
	}

	// Validate prompt
	if strings.TrimSpace(req.Prompt) == "" {
		return &dto.TaskError{Code: "invalid_request", Message: "prompt is required", StatusCode: http.StatusBadRequest, LocalError: true, Error: fmt.Errorf("prompt is required")}
	}

	// Compat: single image
	if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		req.Images = []string{req.Image}
	}

	info.Action = constant.TaskActionGenerate
	c.Set("task_request", req)
	return nil
}

// convertVolcanoContentToTaskSubmit converts Volcano content[] format to TaskSubmitReq
func convertVolcanoContentToTaskSubmit(volcanoReq VolcanoRequestBody) relaycommon.TaskSubmitReq {
	req := relaycommon.TaskSubmitReq{
		Model: volcanoReq.Model,
	}

	var promptParts []string
	var images []string
	var videos []string
	var audios []string

	for _, item := range volcanoReq.Content {
		switch item.Type {
		case "text":
			if item.Text != "" {
				promptParts = append(promptParts, item.Text)
			}
		case "image_url":
			if item.ImageURL != nil && item.ImageURL.URL != "" {
				images = append(images, item.ImageURL.URL)
				// Add role annotation to prompt prefix
				if item.Role != "" {
					promptParts = append(promptParts, fmt.Sprintf("[Image %d] is %s", len(images), item.Role))
				}
			}
		case "video":
			if item.Video != nil && item.Video.URL != "" {
				videos = append(videos, item.Video.URL)
			}
		case "audio":
			if item.Audio != nil && item.Audio.URL != "" {
				audios = append(audios, item.Audio.URL)
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

	return req
}
