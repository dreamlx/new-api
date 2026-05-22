package happyhorse

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolutionRatio720P(t *testing.T) {
	assert.Equal(t, 1.0, ResolutionRatio("720P"))
}

func TestResolutionRatio1080P(t *testing.T) {
	ratio := ResolutionRatio("1080P")
	expected := 1.6 / 0.9
	assert.True(t, math.Abs(ratio-expected) < 1e-6, "expected %f, got %f", expected, ratio)
}

func TestResolutionRatioDefault(t *testing.T) {
	assert.Equal(t, 1.0, ResolutionRatio("unknown"))
}

func TestBillableDurationPrefersOutputVideoDuration(t *testing.T) {
	usage := &Usage{
		OutputVideoDuration: 7.5,
		Duration:            5.0,
	}
	assert.Equal(t, 7.5, BillableDuration(usage))
}

func TestBillableDurationFallsBackToDuration(t *testing.T) {
	usage := &Usage{
		Duration: 5.0,
	}
	assert.Equal(t, 5.0, BillableDuration(usage))
}

func TestBillableDurationNilUsage(t *testing.T) {
	assert.Equal(t, 0.0, BillableDuration(nil))
}

func TestBillableDurationZeroFields(t *testing.T) {
	usage := &Usage{}
	assert.Equal(t, 0.0, BillableDuration(usage))
}

func TestBillableDurationIgnoresInputVideoDuration(t *testing.T) {
	usage := &Usage{
		InputVideoDuration:  10.0,
		OutputVideoDuration: 3.0,
	}
	assert.Equal(t, 3.0, BillableDuration(usage))
}

func TestAdjustBillingOnCompleteUsesOutputSecondsAndQuotaPerUnit(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		Data: []byte(`{"output":{"task_status":"SUCCEEDED"},"usage":{"output_video_duration":3,"duration":5,"SR":1080}}`),
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.9,
				GroupRatio: 2,
				ModelRatio: 99,
			},
		},
	}

	quota := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{})

	expected := int(0.9 * common.QuotaPerUnit * 2 * 3 * (1.6 / 0.9))
	assert.Equal(t, expected, quota)
}

func TestConvertTaskToStatusResponseBodyReturnsNativeFormat(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			UpstreamModelName: ModelT2V,
		},
		Data: []byte(`{"output":{"task_id":"upstream","task_status":"SUCCEEDED","video_url":"https://example.com/video.mp4"},"usage":{"duration":5,"output_video_duration":5.2,"ratio":"16:9"}}`),
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://example.com/video.mp4",
		},
	}

	body, err := ConvertTaskToStatusResponseBody(task)
	require.NoError(t, err)

	var resp NativeStatusResponse
	require.NoError(t, common.Unmarshal(body, &resp))
	assert.Equal(t, "task_public", resp.TaskID)
	assert.Equal(t, NativeStatusCompleted, resp.Status)
	require.NotNil(t, resp.Data)
	assert.Equal(t, ModelT2V, resp.Data.Model)
	assert.Equal(t, ModeT2V, resp.Data.Mode)
	assert.Equal(t, 5, resp.Data.Duration) // prefers output_video_duration (5.2 → 5)
	assert.Equal(t, "16:9", resp.Data.AspectRatio)
	assert.Equal(t, "https://example.com/video.mp4", resp.Data.VideoURL)
	require.Len(t, resp.Data.ResultUrls, 1)
	assert.Equal(t, "https://example.com/video.mp4", resp.Data.ResultUrls[0])
}

func TestConvertTaskToStatusResponseBodyDurationFallback(t *testing.T) {
	task := &model.Task{
		TaskID: "task_dur",
		Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			UpstreamModelName: ModelT2V,
		},
		Data: []byte(`{"output":{"task_status":"SUCCEEDED"},"usage":{"duration":8}}`),
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://example.com/video.mp4",
		},
	}

	body, err := ConvertTaskToStatusResponseBody(task)
	require.NoError(t, err)

	var resp NativeStatusResponse
	require.NoError(t, common.Unmarshal(body, &resp))
	require.NotNil(t, resp.Data)
	assert.Equal(t, 8, resp.Data.Duration) // falls back to duration
}

