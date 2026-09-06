#!/bin/sh
# 在普通用户下构建，替换程序时才请求 sudo。
set -eu
cd "$(dirname "$0")/.."
exec python3 scripts/deploy.py "$@"
