# imagen — usage & prompting guide

MCP server for OpenAI **gpt-image-2.5** image generation and editing.

## Tools

- `generate_image` — create a NEW image from a text prompt. Not for modifying existing images.
- `edit_image` — modify EXISTING image(s) with a prompt.
- `get_usage_guide` — returns this document.

## Inputs (`edit_image`)

Each `images` entry may be an absolute local file path, an `https://` URL, a `data:` URI, or a `file_id:` reference to an OpenAI File API upload. Up to 16 images.

- The first image is the primary subject. Assign each input a numbered role in the prompt ("image 1 = subject, image 2 = style reference, image 3 = clothing") and state how they combine.
- `mask` (same input forms): transparent regions of the mask mark where the model may change the image; the mask must match the input image's dimensions.

## Outputs

- `outputPath` is required and must be **absolute** (e.g. `/Users/me/out/logo.png`). Parent directories are created. A missing extension is filled from `outputFormat`.
- With `n` > 1, files are saved as `name-1.ext` … `name-N.ext`.
- `returnInlineImage` also embeds image bytes in the tool result — costs context; default false.

## Models

- `gpt-image-2.5-flare` — small model, optimized for speed; quality comparable to gpt-image-2. Default.
- `gpt-image-2.5-sunburst` — base model, optimized for quality; best for demanding edits and subject preservation. Slower.
- Dated snapshots `…-2026-09-08` pin behavior.
- The same `quality` label does NOT imply the same quality or latency across models — compare explicitly on your workload before switching.

## Parameters

- `size`: `auto` | `1024x1024` | `1536x1024` | `1024x1536` | custom `WIDTHxHEIGHT`.
  Custom: edges ≤ 3840, multiples of 16, aspect between 1:3 and 3:1, total pixels 655,360–8,294,400. Above `2560x1440` is experimental.
- `quality`: `auto` (default), `low`, `medium`, `high`, `xhigh`, `max`.
  Higher is not better for every prompt — use `xhigh`/`max` only when a quality requirement is unmet; `low` for drafts.
- `background`: `auto` | `opaque` | `transparent`. Transparent requires `outputFormat` png or webp — verify alpha at hair, glass, and edges; a drawn checkerboard is not transparency.
- `outputFormat`: `png` (default) | `jpeg` | `webp`. `outputCompression` (0–100) applies to jpeg/webp only.
- `moderation`: `auto` | `low` (less restrictive filtering).

## Prompting gpt-image-2.5 (model-specific essentials)

- **Edits:** say "change only X" and enumerate what must be preserved — identity, face, product geometry, layout, lighting, labels. Restate the preserve list on EVERY edit; fidelity drifts across turns.
- If a region must remain pixel-identical, composite it post-edit — prompting cannot guarantee pixel-exact preservation.
- **Exact text:** put required wording in quotes with position and typography; spell unusual names letter-by-letter; add "no other text". Use medium/high quality for small or dense text.
- **Transparent cutouts:** request the isolated subject in the prompt AND `background=transparent`; say "no backdrop, scenery, checkerboard, or watermark"; repeat the transparency requirement on later edits.
- **Sketch → render:** treat the prompt as a spec — preserve layout and perspective, and add "do not add new elements or text" to avoid reinterpretation.
- Camera/lens terms are appearance cues, not physical-simulation guarantees; specify scale, atmosphere, and color for mood.
- **Iterate:** feed the previous output back as the next edit input, one change per turn.

## Errors

- `moderation_blocked` — prompt or input hit the filter; revise before retrying (`moderation_details` may indicate the stage).
- `image_generation_user_error` — correctable request issue (size, format, prompt); fix inputs, don't blindly retry.
- Rate-limit / quota / 5xx — retry with backoff; don't retry quota errors without changing the request.
