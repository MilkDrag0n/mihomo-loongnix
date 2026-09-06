#!/bin/sh
# 单独准备可选 Web 首次安装所需的当前构建。
set -eu
exec "$(dirname "$0")/build-current.sh" --web
