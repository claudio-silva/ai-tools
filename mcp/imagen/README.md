# imagen

A lightweight [MCP](https://modelcontextprotocol.io) server for OpenAI **gpt-image-2.5** image generation and editing, over stdio.

Written in Go — one direct dependency ([mark3labs/mcp-go](https://github.com/mark3labs/mcp-go)) plus a minimal `net/http` client for the OpenAI calls (no OpenAI SDK). Compiles to a fully self-contained ~8 MB static binary with no runtime dependencies.

## Features

- **`generate_image`** — create images from text prompts (`gpt-image-2.5-flare` / `gpt-image-2.5-sunburst`)
- **`edit_image`** — edit existing images; inputs can be local files, `https://` URLs, `data:` URIs, or OpenAI `file_id:` refs; optional `mask`
- **`get_usage_guide`** — embedded usage + prompting guide, also delivered automatically at `initialize` via the MCP `instructions` field, so the server is self-sufficient for LLM callers
- Full gpt-image-2.5 parameter surface: `size` (presets or custom `WxH`), `quality` (`auto` → `max`), `background` (incl. `transparent`), `outputFormat`, `outputCompression`, `moderation`, `n`
- Images written to disk; optionally returned inline as MCP image content

## Requirements

- Go 1.24+ (built/tested with 1.27)
- An OpenAI API key with access to gpt-image-2.5 models

## Build & install

The Apple Silicon binary is shipped at `bin/imagen`. Install it into MCP clients with:

```sh
aitools install imagen
```

To rebuild during development:

```sh
make build      # → bin/imagen (static, current arch)
make release    # → darwin arm64 + amd64 + lipo universal binary in bin/
make test vet
```

## Configuration

| Env var | Default | Purpose |
| --- | --- | --- |
| `OPENAI_API_KEY` | — | Required at call time; the server starts without it but tools return a clear error |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Endpoint override (proxies, Azure, etc.) |
| `IMAGEN_MODEL` | `gpt-image-2.5-flare` | Default model |

MCP client config (Claude Desktop `claude_desktop_config.json`, Cursor `.cursor/mcp.json`, Windsurf `~/.codeium/windsurf/mcp_config.json`, Devin `~/.config/devin/mcp_config.json`):

```json
{
  "mcpServers": {
    "imagen": {
      "command": "/path/to/imagen",
      "env": { "OPENAI_API_KEY": "sk-..." }
    }
  }
}
```

## Tools

### `generate_image`

| Param | Required | Notes |
| --- | --- | --- |
| `prompt` | yes | Text describing the image |
| `outputPath` | yes | **Absolute** file path; parent dirs are created; missing extension filled from `outputFormat` |
| `model` | | `gpt-image-2.5-flare` (default), `gpt-image-2.5-sunburst`, or dated `2026-09-08` snapshots |
| `size` | | `auto`, `1024x1024`, `1536x1024`, `1024x1536`, or custom `WxH` (multiples of 16, ratio ≤ 3:1, ≤ 3840/edge) |
| `quality` | | `auto`, `low`, `medium`, `high`, `xhigh`, `max` |
| `background` | | `auto`, `opaque`, `transparent` (transparent requires png/webp) |
| `outputFormat` | | `png` (default), `jpeg`, `webp` |
| `outputCompression` | | 0–100, jpeg/webp only |
| `moderation` | | `auto`, `low` |
| `n` | | 1–10; `n>1` writes `name-1.ext … name-N.ext` |
| `returnInlineImage` | | Also embed image bytes in the result (default false — saves context) |

### `edit_image`

Same parameters, plus:

| Param | Required | Notes |
| --- | --- | --- |
| `images` | yes | 1–16 input images: absolute paths, `https://` URLs, `data:` URIs, or `file_id:` refs. First image = primary subject |
| `mask` | | Transparent regions mark editable areas; must match input dimensions |

### `get_usage_guide`

Returns the embedded [guide.md](guide.md): parameter rules and gpt-image-2.5 prompting techniques (model selection, edit-preserve patterns, text rendering, iteration, error codes).

## Design notes

- **No OpenAI SDK** — the Images API surface used is two JSON endpoints; requests go through a ~100-line `net/http` client (`openai.go`).
- **Edits are sent as a single JSON body** — local files are encoded as base64 data URLs in `images[].image_url` (~33% size overhead vs multipart, fine for the ≤16 image limit).
- **API errors are surfaced verbatim** including `code` (`moderation_blocked`, `image_generation_user_error`) and `moderation_details`, so LLM callers can self-correct.
- **5-minute HTTP timeout** — high-quality renders can take 1–2 minutes.
- Diagnostics go to stderr only; stdout is reserved for JSON-RPC.
