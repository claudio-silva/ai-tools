package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// apiError is a non-2xx response from the OpenAI Images API.
type apiError struct {
	Status  int
	Message string
	Type    string
	Code    string
	Details json.RawMessage // moderation_details, when present
}

func (e *apiError) Error() string {
	s := fmt.Sprintf("openai: status %d: %s", e.Status, e.Message)
	if e.Type != "" {
		s += fmt.Sprintf(" (type=%s", e.Type)
		if e.Code != "" {
			s += ", code=" + e.Code
		}
		s += ")"
	} else if e.Code != "" {
		s += " (code=" + e.Code + ")"
	}
	if len(e.Details) > 0 {
		s += " moderation_details=" + string(e.Details)
	}
	return s
}

// imageRef is an input image for the edits endpoint: either an uploaded
// File API ID or a URL (remote https URL or base64 data URL).
type imageRef struct {
	FileID   string `json:"file_id,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

// genParams mirrors POST /v1/images/generations (gpt-image-2.5 surface).
type genParams struct {
	Model             string `json:"model,omitempty"`
	Prompt            string `json:"prompt"`
	N                 int    `json:"n,omitempty"`
	Size              string `json:"size,omitempty"`
	Quality           string `json:"quality,omitempty"`
	Background        string `json:"background,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
}

// editParams mirrors POST /v1/images/edits.
type editParams struct {
	genParams
	Images []imageRef `json:"images"`
	Mask   *imageRef  `json:"mask,omitempty"`
}

type imagesResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		B64JSON       string `json:"b64_json"`
		RevisedPrompt string `json:"revised_prompt"`
		URL           string `json:"url"`
	} `json:"data"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

type client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func (c *client) generate(ctx context.Context, p genParams) (*imagesResponse, error) {
	return c.post(ctx, "/images/generations", p)
}

func (c *client) edit(ctx context.Context, p editParams) (*imagesResponse, error) {
	return c.post(ctx, "/images/edits", p)
}

func (c *client) post(ctx context.Context, path string, payload any) (*imagesResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message           string          `json:"message"`
				Type              string          `json:"type"`
				Code              string          `json:"code"`
				ModerationDetails json.RawMessage `json:"moderation_details"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
			return nil, &apiError{
				Status:  resp.StatusCode,
				Message: e.Error.Message,
				Type:    e.Error.Type,
				Code:    e.Error.Code,
				Details: e.Error.ModerationDetails,
			}
		}
		return nil, &apiError{Status: resp.StatusCode, Message: string(bytes.TrimSpace(data))}
	}

	var out imagesResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}
