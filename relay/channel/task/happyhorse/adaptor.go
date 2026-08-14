package happyhorse

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

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

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// Native path: read GenerateRequest directly from context
	if v, exists := c.Get("happyhorse_generate_request"); exists {
		if hhReq, ok := v.(GenerateRequest); ok {
			return estimateBillingFromGenerateRequest(hhReq)
		}
	}

	// V1 path: convert TaskSubmitReq -> GenerateRequest
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	hhReq, err := ConvertTaskSubmitReq(req)
	if err != nil {
		return nil
	}
	return estimateBillingFromGenerateRequest(*hhReq)
}

func estimateBillingFromGenerateRequest(hhReq GenerateRequest) map[string]float64 {
	duration := DefaultDuration
	if hhReq.Parameters != nil && hhReq.Parameters.Duration != nil {
		duration = *hhReq.Parameters.Duration
	}
	if duration <= 0 {
		duration = DefaultDuration
	}
	resolution := DefaultResolution
	if hhReq.Parameters != nil && hhReq.Parameters.Resolution != "" {
		resolution = hhReq.Parameters.Resolution
	}
	return map[string]float64{
		"seconds":    float64(duration),
		"resolution": ResolutionRatio(resolution),
	}
}

func (a *TaskAdaptor) AdjustBillingOnSubmit(info *relaycommon.RelayInfo, taskData []byte) map[string]float64 {
	return nil
}

func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	if task.PrivateData.BillingContext == nil {
		return 0
	}

	var resp StatusResponse
	if err := common.Unmarshal(task.Data, &resp); err != nil {
		return 0
	}
	if resp.Usage == nil {
		return 0
	}

	duration := BillableDurationForModel(resp.Usage, task.Properties.UpstreamModelName)
	if duration <= 0 {
		return 0
	}

	bc := task.PrivateData.BillingContext
	if bc == nil || bc.ModelPrice <= 0 {
		return 0
	}

	// Resolution: prefer usage.SR, fall back to submitted resolution ratio
	resolution := DefaultResolution
	sr := resp.Usage.SR.Int()
	if sr == 1080 {
		resolution = Resolution1080P
	} else if sr == 720 {
		resolution = Resolution720P
	} else if bc.OtherRatios != nil {
		if r, ok := bc.OtherRatios["resolution"]; ok && r > 1.0 {
			resolution = Resolution1080P
		}
	}

	quota := int(bc.ModelPrice * common.QuotaPerUnit * bc.GroupRatio * duration * ResolutionRatio(resolution))
	return quota
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/video-synthesis", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	// Native HappyHorse request: already a GenerateRequest, marshal directly
	if v, exists := c.Get("happyhorse_generate_request"); exists {
		if hhReq, ok := v.(GenerateRequest); ok {
			hhReq.Model = info.UpstreamModelName
			body, err := common.Marshal(hhReq)
			if err != nil {
				return nil, fmt.Errorf("marshal generate request: %w", err)
			}
			return bytes.NewReader(body), nil
		}
	}

	// /v1/video/generations: convert TaskSubmitReq -> GenerateRequest
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, fmt.Errorf("get task request: %w", err)
	}
	hhReq, err := ConvertTaskSubmitReq(req)
	if err != nil {
		return nil, err
	}
	hhReq.Model = info.UpstreamModelName
	body, err := common.Marshal(hhReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var dResp StatusResponse
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("unmarshal response: %w", err), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}

	// Check for upstream error before checking task_id
	if errMsg, ok := upstreamErrorMessage(dResp); ok {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("happyhorse upstream error: %s", errMsg), "upstream_error", http.StatusBadRequest)
	}

	upstreamID := dResp.Output.TaskID
	if upstreamID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty in upstream response"), "invalid_response", http.StatusInternalServerError)
	}

	dResp.Output.TaskID = info.PublicTaskID
	if isHappyHorseNativeRequest(c) {
		nativeResp := NativeSubmitResponse{
			TaskID: info.PublicTaskID,
			Status: NativeStatusPending,
		}
		c.JSON(http.StatusOK, nativeResp)
	} else {
		ov := dto.NewOpenAIVideo()
		ov.ID = info.PublicTaskID
		ov.TaskID = info.PublicTaskID
		ov.CreatedAt = time.Now().Unix()
		ov.Model = info.OriginModelName
		c.JSON(http.StatusOK, ov)
	}
	return upstreamID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	uri := fmt.Sprintf("%s/api/v1/tasks/%s", baseUrl, taskID)
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var resp StatusResponse
	if err := common.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal task result: %w", err)
	}

	taskResult := &relaycommon.TaskInfo{Code: 0}

	switch resp.Output.TaskStatus {
	case StatusPending:
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case StatusRunning:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case StatusSucceeded:
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resp.Output.VideoURL
	case StatusFailed:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = firstNonEmpty(resp.Output.Message, resp.Output.Code, StatusFailed)
	case StatusCanceled:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = firstNonEmpty(resp.Output.Message, StatusCanceled)
	case StatusUnknown:
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = firstNonEmpty(resp.Output.Message, StatusUnknown)
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return taskResult, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) DisablePerCallBilling() bool {
	return true
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func isHappyHorseNativeRequest(c *gin.Context) bool {
	return c != nil && c.Request != nil && strings.HasPrefix(c.Request.URL.Path, "/happyhorse/api/")
}

func upstreamErrorMessage(resp StatusResponse) (string, bool) {
	code := resp.Output.Code
	msg := resp.Output.Message
	if code == "" && msg == "" {
		return "", false
	}
	if code != "" && msg != "" {
		return code + ": " + msg, true
	}
	return firstNonEmpty(code, msg), true
}
