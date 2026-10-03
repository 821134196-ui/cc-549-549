#!/usr/bin/env bash
# 墨衡 · 出版社稿酬结算与授权区域系统 —— 一条命令启动
#
# 用法：
#   scripts/run.sh           构建前端 → 构建后端 → 示例数据自检 → 启动服务 (http://localhost:8080)
#   scripts/run.sh verify    仅构建并运行示例数据自检验证（多作者分成/重复导入/越区销售…）
#   PORT=9000 scripts/run.sh 指定端口
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ---- 定位 Go ----
if ! command -v go >/dev/null 2>&1; then
  if [ -x "$HOME/.local/go/bin/go" ]; then
    export PATH="$HOME/.local/go/bin:$PATH"
  else
    echo "未找到 Go。请安装 Go 1.22+（或解压到 ~/.local/go）。" >&2
    exit 1
  fi
fi
export CGO_ENABLED=0          # 使用纯 Go SQLite 驱动，无需 gcc
export GOPATH="${GOPATH:-$HOME/go}"
export GOCACHE="${GOCACHE:-$HOME/.cache/go-build}"

# ---- 1) 构建前端（产物直接输出到 Go embed 目录）----
if [ ! -d frontend/node_modules ]; then
  echo "▶ 安装前端依赖…"
  (cd frontend && npm install --no-audit --no-fund)
fi
echo "▶ 构建 Vue 前端…"
(cd frontend && npm run build)

# ---- 2) 构建后端 ----
echo "▶ 构建 Go 后端…"
mkdir -p bin
go build -o bin/royalty ./backend

# ---- 3) 示例数据自检验证 ----
echo "▶ 运行示例数据自检验证…"
./bin/royalty verify --db=data/verify.db

if [ "${1:-}" = "verify" ]; then
  echo "✅ 自检完成"
  exit 0
fi

# ---- 4) 启动整个项目（首次启动自动写入示例数据）----
PORT="${PORT:-8080}"
PORT="${PORT#:}"
ADDR="${ROYALTY_ADDR:-:$PORT}"
echo "──────────────────────────────────────────────"
echo "  墨衡稿酬系统已就绪：http://localhost:${PORT}"
echo "  编辑 editor@pub.com/editor123  财务 finance@pub.com/finance123  作者 lin@author.com/author123"
echo "──────────────────────────────────────────────"
exec ./bin/royalty serve --addr="$ADDR" --db=data/royalty.db
