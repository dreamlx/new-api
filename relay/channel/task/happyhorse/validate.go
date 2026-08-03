package happyhorse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	if isHappyHorseNativeRequest(c) {
		return validateNativeRequest(c, info)
	}
	taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
	if taskErr != nil {
		return taskErr
	}
	return validateHappyHorseTaskRequest(c)
}

// validateHappyHorseTaskRequest validates V1-path requests after ValidateBasicTaskRequest.
func validateHappyHorseTaskRequest(c *gin.Context) *taskdto.TaskError {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if !IsHappyHorseModel(req.Model) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("unsupported model: %s", req.Model), "invalid_request", http.StatusBadRequest)
	}

	// duration: reject explicit values outside [3,15], allow missing (defaults later)
	// Video Edit does not accept duration at all
	if isDurationExplicit(c) {
		if req.Model == ModelVideoEdit {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("video-edit does not support duration parameter"),
				"invalid_request", http.StatusBadRequest)
		}
		if req.Duration < MinDuration || req.Duration > MaxDuration {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("duration must be between %d and %d seconds", MinDuration, MaxDuration),
				"invalid_request", http.StatusBadRequest)
		}
	}

	// resolution
	if v := getStringFromMetadata(req.Metadata, "resolution"); v != "" {
		if !ValidResolutions[v] {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("unsupported resolution: %s, only 720P and 1080P are supported", v),
				"invalid_request", http.StatusBadRequest)
		}
	}
	if req.Size != "" && req.Size != Resolution720P && req.Size != Resolution1080P {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("unsupported resolution: %s, only 720P and 1080P are supported", req.Size),
			"invalid_request", http.StatusBadRequest)
	}

	// ratio: only T2V and R2V support ratio
	if v := getStringFromMetadata(req.Metadata, "ratio"); v != "" {
		if !ValidRatios[v] {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("unsupported ratio: %s", v),
				"invalid_request", http.StatusBadRequest)
		}
		if !RatioAllowedForModel(req.Model) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("%s does not support ratio parameter", req.Model),
				"invalid_request", http.StatusBadRequest)
		}
	}

	// Convert V1 fields to []MediaItem and validate using shared logic
	media, mediaErr := v1ToMediaItems(req)
	if mediaErr != nil {
		return service.TaskErrorWrapperLocal(mediaErr, "invalid_request", http.StatusBadRequest)
	}

	// Validate media values
	for _, m := range media {
		if m.URL == "" {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("media item url is required"), "invalid_request", http.StatusBadRequest)
		}
		if !isValidMediaValue(m.Type, m.URL) {
			msg := "image media must use http/https url or image base64 data url"
			if m.Type == MediaTypeVideo {
				msg = "video media must use http or https url"
			}
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("%s", msg), "invalid_request", http.StatusBadRequest)
		}
	}

	// Model-specific media validation
	switch req.Model {
	case ModelI2V:
		if err := validateI2VMedia(media); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	case ModelR2V:
		if err := validateR2VMedia(media); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	case ModelVideoEdit:
		if err := validateVideoEditMedia(media); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	}
	return nil
}

// v1ToMediaItems converts V1 TaskSubmitReq fields into []MediaItem
// so that native validation helpers can be reused.
func v1ToMediaItems(req relaycommon.TaskSubmitReq) ([]MediaItem, error) {
	switch req.Model {
	case ModelT2V:
		return nil, nil

	case ModelI2V:
		var items []MediaItem
		seen := map[string]bool{}
		if req.Image != "" {
			items = append(items, MediaItem{Type: MediaTypeFirstFrame, URL: req.Image})
			seen[req.Image] = true
		}
		for _, url := range req.Images {
			if url == "" {
				return nil, fmt.Errorf("images contains empty url")
			}
			if seen[url] {
				continue
			}
			items = append(items, MediaItem{Type: MediaTypeFirstFrame, URL: url})
			seen[url] = true
		}
		if req.InputReference != "" {
			if seen[req.InputReference] {
				return items, nil
			}
			items = append(items, MediaItem{Type: MediaTypeFirstFrame, URL: req.InputReference})
		}
		return items, nil

	case ModelR2V:
		// metadata.media takes priority
		if raw, ok := req.Metadata["media"]; ok {
			return parseMediaFromMetadata(raw)
		}
		var items []MediaItem
		seen := map[string]bool{}
		for _, url := range req.Images {
			if url == "" {
				return nil, fmt.Errorf("images contains empty url")
			}
			if seen[url] {
				continue
			}
			items = append(items, MediaItem{Type: MediaTypeReferenceImage, URL: url})
			seen[url] = true
		}
		if req.Image != "" {
			if seen[req.Image] {
				return items, nil
			}
			items = append(items, MediaItem{Type: MediaTypeReferenceImage, URL: req.Image})
			seen[req.Image] = true
		}
		if req.InputReference != "" {
			if seen[req.InputReference] {
				return items, nil
			}
			items = append(items, MediaItem{Type: MediaTypeReferenceImage, URL: req.InputReference})
		}
		return items, nil

	case ModelVideoEdit:
		videoURL := getStringFromMetadata(req.Metadata, "video_url")
		var items []MediaItem
		if videoURL != "" {
			items = append(items, MediaItem{Type: MediaTypeVideo, URL: videoURL})
		}
		if raw, ok := req.Metadata["reference_images"]; ok {
			urls := toStringSlice(raw)
			for _, url := range urls {
				if url == "" {
					return nil, fmt.Errorf("reference_images contains empty url")
				}
				items = append(items, MediaItem{Type: MediaTypeReferenceImage, URL: url})
			}
		}
		return items, nil

	default:
		return nil, nil
	}
}

