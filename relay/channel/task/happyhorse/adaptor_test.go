package happyhorse

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWireBodyV1T2V(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:    ModelT2V,
		Prompt:   "a city at night",
		Duration: 5,
		Metadata: map[string]interface{}{
			"resolution": "720P",
			"ratio":      "16:9",
		},
	})
	require.NoError(t, err)

	body, err := common.Marshal(got)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &m))

	assert.Equal(t, ModelT2V, m["model"])
	input := m["input"].(map[string]interface{})
	assert.Equal(t, "a city at night", input["prompt"])

	params := m["parameters"].(map[string]interface{})
	assert.Equal(t, "720P", params["resolution"])
	assert.Equal(t, "16:9", params["ratio"])

	assert.Nil(t, m["img_url"])
	assert.Nil(t, params["size"])
	assert.Nil(t, params["prompt_extend"])
}

func TestWireBodyV1I2V(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelI2V,
		Prompt: "animate this",
		Image:  "https://example.com/frame.png",
	})
	require.NoError(t, err)

	body, err := common.Marshal(got)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &m))

	input := m["input"].(map[string]interface{})
	media := input["media"].([]interface{})
	require.Len(t, media, 1)
	item := media[0].(map[string]interface{})
	assert.Equal(t, MediaTypeFirstFrame, item["type"])
	assert.Equal(t, "https://example.com/frame.png", item["url"])
}

func TestWireBodyV1R2V(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelR2V,
		Prompt: "character picks up fan",
		Images: []string{"https://example.com/person.png", "https://example.com/fan.png"},
	})
	require.NoError(t, err)

	body, err := common.Marshal(got)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &m))

	input := m["input"].(map[string]interface{})
	media := input["media"].([]interface{})
	require.Len(t, media, 2)
	item0 := media[0].(map[string]interface{})
	assert.Equal(t, MediaTypeReferenceImage, item0["type"])
}

func TestWireBodyV1VideoEdit(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelVideoEdit,
		Prompt: "put on sweater",
		Metadata: map[string]interface{}{
			"video_url": "https://example.com/input.mp4",
			"reference_images": []interface{}{
				"https://example.com/ref.png",
			},
		},
	})
	require.NoError(t, err)

	body, err := common.Marshal(got)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &m))

	input := m["input"].(map[string]interface{})
	media := input["media"].([]interface{})
	require.Len(t, media, 2)
	videoItem := media[0].(map[string]interface{})
	assert.Equal(t, MediaTypeVideo, videoItem["type"])
	refItem := media[1].(map[string]interface{})
	assert.Equal(t, MediaTypeReferenceImage, refItem["type"])
}

func TestWireBodyNativeStructured(t *testing.T) {
	d5 := 5
	sound := false
	hhReq := GenerateRequest{
		Model: ModelT2V,
		Input: Input{
			Prompt: "a city at night",
			Media: []MediaItem{
				{Type: MediaTypeFirstFrame, URL: "https://example.com/frame.png"},
			},
		},
		Parameters: &Parameters{
			Resolution: "1080P",
			Ratio:      "9:16",
			Duration:   &d5,
			Quality:    "pro",
			Sound:      &sound,
		},
	}

	body, err := common.Marshal(hhReq)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &m))

	assert.Equal(t, ModelT2V, m["model"])
	input := m["input"].(map[string]interface{})
	assert.Equal(t, "a city at night", input["prompt"])
	media := input["media"].([]interface{})
	require.Len(t, media, 1)

	params := m["parameters"].(map[string]interface{})
	assert.Equal(t, "1080P", params["resolution"])
	assert.Equal(t, "9:16", params["ratio"])
	assert.Equal(t, "pro", params["quality"])

	assert.Nil(t, m["img_url"])
	assert.Nil(t, params["size"])
	assert.Nil(t, params["prompt_extend"])
}

func TestUpstreamErrorMessageBoth(t *testing.T) {
	resp := StatusResponse{
		Output: StatusOutput{
			Code:    "InvalidParameter",
			Message: "invalid media url",
		},
	}
	msg, ok := upstreamErrorMessage(resp)
	assert.True(t, ok)
	assert.Equal(t, "InvalidParameter: invalid media url", msg)
}

func TestUpstreamErrorMessageCodeOnly(t *testing.T) {
	resp := StatusResponse{
		Output: StatusOutput{
			Code: "InvalidParameter",
		},
	}
	msg, ok := upstreamErrorMessage(resp)
	assert.True(t, ok)
	assert.Equal(t, "InvalidParameter", msg)
}

func TestUpstreamErrorMessageMsgOnly(t *testing.T) {
	resp := StatusResponse{
		Output: StatusOutput{
			Message: "invalid media url",
		},
	}
	msg, ok := upstreamErrorMessage(resp)
	assert.True(t, ok)
	assert.Equal(t, "invalid media url", msg)
}

