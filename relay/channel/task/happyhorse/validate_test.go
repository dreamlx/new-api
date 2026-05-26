package happyhorse

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateNativeT2V(t *testing.T) {
	d5 := 5
	req := GenerateRequest{
		Model: ModelT2V,
		Input: Input{Prompt: "a city at night"},
		Parameters: &Parameters{
			Resolution: "720P",
			Duration:   &d5,
		},
	}
	taskReq := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, ModelT2V, taskReq.Model)
	assert.Equal(t, "a city at night", taskReq.Prompt)
	assert.Equal(t, 5, taskReq.Duration)
	assert.Equal(t, "720P", taskReq.Size)
}

func TestValidateNativeT2VDefaultDuration(t *testing.T) {
	req := GenerateRequest{
		Model: ModelT2V,
		Input: Input{Prompt: "test"},
	}
	taskReq := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, 0, taskReq.Duration) // ConvertTaskSubmitReq defaults to 5
}

func TestValidateNativeI2VFirstFrame(t *testing.T) {
	req := GenerateRequest{
		Model: ModelI2V,
		Input: Input{
			Prompt: "make the cat run",
			Media: []MediaItem{
				{Type: MediaTypeFirstFrame, URL: "https://example.com/frame.png"},
			},
		},
	}
	taskReq := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, "https://example.com/frame.png", taskReq.Image)
}

func TestValidateNativeR2VReferenceImages(t *testing.T) {
	req := GenerateRequest{
		Model: ModelR2V,
		Input: Input{
			Prompt: "character picks up fan",
			Media: []MediaItem{
				{Type: MediaTypeReferenceImage, URL: "https://example.com/person.png"},
				{Type: MediaTypeReferenceImage, URL: "https://example.com/fan.png"},
			},
		},
	}
	taskReq := generateRequestToTaskSubmitReq(req)
	require.Len(t, taskReq.Images, 2)
	assert.Equal(t, "https://example.com/person.png", taskReq.Images[0])
	assert.Equal(t, "https://example.com/fan.png", taskReq.Images[1])
}

func TestValidateNativeVideoEdit(t *testing.T) {
	req := GenerateRequest{
		Model: ModelVideoEdit,
		Input: Input{
			Prompt: "put on sweater",
			Media: []MediaItem{
				{Type: MediaTypeVideo, URL: "https://example.com/input.mp4"},
				{Type: MediaTypeReferenceImage, URL: "https://example.com/ref.png"},
			},
		},
	}
	taskReq := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, "https://example.com/input.mp4", taskReq.Metadata["video_url"])
	refImgs, ok := taskReq.Metadata["reference_images"].([]string)
	require.True(t, ok)
	require.Len(t, refImgs, 1)
	assert.Equal(t, "https://example.com/ref.png", refImgs[0])
}

func TestValidateNativeParametersPassthrough(t *testing.T) {
	d10 := 10
	seed := 42
	watermark := true
	req := GenerateRequest{
		Model: ModelT2V,
		Input: Input{Prompt: "test"},
		Parameters: &Parameters{
			Resolution: "1080P",
			Ratio:      "16:9",
			Duration:   &d10,
			Seed:       &seed,
			Watermark:  &watermark,
		},
	}
	taskReq := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, 10, taskReq.Duration)
	assert.Equal(t, "1080P", taskReq.Size)
	assert.Equal(t, "1080P", taskReq.Metadata["resolution"])
	assert.Equal(t, "16:9", taskReq.Metadata["ratio"])
	assert.Equal(t, 42, taskReq.Metadata["seed"])
	assert.Equal(t, true, taskReq.Metadata["watermark"])
}

