package main

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

//go:embed guide.md
var guideMD string

var version = "dev"

const description = "Image generation and editing via OpenAI gpt-image-2.5 (flare/sunburst models)"

const instructions = "Call get_usage_guide for parameter rules and gpt-image-2.5 prompting techniques"

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "imagen: warning: OPENAI_API_KEY not set; tool calls will fail until configured")
	}
	baseURL := strings.TrimSuffix(os.Getenv("OPENAI_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := os.Getenv("IMAGEN_MODEL")
	if model == "" {
		model = "gpt-image-2.5-flare"
	}

	h := &handlers{
		client: &client{
			apiKey:  apiKey,
			baseURL: baseURL,
			http:    &http.Client{Timeout: 5 * time.Minute},
		},
		defaultModel: model,
	}

	s := server.NewMCPServer("imagen", version,
		server.WithDescription(description),
		server.WithToolCapabilities(false),
		server.WithRecovery(),
		server.WithInstructions(instructions),
	)
	s.AddTool(generateTool(model), h.generateImage)
	s.AddTool(editTool(model), h.editImage)
	s.AddTool(usageGuideTool(), h.usageGuide)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "imagen: %v\n", err)
		os.Exit(1)
	}
}
