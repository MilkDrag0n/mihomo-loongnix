#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
. ./scripts/web-toolchain.sh
npm ci --prefix web
npm run check --prefix web
npm test --prefix web
go test -count=1 ./internal/webconfig ./internal/webgateway ./cmd/mihomo-web
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/tests -p 'test_deploy_web.py'
