package happyhorse

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultPending(t *testing.T) {
	result, err := ParseTaskResultHelper(StatusPending)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusQueued, result.Status)
	assert.Equal(t, "10%", result.Progress)
}

func TestParseTaskResultRunning(t *testing.T) {
	result, err := ParseTaskResultHelper(StatusRunning)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, result.Status)
	assert.Equal(t, "50%", result.Progress)
}

func TestParseTaskResultSucceeded(t *testing.T) {
	body := buildStatusResponseBody(StatusSucceeded, "https://example.com/video.mp4", "")
	result, err := parseTaskResultFromBody(body)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Equal(t, "https://example.com/video.mp4", result.Url)
}

func TestParseTaskResultFailed(t *testing.T) {
	body := buildStatusResponseBodyWithMessage(StatusFailed, "internal error")
	result, err := parseTaskResultFromBody(body)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	assert.Contains(t, result.Reason, "internal error")
}

func TestParseTaskResultCanceled(t *testing.T) {
	body := buildStatusResponseBody(StatusCanceled, "", "")
	result, err := parseTaskResultFromBody(body)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	assert.Contains(t, result.Reason, StatusCanceled)
}

func TestParseTaskResultUnknown(t *testing.T) {
	body := buildStatusResponseBody(StatusUnknown, "", "")
	result, err := parseTaskResultFromBody(body)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	assert.Contains(t, result.Reason, StatusUnknown)
}

func ParseTaskResultHelper(status string) (*relaycommon.TaskInfo, error) {
	adaptor := &TaskAdaptor{}
	return adaptor.ParseTaskResult([]byte(`{"output":{"task_status":"` + status + `"}}`))
}

func buildStatusResponseBody(status, videoURL, message string) string {
	msg := ""
	if message != "" {
		msg = `,"message":"` + message + `"`
	}
	vurl := ""
	if videoURL != "" {
		vurl = `,"video_url":"` + videoURL + `"`
	}
	return `{"output":{"task_status":"` + status + `"` + vurl + msg + `}}`
}

func buildStatusResponseBodyWithMessage(status, message string) string {
	return buildStatusResponseBody(status, "", message)
}

func parseTaskResultFromBody(body string) (*relaycommon.TaskInfo, error) {
	adaptor := &TaskAdaptor{}
	return adaptor.ParseTaskResult([]byte(body))
}
