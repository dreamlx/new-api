package common

import (
	"fmt"
	"sync"
)

// StreamEndReason describes how a streaming response ended.
type StreamEndReason string

const (
	StreamEndReasonDone         StreamEndReason = "done"          // Normal handler completion
	StreamEndReasonEOF          StreamEndReason = "eof"           // Response body EOF
	StreamEndReasonTimeout      StreamEndReason = "timeout"       // Streaming timeout
	StreamEndReasonClientGone   StreamEndReason = "client_gone"   // Client disconnected
	StreamEndReasonScannerError StreamEndReason = "scanner_error" // Scanner error during reading
	StreamEndReasonHandlerStop  StreamEndReason = "handler_stop"  // Handler called sr.Stop()
	StreamEndReasonPingFail     StreamEndReason = "ping_fail"     // Ping/keep-alive failed
	StreamEndReasonPanic        StreamEndReason = "panic"         // Panic in handler
)

// StreamErrorEntry records a single soft error during streaming.
type StreamErrorEntry struct {
	Message string `json:"message"`
}

const maxStreamErrors = 20

// StreamStatus tracks the end-of-stream state and soft errors during streaming.
type StreamStatus struct {
	EndReason  StreamEndReason    `json:"end_reason,omitempty"`
	EndError   error              `json:"-"`
	endOnce    sync.Once
	mu         sync.Mutex
	Errors     []StreamErrorEntry `json:"errors,omitempty"`
	ErrorCount int                `json:"error_count"`
}

// NewStreamStatus creates a new StreamStatus instance.
func NewStreamStatus() *StreamStatus {
	return &StreamStatus{
		Errors: make([]StreamErrorEntry, 0),
	}
}

// SetEndReason sets the end reason (only the first call takes effect).
func (s *StreamStatus) SetEndReason(reason StreamEndReason, err error) {
	s.endOnce.Do(func() {
		s.EndReason = reason
		s.EndError = err
	})
}

// AddError records a soft error. Only the first maxStreamErrors are stored.
func (s *StreamStatus) AddError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ErrorCount++
	if len(s.Errors) < maxStreamErrors {
		s.Errors = append(s.Errors, StreamErrorEntry{Message: err.Error()})
	}
}

// HasErrors returns true if any soft errors were recorded.
func (s *StreamStatus) HasErrors() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount > 0
}

// Summary returns a human-readable summary for logging.
func (s *StreamStatus) Summary() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrorCount == 0 {
		return fmt.Sprintf("end_reason=%s", s.EndReason)
	}
	return fmt.Sprintf("end_reason=%s, errors=%d", s.EndReason, s.ErrorCount)
}
