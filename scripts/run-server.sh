#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export CGO_ENABLED=1
export GOTOOLCHAIN=local
export CGO_LDFLAGS="-L${ROOT}/third_party/tokenizers"
export LAYA_MODEL_DIR="${LAYA_MODEL_DIR:-${ROOT}/models/snake}"
exec go run ./cmd/server "$@"
