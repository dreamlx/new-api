package seedance

import (
	"errors"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/dto"
	taskseedance "github.com/QuantumNous/new-api/relay/channel/task/seedance"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct{}

func (a *Adaptor) Init(_ *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return "", unsupported()
}

func (a *Adaptor) SetupRequestHeader(_ *gin.Context, _ *http.Header, _ *relaycommon.RelayInfo) error {
	return unsupported()
}

func (a *Adaptor) ConvertOpenAIRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ *dto.GeneralOpenAIRequest) (any, error) {
	return nil, unsupported()
}

func (a *Adaptor) ConvertRerankRequest(_ *gin.Context, _ int, _ dto.RerankRequest) (any, error) {
	return nil, unsupported()
}

func (a *Adaptor) ConvertEmbeddingRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.EmbeddingRequest) (any, error) {
	return nil, unsupported()
}

func (a *Adaptor) ConvertAudioRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.AudioRequest) (io.Reader, error) {
	return nil, unsupported()
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.ImageRequest) (any, error) {
	return nil, unsupported()
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.OpenAIResponsesRequest) (any, error) {
	return nil, unsupported()
}

func (a *Adaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ io.Reader) (any, error) {
	return nil, unsupported()
}

func (a *Adaptor) DoResponse(_ *gin.Context, _ *http.Response, _ *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return nil, types.NewError(unsupported(), types.ErrorCodeInvalidRequest)
}

func (a *Adaptor) GetModelList() []string {
	return taskseedance.ModelList
}

func (a *Adaptor) GetChannelName() string {
	return taskseedance.ChannelName
}

func (a *Adaptor) ConvertClaudeRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ *dto.ClaudeRequest) (any, error) {
	return nil, unsupported()
}

func (a *Adaptor) ConvertGeminiRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ *dto.GeminiChatRequest) (any, error) {
	return nil, unsupported()
}

func unsupported() error {
	return errors.New("seedance supports video task relay only")
}
