package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const fixture = `openapi: 3.0.0
info:
  version: 1.0.0
components:
  schemas:
    Region:
      type: string
      example: eu-west
      enum: [eu-west, na-east]
    ApplicationResponse:
      type: object
      properties:
        capabilities:
          type: object
          description: Configuration for the products
          x-internal: true
          properties:
            voice:
              type: object
              properties:
                region:
                  $ref: '#/components/schemas/Region'
                signed_callbacks:
                  type: boolean
                  default: "true"
                  example: false
                timeout:
                  type: integer
                  minimum: 1
                  maximum: 10
                  nullable: true
`

func TestCapabilitiesConvertsTheDefinitionSubtree(t *testing.T) {
	got, err := capabilities([]byte(fixture))
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"type":                 "object",
		"description":          "Configuration for the products",
		"additionalProperties": true,
		"properties": map[string]any{
			"voice": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
				"properties": map[string]any{
					"region":           map[string]any{"type": "string", "enum": []any{"eu-west", "na-east"}},
					"signed_callbacks": map[string]any{"type": "boolean", "default": true},
					"timeout":          map[string]any{"type": []any{"integer", "null"}, "minimum": 1, "maximum": 10},
				},
			},
		},
	}, got)
}

func TestCapabilitiesFailsOnARefOutsideTheDefinition(t *testing.T) {
	spec := `components:
  schemas:
    ApplicationResponse:
      properties:
        capabilities:
          type: object
          properties:
            voice:
              $ref: 'other.yml#/components/schemas/Voice'
`
	_, err := capabilities([]byte(spec))
	require.ErrorContains(t, err, `unsupported $ref "other.yml#/components/schemas/Voice"`)
}

func TestCapabilitiesFailsWithoutTheCapabilitiesObject(t *testing.T) {
	_, err := capabilities([]byte("components:\n  schemas: {}\n"))
	require.ErrorContains(t, err, "components.schemas.ApplicationResponse.properties.capabilities not found")
}

func TestRunWritesTheSameSchemaEveryTime(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.yml")
	out := filepath.Join(dir, "vcr.schema.json")
	require.NoError(t, os.WriteFile(spec, []byte(fixture), 0o600))

	require.NoError(t, run(spec, out))
	first, err := os.ReadFile(out)
	require.NoError(t, err)
	require.NoError(t, run(spec, out))
	second, err := os.ReadFile(out)
	require.NoError(t, err)

	require.Equal(t, string(first), string(second))
	require.Contains(t, string(first), `"$schema": "http://json-schema.org/draft-07/schema#"`)
	require.Contains(t, string(first), `"application-capabilities": {`)
	require.Equal(t, byte('\n'), first[len(first)-1])
}
