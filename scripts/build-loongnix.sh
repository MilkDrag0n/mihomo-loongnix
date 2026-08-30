#!/bin/sh
set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

machine=$(uname -m)
case "$machine" in
	loongarch64|loong64) ;;
	*) printf '警告：当前机器是 %s，将交叉构建 linux/loong64。\n' "$machine" >&2 ;;
esac

if [ "$(go env GOARCH)" != "loong64" ] && [ "$machine" = "loongarch64" ]; then
	printf '错误：当前 Go 工具链未识别 loong64，请使用 Loongnix 自带 Go。\n' >&2
	exit 1
fi

./scripts/check-secrets.sh
go test ./...
mkdir -p build/linux
CGO_ENABLED=0 GOOS=linux GOARCH=loong64 go build -trimpath -ldflags='-s -w' -o build/linux/mihomo-tui-linux-loong64 .
printf '构建完成：%s/build/linux/mihomo-tui-linux-loong64\n' "$repo_root"
