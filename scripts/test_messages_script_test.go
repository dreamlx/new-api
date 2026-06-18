package scripts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTestMessagesScriptPrintsNonStreamAndStreamStructures(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("unexpected anthropic-version header: %q", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var payload struct {
			Stream bool `json:"stream"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if payload.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: message_start\n")
			_, _ = io.WriteString(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_stream\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-test\",\"content\":[],\"usage\":{\"input_tokens\":12,\"output_tokens\":0}}}\n\n")
			_, _ = io.WriteString(w, "event: content_block_delta\n")
			_, _ = io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n")
			_, _ = io.WriteString(w, "event: message_delta\n")
			_, _ = io.WriteString(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n")
			_, _ = io.WriteString(w, "event: message_stop\n")
			_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
			return
		}

		_, _ = io.WriteString(w, `{"id":"msg_nonstream","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":12,"output_tokens":7}}`)
	}))
	defer server.Close()

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "scripts", "test_messages.sh")
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"BASE_URL="+server.URL,
		"RELAY_TOKEN=test-token",
		"MESSAGE_MODEL=claude-test",
		"ANTHROPIC_VERSION=2023-06-01",
		"MESSAGE_PROMPT=print structure test",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\noutput:\n%s", err, output)
	}

	text := string(output)
	for _, want := range []string{
		"非流式响应",
		"stream=false",
		"msg_nonstream",
		"input_tokens",
		"output_tokens",
		"流式响应",
		"stream=true",
		"message_start",
		"content_block_delta",
		"message_delta",
		"message_stop",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %q\nfull output:\n%s", want, text)
		}
	}
}
