package gemini

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanFunctionParameters_DropsConstInsideAnyOf pins the only branch of
// CleanFunctionParameters not covered by service/relayconvert/request_registry_test.go:
// anyOf items are recursively cleaned, so an unsupported keyword (const) nested
// inside anyOf is dropped while anyOf itself is kept and item types normalized.
//
// Canary once logged gemini 400s: "Unknown name "const" at '...any_of[1]'".
func TestCleanFunctionParameters_DropsConstInsideAnyOf(t *testing.T) {
	t.Parallel()

	params := map[string]interface{}{
		"type": "object",
		"anyOf": []interface{}{
			map[string]interface{}{"type": "string"},
			map[string]interface{}{"const": "x", "type": "string"}, // const dropped, anyOf item kept
		},
	}

	cleaned := CleanFunctionParameters(params)
	cm, ok := cleaned.(map[string]interface{})
	require.True(t, ok)

	anyOf, ok := cm["anyOf"].([]interface{})
	require.True(t, ok)
	require.Len(t, anyOf, 2)

	second, ok := anyOf[1].(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, second, "const", "const inside anyOf must be dropped")
	// normalizeGeminiSchemaTypeAndNullable uppercases type strings (string → STRING).
	assert.Equal(t, "STRING", second["type"])
}