func TestConvertTaskToStatusResponseBodyModeMapping(t *testing.T) {
	task := &model.Task{
		TaskID: "task_i2v",
		Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			UpstreamModelName: ModelI2V,
		},
		Data: []byte(`{"output":{"task_status":"SUCCEEDED"},"usage":{}}`),
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://example.com/video.mp4",
		},
	}

	body, err := ConvertTaskToStatusResponseBody(task)
	require.NoError(t, err)

	var resp NativeStatusResponse
	require.NoError(t, common.Unmarshal(body, &resp))
	require.NotNil(t, resp.Data)
	assert.Equal(t, ModeI2V, resp.Data.Mode)
}

func TestConvertTaskToStatusResponseBodyFailedTask(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_fail",
		Status:     model.TaskStatusFailure,
		FailReason: "internal error",
		Data:       []byte(`{}`),
	}

	body, err := ConvertTaskToStatusResponseBody(task)
	require.NoError(t, err)

	var resp NativeStatusResponse
	require.NoError(t, common.Unmarshal(body, &resp))
	assert.Equal(t, "task_fail", resp.TaskID)
	assert.Equal(t, NativeStatusFailed, resp.Status)
	assert.Equal(t, "internal error", resp.Message)
}

func TestToNativeStatus(t *testing.T) {
	assert.Equal(t, NativeStatusPending, toNativeStatus(model.TaskStatusQueued))
	assert.Equal(t, NativeStatusPending, toNativeStatus(model.TaskStatusSubmitted))
	assert.Equal(t, NativeStatusRunning, toNativeStatus(model.TaskStatusInProgress))
	assert.Equal(t, NativeStatusCompleted, toNativeStatus(model.TaskStatusSuccess))
	assert.Equal(t, NativeStatusFailed, toNativeStatus(model.TaskStatusFailure))
}

func TestAdjustBillingOnCompleteRefundDirection(t *testing.T) {
	adaptor := &TaskAdaptor{}
	// Pre-consumed for 5 seconds at 1080P, but actual output is only 3 seconds
	task := &model.Task{
		Data: []byte(`{"output":{"task_status":"SUCCEEDED"},"usage":{"output_video_duration":3,"duration":5,"SR":1080}}`),
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.9,
				GroupRatio: 2,
				ModelRatio: 99,
			},
		},
	}

	quota := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{})

	// Actual quota for 3s at 1080P
	actualQuota := int(0.9 * common.QuotaPerUnit * 2 * 3 * (1.6 / 0.9))
	// Pre-consumed quota for 5s at 1080P
	preConsumedQuota := int(0.9 * common.QuotaPerUnit * 2 * 5 * (1.6 / 0.9))
	assert.Equal(t, actualQuota, quota)
	assert.Less(t, quota, preConsumedQuota)
}

func TestAdjustBillingOnCompleteResolutionFallback(t *testing.T) {
	adaptor := &TaskAdaptor{}
	// Upstream returns no SR field, but billing context has resolution ratio from submit
	task := &model.Task{
		Data: []byte(`{"output":{"task_status":"SUCCEEDED"},"usage":{"output_video_duration":5}}`),
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 0.9,
				GroupRatio: 2,
				ModelRatio: 99,
				OtherRatios: map[string]float64{
					"resolution": 1.6 / 0.9, // 1080P ratio saved at submit time
				},
			},
		},
	}

	quota := adaptor.AdjustBillingOnComplete(task, &relaycommon.TaskInfo{})

	// Should use 1080P ratio from OtherRatios, not default 720P
	expected := int(0.9 * common.QuotaPerUnit * 2 * 5 * (1.6 / 0.9))
	assert.Equal(t, expected, quota)
}