func TestValidateNativeEstimateBillingWorks(t *testing.T) {
	d8 := 8
	req := GenerateRequest{
		Model: ModelT2V,
		Input: Input{Prompt: "a city at night"},
		Parameters: &Parameters{
			Resolution: "1080P",
			Duration:   &d8,
		},
	}
	taskReq := generateRequestToTaskSubmitReq(req)

	converted, err := ConvertTaskSubmitReq(taskReq)
	require.NoError(t, err)
	assert.Equal(t, 8, *converted.Parameters.Duration)
	assert.Equal(t, Resolution1080P, converted.Parameters.Resolution)

	ratios := map[string]float64{
		"seconds":    float64(*converted.Parameters.Duration),
		"resolution": ResolutionRatio(converted.Parameters.Resolution),
	}
	assert.Equal(t, 8.0, ratios["seconds"])
	assert.InDelta(t, 1.6/0.9, ratios["resolution"], 1e-6)
}

func TestCountMediaType(t *testing.T) {
	media := []MediaItem{
		{Type: MediaTypeVideo, URL: "https://example.com/video.mp4"},
		{Type: MediaTypeReferenceImage, URL: "https://example.com/ref.png"},
		{Type: MediaTypeReferenceImage, URL: "https://example.com/ref2.png"},
	}
	assert.Equal(t, 1, countMediaType(media, MediaTypeVideo))
	assert.Equal(t, 2, countMediaType(media, MediaTypeReferenceImage))
	assert.Equal(t, 0, countMediaType(media, MediaTypeFirstFrame))
	assert.Equal(t, 0, countMediaType(nil, MediaTypeVideo))
}

// --- Duration validation tests ---

func setupGinContext(body []byte, path string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	_, _ = common.GetRequestBody(c)
	return c
}

