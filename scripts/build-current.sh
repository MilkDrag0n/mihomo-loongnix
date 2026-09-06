#!/bin/sh
# 构建当前工作区，不要求提交，不运行测试，不按版本归档。
set -eu
cd "$(dirname "$0")/.."
[ "$(id -u)" -ne 0 ] || { echo '请用普通用户构建，不要加 sudo。' >&2; exit 1; }
case "${1:-}" in ""|--web) ;; *) echo '用法：build-current.sh [--web]' >&2; exit 1;; esac
[ "$#" -le 1 ] || exit 1
case "$(uname -m)" in loongarch64|loong64) ;; *) echo '此脚本用于 LoongArch Linux。' >&2; exit 1;; esac
build_dir="${XDG_DATA_HOME:-$HOME/.local/share}/mihomo-loongnix/build/current"
mkdir -p "$build_dir"
echo '构建当前 TUI／管理器代码……'
CGO_ENABLED=0 GOOS=linux GOARCH=loong64 go build -trimpath -o "$build_dir/mihomo-tui" .
if [ "${1:-}" = --web ]; then
 . ./scripts/web-toolchain.sh
 echo '构建当前 Web 代码……'
 pnpm --dir web install --frozen-lockfile
 MIHOMO_WEB_DIST="$build_dir/static" pnpm --dir web build
 CGO_ENABLED=0 GOOS=linux GOARCH=loong64 go build -trimpath -o "$build_dir/mihomo-web" ./cmd/mihomo-web
 cp deploy/web/mihomo-web.service "$build_dir/mihomo-web.service"
fi
echo "构建完成：$build_dir"
