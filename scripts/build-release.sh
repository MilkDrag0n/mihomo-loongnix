#!/bin/sh
# 兼容原构建入口；现在输出固定目录，不要求提交号。
set -eu
exec "$(dirname "$0")/build-current.sh" "$@"
