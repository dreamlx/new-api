package happyhorse

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertT2V(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelT2V,
		Prompt: "a cardboard city lights up at night",
		Metadata: map[string]interface{}{
			"resolution": "720P",
			"ratio":      "16:9",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, ModelT2V, got.Model)
	assert.Equal(t, "a cardboard city lights up at night", got.Input.Prompt)
	assert.Nil(t, got.Input.Media)
	assert.Equal(t, "720P", got.Parameters.Resolution)
	assert.Equal(t, "16:9", got.Parameters.Ratio)
	assert.Equal(t, DefaultDuration, *got.Parameters.Duration)
}

func TestConvertT2VDefaultDuration(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelT2V,
		Prompt: "test prompt",
	})
	require.NoError(t, err)
	assert.Equal(t, DefaultDuration, *got.Parameters.Duration)
	assert.Equal(t, DefaultResolution, got.Parameters.Resolution)
}

func TestConvertI2VUsesImageBeforeImages(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelI2V,
		Prompt: "make the cat run",
		Image:  "https://example.com/first-frame.png",
		Images: []string{"https://example.com/alt.png"},
	})
	require.NoError(t, err)
	require.Len(t, got.Input.Media, 1)
	assert.Equal(t, MediaTypeFirstFrame, got.Input.Media[0].Type)
	assert.Equal(t, "https://example.com/first-frame.png", got.Input.Media[0].URL)
}

func TestConvertI2VUsesImagesZeroBeforeInputReference(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:          ModelI2V,
		Prompt:         "make the cat run",
		Images:         []string{"https://example.com/alt.png"},
		InputReference: "https://example.com/ref.png",
	})
	require.NoError(t, err)
	require.Len(t, got.Input.Media, 1)
	assert.Equal(t, MediaTypeFirstFrame, got.Input.Media[0].Type)
	assert.Equal(t, "https://example.com/alt.png", got.Input.Media[0].URL)
}

func TestConvertI2VUsesInputReferenceAsFallback(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:          ModelI2V,
		Prompt:         "make the cat run",
		InputReference: "https://example.com/ref.png",
	})
	require.NoError(t, err)
	require.Len(t, got.Input.Media, 1)
	assert.Equal(t, MediaTypeFirstFrame, got.Input.Media[0].Type)
	assert.Equal(t, "https://example.com/ref.png", got.Input.Media[0].URL)
}

func TestConvertI2VNoImage(t *testing.T) {
	_, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelI2V,
		Prompt: "make the cat run",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "first frame image")
}

func TestConvertR2VMediaDirectPass(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelR2V,
		Prompt: "character picks up fan",
		Metadata: map[string]interface{}{
			"media": []interface{}{
				map[string]interface{}{"type": "reference_image", "url": "https://example.com/person.png"},
				map[string]interface{}{"type": "reference_image", "url": "https://example.com/fan.png"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, got.Input.Media, 2)
	assert.Equal(t, MediaTypeReferenceImage, got.Input.Media[0].Type)
	assert.Equal(t, "https://example.com/person.png", got.Input.Media[0].URL)
	assert.Equal(t, MediaTypeReferenceImage, got.Input.Media[1].Type)
	assert.Equal(t, "https://example.com/fan.png", got.Input.Media[1].URL)
}

func TestConvertR2VImagesToReferenceImage(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelR2V,
		Prompt: "character picks up fan",
		Images: []string{"https://example.com/person.png", "https://example.com/fan.png"},
	})
	require.NoError(t, err)
	require.Len(t, got.Input.Media, 2)
	assert.Equal(t, MediaTypeReferenceImage, got.Input.Media[0].Type)
	assert.Equal(t, "https://example.com/person.png", got.Input.Media[0].URL)
}

func TestConvertR2VNoImages(t *testing.T) {
	_, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelR2V,
		Prompt: "character picks up fan",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reference image")
}

