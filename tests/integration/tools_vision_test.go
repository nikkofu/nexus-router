package integration

import (
	"testing"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/capabilities"
)

func TestRejectsVisionWhenPolicyDisallowsIt(t *testing.T) {
	req := canonical.Request{
		PublicModel: "openai/gpt-4.1",
		Conversation: []canonical.Turn{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{
					{
						Type: canonical.ContentTypeImage,
						Image: &canonical.ImageInput{
							URL:      "https://example.com/cat.png",
							MIMEType: "image/png",
						},
					},
				},
			},
		},
	}

	policy := auth.ClientPolicy{
		ID:             "no-vision",
		AllowVision:    false,
		AllowTools:     true,
		AllowStreaming: true,
	}

	err := capabilities.ValidateRequest(capabilities.DefaultRegistry(), policy, req)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestAcceptsManagedModelFamily(t *testing.T) {
	req := canonical.Request{
		PublicModel: "anthropic/claude-sonnet-4-5",
	}

	policy := auth.ClientPolicy{
		ID:              "default",
		AllowVision:     true,
		AllowTools:      true,
		AllowStructured: true,
		AllowStreaming:  true,
	}

	if err := capabilities.ValidateRequest(capabilities.DefaultRegistry(), policy, req); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
}
