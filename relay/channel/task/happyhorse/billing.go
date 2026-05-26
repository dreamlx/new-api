package happyhorse

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func ResolutionRatio(resolution string) float64 {
	switch strings.ToUpper(resolution) {
	case Resolution1080P:
		return 1.6 / 0.9
	default:
		return 1.0
	}
}

func BillableDuration(usage *Usage) float64 {
	if usage == nil {
		return 0
	}
	return billableDurationForModel(usage, "")
}

// BillableDurationForModel computes the billable seconds for a given model.
// Video Edit bills input + output duration; other models bill output duration only.
func BillableDurationForModel(usage *Usage, model string) float64 {
	if usage == nil {
		return 0
	}
	return billableDurationForModel(usage, model)
}

func billableDurationForModel(usage *Usage, model string) float64 {
	if model == ModelVideoEdit {
		total := usage.InputVideoDuration + usage.OutputVideoDuration
		if total > 0 {
			return total
		}
		// Fallback: if upstream only returns duration, use it conservatively
		if usage.Duration > 0 {
			return usage.Duration
		}
		return 0
	}
	if usage.OutputVideoDuration > 0 {
		return usage.OutputVideoDuration
	}
	if usage.Duration > 0 {
		return usage.Duration
	}
	return 0
}

func ConvertTaskToStatusResponseBody(task *model.Task) ([]byte, error) {
	resp := NativeStatusResponse{
		TaskID: task.TaskID,
		Status: toNativeStatus(task.Status),
	}

	if task.Status == model.TaskStatusSuccess {
		videoURL := task.GetResultURL()
		if videoURL != "" {
			data := &NativeStatusData{
				VideoURL:   videoURL,
				ResultUrls: []string{videoURL},
			}

			// Model/mode from task properties
			upstreamModel := task.Properties.UpstreamModelName
			data.Model = upstreamModel
			if mode, ok := ModelToMode[upstreamModel]; ok {
				data.Mode = mode
			}

			// Duration/aspect_ratio from upstream usage
			var upstream StatusResponse
			if len(task.Data) > 0 {
				_ = common.Unmarshal(task.Data, &upstream)
			}
			if upstream.Usage != nil {
				if upstream.Usage.OutputVideoDuration > 0 {
					data.Duration = int(upstream.Usage.OutputVideoDuration)
				} else if upstream.Usage.Duration > 0 {
					data.Duration = int(upstream.Usage.Duration)
				}
				if upstream.Usage.Ratio != "" {
					data.AspectRatio = upstream.Usage.Ratio
				}
			}

			resp.Data = data
		}
	}

	if task.Status == model.TaskStatusFailure {
		resp.Message = firstNonEmpty(task.FailReason, "Task failed")
	}

	return common.Marshal(resp)
}

func toNativeStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusQueued, model.TaskStatusSubmitted, model.TaskStatusNotStart:
		return NativeStatusPending
	case model.TaskStatusInProgress:
		return NativeStatusRunning
	case model.TaskStatusSuccess:
		return NativeStatusCompleted
	case model.TaskStatusFailure:
		return NativeStatusFailed
	default:
		return NativeStatusFailed
	}
}

