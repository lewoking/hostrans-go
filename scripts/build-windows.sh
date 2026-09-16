#!/usr/bin/env bash
# Cross-compile hostrans.exe. Injects AI key from .secrets (gitignored).
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
ver="${1:-dev}"
out="${2:-/tmp/hostrans.exe}"

if [[ -f .secrets ]]; then
  set -a
  # shellcheck disable=SC1091
  source .secrets
  set +a
fi

if [[ -z "${HOSTTRANS_AI_KEY:-}" ]]; then
  echo "missing HOSTTRANS_AI_KEY (put it in .secrets or env)" >&2
  exit 1
fi

base="${HOSTTRANS_AI_BASE:-https://gateway.ai.cloudflare.com/v1/64d53ca476db7004bc2b51e1d9db2dad/translation/compat}"
model="${HOSTTRANS_AI_MODEL:-dynamic/free}"

ldflags="-s -w -H windowsgui"
ldflags+=" -X hostrans/dlog.Version=${ver}"
ldflags+=" -X hostrans/translator.AIKey=${HOSTTRANS_AI_KEY}"
ldflags+=" -X hostrans/translator.AIBase=${base}"
ldflags+=" -X hostrans/translator.AIModel=${model}"

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$ldflags" -o "$out" .
echo "built $out version=$ver model=$model"
