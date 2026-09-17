package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var modelIDs = []string{
	"gpt-image-2.5-flare",
	"gpt-image-2.5-sunburst",
	"gpt-image-2.5-flare-2026-09-08",
	"gpt-image-2.5-sunburst-2026-09-08",
}

type handlers struct {
	client       *client
	defaultModel string
}

// imageOptions are the generation parameters shared by generate_image and edit_image.
func imageOptions(defaultModel string) []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("model",
			mcp.Description("gpt-image-2.5 model ID"),
			mcp.Enum(modelIDs...),
			mcp.DefaultString(defaultModel),
		),
		mcp.WithString("size",
			mcp.Description(`"auto" or WIDTHxHEIGHT (e.g. 1024x1024, 1536x1024, 1024x1536). Custom: edges <=3840 & multiples of 16, ratio <=3:1`),
			mcp.DefaultString("auto"),
		),
		mcp.WithString("quality",
			mcp.Enum("auto", "low", "medium", "high", "xhigh", "max"),
			mcp.DefaultString("auto"),
			mcp.Description("xhigh/max only when a quality requirement is unmet"),
		),
		mcp.WithString("background",
			mcp.Enum("auto", "opaque", "transparent"),
			mcp.DefaultString("auto"),
			mcp.Description("transparent requires outputFormat png or webp"),
		),
		mcp.WithString("outputFormat",
			mcp.Enum("png", "jpeg", "webp"),
			mcp.DefaultString("png"),
		),
		mcp.WithNumber("outputCompression",
			mcp.Min(0), mcp.Max(100),
			mcp.Description("0-100, jpeg/webp only"),
		),
		mcp.WithString("moderation",
			mcp.Enum("auto", "low"),
			mcp.DefaultString("auto"),
		),
		mcp.WithNumber("n",
			mcp.Min(1), mcp.Max(10),
			mcp.DefaultNumber(1),
			mcp.Description("number of images; n>1 saves name-1.ext .. name-N.ext"),
		),
		mcp.WithBoolean("returnInlineImage",
			mcp.DefaultBool(false),
			mcp.Description("also embed image bytes in the result (uses context window)"),
		),
	}
}

func generateTool(defaultModel string) mcp.Tool {
	opts := []mcp.ToolOption{
		mcp.WithDescription("Generate a NEW image from a text prompt. Use only for creating images from scratch, not for modifying existing ones. Call get_usage_guide for parameter rules and prompting techniques."),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("text describing the image to create")),
		mcp.WithString("outputPath", mcp.Required(), mcp.Description("absolute file path to write the image to, e.g. /Users/me/out/logo.png")),
	}
	opts = append(opts, imageOptions(defaultModel)...)
	return mcp.NewTool("generate_image", opts...)
}

func editTool(defaultModel string) mcp.Tool {
	opts := []mcp.ToolOption{
		mcp.WithDescription("Edit EXISTING image(s) with a prompt. The first image is the primary subject; give each input a numbered role (subject, style, clothing, background). Call get_usage_guide for parameter rules and prompting techniques."),
		mcp.WithArray("images",
			mcp.Required(),
			mcp.WithStringItems(),
			mcp.Description("1-16 input images: absolute file paths, https:// URLs, data: URIs, or file_id: refs"),
		),
		mcp.WithString("mask",
			mcp.Description("optional mask (same forms as images); transparent regions mark editable areas; must match input dimensions"),
		),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("what to change; state 'change only X' plus what to preserve")),
		mcp.WithString("outputPath", mcp.Required(), mcp.Description("absolute file path to write the image to, e.g. /Users/me/out/logo.png")),
	}
	opts = append(opts, imageOptions(defaultModel)...)
	return mcp.NewTool("edit_image", opts...)
}

func usageGuideTool() mcp.Tool {
	return mcp.NewTool("get_usage_guide",
		mcp.WithDescription("Return the imagen usage and gpt-image-2.5 prompting guide (parameter rules, edit patterns, text rendering tips)."),
	)
}

// commonArgs extracts the shared generation params and validates them.
func (h *handlers) commonArgs(req mcp.CallToolRequest) (genParams, string, bool, error) {
	out, err := req.RequireString("outputPath")
	if err != nil {
		return genParams{}, "", false, err
	}
	if !filepath.IsAbs(out) {
		return genParams{}, "", false, fmt.Errorf("outputPath must be an absolute path, got %q", out)
	}

	p := genParams{
		Model:        req.GetString("model", h.defaultModel),
		Size:         req.GetString("size", "auto"),
		Quality:      req.GetString("quality", "auto"),
		Background:   req.GetString("background", "auto"),
		OutputFormat: req.GetString("outputFormat", "png"),
		Moderation:   req.GetString("moderation", "auto"),
		N:            req.GetInt("n", 1),
	}
	if p.Background == "transparent" && p.OutputFormat != "png" && p.OutputFormat != "webp" {
		return genParams{}, "", false, fmt.Errorf("background=transparent requires outputFormat png or webp, got %q", p.OutputFormat)
	}
	if c := req.GetFloat("outputCompression", -1); c >= 0 {
		if p.OutputFormat == "png" {
			return genParams{}, "", false, fmt.Errorf("outputCompression applies to jpeg/webp only, not png")
		}
		v := int(c)
		p.OutputCompression = &v
	}
	return p, out, req.GetBool("returnInlineImage", false), nil
}

