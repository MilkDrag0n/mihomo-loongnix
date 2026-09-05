#!/bin/sh
# 可由调用者提供 Node/pnpm；本服务器使用仓库外用户工具链。
set -eu
web_tools="${XDG_DATA_HOME:-$HOME/.local/share}/mihomo-loongnix/toolchains"
if ! command -v node >/dev/null 2>&1; then
 PATH="$web_tools/node-current/bin:$PATH"
fi
if ! command -v pnpm >/dev/null 2>&1; then
 PATH="$web_tools/pnpm/bin:$PATH"
fi
export PATH
command -v node >/dev/null 2>&1 || { echo '缺少 Node（>=22.12），请按 Web 文档准备构建环境。' >&2; exit 1; }
command -v pnpm >/dev/null 2>&1 || { echo '缺少 pnpm 10.34.5。' >&2; exit 1; }
[ "$(pnpm --version)" = '10.34.5' ] || { echo '请使用 pnpm 10.34.5。' >&2; exit 1; }
