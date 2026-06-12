package seedance

import (
	"encoding/json"
	"math"
	"testing"
)

// ============================
// Test calculateCombinedRatio
// ============================

func TestCalculateCombinedRatio(t *testing.T) {
	tests := []struct {
		name          string
		modelName     string
		resolution    string
		hasVideo      bool
		expectedRatio float64
	}{
		// Main model - 480p/720p without video (baseline)
		{"main-720p-no-video", ModelDreamina20, Resolution720P, false, 1.0},
		{"main-480p-no-video", ModelDreamina20, Resolution480P, false, 1.0},
		{"doubao-720p-no-video", ModelDoubao20, Resolution720P, false, 1.0},

		// Main model - 480p/720p with video
		{"main-720p-video", ModelDreamina20, Resolution720P, true, 28.0 / 46.0},
		{"main-480p-video", ModelDreamina20, Resolution480P, true, 28.0 / 46.0},
		{"doubao-720p-video", ModelDoubao20, Resolution720P, true, 28.0 / 46.0},

		// Main model - 1080p without video
		{"main-1080p-no-video", ModelDreamina20, Resolution1080P, false, 51.0 / 46.0},
		{"doubao-1080p-no-video", ModelDoubao20, Resolution1080P, false, 51.0 / 46.0},

		// Main model - 1080p with video
		{"main-1080p-video", ModelDreamina20, Resolution1080P, true, 31.0 / 46.0},
		{"doubao-1080p-video", ModelDoubao20, Resolution1080P, true, 31.0 / 46.0},

		// Fast model - 480p/720p without video
		{"fast-720p-no-video", ModelDreamina20Fast, Resolution720P, false, 1.0},
		{"fast-480p-no-video", ModelDreamina20Fast, Resolution480P, false, 1.0},
		{"doubao-fast-720p-no-video", ModelDoubao20Fast, Resolution720P, false, 1.0},

		// Fast model - 480p/720p with video
		{"fast-720p-video", ModelDreamina20Fast, Resolution720P, true, 22.0 / 37.0},
		{"fast-480p-video", ModelDreamina20Fast, Resolution480P, true, 22.0 / 37.0},
		{"doubao-fast-720p-video", ModelDoubao20Fast, Resolution720P, true, 22.0 / 37.0},

		// Fast model - 1080p (should not happen after validation, but test for defensive programming)
		{"fast-1080p-no-video", ModelDreamina20Fast, Resolution1080P, false, 1.0},
		{"fast-1080p-video", ModelDreamina20Fast, Resolution1080P, true, 22.0 / 37.0},

		// Unknown model (should return 1.0)
		{"unknown-model", "unknown-model", Resolution720P, false, 1.0},
		{"unknown-model-video", "unknown-model", Resolution720P, true, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := calculateCombinedRatio(tt.modelName, tt.resolution, tt.hasVideo)
			if !floatEqual(actual, tt.expectedRatio, 0.0001) {
				t.Errorf("calculateCombinedRatio(%q, %q, %v) = %v, want %v",
					tt.modelName, tt.resolution, tt.hasVideo, actual, tt.expectedRatio)
			}
		})
	}
}

// ============================
// Test normalizeResolution
// ============================

