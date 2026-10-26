#!/bin/sh
# 可由调用者提供 Node/npm；本服务器使用仓库外用户工具链。
set -eu
web_tools="${XDG_DATA_HOME:-$HOME/.local/share}/mihomo-loongnix/toolchains"
if ! command -v node >/dev/null 2>&1; then
 PATH="$web_tools/node-current/bin:$PATH"
fi
export PATH
command -v node >/dev/null 2>&1 || { echo '缺少 Node（>=22.12），请按 Web 文档准备构建环境。' >&2; exit 1; }
command -v npm >/dev/null 2>&1 || { echo '缺少随 Node 分发的 npm。' >&2; exit 1; }
