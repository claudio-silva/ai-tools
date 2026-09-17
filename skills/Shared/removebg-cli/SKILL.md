---
name: removebg-cli
description: Remove image backgrounds with the official remove.bg CLI or HTTP API while controlling preview/full resolution, API credits, transparency, output formats, and validation. Use for background extraction, transparent cutouts, alpha mattes, removebg CLI troubleshooting, or direct remove.bg API calls.
---

# remove.bg CLI

Use the official CLI first. Use the HTTP API directly when the CLI is unavailable, unreliable, or response headers are needed.

## Credit-safe workflow

1. Inspect pixel dimensions, megapixels, file size, and alpha before calling the API.
2. Treat every request as an external upload. Confirm that the user authorized uploading the specific image unless that authorization is already explicit.
3. Use `preview` for drafts, mask evaluation, and layout proofs. Preview output is limited to 0.25 MP (for example 625×400), costs 0.25 credit after the monthly allowance, and the first 50 API/app previews per month are free.
4. If the original is at most 0.25 MP, always use `--size preview`; never spend a full credit on it.
5. Use `full` only for an approved final image. Ask for explicit confirmation immediately before each full-size processing request.
6. Never use `auto` by default: for inputs above 0.25 MP it can select full size and charge 1 credit.
7. Report the chosen size, successful request count, output dimensions, and expected credit charge.

## Authenticate safely

Prefer `REMOVE_BG_API_KEY` from the environment. If it is missing in the current shell, run `source ~/.zshrc` in the same process as the CLI call. Never print the key, save it in the skill, or expose it in command output.

## Run the CLI

Create the output directory first. In Codex terminal execution, allocate a PTY/TTY: CLI 2.0.0 may exit silently without one.

Preview proof:

```sh
source ~/.zshrc
removebg input.png --size preview --type graphic --channels rgba --format png --skip-png-format-optimization --output-directory output
```

Approved final up to 10 MP:

```sh
source ~/.zshrc
removebg input.png --size full --type graphic --channels rgba --format png --skip-png-format-optimization --output-directory output
```

For batches, inspect the file list first and keep the default batch confirmation. Do not disable it unless the user explicitly authorizes the exact batch.

## Choose formats

- Use PNG with RGBA for transparent outputs up to 10 MP.
- For transparent outputs above 10 MP and up to 50 MP, request WebP or ZIP. Prefer ZIP for speed and lossless alpha separation, then use `removebg zip2png --file result.zip` when a PNG working file is required.
- Use JPG only when transparency is not required.
- Maximum supported input/output is 50 MP; images above the limit are resized. The documented example maximum is 5774×8660.

## Validate every result

1. Confirm expected dimensions and an actual alpha channel.
2. Inspect the cutout on both dark and light solid backgrounds at 100%.
3. Check pale subjects, hair, lace, paper, glass, semi-transparent edges, and edge halos.
4. Compare against the source for missing objects or clipped margins.
5. Preserve the original and save results non-destructively with explicit `preview` or `full` in the filename.

For direct API syntax, response headers, format limits, and failure handling, read [references/api.md](references/api.md).
