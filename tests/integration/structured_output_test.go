package integration

import (
	"testing"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/capabilities"
)

func TestRejectsUnsupportedSchemaKeyword(t *testing.T) {
	req := canonical.Request{
		PublicModel: "anthropic/claude-sonnet-4-5",
		ResponseContract: canonical.ResponseContract{
			Kind: canonical.ResponseContractJSONSchema,
			Schema: map[string]any{
				"oneOf": []any{},
			},
		},
	}

	policy := auth.ClientPolicy{
		ID:              "structured",
		AllowStructured: true,
		AllowVision:     true,
		AllowTools:      true,
		AllowStreaming:  true,
	}

	err := capabilities.ValidateRequest(capabilities.DefaultRegistry(), policy, req)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestStructuredOutputAcceptsManagedSubset(t *testing.T) {
	req := canonical.Request{
		PublicModel: "anthropic/claude-sonnet-4-5",
		ResponseContract: canonical.ResponseContract{
			Kind: canonical.ResponseContractJSONSchema,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"answer": map[string]any{
						"type": "string",
					},
					"confidence": map[string]any{
						"type": "number",
					},
				},
				"required":             []any{"answer"},
				"additionalProperties": false,
			},
		},
	}

	policy := auth.ClientPolicy{
		ID:              "structured",
		AllowStructured: true,
		AllowVision:     true,
		AllowTools:      true,
		AllowStreaming:  true,
	}

	if err := capabilities.ValidateRequest(capabilities.DefaultRegistry(), policy, req); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
}
