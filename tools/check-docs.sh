#!/usr/bin/env bash
set -euo pipefail

vm_repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$vm_repo_root"

vm_check_root=$(mktemp -d)
trap 'rm -rf -- "$vm_check_root"' EXIT
vm_pairs=(
  README
  SECURITY
  docs/AGENT-GUIDE
  docs/FAQ
  docs/MCP-CONFIG
  docs/ONBOARDING
  examples/README
)

for vm_base in "${vm_pairs[@]}"; do
  vm_en="${vm_base}.md"
  vm_zh="${vm_base}_ZH.md"
  test -f "$vm_en"
  test -f "$vm_zh"
  vm_en_headings=$(grep -Ec '^#{1,4} ' "$vm_en")
  vm_zh_headings=$(grep -Ec '^#{1,4} ' "$vm_zh")
  if [[ "$vm_en_headings" != "$vm_zh_headings" ]]; then
    printf 'heading-count mismatch: %s=%s %s=%s\n' "$vm_en" "$vm_en_headings" "$vm_zh" "$vm_zh_headings" >&2
    exit 1
  fi
done

vm_broken=0
while IFS= read -r -d '' vm_doc; do
  vm_doc_dir=$(dirname "$vm_doc")
  while IFS= read -r vm_link; do
    vm_target=${vm_link#](}
    vm_target=${vm_target%)}
    case "$vm_target" in
      http:*|https:*|mailto:*|memory:*|'') continue ;;
    esac
    vm_target=${vm_target%%#*}
    vm_target=${vm_target#<}
    vm_target=${vm_target%>}
    if [[ -n "$vm_target" && ! -e "$vm_doc_dir/$vm_target" ]]; then
      printf 'broken local Markdown link: %s -> %s\n' "$vm_doc" "$vm_target" >&2
      vm_broken=1
    fi
  done < <(grep -oE '\]\([^)]+\)' "$vm_doc" || true)
done < <(find . -path './.git' -prune -o -type f -name '*.md' -print0)
test "$vm_broken" -eq 0

go build -trimpath -o "$vm_check_root/memauthority" ./cmd/memauthority
go build -trimpath -o "$vm_check_root/v-memory" ./cmd/v-memory
"$vm_check_root/memauthority" init "$vm_check_root/vault" >/dev/null
cp -R examples/sample-vault/. "$vm_check_root/vault/"
git -C "$vm_check_root/vault" add INDEX.yaml demo
git -C "$vm_check_root/vault" \
  -c user.name='MemAuthority Example' \
  -c user.email='example@memauthority.invalid' \
  commit -m 'example: add sample memory project' >/dev/null
vm_validation=$("$vm_check_root/memauthority" validate --json "$vm_check_root/vault")
legacy_version=$("$vm_check_root/v-memory" version)
grep -Fq 'v-memory 1.3.2' <<<"$legacy_version"
grep -Fq '"valid":true' <<<"$vm_validation"

printf 'documentation checks passed\n'
