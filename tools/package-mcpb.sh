#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 <version> <goos> <goarch> <output-dir>" >&2
  exit 2
fi

version=$1
goos=$2
goarch=$3
output_dir=$4

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid semantic version: $version" >&2
  exit 2
fi
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

case "$goos/$goarch" in
  linux/amd64) suffix=linux-x64; platform=linux; binary=memauthority ;;
  linux/arm64) suffix=linux-arm64; platform=linux; binary=memauthority ;;
  darwin/amd64) suffix=osx-x64; platform=darwin; binary=memauthority ;;
  darwin/arm64) suffix=osx-arm64; platform=darwin; binary=memauthority ;;
  windows/amd64) suffix=win-x64; platform=win32; binary=memauthority.exe ;;
  windows/arm64) suffix=win-arm64; platform=win32; binary=memauthority.exe ;;
  *) echo "unsupported target: $goos/$goarch" >&2; exit 2 ;;
esac

command -v go >/dev/null
command -v mcpb >/dev/null
command -v python3 >/dev/null

mkdir -p "$output_dir"
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
stage="$work/stage"
mkdir -p "$stage/server"

(
  cd "$repo_root"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -buildvcs=false -o "$stage/server/$binary" ./cmd/memauthority
)

python3 - "$repo_root/packaging/mcpb/manifest.template.json" "$stage/manifest.json" "$version" "$binary" "$platform" <<'PY'
import json, sys
src, dst, version, binary, platform = sys.argv[1:]
raw = open(src, encoding='utf-8').read()
raw = raw.replace('__VERSION__', version).replace('__BINARY__', binary).replace('__PLATFORM__', platform)
data = json.loads(raw)
with open(dst, 'w', encoding='utf-8', newline='\n') as f:
    json.dump(data, f, indent=2, ensure_ascii=False)
    f.write('\n')
PY

cp "$repo_root/LICENSE" "$stage/LICENSE"
cp "$repo_root/NOTICE" "$stage/NOTICE"
cp "$repo_root/THIRD_PARTY_NOTICES.md" "$stage/THIRD_PARTY_NOTICES.md"
cp -R "$repo_root/THIRD_PARTY_LICENSES" "$stage/THIRD_PARTY_LICENSES"

# Fix file timestamps so repeated packaging from the same source is reproducible.
find "$stage" -type f -exec touch -t 200001010000 {} +
if [[ "$goos" != windows ]]; then
  chmod 0755 "$stage/server/$binary"
fi

mcpb validate "$stage" >/dev/null
artifact="$output_dir/memauthority-${version}-${suffix}.mcpb"
go run "$repo_root/tools/mcpbzip" "$stage" "$artifact"
mcpb info "$artifact" >/dev/null
sha256sum "$artifact" | awk '{print $1 "  " $2}'
