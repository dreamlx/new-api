package helper

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// StreamResult is passed to streaming callbacks to report errors and signal completion.
type StreamResult struct {
	status  *relaycommon.StreamStatus
	stopped bool
}

// NewStreamResult creates a new StreamResult wrapping the given StreamStatus.
func NewStreamResult(status *relaycommon.StreamStatus) *StreamResult {
	return &StreamResult{status: status}
}

// Error records a soft error. The stream continues processing.
func (sr *StreamResult) Error(err error) {
	if sr.status != nil {
		sr.status.AddError(err)
	}
}

// Stop records a fatal error and signals the stream to stop.
func (sr *StreamResult) Stop(err error) {
	sr.stopped = true
	if sr.status != nil {
		sr.status.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
		if err != nil {
			sr.status.AddError(err)
		}
	}
}

// Done signals normal completion. The stream stops.
func (sr *StreamResult) Done() {
	sr.stopped = true
	if sr.status != nil {
		sr.status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	}
}

// IsStopped returns true if Stop() or Done() was called.
func (sr *StreamResult) IsStopped() bool {
	return sr.stopped
}
