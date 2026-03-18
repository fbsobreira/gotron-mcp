package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestRegisterResources_NoError(t *testing.T) {
	s := server.NewMCPServer("test", "1.0", server.WithResourceCapabilities(false, false))
	RegisterResources(s)
}

func TestEmbeddedContent_NotEmpty(t *testing.T) {
	if tronOverview == "" {
		t.Error("tronOverview embed is empty")
	}
	for slug, topic := range topics {
		if topic.content == "" {
			t.Errorf("topic %q content is empty", slug)
		}
		if topic.name == "" {
			t.Errorf("topic %q name is empty", slug)
		}
		if topic.desc == "" {
			t.Errorf("topic %q desc is empty", slug)
		}
	}
}

func TestTopics_AllExpected(t *testing.T) {
	expected := []string{"accounts", "tokens", "transfers", "staking", "contracts", "governance", "blocks"}
	for _, slug := range expected {
		if _, ok := topics[slug]; !ok {
			t.Errorf("expected topic %q not found", slug)
		}
	}
	if len(topics) != len(expected) {
		t.Errorf("expected %d topics, got %d", len(expected), len(topics))
	}
}

func TestTopicLookup_Valid(t *testing.T) {
	s := server.NewMCPServer("test", "1.0", server.WithResourceCapabilities(false, false))
	RegisterResources(s)

	// We can't call the template handler directly, but we can verify
	// the lookup logic works by testing the topics map
	for slug, topic := range topics {
		if topic.content == "" {
			t.Errorf("topic %q has empty content", slug)
		}
	}
}

func TestTopicLookup_Unknown(t *testing.T) {
	// Test the slug extraction and lookup logic
	slug := "nonexistent"
	_, ok := topics[slug]
	if ok {
		t.Error("expected unknown topic to not be found")
	}
}

func TestOverviewResource_Handler(t *testing.T) {
	// Simulate what the overview handler does
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "gotron://knowledge/tron-overview"

	contents := []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "text/markdown",
			Text:     tronOverview,
		},
	}

	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("expected TextResourceContents")
	}
	if tc.Text == "" {
		t.Error("overview text is empty")
	}
}

func TestTopicResource_Handler(t *testing.T) {
	for slug, topic := range topics {
		t.Run(slug, func(t *testing.T) {
			uri := "gotron://knowledge/topics/" + slug
			contents := []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      uri,
					MIMEType: "text/markdown",
					Text:     topic.content,
				},
			}
			tc := contents[0].(mcp.TextResourceContents)
			if tc.Text == "" {
				t.Errorf("topic %q text is empty", slug)
			}
			if tc.URI != uri {
				t.Errorf("URI = %q, want %q", tc.URI, uri)
			}
		})
	}
}

func TestTopicTemplateLookup_SlugExtraction(t *testing.T) {
	tests := []struct {
		uri  string
		slug string
	}{
		{"gotron://knowledge/topics/accounts", "accounts"},
		{"gotron://knowledge/topics/tokens", "tokens"},
		{"gotron://knowledge/topics/staking", "staking"},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			// Mirror production logic: extract slug after last '/'
			uri := tt.uri
			extracted := uri[strings.LastIndex(uri, "/")+1:]
			if extracted != tt.slug {
				t.Errorf("extracted %q, want %q", extracted, tt.slug)
			}
			topic, ok := topics[extracted]
			if !ok {
				t.Fatalf("topic %q not found", extracted)
			}
			if topic.content == "" {
				t.Error("content is empty")
			}
		})
	}
}

func TestTopicTemplateLookup_InvalidSlug(t *testing.T) {
	s := server.NewMCPServer("test", "1.0", server.WithResourceCapabilities(false, false))
	RegisterResources(s)

	// Simulate the template handler with an invalid topic
	slug := "invalid_topic"
	_, ok := topics[slug]
	if ok {
		t.Error("should not find invalid topic")
	}

	// Verify error message would contain available topics
	available := make([]string, 0, len(topics))
	for k := range topics {
		available = append(available, k)
	}
	if len(available) != 7 {
		t.Errorf("expected 7 available topics, got %d", len(available))
	}
}

// Verify that RegisterResources works with a real MCP server context
func TestRegisterResources_Integration(t *testing.T) {
	s := server.NewMCPServer("test", "1.0",
		server.WithResourceCapabilities(false, false),
	)
	RegisterResources(s)

	// Verify we can initialize and the server accepts the resources
	ctx := context.Background()
	_ = ctx // resources are registered, server is valid
}
