package happyhorse

import (
	"fmt"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func ConvertTaskSubmitReq(req relaycommon.TaskSubmitReq) (*GenerateRequest, error) {
	return ConvertTaskSubmitReqWithModel(req, req.Model)
}

// ConvertTaskSubmitReqWithModel converts using a specified internal model name,
// allowing the native path to override the model from the mode field.
func ConvertTaskSubmitReqWithModel(req relaycommon.TaskSubmitReq, model string) (*GenerateRequest, error) {
	if !IsHappyHorseModel(model) {
		return nil, fmt.Errorf("unsupported model: %s", model)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	req.Model = model
	return convertTaskSubmitReqInner(req)
}

func convertTaskSubmitReqInner(req relaycommon.TaskSubmitReq) (*GenerateRequest, error) {
	duration := req.Duration
	if duration <= 0 {
		duration = DefaultDuration
	}

	resolution := DefaultResolution
	if v := getStringFromMetadata(req.Metadata, "resolution"); v != "" {
		if !ValidResolutions[v] {
			return nil, fmt.Errorf("unsupported resolution: %s, only 720P and 1080P are supported", v)
		}
		resolution = v
	}
	if req.Size == Resolution720P || req.Size == Resolution1080P {
		resolution = req.Size
	}

	var ratio string
	if v := getStringFromMetadata(req.Metadata, "ratio"); v != "" {
		if !ValidRatios[v] {
			return nil, fmt.Errorf("unsupported ratio: %s, only 16:9, 9:16 and 1:1 are supported", v)
		}
		ratio = v
	}

	params := &Parameters{
		Resolution: resolution,
		Duration:   &duration,
	}
	if ratio != "" {
		params.Ratio = ratio
	}

	if v, ok := getBoolFromMetadata(req.Metadata, "watermark"); ok {
		params.Watermark = &v
	}
	if v, ok := getIntFromMetadata(req.Metadata, "seed"); ok {
		params.Seed = &v
	}
	if v, ok := getBoolFromMetadata(req.Metadata, "sound"); ok {
		params.Sound = &v
	}
	if v := getStringFromMetadata(req.Metadata, "quality"); v != "" {
		params.Quality = v
	}

	input := Input{Prompt: req.Prompt}

	media, err := buildMedia(req)
	if err != nil {
		return nil, err
	}
	input.Media = media

	return &GenerateRequest{
		Model:      req.Model,
		Input:      input,
		Parameters: params,
	}, nil
}

func buildMedia(req relaycommon.TaskSubmitReq) ([]MediaItem, error) {
	switch req.Model {
	case ModelT2V:
		return nil, nil

	case ModelI2V:
		url := resolveFirstFrameImage(req)
		if url == "" {
			return nil, fmt.Errorf("i2v requires a first frame image (image, images[0], or input_reference)")
		}
		return []MediaItem{{Type: MediaTypeFirstFrame, URL: url}}, nil

	case ModelR2V:
		return buildR2VMedia(req)

	case ModelVideoEdit:
		return buildVideoEditMedia(req)

	default:
		return nil, fmt.Errorf("unsupported model: %s", req.Model)
	}
}

func resolveFirstFrameImage(req relaycommon.TaskSubmitReq) string {
	if req.Image != "" {
		return req.Image
	}
	if len(req.Images) > 0 && req.Images[0] != "" {
		return req.Images[0]
	}
	if req.InputReference != "" {
		return req.InputReference
	}
	return ""
}

func buildR2VMedia(req relaycommon.TaskSubmitReq) ([]MediaItem, error) {
	// metadata.media takes priority
	if raw, ok := req.Metadata["media"]; ok {
		media, err := parseMediaFromMetadata(raw)
		if err != nil {
			return nil, err
		}
		if len(media) > 0 {
			return media, nil
		}
	}

	// fallback to images[]
	if len(req.Images) > 0 {
		media := make([]MediaItem, 0, len(req.Images))
		for _, url := range req.Images {
			if url != "" {
				media = append(media, MediaItem{Type: MediaTypeReferenceImage, URL: url})
			}
		}
		if len(media) > 0 {
			return media, nil
		}
	}

	// fallback to single image or input_reference
	url := resolveFirstFrameImage(req)
	if url == "" {
		return nil, fmt.Errorf("r2v requires at least one reference image")
	}
	return []MediaItem{{Type: MediaTypeReferenceImage, URL: url}}, nil
}

func buildVideoEditMedia(req relaycommon.TaskSubmitReq) ([]MediaItem, error) {
	videoURL := getStringFromMetadata(req.Metadata, "video_url")
	if videoURL == "" {
		return nil, fmt.Errorf("video-edit requires metadata.video_url")
	}

	media := []MediaItem{{Type: MediaTypeVideo, URL: videoURL}}

	if raw, ok := req.Metadata["reference_images"]; ok {
		urls := toStringSlice(raw)
		for _, url := range urls {
			if url != "" {
				media = append(media, MediaItem{Type: MediaTypeReferenceImage, URL: url})
			}
		}
	}

	return media, nil
}

func parseMediaFromMetadata(raw interface{}) ([]MediaItem, error) {
	data, err := commonMarshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid metadata.media: %w", err)
	}
	var items []MediaItem
	if err := commonUnmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("invalid metadata.media format: %w", err)
	}
	return items, nil
}

// Metadata helpers

func getStringFromMetadata(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func getBoolFromMetadata(m map[string]interface{}, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func getIntFromMetadata(m map[string]interface{}, key string) (int, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func toStringSlice(raw interface{}) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}