func TestNormalizeResolution(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Valid inputs
		{"480p", Resolution480P},
		{"720p", Resolution720P},
		{"1080p", Resolution1080P},

		// Case insensitive
		{"480P", Resolution480P},
		{"720P", Resolution720P},
		{"1080P", Resolution1080P},

		// Empty and default
		{"", Resolution720P},

		// Invalid/unsupported (should default to 720p)
		{"invalid", Resolution720P},
		{"360p", Resolution720P},
		{"2160p", Resolution720P},
		{"4k", Resolution720P},
		{"random", Resolution720P},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			actual := normalizeResolution(tt.input)
			if actual != tt.expected {
				t.Errorf("normalizeResolution(%q) = %q, want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

// ============================
// Test IsFastModel
// ============================

func TestIsFastModel(t *testing.T) {
	tests := []struct {
		modelName string
		expected  bool
	}{
		{ModelDreamina20Fast, true},
		{ModelDoubao20Fast, true},
		{ModelDreamina20, false},
		{ModelDoubao20, false},
		{"unknown-model", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelName, func(t *testing.T) {
			actual := IsFastModel(tt.modelName)
			if actual != tt.expected {
				t.Errorf("IsFastModel(%q) = %v, want %v", tt.modelName, actual, tt.expected)
			}
		})
	}
}

// ============================
// Test convertVolcanoContentToTaskSubmit
// ============================

func TestConvertVolcanoContentToTaskSubmit(t *testing.T) {
	tests := []struct {
		name        string
		volcanoReq  VolcanoRequestBody
		raw         map[string]json.RawMessage
		wantModel   string
		wantPrompt  string
		wantImages  []string
		wantVideos  []string
		wantAudios  []string
		wantDuration int
		wantSize    string
		checkMetadata func(*testing.T, map[string]interface{})
	}{
		{
			name: "text-only",
			volcanoReq: VolcanoRequestBody{
				Model: ModelDreamina20,
				Content: []ContentItemInput{
					{Type: "text", Text: "A cat playing with yarn"},
				},
			},
			raw: map[string]json.RawMessage{
				"model":   json.RawMessage(`"dreamina-seedance-2-0-260128"`),
				"content": json.RawMessage(`[{"type":"text","text":"A cat playing with yarn"}]`),
			},
			wantModel:  ModelDreamina20,
			wantPrompt: "A cat playing with yarn",
			wantImages: nil,
			wantVideos: nil,
			wantAudios: nil,
			checkMetadata: func(t *testing.T, m map[string]interface{}) {
				if m == nil {
					t.Error("metadata should not be nil")
					return
				}
				// Text-only content doesn't generate content items in metadata
				// (no media items to preserve structure for)
				if _, ok := m["content"]; ok {
					t.Error("text-only request should not have content in metadata")
				}
			},
		},
		{
			name: "image-with-role",
			volcanoReq: VolcanoRequestBody{
				Model: ModelDreamina20,
				Content: []ContentItemInput{
					{Type: "text", Text: "animate this image"},
					{
						Type: "image_url",
						ImageURL: &struct{ URL string `json:"url"` }{
							URL: "https://example.com/img.png",
						},
						Role: "first_frame",
					},
				},
			},
			raw: map[string]json.RawMessage{
				"model": json.RawMessage(`"dreamina-seedance-2-0-260128"`),
				"content": json.RawMessage(`[
					{"type":"text","text":"animate this image"},
					{"type":"image_url","image_url":{"url":"https://example.com/img.png"},"role":"first_frame"}
				]`),
			},
			wantModel:  ModelDreamina20,
			wantPrompt: "animate this image",
			wantImages: []string{"https://example.com/img.png"},
			wantVideos: nil,
			wantAudios: nil,
			checkMetadata: func(t *testing.T, m map[string]interface{}) {
				if m == nil {
					t.Error("metadata should not be nil")
					return
				}
				content, ok := m["content"].([]map[string]interface{})
				if !ok || len(content) == 0 {
					t.Error("metadata should have content with image item")
					return
				}
				// Check that role is preserved
				if content[0]["role"] != "first_frame" {
					t.Errorf("role should be preserved, got %v", content[0]["role"])
				}
			},
		},
		{
			name: "video-conversion",
			volcanoReq: VolcanoRequestBody{
				Model: ModelDreamina20,
				Content: []ContentItemInput{
					{Type: "text", Text: "continue this video"},
					{
						Type: "video",
						Video: &struct{ URL string `json:"url"` }{
							URL: "https://example.com/video.mp4",
						},
					},
				},
			},
			raw: map[string]json.RawMessage{
				"model": json.RawMessage(`"dreamina-seedance-2-0-260128"`),
				"content": json.RawMessage(`[
					{"type":"text","text":"continue this video"},
					{"type":"video","video":{"url":"https://example.com/video.mp4"}}
				]`),
			},
			wantModel:  ModelDreamina20,
			wantPrompt: "continue this video",
			wantImages: nil,
			wantVideos: []string{"https://example.com/video.mp4"},
			wantAudios: nil,
			checkMetadata: func(t *testing.T, m map[string]interface{}) {
				if m == nil {
					t.Error("metadata should not be nil")
					return
				}
				content, ok := m["content"].([]map[string]interface{})
				if !ok || len(content) == 0 {
					t.Error("metadata should have content with video item")
					return
				}
				// Check that video type is converted to video_url
				if content[0]["type"] != "video_url" {
					t.Errorf("video type should be converted to video_url, got %v", content[0]["type"])
				}
			},
		},
		{
			name: "audio-conversion",
			volcanoReq: VolcanoRequestBody{
				Model: ModelDreamina20,
				Content: []ContentItemInput{
					{Type: "text", Text: "lip sync this"},
					{
						Type: "audio",
						Audio: &struct{ URL string `json:"url"` }{
							URL: "https://example.com/audio.mp3",
						},
					},
				},
			},
			raw: map[string]json.RawMessage{
				"model": json.RawMessage(`"dreamina-seedance-2-0-260128"`),
				"content": json.RawMessage(`[
					{"type":"text","text":"lip sync this"},
					{"type":"audio","audio":{"url":"https://example.com/audio.mp3"}}
				]`),
			},
			wantModel:  ModelDreamina20,
			wantPrompt: "lip sync this",
			wantImages: nil,
			wantVideos: nil,
			wantAudios: []string{"https://example.com/audio.mp3"},
			checkMetadata: func(t *testing.T, m map[string]interface{}) {
				if m == nil {
					t.Error("metadata should not be nil")
					return
				}
				content, ok := m["content"].([]map[string]interface{})
				if !ok || len(content) == 0 {
					t.Error("metadata should have content with audio item")
					return
				}
				// Check that audio type is converted to audio_url
				if content[0]["type"] != "audio_url" {
					t.Errorf("audio type should be converted to audio_url, got %v", content[0]["type"])
				}
			},
		},
		{
			name: "advanced-params-passthrough",
			volcanoReq: VolcanoRequestBody{
				Model: ModelDreamina20,
				Content: []ContentItemInput{
					{Type: "text", Text: "test advanced params"},
				},
				Duration: intPtr(10),
				Seed:     intPtr(12345),
				Resolution: "1080p",
			},
			raw: map[string]json.RawMessage{
				"model":      json.RawMessage(`"dreamina-seedance-2-0-260128"`),
				"content":    json.RawMessage(`[{"type":"text","text":"test advanced params"}]`),
				"duration":   json.RawMessage(`10`),
				"seed":       json.RawMessage(`12345`),
				"resolution": json.RawMessage(`"1080p"`),
				"ratio":      json.RawMessage(`"9:16"`),
				"watermark":  json.RawMessage(`false`),
			},
			wantModel:    ModelDreamina20,
			wantPrompt:   "test advanced params",
			wantDuration: 10,
			wantSize:     "1080p",
			checkMetadata: func(t *testing.T, m map[string]interface{}) {
				if m == nil {
					t.Error("metadata should not be nil")
					return
				}
				// Check that advanced params are passed through
				if _, ok := m["ratio"]; !ok {
					t.Error("ratio should be in metadata")
				}
				if _, ok := m["watermark"]; !ok {
					t.Error("watermark should be in metadata")
				}
				if _, ok := m["duration"]; !ok {
					t.Error("duration should be in metadata")
				}
				if _, ok := m["seed"]; !ok {
					t.Error("seed should be in metadata")
				}
				if _, ok := m["resolution"]; !ok {
					t.Error("resolution should be in metadata")
				}
				// content and model should be excluded from passthrough
			},
		},
		{
			name: "multiple-text-items",
			volcanoReq: VolcanoRequestBody{
				Model: ModelDreamina20,
				Content: []ContentItemInput{
					{Type: "text", Text: "First sentence."},
					{Type: "text", Text: "Second sentence."},
				},
			},
			raw: map[string]json.RawMessage{
				"model": json.RawMessage(`"dreamina-seedance-2-0-260128"`),
				"content": json.RawMessage(`[
					{"type":"text","text":"First sentence."},
					{"type":"text","text":"Second sentence."}
				]`),
			},
			wantModel:  ModelDreamina20,
			wantPrompt: "First sentence. Second sentence.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := convertVolcanoContentToTaskSubmit(tt.volcanoReq, tt.raw)

			// Check basic fields
			if actual.Model != tt.wantModel {
				t.Errorf("Model = %v, want %v", actual.Model, tt.wantModel)
			}
			if actual.Prompt != tt.wantPrompt {
				t.Errorf("Prompt = %v, want %v", actual.Prompt, tt.wantPrompt)
			}

			// Check arrays
			if !stringSliceEqual(actual.Images, tt.wantImages) {
				t.Errorf("Images = %v, want %v", actual.Images, tt.wantImages)
			}
			if !stringSliceEqual(actual.Videos, tt.wantVideos) {
				t.Errorf("Videos = %v, want %v", actual.Videos, tt.wantVideos)
			}
			if !stringSliceEqual(actual.Audios, tt.wantAudios) {
				t.Errorf("Audios = %v, want %v", actual.Audios, tt.wantAudios)
			}

			// Check optional fields
			if tt.wantDuration > 0 && actual.Duration != tt.wantDuration {
				t.Errorf("Duration = %v, want %v", actual.Duration, tt.wantDuration)
			}
			if tt.wantSize != "" && actual.Size != tt.wantSize {
				t.Errorf("Size = %v, want %v", actual.Size, tt.wantSize)
			}

			// Check metadata with custom validator
			if tt.checkMetadata != nil {
				tt.checkMetadata(t, actual.Metadata)
			}
		})
	}
}

// ============================
// Test GetVideoInputRatio
// ============================

func TestGetVideoInputRatio(t *testing.T) {
	tests := []struct {
		modelName     string
		expectedRatio float64
		expectedOk    bool
	}{
		{ModelDreamina20, 28.0 / 46.0, true},
		{ModelDreamina20Fast, 22.0 / 37.0, true},
		{ModelDoubao20, 28.0 / 46.0, true},
		{ModelDoubao20Fast, 22.0 / 37.0, true},
		{"unknown-model", 0.0, false},
		{"", 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.modelName, func(t *testing.T) {
			ratio, ok := GetVideoInputRatio(tt.modelName)
			if ok != tt.expectedOk {
				t.Errorf("GetVideoInputRatio(%q) ok = %v, want %v", tt.modelName, ok, tt.expectedOk)
			}
			if ok && !floatEqual(ratio, tt.expectedRatio, 0.0001) {
				t.Errorf("GetVideoInputRatio(%q) ratio = %v, want %v", tt.modelName, ratio, tt.expectedRatio)
			}
		})
	}
}

// ============================
// Helper functions
// ============================

func floatEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func intPtr(v int) *int {
	return &v
}
