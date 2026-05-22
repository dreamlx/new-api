package happyhorse

const (
	ChannelName = "happyhorse"

	ModelT2V       = "happyhorse-1.0-t2v"
	ModelI2V       = "happyhorse-1.0-i2v"
	ModelR2V       = "happyhorse-1.0-r2v"
	ModelVideoEdit = "happyhorse-1.0-video-edit"

	DefaultDuration   = 5
	DefaultResolution = "720P"

	StatusPending   = "PENDING"
	StatusRunning   = "RUNNING"
	StatusSucceeded = "SUCCEEDED"
	StatusFailed    = "FAILED"
	StatusCanceled  = "CANCELED"
	StatusUnknown   = "UNKNOWN"

	NativeStatusPending   = "pending"
	NativeStatusRunning   = "running"
	NativeStatusCompleted = "completed"
	NativeStatusFailed    = "failed"

	Resolution720P  = "720P"
	Resolution1080P = "1080P"

	Ratio16x9 = "16:9"
	Ratio9x16 = "9:16"
	Ratio1x1  = "1:1"

	MediaTypeVideo          = "video"
	MediaTypeFirstFrame     = "first_frame"
	MediaTypeReferenceImage = "reference_image"

	ModeT2V       = "text-to-video"
	ModeI2V       = "image-to-video"
	ModeR2V       = "reference-to-video"
	ModeVideoEdit = "video-edit"
)

var ModelList = []string{
	ModelT2V,
	ModelI2V,
	ModelR2V,
	ModelVideoEdit,
}

func IsHappyHorseModel(model string) bool {
	switch model {
	case ModelT2V, ModelI2V, ModelR2V, ModelVideoEdit:
		return true
	default:
		return false
	}
}

var ModelToMode = map[string]string{
	ModelT2V:       ModeT2V,
	ModelI2V:       ModeI2V,
	ModelR2V:       ModeR2V,
	ModelVideoEdit: ModeVideoEdit,
}

var ValidResolutions = map[string]bool{
	Resolution720P:  true,
	Resolution1080P: true,
}

var ValidRatios = map[string]bool{
	Ratio16x9: true,
	Ratio9x16: true,
	Ratio1x1:  true,
}
