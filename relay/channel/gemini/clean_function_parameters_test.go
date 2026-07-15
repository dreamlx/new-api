package gemini

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCleanFunctionParametersDropsUnsupportedKeywords verifies the allowlist-based
// cleaner removes JSON-Schema keywords Gemini function_declarations rejects
// (exclusiveMinimum, propertyNames, const) — including when nested inside anyOf.
//
// Canary once logged gemini 400s: "Unknown name "const" at '...any_of[1]'",
// "Unknown name "exclusiveMinimum"...", "Unknown name "propertyNames"...".
// This test pins that the cleaner drops them (standard json.Unmarshal yields
// []interface{}/map[string]interface{}, so the anyOf type assertion + recursion
// at relay-gemini.go:774-781 reaches and filters them).
func TestCleanFunctionParametersDropsUnsupportedKeywords(t *testing.T) {
	t.Parallel()

	params := map[string]interface{}{
		"type": "object",
		"anyOf": []interface{}{
			map[string]interface{}{"type": "string"},
			map[string]interface{}{"const": "x", "type": "string"}, // const must be dropped, anyOf kept
		},
		"properties": map[string]interface{}{
			"p": map[string]interface{}{
				"type":             "integer",
				"minimum":          0, // allowed, kept
				"exclusiveMinimum": 0, // not in allowlist, dropped
				"propertyNames":    map[string]interface{}{}, // dropped
			},
		},
	}

	cleaned := cleanFunctionParameters(params)
	cm, ok := cleaned.(map[string]interface{})
	require.True(t, ok)

	// anyOf is in the allowlist → kept; its items are recursively cleaned.
	anyOf, ok := cm["anyOf"].([]interface{})
	require.True(t, ok)
	require.Len(t, anyOf, 2)
	second, ok := anyOf[1].(map[string]interface{})
	require.True(t, ok)
	require.NotContains(t, second, "const", "const inside anyOf must be dropped")
	// normalizeGeminiSchemaTypeAndNullable uppercases type strings (string→STRING).
	require.Equal(t, "STRING", second["type"])

	// Nested properties: minimum kept, exclusiveMinimum/propertyNames dropped.
	props, ok := cm["properties"].(map[string]interface{})
	require.True(t, ok)
	p, ok := props["p"].(map[string]interface{})
	require.True(t, ok)
	require.Contains(t, p, "minimum")
	require.NotContains(t, p, "exclusiveMinimum")
	require.NotContains(t, p, "propertyNames")
}
