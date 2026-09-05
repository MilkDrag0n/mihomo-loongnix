#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
. ./scripts/web-toolchain.sh
pnpm --dir web install --frozen-lockfile
pnpm --dir web check
pnpm --dir web test
go test -count=1 ./internal/webconfig ./internal/webgateway ./cmd/mihomo-web
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/tests -p 'test_deploy_web.py'