func (h *handlers) checkKey() error {
	if h.client.apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is not set; configure it in the MCP server env")
	}
	return nil
}

func (h *handlers) generateImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkKey(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	prompt, err := req.RequireString("prompt")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	p, out, inline, err := h.commonArgs(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	p.Prompt = prompt

	resp, err := h.client.generate(ctx, p)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return saveResults(resp, out, p.OutputFormat, inline)
}

func (h *handlers) editImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := h.checkKey(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	prompt, err := req.RequireString("prompt")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	inputs, err := req.RequireStringSlice("images")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	p, out, inline, err := h.commonArgs(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	p.Prompt = prompt

	ep := editParams{genParams: p, Images: make([]imageRef, 0, len(inputs))}
	for _, in := range inputs {
		ref, err := resolveImageRef(in)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		ep.Images = append(ep.Images, ref)
	}
	if m := req.GetString("mask", ""); m != "" {
		ref, err := resolveImageRef(m)
		if err != nil {
			return mcp.NewToolResultError(fmt.Errorf("mask: %w", err).Error()), nil
		}
		ep.Mask = &ref
	}

	resp, err := h.client.edit(ctx, ep)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return saveResults(resp, out, p.OutputFormat, inline)
}

func (h *handlers) usageGuide(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(guideMD), nil
}

// resolveImageRef classifies an input image string into an API image reference:
// file_id: refs pass through as file IDs; data: URIs and http(s) URLs pass
// through as image_url; anything else is read from disk and sent as a data URL.
func resolveImageRef(s string) (imageRef, error) {
	switch {
	case strings.HasPrefix(s, "file_id:"):
		return imageRef{FileID: strings.TrimPrefix(s, "file_id:")}, nil
	case strings.HasPrefix(s, "data:"), strings.HasPrefix(s, "http://"), strings.HasPrefix(s, "https://"):
		return imageRef{ImageURL: s}, nil
	}
	raw, err := os.ReadFile(s)
	if err != nil {
		return imageRef{}, fmt.Errorf("read image %q: %w", s, err)
	}
	mime := http.DetectContentType(raw)
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
	default:
		return imageRef{}, fmt.Errorf("unsupported image type %q in %s (want png/jpeg/webp/gif)", mime, s)
	}
	return imageRef{ImageURL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)}, nil
}

// saveResults decodes b64 payloads, writes files, and builds the tool result.
func saveResults(resp *imagesResponse, out, format string, inline bool) (*mcp.CallToolResult, error) {
	if len(resp.Data) == 0 {
		return mcp.NewToolResultError("openai: response contained no images"), nil
	}
	paths := outputPaths(out, format, len(resp.Data))
	content := []mcp.Content{}
	var saved []string

	for i, d := range resp.Data {
		if d.B64JSON == "" {
			return mcp.NewToolResultError("openai: image returned without b64_json data"), nil
		}
		raw, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decode image %d: %v", i+1, err)), nil
		}
		if err := os.MkdirAll(filepath.Dir(paths[i]), 0o755); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := os.WriteFile(paths[i], raw, 0o644); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		saved = append(saved, paths[i])
		if inline {
			content = append(content, mcp.NewImageContent(d.B64JSON, "image/"+format))
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d image(s) saved:\n", len(saved))
	for _, p := range saved {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	if rp := resp.Data[0].RevisedPrompt; rp != "" {
		fmt.Fprintf(&b, "revised prompt: %s\n", rp)
	}
	if resp.Usage != nil {
		fmt.Fprintf(&b, "tokens: %d input, %d output\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	}
	content = append(content, mcp.NewTextContent(strings.TrimRight(b.String(), "\n")))

	return &mcp.CallToolResult{Content: content}, nil
}

// outputPaths expands outputPath for n images: n==1 uses the path as given
// (extension filled from format if missing); n>1 produces name-1.ext .. name-N.ext.
func outputPaths(out, format string, n int) []string {
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	if filepath.Ext(out) == "" {
		out += "." + ext
	}
	if n <= 1 {
		return []string{out}
	}
	e := filepath.Ext(out)
	base := strings.TrimSuffix(out, e)
	paths := make([]string, n)
	for i := range paths {
		paths[i] = fmt.Sprintf("%s-%d%s", base, i+1, e)
	}
	return paths
}
