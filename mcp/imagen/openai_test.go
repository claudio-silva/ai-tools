package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

var pngBytes = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 32)...)

func reqWith(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

func newTestServer(t *testing.T, handler http.HandlerFunc) (*client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &client{apiKey: "sk-test", baseURL: srv.URL, http: srv.Client()}, srv
}

func captureBody(t *testing.T, wantStatus int, response string) (http.HandlerFunc, *map[string]any, **http.Request) {
	t.Helper()
	var body map[string]any
	var lastReq *http.Request
	return func(w http.ResponseWriter, r *http.Request) {
		lastReq = r
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(wantStatus)
		w.Write([]byte(response))
	}, &body, &lastReq
}

func TestGenerateRequestShape(t *testing.T) {
	handler, body, lastReq := captureBody(t, 200, `{"created":1,"data":[{"b64_json":"aGk="}]}`)
	c, _ := newTestServer(t, handler)

	_, err := c.generate(context.Background(), genParams{
		Model: "gpt-image-2.5-flare", Prompt: "a cat", Size: "auto", Quality: "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := (*lastReq).Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("auth header = %q", got)
	}
	if (*body)["model"] != "gpt-image-2.5-flare" || (*body)["prompt"] != "a cat" {
		t.Fatalf("body = %v", *body)
	}
	for _, absent := range []string{"output_compression", "moderation", "n", "images", "mask"} {
		if _, ok := (*body)[absent]; ok {
			t.Fatalf("field %q should be omitted: %v", absent, *body)
		}
	}
}

func TestEditRequestShape(t *testing.T) {
	handler, body, _ := captureBody(t, 200, `{"created":1,"data":[{"b64_json":"aGk="}]}`)
	c, _ := newTestServer(t, handler)

	comp := 80
	_, err := c.edit(context.Background(), editParams{
		genParams: genParams{Model: "gpt-image-2.5-sunburst", Prompt: "swap bg",
			OutputCompression: &comp, Background: "transparent", OutputFormat: "png"},
		Images: []imageRef{{ImageURL: "https://example.com/a.png"}, {FileID: "file-abc"}},
		Mask:   &imageRef{ImageURL: "data:image/png;base64,aGk="},
	})
	if err != nil {
		t.Fatal(err)
	}
	imgs, ok := (*body)["images"].([]any)
	if !ok || len(imgs) != 2 {
		t.Fatalf("images = %v", (*body)["images"])
	}
	if imgs[1].(map[string]any)["file_id"] != "file-abc" {
		t.Fatalf("file_id ref lost: %v", imgs[1])
	}
	if (*body)["mask"].(map[string]any)["image_url"] == "" {
		t.Fatalf("mask = %v", (*body)["mask"])
	}
	if (*body)["output_compression"].(float64) != 80 {
		t.Fatalf("output_compression = %v", (*body)["output_compression"])
	}
}

func TestResolveImageRef(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "in.png")
	os.WriteFile(png, pngBytes, 0o644)
	txt := filepath.Join(dir, "in.txt")
	os.WriteFile(txt, []byte("hello"), 0o644)

	if r, _ := resolveImageRef("file_id:file-xyz"); r.FileID != "file-xyz" {
		t.Fatalf("file_id: %v", r)
	}
	if r, _ := resolveImageRef("https://x/y.png"); r.ImageURL != "https://x/y.png" {
		t.Fatalf("url: %v", r)
	}
	if r, _ := resolveImageRef("data:image/png;base64,AA=="); !strings.HasPrefix(r.ImageURL, "data:") {
		t.Fatalf("data uri: %v", r)
	}
	r, err := resolveImageRef(png)
	if err != nil || !strings.HasPrefix(r.ImageURL, "data:image/png;base64,") {
		t.Fatalf("local png: %v %v", r, err)
	}
	if _, err := resolveImageRef(filepath.Join(dir, "missing.png")); err == nil {
		t.Fatal("expected error for missing file")
	}
	if _, err := resolveImageRef(txt); err == nil || !strings.Contains(err.Error(), "unsupported image type") {
		t.Fatalf("expected type error, got %v", err)
	}
}

func TestAPIErrorMapping(t *testing.T) {
	handler, _, _ := captureBody(t, 400, `{"error":{"message":"blocked","type":"image_generation_user_error","code":"moderation_blocked","moderation_details":{"moderation_stage":"input"}}}`)
	c, _ := newTestServer(t, handler)

	_, err := c.generate(context.Background(), genParams{Prompt: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := err.(*apiError)
	if !ok {
		t.Fatalf("got %T: %v", err, err)
	}
	if ae.Code != "moderation_blocked" || ae.Status != 400 || !strings.Contains(err.Error(), "moderation_details") {
		t.Fatalf("apiError = %+v", ae)
	}
}

func TestOutputPaths(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "img.png")

	if got := outputPaths(base, "png", 1); len(got) != 1 || got[0] != base {
		t.Fatalf("n=1: %v", got)
	}
	got := outputPaths(base, "png", 3)
	want := []string{filepath.Join(dir, "img-1.png"), filepath.Join(dir, "img-2.png"), filepath.Join(dir, "img-3.png")}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("n=3: %v, want %v", got, want)
		}
	}
	if got := outputPaths(filepath.Join(dir, "img"), "jpeg", 1); got[0] != filepath.Join(dir, "img.jpg") {
		t.Fatalf("ext fill: %v", got)
	}
}