func TestConvertVideoEdit(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelVideoEdit,
		Prompt: "put on striped sweater",
		Metadata: map[string]interface{}{
			"video_url": "https://example.com/input.mp4",
			"reference_images": []interface{}{
				"https://example.com/ref-1.png",
				"https://example.com/ref-2.png",
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, got.Input.Media, 3)
	assert.Equal(t, MediaTypeVideo, got.Input.Media[0].Type)
	assert.Equal(t, "https://example.com/input.mp4", got.Input.Media[0].URL)
	assert.Equal(t, MediaTypeReferenceImage, got.Input.Media[1].Type)
	assert.Equal(t, "https://example.com/ref-1.png", got.Input.Media[1].URL)
	assert.Equal(t, MediaTypeReferenceImage, got.Input.Media[2].Type)
	assert.Equal(t, "https://example.com/ref-2.png", got.Input.Media[2].URL)
}

func TestConvertVideoEditMissingVideoURL(t *testing.T) {
	_, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelVideoEdit,
		Prompt: "put on striped sweater",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "video_url")
}

func TestConvertRejectsUnsupportedResolution(t *testing.T) {
	_, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelT2V,
		Prompt: "test",
		Metadata: map[string]interface{}{
			"resolution": "480P",
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported resolution")
}

func TestConvertRejectsUnsupportedRatio(t *testing.T) {
	_, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelT2V,
		Prompt: "test",
		Metadata: map[string]interface{}{
			"ratio": "4:3",
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ratio")
}

func TestConvertSizeAsResolution(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelT2V,
		Prompt: "test",
		Size:   "1080P",
	})
	require.NoError(t, err)
	assert.Equal(t, Resolution1080P, got.Parameters.Resolution)
}

func TestConvertPromptRequired(t *testing.T) {
	_, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model: ModelT2V,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prompt is required")
}

func TestConvertUnsupportedModel(t *testing.T) {
	_, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  "unknown-model",
		Prompt: "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported model")
}

func TestConvertWatermarkAndSeed(t *testing.T) {
	watermark := true
	seed := 42
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelT2V,
		Prompt: "test",
		Metadata: map[string]interface{}{
			"watermark": watermark,
			"seed":      float64(seed),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, got.Parameters.Watermark)
	assert.True(t, *got.Parameters.Watermark)
	require.NotNil(t, got.Parameters.Seed)
	assert.Equal(t, seed, *got.Parameters.Seed)
}

func TestConvertSoundPassThrough(t *testing.T) {
	sound := false
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelT2V,
		Prompt: "test",
		Metadata: map[string]interface{}{
			"sound": sound,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, got.Parameters.Sound)
	assert.False(t, *got.Parameters.Sound)
}

func TestConvertQualityPassThrough(t *testing.T) {
	got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
		Model:  ModelT2V,
		Prompt: "test",
		Metadata: map[string]interface{}{
			"quality": "high",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "high", got.Parameters.Quality)
}

func TestGenerateRequestToTaskSubmitReqT2V(t *testing.T) {
	d5 := 5
	req := GenerateRequest{
		Model: ModelT2V,
		Input: Input{Prompt: "a city at night"},
		Parameters: &Parameters{
			Resolution: "720P",
			Duration:   &d5,
		},
	}
	got := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, ModelT2V, got.Model)
	assert.Equal(t, "a city at night", got.Prompt)
	assert.Equal(t, 5, got.Duration)
	assert.Equal(t, "720P", got.Size)
}

func TestGenerateRequestToTaskSubmitReqI2V(t *testing.T) {
	req := GenerateRequest{
		Model: ModelI2V,
		Input: Input{
			Prompt: "animate this",
			Media: []MediaItem{
				{Type: MediaTypeFirstFrame, URL: "https://example.com/frame.png"},
			},
		},
	}
	got := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, "https://example.com/frame.png", got.Image)
}

func TestGenerateRequestToTaskSubmitReqR2V(t *testing.T) {
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
	got := generateRequestToTaskSubmitReq(req)
	require.Len(t, got.Images, 2)
	assert.Equal(t, "https://example.com/person.png", got.Images[0])
	assert.Equal(t, "https://example.com/fan.png", got.Images[1])
}

func TestGenerateRequestToTaskSubmitReqVideoEdit(t *testing.T) {
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
	got := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, "https://example.com/input.mp4", got.Metadata["video_url"])
}

func TestGenerateRequestToTaskSubmitReqNoParameters(t *testing.T) {
	req := GenerateRequest{
		Model: ModelT2V,
		Input: Input{Prompt: "test prompt"},
	}
	got := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, "test prompt", got.Prompt)
	assert.Equal(t, ModelT2V, got.Model)
	assert.Equal(t, 0, got.Duration)
}
