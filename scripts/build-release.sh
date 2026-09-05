#!/bin/sh
# 从干净的已提交源码构建，产物放在仓库外并记录来源。
set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
if [ -n "$(git status --porcelain --untracked-files=all)" ]; then
    printf '错误：请先提交或妥善保存工作区修改，再构建可部署程序。\n' >&2
    exit 1
fi

commit=$(git rev-parse HEAD)
artifact_root="${XDG_DATA_HOME:-$HOME/.local/share}/mihomo-loongnix/builds"
mkdir -p "$artifact_root"
artifact_root=$(cd "$artifact_root" && pwd -P)
case "$artifact_root/" in
    "$repo_root/"*) printf '错误：构建产物必须保存在源码目录之外。\n' >&2; exit 1 ;;
esac
destination="$artifact_root/$commit"
if [ -e "$destination" ]; then
    printf '错误：该提交已有构建，请先核对已有产物：%s\n' "$destination" >&2
    exit 1
fi

./scripts/check-secrets.sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/tests
go test -count=1 ./...
go vet ./...
temporary=$(mktemp -d "$artifact_root/.build-XXXXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
CGO_ENABLED=0 GOOS=linux GOARCH=loong64 go build -trimpath -buildvcs=true -o "$temporary/mihomo-tui-linux-loong64" .
if [ "$(git rev-parse HEAD)" != "$commit" ] || [ -n "$(git status --porcelain --untracked-files=all)" ]; then
    printf '错误：构建期间源码发生变化，丢弃本次产物。\n' >&2
    exit 1
fi
go version -m "$temporary/mihomo-tui-linux-loong64" > "$temporary/BUILD-INFO.txt"
if ! grep -q 'vcs.modified=false' "$temporary/BUILD-INFO.txt" || ! grep -q "vcs.revision=$commit" "$temporary/BUILD-INFO.txt"; then
    printf '错误：程序缺少可验证的干净提交信息。\n' >&2
    exit 1
fi
{
    printf '\nsource=https://github.com/MilkDrag0n/mihomo-loongnix\ncommit=%s\n' "$commit"
    printf 'built_at_utc=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    go version
} >> "$temporary/BUILD-INFO.txt"
(cd "$temporary" && sha256sum mihomo-tui-linux-loong64 BUILD-INFO.txt > SHA256SUMS)
mv "$temporary" "$destination"
trap - EXIT HUP INT TERM
printf '构建完成：%s\n' "$destination"
