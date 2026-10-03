#!/usr/bin/env bash
# 一条命令启动整个项目：初始化/复用 SQLite、自动播种示例数据、导入示例销量、启动 Web 服务。
set -euo pipefail
cd "$(dirname "$0")"

# 若系统没有 go，使用脚本同级的 sdk 或 ~/sdk/go
if ! command -v go >/dev/null 2>&1; then
  for d in "./.tools/go" "$HOME/sdk/go" "/usr/local/go"; do
    if [ -x "$d/bin/go" ]; then export PATH="$d/bin:$PATH"; break; fi
  done
fi
command -v go >/dev/null 2>&1 || { echo "未找到 Go，请先安装 Go 1.21+"; exit 1; }

ADDR="${ADDR:-:8080}"
DB="${DB:-data/royalty.db}"

echo "──────────────────────────────────────────────────────"
echo " 出版社稿酬结算与授权区域系统"
echo " 地址: http://localhost${ADDR}    数据库: ${DB}"
echo " 账号: editor/editor123 · finance/finance123 · wangming/author123 · lihua/author123"
echo "──────────────────────────────────────────────────────"

# 首次运行自动建库并播种；重跑幂等（已有数据不会覆盖）
if [ ! -f "$DB" ]; then
  echo ">> 首次运行：初始化数据库与示例数据…"
  go run . seed --db "$DB"
fi

exec go run . serve --addr "$ADDR" --db "$DB" --web web
