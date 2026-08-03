package ospreyai

import (
	"fmt"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// ErrorHandler handles OspreyAI error responses
type ErrorHandler struct{}

// HandleError converts OspreyAI error to standard new-api error format
// Normalizes different error response formats across protocols
func (eh *ErrorHandler) HandleError(statusCode int, errorBody []byte) *types.NewAPIError {
	// Parse error based on protocol and status code
	errorMsg := parseErrorMessage(errorBody)

	// Map HTTP status codes to error codes
	errorCode := mapStatusToErrorCode(statusCode)

	// Create standardized error response
	opts := []types.NewAPIErrorOptions{
		types.ErrOptionWithSkipRetry(),
	}

	return types.NewError(
		fmt.Errorf("ospreyai error: %s", errorMsg),
		errorCode,
		opts...,
	)
}

// parseErrorMessage extracts error message from response body
func parseErrorMessage(errorBody []byte) string {
	if len(errorBody) == 0 {
		return "empty error response"
	}

	// Try to parse as JSON to extract error details
	// Different protocols return error messages in different formats
	// This is a basic implementation that can be enhanced per protocol

	return string(errorBody)
}

// mapStatusToErrorCode maps HTTP status codes to new-api error codes
func mapStatusToErrorCode(statusCode int) types.ErrorCode {
	switch statusCode {
	case 400:
		return types.ErrorCodeInvalidRequest
	case 401:
		// No ErrorCodeInvalidAPIKey in types, use InvalidRequest
		return types.ErrorCodeInvalidRequest
	case 403:
		return types.ErrorCodeInsufficientUserQuota
	case 404:
		return types.ErrorCodeInvalidRequest
	case 429:
		// No ErrorCodeRateLimitExceeded in types, use custom
		return types.ErrorCodeInvalidRequest
	case 500, 502, 503:
		return types.ErrorCodeBadResponseStatusCode
	default:
		return types.ErrorCodeBadResponseStatusCode
	}
}

// IsRetryableError determines if an error should trigger retry
func IsRetryableError(statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