func TestUpstreamErrorMessageEmpty(t *testing.T) {
	resp := StatusResponse{
		Output: StatusOutput{
			TaskID: "abc123",
		},
	}
	_, ok := upstreamErrorMessage(resp)
	assert.False(t, ok)
}

// --- BuildRequestBody integration tests ---

func newTestContext(path string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
}

func parseBodyToMap(t *testing.T, r io.Reader) map[string]interface{} {
	t.Helper()
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, common.Unmarshal(data, &m))
	return m
}

func TestBuildRequestBodyNativePath(t *testing.T) {
	d5 := 5
	c := newTestContext("/happyhorse/api/generate")
	c.Set("happyhorse_generate_request", GenerateRequest{
		Model: ModelT2V,
		Input: Input{
			Prompt: "a city at night",
		},
		Parameters: &Parameters{
			Resolution: "1080P",
			Duration:   &d5,
		},
	})

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelT2V,
		},
	}

	a := &TaskAdaptor{}
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)

	m := parseBodyToMap(t, reader)
	assert.Equal(t, ModelT2V, m["model"])
	input := m["input"].(map[string]interface{})
	assert.Equal(t, "a city at night", input["prompt"])
	params := m["parameters"].(map[string]interface{})
	assert.Equal(t, "1080P", params["resolution"])

	// No Ali schema fields
	assert.Nil(t, m["img_url"])
	assert.Nil(t, params["size"])
	assert.Nil(t, params["prompt_extend"])
}

func TestBuildRequestBodyV1Path(t *testing.T) {
	c := newTestContext("/v1/video/generations")
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    ModelI2V,
		Prompt:   "animate this",
		Image:    "https://example.com/frame.png",
		Duration: 5,
		Metadata: map[string]interface{}{
			"resolution": "720P",
			"ratio":      "16:9",
		},
	})

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelI2V,
		},
	}

	a := &TaskAdaptor{}
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)

	m := parseBodyToMap(t, reader)
	assert.Equal(t, ModelI2V, m["model"])
	input := m["input"].(map[string]interface{})
	assert.Equal(t, "animate this", input["prompt"])
	media := input["media"].([]interface{})
	require.Len(t, media, 1)
	item := media[0].(map[string]interface{})
	assert.Equal(t, MediaTypeFirstFrame, item["type"])
	assert.Equal(t, "https://example.com/frame.png", item["url"])

	// No Ali schema fields
	assert.Nil(t, m["img_url"])
	params := m["parameters"].(map[string]interface{})
	assert.Nil(t, params["size"])
	assert.Nil(t, params["prompt_extend"])
}

func TestBuildRequestBodyNativePathOverridesModel(t *testing.T) {
	d5 := 5
	c := newTestContext("/happyhorse/api/generate")
	c.Set("happyhorse_generate_request", GenerateRequest{
		Model: ModelT2V,
		Input: Input{
			Prompt: "test prompt",
		},
		Parameters: &Parameters{
			Duration: &d5,
		},
	})

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelI2V, // channel config overrides
		},
	}

	a := &TaskAdaptor{}
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)

	m := parseBodyToMap(t, reader)
	assert.Equal(t, ModelI2V, m["model"]) // should use UpstreamModelName from channel config
}

func TestBuildRequestBodyV1PathR2V(t *testing.T) {
	c := newTestContext("/v1/video/generations")
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  ModelR2V,
		Prompt: "character picks up fan",
		Images: []string{"https://example.com/person.png", "https://example.com/fan.png"},
	})

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelR2V,
		},
	}

	a := &TaskAdaptor{}
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)

	m := parseBodyToMap(t, reader)
	input := m["input"].(map[string]interface{})
	media := input["media"].([]interface{})
	require.Len(t, media, 2)
	item0 := media[0].(map[string]interface{})
	assert.Equal(t, MediaTypeReferenceImage, item0["type"])
	item1 := media[1].(map[string]interface{})
	assert.Equal(t, MediaTypeReferenceImage, item1["type"])
}

func TestBuildRequestBodyV1PathVideoEdit(t *testing.T) {
	c := newTestContext("/v1/video/generations")
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  ModelVideoEdit,
		Prompt: "put on sweater",
		Metadata: map[string]interface{}{
			"video_url": "https://example.com/input.mp4",
			"reference_images": []interface{}{
				"https://example.com/ref.png",
			},
		},
	})

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelVideoEdit,
		},
	}

	a := &TaskAdaptor{}
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)

	m := parseBodyToMap(t, reader)
	input := m["input"].(map[string]interface{})
	media := input["media"].([]interface{})
	require.Len(t, media, 2)
	videoItem := media[0].(map[string]interface{})
	assert.Equal(t, MediaTypeVideo, videoItem["type"])
	refItem := media[1].(map[string]interface{})
	assert.Equal(t, MediaTypeReferenceImage, refItem["type"])
}
