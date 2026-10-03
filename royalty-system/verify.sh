#!/usr/bin/env bash
# 业务规则一键验证（多作者分成 / 重复导入 / 跨区销售 / 跨期退货 / 到期补传 / 补差单 / 付款回执）
set -euo pipefail
cd "$(dirname "$0")"
if ! command -v go >/dev/null 2>&1; then
  for d in "./.tools/go" "$HOME/sdk/go" "/usr/local/go"; do
    [ -x "$d/bin/go" ] && export PATH="$d/bin:$PATH" && break
  done
fi
exec go run . verify "$@"
