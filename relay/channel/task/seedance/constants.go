package seedance

import "github.com/QuantumNous/new-api/relay/channel/task/taskcommon/volcano"

const (
	ChannelName = "seedance"

	// Model names - dreamina prefix
	ModelDreamina20     = "dreamina-seedance-2-0-260128"
	ModelDreamina20Fast = "dreamina-seedance-2-0-fast-260128"

	// Model names - doubao prefix (compatible aliases)
	ModelDoubao20     = "doubao-seedance-2-0-260128"
	ModelDoubao20Fast = "doubao-seedance-2-0-fast-260128"

	// Resolution constants (status constants live in volcano package)
	Resolution480P  = "480p"
	Resolution720P  = "720p"
	Resolution1080P = "1080p"
)

// Status constants re-exported from volcano package for convenience within seedance
const (
	StatusPending    = volcano.StatusPending
	StatusQueued     = volcano.StatusQueued
	StatusProcessing = volcano.StatusProcessing
	StatusRunning    = volcano.StatusRunning
	StatusSucceeded  = volcano.StatusSucceeded
	StatusFailed     = volcano.StatusFailed
)

var ModelList = []string{
	ModelDreamina20,
	ModelDreamina20Fast,
	ModelDoubao20,
	ModelDoubao20Fast,
}

// videoInputRatioMap 视频输入折扣比率（含视频单价 / 不含视频单价）。
// 管理员应将 ModelRatio 设置为"不含视频"的较高费率，系统在检测到视频输入时自动乘以此折扣。
var videoInputRatioMap = map[string]float64{
	ModelDreamina20:     28.0 / 46.0, // ~0.6087
	ModelDreamina20Fast: 22.0 / 37.0, // ~0.5946
	ModelDoubao20:       28.0 / 46.0, // ~0.6087
	ModelDoubao20Fast:   22.0 / 37.0, // ~0.5946
}

// resolution1080PRatioMap 1080P 分辨率倍率（1080P 单价 / 基准单价）。
// 仅主版本支持 1080P，基准是 480P/720P。
var resolution1080PRatioMap = map[string]float64{
	ModelDreamina20: 51.0 / 46.0, // ~1.1087 (不含视频输入)
	ModelDoubao20:   51.0 / 46.0, // ~1.1087 (不含视频输入)
}

// resolution1080PWithVideoRatioMap 1080P + 输入含视频倍率（1080P 含视频单价 / 基准单价）。
var resolution1080PWithVideoRatioMap = map[string]float64{
	ModelDreamina20: 31.0 / 46.0, // ~0.6739
	ModelDoubao20:   31.0 / 46.0, // ~0.6739
}

func GetVideoInputRatio(modelName string) (float64, bool) {
	r, ok := videoInputRatioMap[modelName]
	return r, ok
}

func GetResolution1080PRatio(modelName string) (float64, bool) {
	r, ok := resolution1080PRatioMap[modelName]
	return r, ok
}

func GetResolution1080PWithVideoRatio(modelName string) (float64, bool) {
	r, ok := resolution1080PWithVideoRatioMap[modelName]
	return r, ok
}

// IsFastModel returns true if the model is a fast variant
func IsFastModel(modelName string) bool {
	return modelName == ModelDreamina20Fast || modelName == ModelDoubao20Fast
}