func validateNativeRequest(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	var req GenerateRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}

	if !IsHappyHorseModel(req.Model) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("unsupported model: %s; supported models: happyhorse-1.0-t2v, happyhorse-1.0-i2v, happyhorse-1.0-r2v, happyhorse-1.0-video-edit", req.Model),
			"invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Input.Prompt) == "" {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}

	// Validate parameters
	if req.Parameters != nil {
		if req.Parameters.Resolution != "" && !ValidResolutions[req.Parameters.Resolution] {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("unsupported resolution: %s, only 720P and 1080P are supported", req.Parameters.Resolution),
				"invalid_request", http.StatusBadRequest)
		}
		if req.Parameters.Ratio != "" {
			if !ValidRatios[req.Parameters.Ratio] {
				return service.TaskErrorWrapperLocal(
					fmt.Errorf("unsupported ratio: %s", req.Parameters.Ratio),
					"invalid_request", http.StatusBadRequest)
			}
			if !RatioAllowedForModel(req.Model) {
				return service.TaskErrorWrapperLocal(
					fmt.Errorf("%s does not support ratio parameter", req.Model),
					"invalid_request", http.StatusBadRequest)
			}
		}
		// Video Edit does not accept duration
		if req.Parameters.Duration != nil {
			if req.Model == ModelVideoEdit {
				return service.TaskErrorWrapperLocal(
					fmt.Errorf("video-edit does not support duration parameter"),
					"invalid_request", http.StatusBadRequest)
			}
			if *req.Parameters.Duration < MinDuration || *req.Parameters.Duration > MaxDuration {
				return service.TaskErrorWrapperLocal(
					fmt.Errorf("duration must be between %d and %d seconds", MinDuration, MaxDuration),
					"invalid_request", http.StatusBadRequest)
			}
		}
	}

	// Validate media values
	for _, m := range req.Input.Media {
		if m.URL == "" {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("media item url is required"), "invalid_request", http.StatusBadRequest)
		}
		if !isValidMediaValue(m.Type, m.URL) {
			msg := "image media must use http/https url or image base64 data url"
			if m.Type == MediaTypeVideo {
				msg = "video media must use http or https url"
			}
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("%s", msg), "invalid_request", http.StatusBadRequest)
		}
	}

	// Model-specific media validation
	switch req.Model {
	case ModelI2V:
		if err := validateI2VMedia(req.Input.Media); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	case ModelR2V:
		if err := validateR2VMedia(req.Input.Media); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	case ModelVideoEdit:
		if err := validateVideoEditMedia(req.Input.Media); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
	}

	taskReq := generateRequestToTaskSubmitReq(req)

	c.Set("happyhorse_generate_request", req)

	info.Action = constant.TaskActionGenerate
	c.Set("task_request", taskReq)
	return nil
}

// --- Media validation helpers ---

func validateI2VMedia(media []MediaItem) error {
	count := countMediaType(media, MediaTypeFirstFrame)
	if count == 0 {
		return fmt.Errorf("i2v requires exactly 1 first_frame media item")
	}
	if count > 1 {
		return fmt.Errorf("i2v supports exactly 1 first_frame media item, got %d", count)
	}
	for _, m := range media {
		if m.Type != MediaTypeFirstFrame {
			return fmt.Errorf("i2v only allows first_frame media, got %s", m.Type)
		}
	}
	return nil
}

func validateR2VMedia(media []MediaItem) error {
	refCount := countMediaType(media, MediaTypeReferenceImage)
	if refCount == 0 {
		return fmt.Errorf("r2v requires at least 1 reference_image media item")
	}
	if refCount > MaxR2VRefImages {
		return fmt.Errorf("r2v supports at most %d reference images, got %d", MaxR2VRefImages, refCount)
	}
	for _, m := range media {
		if m.Type != MediaTypeReferenceImage {
			return fmt.Errorf("r2v only allows reference_image media, got %s", m.Type)
		}
	}
	return nil
}

