package happyhorse

import (
	"testing"

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
	sound := false
	seed := 42
	watermark := true
	req := GenerateRequest{
		Model: ModelT2V,
		Input: Input{Prompt: "test"},
		Parameters: &Parameters{
			Resolution: "1080P",
			Ratio:      "16:9",
			Duration:   &d10,
			Quality:    "pro",
			Sound:      &sound,
			Seed:       &seed,
			Watermark:  &watermark,
		},
	}
	taskReq := generateRequestToTaskSubmitReq(req)
	assert.Equal(t, 10, taskReq.Duration)
	assert.Equal(t, "1080P", taskReq.Size)
	assert.Equal(t, "1080P", taskReq.Metadata["resolution"])
	assert.Equal(t, "16:9", taskReq.Metadata["ratio"])
	assert.Equal(t, "pro", taskReq.Metadata["quality"])
	assert.Equal(t, false, taskReq.Metadata["sound"])
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
		"duration":   float64(*converted.Parameters.Duration),
		"resolution": ResolutionRatio(converted.Parameters.Resolution),
	}
	assert.Equal(t, 8.0, ratios["duration"])
	assert.InDelta(t, 1.6/0.9, ratios["resolution"], 1e-6)
}

func TestHasMediaType(t *testing.T) {
	media := []MediaItem{
		{Type: MediaTypeVideo, URL: "https://example.com/video.mp4"},
		{Type: MediaTypeReferenceImage, URL: "https://example.com/ref.png"},
	}
	assert.True(t, hasMediaType(media, MediaTypeVideo))
	assert.True(t, hasMediaType(media, MediaTypeReferenceImage))
	assert.False(t, hasMediaType(media, MediaTypeFirstFrame))
	assert.False(t, hasMediaType(nil, MediaTypeVideo))
}