func TestCommonArgsValidation(t *testing.T) {
	h := &handlers{defaultModel: "gpt-image-2.5-flare"}
	abs := filepath.Join(t.TempDir(), "o.png")

	if _, _, _, err := h.commonArgs(reqWith(map[string]any{"outputPath": "rel/o.png"})); err == nil {
		t.Fatal("relative outputPath should fail")
	}
	if _, _, _, err := h.commonArgs(reqWith(map[string]any{
		"outputPath": abs, "background": "transparent", "outputFormat": "jpeg",
	})); err == nil {
		t.Fatal("transparent+jpeg should fail")
	}
	if _, _, _, err := h.commonArgs(reqWith(map[string]any{
		"outputPath": abs, "outputCompression": 50.0, "outputFormat": "png",
	})); err == nil {
		t.Fatal("compression+png should fail")
	}
	p, out, inline, err := h.commonArgs(reqWith(map[string]any{
		"outputPath": abs, "outputCompression": 50.0, "outputFormat": "webp", "n": 2.0,
	}))
	if err != nil || *p.OutputCompression != 50 || p.N != 2 || out != abs || inline {
		t.Fatalf("valid args: %+v %v %v %v", p, out, inline, err)
	}
}

func TestSaveResults(t *testing.T) {
	dir := t.TempDir()
	raw := pngBytes
	resp := &imagesResponse{}
	json.Unmarshal([]byte(`{"created":1,"data":[{"b64_json":"`+base64.StdEncoding.EncodeToString(raw)+`","revised_prompt":"rp"}],"usage":{"input_tokens":10,"output_tokens":100,"total_tokens":110}}`), resp)

	res, err := saveResults(resp, filepath.Join(dir, "sub", "o.png"), "png", true)
	if err != nil || res.IsError {
		t.Fatalf("saveResults: %v %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "o.png")); err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if len(res.Content) != 2 {
		t.Fatalf("want text+image content, got %d", len(res.Content))
	}
	if _, ok := res.Content[0].(mcp.ImageContent); !ok {
		t.Fatalf("first content should be ImageContent, got %T", res.Content[0])
	}
}

func TestUsageGuide(t *testing.T) {
	h := &handlers{}
	res, err := h.usageGuide(context.Background(), reqWith(nil))
	if err != nil || res.IsError || len(res.Content) == 0 {
		t.Fatalf("usageGuide: %v %v", res, err)
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok || !strings.Contains(tc.Text, "gpt-image-2.5") {
		t.Fatalf("guide content wrong: %v", res.Content[0])
	}
}