func validateVideoEditMedia(media []MediaItem) error {
	videoCount := countMediaType(media, MediaTypeVideo)
	if videoCount == 0 {
		return fmt.Errorf("video-edit requires exactly 1 video media item")
	}
	if videoCount > 1 {
		return fmt.Errorf("video-edit supports exactly 1 video media item, got %d", videoCount)
	}
	refCount := countMediaType(media, MediaTypeReferenceImage)
	if refCount > MaxVideoEditRefs {
		return fmt.Errorf("video-edit supports at most %d reference images, got %d", MaxVideoEditRefs, refCount)
	}
	for _, m := range media {
		if m.Type != MediaTypeVideo && m.Type != MediaTypeReferenceImage {
			return fmt.Errorf("video-edit only allows video and reference_image media, got %s", m.Type)
		}
	}
	return nil
}

func countMediaType(media []MediaItem, typ string) int {
	n := 0
	for _, m := range media {
		if m.Type == typ {
			n++
		}
	}
	return n
}

func isHTTPURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// isValidMediaValue checks whether a media value is valid for the given media type.
// Image types (first_frame, reference_image) accept HTTP/HTTPS URLs or image base64 data URLs.
// Video type only accepts HTTP/HTTPS URLs.
func isValidMediaValue(mediaType, value string) bool {
	if isHTTPURL(value) {
		return true
	}
	switch mediaType {
	case MediaTypeFirstFrame, MediaTypeReferenceImage:
		return isValidImageDataURL(value)
	default:
		return false
	}
}

// isValidImageDataURL validates data:image/{jpeg|jpg|png|webp};base64,{non-empty}
func isValidImageDataURL(value string) bool {
	if !strings.HasPrefix(value, "data:image/") {
		return false
	}
	rest := value[len("data:image/"):]
	semiIdx := strings.Index(rest, ";")
	if semiIdx <= 0 {
		return false
	}
	subtype := rest[:semiIdx]
	switch subtype {
	case "jpeg", "jpg", "png", "webp":
	default:
		return false
	}
	rest = rest[semiIdx+1:]
	if !strings.HasPrefix(rest, "base64,") {
		return false
	}
	return len(rest) > len("base64,")
}

// isDurationExplicit checks whether the original request JSON contained a "duration" key.
// TaskSubmitReq.Duration is a plain int (zero-value indistinguishable from "not sent"),
// so we must re-read the cached body to tell the difference.
func isDurationExplicit(c *gin.Context) bool {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return false
	}
	data, err := storage.Bytes()
	if err != nil {
		return false
	}
	var probe struct {
		Duration json.RawMessage `json:"duration"`
	}
	if err := common.Unmarshal(data, &probe); err != nil {
		return false
	}
	return len(probe.Duration) > 0
}

func generateRequestToTaskSubmitReq(req GenerateRequest) relaycommon.TaskSubmitReq {
	taskReq := relaycommon.TaskSubmitReq{
		Prompt: req.Input.Prompt,
		Model:  req.Model,
	}

	// Map media items to TaskSubmitReq fields
	for _, m := range req.Input.Media {
		switch m.Type {
		case MediaTypeFirstFrame:
			if taskReq.Image == "" {
				taskReq.Image = m.URL
			}
		case MediaTypeReferenceImage:
			if req.Model == ModelVideoEdit {
				if taskReq.Metadata == nil {
					taskReq.Metadata = make(map[string]interface{})
				}
				taskReq.Metadata["reference_images"] = append(
					toStringSlice(taskReq.Metadata["reference_images"]), m.URL)
			} else {
				taskReq.Images = append(taskReq.Images, m.URL)
			}
		case MediaTypeVideo:
			if taskReq.Metadata == nil {
				taskReq.Metadata = make(map[string]interface{})
			}
			if _, ok := taskReq.Metadata["video_url"]; !ok {
				taskReq.Metadata["video_url"] = m.URL
			}
		}
	}

	// Map parameters
	if req.Parameters != nil {
		if req.Parameters.Duration != nil && req.Model != ModelVideoEdit {
			taskReq.Duration = *req.Parameters.Duration
		}
		if req.Parameters.Resolution != "" {
			taskReq.Size = req.Parameters.Resolution
			if taskReq.Metadata == nil {
				taskReq.Metadata = make(map[string]interface{})
			}
			taskReq.Metadata["resolution"] = req.Parameters.Resolution
		}
		if req.Parameters.Ratio != "" {
			if taskReq.Metadata == nil {
				taskReq.Metadata = make(map[string]interface{})
			}
			taskReq.Metadata["ratio"] = req.Parameters.Ratio
		}
		if req.Parameters.Watermark != nil {
			if taskReq.Metadata == nil {
				taskReq.Metadata = make(map[string]interface{})
			}
			taskReq.Metadata["watermark"] = *req.Parameters.Watermark
		}
		if req.Parameters.Seed != nil {
			if taskReq.Metadata == nil {
				taskReq.Metadata = make(map[string]interface{})
			}
			taskReq.Metadata["seed"] = *req.Parameters.Seed
		}
	}

	return taskReq
}
