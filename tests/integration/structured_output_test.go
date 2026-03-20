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
