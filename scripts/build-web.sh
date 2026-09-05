#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
. ./scripts/web-toolchain.sh
[ -z "$(git status --porcelain --untracked-files=all)" ] || { echo '请先提交工作区修改。' >&2; exit 1; }
commit=$(git rev-parse HEAD)
artifact_root="${XDG_DATA_HOME:-$HOME/.local/share}/mihomo-loongnix/builds/web"
mkdir -p "$artifact_root"
artifact_root=$(cd "$artifact_root" && pwd -P)
case "$artifact_root/" in "$(pwd -P)/"*) echo '产物不可放在源码目录内。' >&2; exit 1;; esac
destination="$artifact_root/$commit"
[ ! -e "$destination" ] || { echo '该提交已有 Web 构建，请勿覆盖。' >&2; exit 1; }
temporary=$(mktemp -d "$artifact_root/.build-XXXXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
./scripts/check-secrets.sh
pnpm --dir web install --frozen-lockfile
MIHOMO_WEB_DIST="$temporary/static" pnpm --dir web build
CGO_ENABLED=0 GOOS=linux GOARCH=loong64 go build -trimpath -buildvcs=true -o "$temporary/mihomo-web-linux-loong64" ./cmd/mihomo-web
cp deploy/web/mihomo-web.service "$temporary/mihomo-web.service"
go version -m "$temporary/mihomo-web-linux-loong64" > "$temporary/BUILD-INFO.txt"
grep -q "vcs.revision=$commit" "$temporary/BUILD-INFO.txt"
grep -q 'vcs.modified=false' "$temporary/BUILD-INFO.txt"
{
 printf '\ncommit=%s\n' "$commit"
 printf 'node=%s\npnpm=%s\n' "$(node --version)" "$(pnpm --version)"
 sha256sum web/pnpm-lock.yaml
} >> "$temporary/BUILD-INFO.txt"
python3 - "$temporary" <<'PYWEB'
import hashlib,sys
from pathlib import Path
p=Path(sys.argv[1])
(p/'SHA256SUMS').write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest()+'  '+f.relative_to(p).as_posix()+'\n' for f in sorted(p.rglob('*')) if f.is_file()))
PYWEB
[ "$(git rev-parse HEAD)" = "$commit" ] && [ -z "$(git status --porcelain --untracked-files=all)" ] || { echo '构建期间源码发生变化。' >&2; exit 1; }
mv "$temporary" "$destination"
trap - EXIT HUP INT TERM
printf 'Web 构建完成：%s\n' "$destination"
