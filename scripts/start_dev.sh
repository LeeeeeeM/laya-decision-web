#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ ! -f .env ]; then
  echo "❌ 缺少 .env（可先: cp .env.example .env）"
  exit 1
fi

# shellcheck disable=SC1091
set -a
source .env
set +a

export CGO_ENABLED=1
export GOTOOLCHAIN=local
export CGO_LDFLAGS="-L${ROOT}/third_party/tokenizers"
export LAYA_MODEL_DIR="${LAYA_MODEL_DIR:-${ROOT}/models/snake}"

ADDR="${LISTEN_ADDR:-127.0.0.1:18080}"
PORT="${ADDR##*:}"

echo "🚀 启动 Go 后端（Air 热重载）"
echo "📁 工作目录: ${ROOT}"
echo "🌐 LISTEN_ADDR=${ADDR}  （前端 Vite 代理应指向此端口）"
echo "📦 LAYA_MODEL_DIR=${LAYA_MODEL_DIR}"
echo "⚡ 修改 Go 代码后会自动重新编译并重启"
echo ""

if [ ! -f "${ROOT}/third_party/tokenizers/libtokenizers.a" ] && \
   [ ! -f "${ROOT}/third_party/tokenizers/libtokenizers.darwin-arm64.a" ]; then
  # lib may be named libtokenizers.a after extract
  if ! ls "${ROOT}/third_party/tokenizers/"*.a >/dev/null 2>&1; then
    echo "⚠️  未找到 third_party/tokenizers 静态库，可先: make deps"
  fi
fi

# 避免再连到旧进程：开发时常忘了停掉 bin/ 下的常驻服务
if command -v lsof >/dev/null 2>&1; then
  PIDS="$(lsof -nP -iTCP:"${PORT}" -sTCP:LISTEN -t 2>/dev/null || true)"
  if [ -n "${PIDS}" ]; then
    echo "⚠️  端口 ${PORT} 已被占用，结束旧进程: ${PIDS}"
    # shellcheck disable=SC2086
    kill ${PIDS} 2>/dev/null || true
    sleep 0.5
  fi
fi

run_air() {
  echo "✅ 使用: $1"
  exec "$1"
}

if command -v air >/dev/null 2>&1; then
  run_air "$(command -v air)"
elif [ -x "${HOME}/go/bin/air" ]; then
  run_air "${HOME}/go/bin/air"
elif [ -x /usr/local/bin/air ]; then
  run_air /usr/local/bin/air
else
  echo "❌ 未找到 air"
  echo "请安装: go install github.com/air-verse/air@latest"
  echo "并确保 \$HOME/go/bin 在 PATH 中"
  exit 1
fi
