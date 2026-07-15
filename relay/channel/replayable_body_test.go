package channel

import (
	"bytes"
	"io"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

// makeReplayableBody gates whether an upstream request body is buffered into RAM
// so net/http can replay it on an HTTP/2 GOAWAY/PROTOCOL_ERROR mid-stream retry
// (net/http auto-installs Request.GetBody for *bytes.Reader). Small bodies are
// buffered; oversized/unknown-size bodies pass through unbuffered to avoid OOM.

func TestMakeReplayableBody_BuffersSmallBodyAsBytesReader(t *testing.T) {
	t.Parallel()
	info := &relaycommon.RelayInfo{UpstreamRequestBodySize: 12}
	body, err := makeReplayableBody(strings.NewReader(`{"model":"x"}`), info)
	require.NoError(t, err)
	br, ok := body.(*bytes.Reader)
	require.True(t, ok, "small body must be buffered as *bytes.Reader so net/http auto-sets GetBody")
	content, err := io.ReadAll(br)
	require.NoError(t, err)
	require.Equal(t, `{"model":"x"}`, string(content))
}

func TestMakeReplayableBody_PassesThroughOversizedBody(t *testing.T) {
	t.Parallel()
	original := strings.NewReader("oversized-body")
	info := &relaycommon.RelayInfo{UpstreamRequestBodySize: replayBodyLimit + 1}
	body, err := makeReplayableBody(original, info)
	require.NoError(t, err)
	require.Same(t, original, body, "oversized body must pass through unbuffered to avoid OOM")
}

func TestMakeReplayableBody_PassesThroughUnknownSize(t *testing.T) {
	t.Parallel()
	original := strings.NewReader("unknown-size")
	info := &relaycommon.RelayInfo{UpstreamRequestBodySize: 0} // handler did not set size
	body, err := makeReplayableBody(original, info)
	require.NoError(t, err)
	require.Same(t, original, body, "unknown-size body must pass through rather than buffer blindly")
}

func TestMakeReplayableBody_NilInfoPassesThrough(t *testing.T) {
	t.Parallel()
	original := strings.NewReader("x")
	body, err := makeReplayableBody(original, nil)
	require.NoError(t, err)
	require.Same(t, original, body)
}
