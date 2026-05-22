package happyhorse

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if isHappyHorseNativeRequest(c) {
		return validateNativeRequest(c, info)
	}
	taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
	if taskErr != nil {
		return taskErr
	}
	return validateHappyHorseTaskRequest(c)
}

// validateHappyHorseTaskRequest validates resolution/ratio and model-specific media
// requirements that ConvertTaskSubmitReq would otherwise check at BuildRequestBody time.
// Doing this in ValidateRequestAndSetAction ensures 400 (not 500) and LocalError=true.
func validateHappyHorseTaskRequest(c *gin.Context) *dto.TaskError {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if !IsHappyHorseModel(req.Model) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("unsupported model: %s", req.Model), "invalid_request", http.StatusBadRequest)
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
	// ratio
	if v := getStringFromMetadata(req.Metadata, "ratio"); v != "" {
		if !ValidRatios[v] {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("unsupported ratio: %s, only 16:9, 9:16 and 1:1 are supported", v),
				"invalid_request", http.StatusBadRequest)
		}
	}
	// model-specific media requirements
	switch req.Model {
	case ModelI2V:
		if resolveFirstFrameImage(req) == "" {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("i2v requires a first frame image (image, images[0], or input_reference)"),
				"invalid_request", http.StatusBadRequest)
		}
	case ModelR2V:
		if len(req.Images) == 0 && req.Image == "" && req.InputReference == "" {
			if _, ok := req.Metadata["media"]; !ok {
				return service.TaskErrorWrapperLocal(
					fmt.Errorf("r2v requires at least one reference image"),
					"invalid_request", http.StatusBadRequest)
			}
		}
	case ModelVideoEdit:
		if getStringFromMetadata(req.Metadata, "video_url") == "" {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("video-edit requires metadata.video_url"),
				"invalid_request", http.StatusBadRequest)
		}
	}
	return nil
}

// validateNativeRequest parses the structured HappyHorse GenerateRequest format
// and normalizes it into a TaskSubmitReq for downstream billing/relay.
func validateNativeRequest(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
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
		if req.Parameters.Ratio != "" && !ValidRatios[req.Parameters.Ratio] {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("unsupported ratio: %s, only 16:9, 9:16 and 1:1 are supported", req.Parameters.Ratio),
				"invalid_request", http.StatusBadRequest)
		}
	}

	// Model-specific media validation
	switch req.Model {
	case ModelI2V:
		if !hasMediaType(req.Input.Media, MediaTypeFirstFrame) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("i2v requires a first_frame media item"),
				"invalid_request", http.StatusBadRequest)
		}
	case ModelR2V:
		if !hasMediaType(req.Input.Media, MediaTypeReferenceImage) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("r2v requires at least one reference_image media item"),
				"invalid_request", http.StatusBadRequest)
		}
	case ModelVideoEdit:
		if !hasMediaType(req.Input.Media, MediaTypeVideo) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("video-edit requires a video media item"),
				"invalid_request", http.StatusBadRequest)
		}
	}

	taskReq := generateRequestToTaskSubmitReq(req)

	c.Set("happyhorse_generate_request", req)

	info.Action = constant.TaskActionGenerate
	c.Set("task_request", taskReq)
	return nil
}

func hasMediaType(media []MediaItem, typ string) bool {
	for _, m := range media {
		if m.Type == typ {
			return true
		}
	}
	return false
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

	// For r2v, also set Images if not already set from reference_image items
	if req.Model == ModelR2V && len(taskReq.Images) > 0 {
		// Already populated from media items above
	}

	// Map parameters
	if req.Parameters != nil {
		if req.Parameters.Duration != nil {
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
		if req.Parameters.Quality != "" {
			if taskReq.Metadata == nil {
				taskReq.Metadata = make(map[string]interface{})
			}
			taskReq.Metadata["quality"] = req.Parameters.Quality
		}
		if req.Parameters.Sound != nil {
			if taskReq.Metadata == nil {
				taskReq.Metadata = make(map[string]interface{})
			}
			taskReq.Metadata["sound"] = *req.Parameters.Sound
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
