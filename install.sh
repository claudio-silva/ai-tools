#!/bin/sh
# Install or update aitools from the latest GitHub release:
#   curl -fsSL https://raw.githubusercontent.com/claudio-silva/ai-tools/main/install.sh | sh
# Set AITOOLS_VERSION (e.g. 0.2.0) to pick a release.
set -eu

repo="claudio-silva/ai-tools"
fail() { echo "aitools install: $*" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) os=darwin ;;
  *) fail "$(uname -s) is not supported yet (macOS only)" ;;
esac
case "$(uname -m)" in
  arm64 | aarch64) arch=arm64 ;;
  x86_64 | amd64) arch=amd64 ;;
  *) fail "unsupported CPU $(uname -m)" ;;
esac

if [ -n "${AITOOLS_VERSION:-}" ]; then
  base="https://github.com/$repo/releases/download/v${AITOOLS_VERSION#v}"
else
  base="https://github.com/$repo/releases/latest/download"
fi
asset="aitools-$os-$arch"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "$base/$asset" -o "$tmp/aitools" || fail "could not download $base/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "could not download $base/checksums.txt"
expected="$(awk -v a="$asset" '$2 == a { print $1 }' "$tmp/checksums.txt")"
actual="$(shasum -a 256 "$tmp/aitools" | awk '{ print $1 }')"
[ -n "$expected" ] && [ "$expected" = "$actual" ] || fail "checksum mismatch for $asset"

chmod +x "$tmp/aitools"
# stdin is this script when piped from curl; keep setup from reading it.
"$tmp/aitools" setup </dev/null
"$tmp/aitools" version
