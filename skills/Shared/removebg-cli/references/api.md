# remove.bg HTTP API reference

Official endpoint: `POST https://api.remove.bg/v1.0/removebg`

## cURL examples

Preview from a local file:

```sh
source ~/.zshrc
curl -fS -H "X-API-Key: $REMOVE_BG_API_KEY" \
  -F "image_file=@/path/to/input.png" \
  -F "size=preview" \
  -F "type=graphic" \
  -F "format=png" \
  -D /tmp/removebg-headers.txt \
  -o /path/to/output-preview.png \
  https://api.remove.bg/v1.0/removebg
```

Approved full-size result:

```sh
source ~/.zshrc
curl -fS -H "X-API-Key: $REMOVE_BG_API_KEY" \
  -F "image_file=@/path/to/input.png" \
  -F "size=full" \
  -F "type=graphic" \
  -F "format=png" \
  -D /tmp/removebg-headers.txt \
  -o /path/to/output-full.png \
  https://api.remove.bg/v1.0/removebg
```

Use `image_url` instead of `image_file` for a public source URL.

## Size and credit rules

- `preview` (aliases: `small`, `regular`): up to 0.25 MP, such as 625×400; 0.25 credit when not free.
- Accounts receive 50 free API/app previews per month.
- `full`: original resolution within format limits; 1 credit.
- `50MP`: up to 50 MP; use ZIP, WebP, or JPG as appropriate.
- Avoid `auto` for cost-controlled work. It uses preview for inputs at or below 0.25 MP and full size for larger inputs.
- The response header `X-Credits-Charged` reports the actual charge.

## Limits and formats

- Maximum input file size: 22 MB.
- Maximum supported resolution: 50 MP.
- PNG: transparency, up to 10 MP.
- WebP: transparency, up to 50 MP.
- ZIP: transparency as `color.jpg` plus `alpha.png`, up to 50 MP; fastest option.
- JPG: no transparency, up to 50 MP.

## Useful response headers

- `X-Width`, `X-Height`: result dimensions.
- `X-Credits-Charged`: credits used.
- `X-Foreground-Top`, `X-Foreground-Left`, `X-Foreground-Width`, `X-Foreground-Height`: detected foreground bounds.
- `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After`: throttling state.

## Failure handling

- A `429` response is not charged. Respect `Retry-After`; do not busy-retry.
- Preserve error bodies when diagnosing `400` responses.
- On `403`, verify only that the environment variable exists; never display its value.
- The API supports up to 500 one-megapixel images per minute, decreasing proportionally with resolution.

Sources: official remove.bg API documentation, pricing/help pages, and API changelog.