func TestValidateNativeDurationZero(t *testing.T) {
	d0 := 0
	req := GenerateRequest{
		Model:      ModelT2V,
		Input:      Input{Prompt: "test"},
		Parameters: &Parameters{Duration: &d0},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Contains(t, taskErr.Message, "duration must be between")
}

func TestValidateNativeDurationTooLarge(t *testing.T) {
	d16 := 16
	req := GenerateRequest{
		Model:      ModelT2V,
		Input:      Input{Prompt: "test"},
		Parameters: &Parameters{Duration: &d16},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "duration must be between")
}

func TestValidateNativeDurationMinAllowed(t *testing.T) {
	d3 := 3
	req := GenerateRequest{
		Model:      ModelT2V,
		Input:      Input{Prompt: "test"},
		Parameters: &Parameters{Duration: &d3},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	assert.Nil(t, taskErr)
}

func TestValidateNativeDurationMaxAllowed(t *testing.T) {
	d15 := 15
	req := GenerateRequest{
		Model:      ModelT2V,
		Input:      Input{Prompt: "test"},
		Parameters: &Parameters{Duration: &d15},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	assert.Nil(t, taskErr)
}

func TestValidateNativeDurationMissing(t *testing.T) {
	req := GenerateRequest{
		Model: ModelT2V,
		Input: Input{Prompt: "test"},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	assert.Nil(t, taskErr)
}

// Video Edit rejects duration
func TestValidateVideoEditRejectsDuration(t *testing.T) {
	d5 := 5
	req := GenerateRequest{
		Model: ModelVideoEdit,
		Input: Input{
			Prompt: "test",
			Media: []MediaItem{
				{Type: MediaTypeVideo, URL: "https://example.com/input.mp4"},
			},
		},
		Parameters: &Parameters{Duration: &d5},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "video-edit does not support duration")
}

// I2V rejects ratio
func TestValidateI2VRejectsRatio(t *testing.T) {
	req := GenerateRequest{
		Model: ModelI2V,
		Input: Input{
			Prompt: "test",
			Media: []MediaItem{
				{Type: MediaTypeFirstFrame, URL: "https://example.com/frame.png"},
			},
		},
		Parameters: &Parameters{Ratio: "16:9"},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "does not support ratio")
}

// Video Edit rejects ratio
func TestValidateVideoEditRejectsRatio(t *testing.T) {
	req := GenerateRequest{
		Model: ModelVideoEdit,
		Input: Input{
			Prompt: "test",
			Media: []MediaItem{
				{Type: MediaTypeVideo, URL: "https://example.com/input.mp4"},
			},
		},
		Parameters: &Parameters{Ratio: "16:9"},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "does not support ratio")
}

// I2V requires exactly 1 first_frame
func TestValidateI2VMultipleFirstFrames(t *testing.T) {
	req := GenerateRequest{
		Model: ModelI2V,
		Input: Input{
			Prompt: "test",
			Media: []MediaItem{
				{Type: MediaTypeFirstFrame, URL: "https://example.com/a.png"},
				{Type: MediaTypeFirstFrame, URL: "https://example.com/b.png"},
			},
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "exactly 1 first_frame")
}

// R2V rejects >9 reference images
func TestValidateR2VTooManyRefs(t *testing.T) {
	media := make([]MediaItem, 10)
	for i := range media {
		media[i] = MediaItem{Type: MediaTypeReferenceImage, URL: "https://example.com/ref.png"}
	}
	req := GenerateRequest{
		Model: ModelR2V,
		Input: Input{Prompt: "test", Media: media},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "at most 9 reference images")
}

// R2V rejects non-reference_image media type
func TestValidateR2VRejectsVideoMedia(t *testing.T) {
	req := GenerateRequest{
		Model: ModelR2V,
		Input: Input{
			Prompt: "test",
			Media: []MediaItem{
				{Type: MediaTypeReferenceImage, URL: "https://example.com/ref.png"},
				{Type: MediaTypeVideo, URL: "https://example.com/vid.mp4"},
			},
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "only allows reference_image")
}

// Video Edit rejects >1 video
func TestValidateVideoEditMultipleVideos(t *testing.T) {
	req := GenerateRequest{
		Model: ModelVideoEdit,
		Input: Input{
			Prompt: "test",
			Media: []MediaItem{
				{Type: MediaTypeVideo, URL: "https://example.com/a.mp4"},
				{Type: MediaTypeVideo, URL: "https://example.com/b.mp4"},
			},
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "exactly 1 video")
}

// Video Edit rejects >5 reference images
func TestValidateVideoEditTooManyRefs(t *testing.T) {
	media := []MediaItem{
		{Type: MediaTypeVideo, URL: "https://example.com/vid.mp4"},
	}
	for i := 0; i < 6; i++ {
		media = append(media, MediaItem{Type: MediaTypeReferenceImage, URL: "https://example.com/ref.png"})
	}
	req := GenerateRequest{
		Model: ModelVideoEdit,
		Input: Input{Prompt: "test", Media: media},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "at most 5 reference images")
}

// Media URL validation
func TestValidateMediaEmptyURL(t *testing.T) {
	req := GenerateRequest{
		Model: ModelI2V,
		Input: Input{
			Prompt: "test",
			Media:  []MediaItem{{Type: MediaTypeFirstFrame, URL: ""}},
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "media item url is required")
}

func TestValidateMediaInvalidScheme(t *testing.T) {
	req := GenerateRequest{
		Model: ModelI2V,
		Input: Input{
			Prompt: "test",
			Media:  []MediaItem{{Type: MediaTypeFirstFrame, URL: "ftp://example.com/frame.png"}},
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "http or https")
}

// T2V/R2V accept 4:3 and 3:4 ratios
func TestValidateNativeRatio4x3(t *testing.T) {
	d5 := 5
	req := GenerateRequest{
		Model: ModelT2V,
		Input: Input{Prompt: "test"},
		Parameters: &Parameters{
			Ratio:    "4:3",
			Duration: &d5,
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	assert.Nil(t, taskErr)
}

func TestValidateNativeRatio3x4R2V(t *testing.T) {
	req := GenerateRequest{
		Model: ModelR2V,
		Input: Input{
			Prompt: "test",
			Media: []MediaItem{
				{Type: MediaTypeReferenceImage, URL: "https://example.com/ref.png"},
			},
		},
		Parameters: &Parameters{Ratio: "3:4"},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/happyhorse/api/generate")
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	assert.Nil(t, taskErr)
}

// V1 path: duration validation
func TestValidateV1DurationTooSmall(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:    ModelT2V,
		Prompt:   "test",
		Duration: 2,
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "duration must be between")
}

func TestValidateV1DurationTooLarge(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:    ModelT2V,
		Prompt:   "test",
		Duration: 16,
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "duration must be between")
}

func TestValidateV1DurationMinAllowed(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:    ModelT2V,
		Prompt:   "test",
		Duration: 3,
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	assert.Nil(t, taskErr)
}

func TestValidateV1DurationMissing(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  ModelT2V,
		Prompt: "test",
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	assert.Nil(t, taskErr)
}

// --- V1 bypass-path tests ---

// V1 Video Edit with explicit duration → 400
func TestValidateV1VideoEditRejectsDuration(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:    ModelVideoEdit,
		Prompt:   "test",
		Duration: 5,
		Metadata: map[string]interface{}{
			"video_url": "https://example.com/input.mp4",
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "video-edit does not support duration")
}

// V1 R2V with metadata.media containing too many reference images → 400
func TestValidateV1R2VMetadataMediaTooManyRefs(t *testing.T) {
	refs := make([]interface{}, 10)
	for i := range refs {
		refs[i] = map[string]interface{}{
			"type": MediaTypeReferenceImage,
			"url":  "https://example.com/ref.png",
		}
	}
	req := relaycommon.TaskSubmitReq{
		Model:  ModelR2V,
		Prompt: "test",
		Metadata: map[string]interface{}{
			"media": refs,
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "at most 9 reference images")
}

// V1 media URL validation: non-http scheme → 400
func TestValidateV1MediaInvalidScheme(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  ModelI2V,
		Prompt: "test",
		Image:  "ftp://example.com/frame.png",
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "http or https")
}

// V1 I2V with multiple images → 400 (exactly 1 first_frame required)
func TestValidateV1I2VMultipleImages(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  ModelI2V,
		Prompt: "test",
		Images: []string{"https://example.com/a.png", "https://example.com/b.png"},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "exactly 1 first_frame")
}

// V1 Video Edit with >5 reference_images → 400
func TestValidateV1VideoEditTooManyRefs(t *testing.T) {
	refs := make([]interface{}, 6)
	for i := range refs {
		refs[i] = "https://example.com/ref.png"
	}
	req := relaycommon.TaskSubmitReq{
		Model:  ModelVideoEdit,
		Prompt: "test",
		Metadata: map[string]interface{}{
			"video_url":        "https://example.com/input.mp4",
			"reference_images": refs,
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "at most 5 reference images")
}

// V1 empty URL in images[] → 400
func TestValidateV1I2VEmptyURLInImages(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  ModelI2V,
		Prompt: "test",
		Images: []string{"https://example.com/a.png", ""},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "empty url")
}

// V1 empty URL in reference_images → 400
func TestValidateV1VideoEditEmptyURLInRefImages(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  ModelVideoEdit,
		Prompt: "test",
		Metadata: map[string]interface{}{
			"video_url":        "https://example.com/input.mp4",
			"reference_images": []interface{}{"https://example.com/ref.png", ""},
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "empty url")
}

// V1 Video Edit rejects ratio
func TestValidateV1VideoEditRejectsRatio(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  ModelVideoEdit,
		Prompt: "test",
		Metadata: map[string]interface{}{
			"video_url": "https://example.com/input.mp4",
			"ratio":     "16:9",
		},
	}
	body, err := common.Marshal(req)
	require.NoError(t, err)

	c := setupGinContext(body, "/v1/video/generations")
	var parsed relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(c, &parsed))
	c.Set("task_request", parsed)

	taskErr := validateHappyHorseTaskRequest(c)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "does not support ratio")
}
