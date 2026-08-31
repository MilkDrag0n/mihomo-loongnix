#!/bin/sh
set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

failed=0
forbidden_path='(^|/)(secrets|private|subscriptions|proxy_providers|profiles|logs|runtime|state)(/|$)|(^|/)(config|runtime|subscription)(-[^/]*)?\.ya?ml$|(^|/)\.env($|\.)|\.(key|p12|pfx|token|secret)$'

if ! git ls-files | while IFS= read -r path; do
	case "$path" in
		*.example.yaml|*.example.yml|*.sample.yaml|*.sample.yml|*.example.pem|.env.example)
			continue
			;;
	esac
	if printf '%s\n' "$path" | grep -Eq "$forbidden_path"; then
		printf '拒绝：运行配置或私密文件已被 Git 跟踪：%s\n' "$path" >&2
		exit 23
	fi
done; then
	failed=1
fi

key_report=$(mktemp)
credential_report=$(mktemp)
trap 'rm -f "$key_report" "$credential_report"' EXIT HUP INT TERM

if git grep --cached -n -E '^-----BEGIN ([A-Z0-9 ]+ )?PRIVATE KEY-----' -- . ':!scripts/check-secrets.sh' >"$key_report" 2>/dev/null; then
	printf '拒绝：检测到被 Git 跟踪的私钥：\n' >&2
	cat "$key_report" >&2
	failed=1
fi

if ! git ls-files '*.yaml' '*.yml' '*.json' '*.toml' '*.conf' '*.ini' '*.env' | while IFS= read -r path; do
	[ -n "$path" ] || continue
	if git show ":$path" | grep -Ein '^[[:space:]]*(password|passwd|token|api[_-]?key|secret|pppoe-(user|password))[[:space:]]*[:=][[:space:]]*[^<{$*[:space:]#][^[:space:]#]*' | grep -Eiv '(example|placeholder|replace-me|changeme|redacted|dummy|test-only|secret://)' >"$credential_report"; then
		printf '拒绝：%s 中疑似包含真实凭据：\n' "$path" >&2
		cat "$credential_report" >&2
		exit 24
	fi
done; then
	failed=1
fi

if command -v gitleaks >/dev/null 2>&1; then
	if ! gitleaks git --redact --no-banner; then
		printf '拒绝：gitleaks 检测到疑似泄露。\n' >&2
		failed=1
	fi
else
	printf '提示：未安装 gitleaks，已完成内置基础扫描。\n'
fi

if [ "$failed" -ne 0 ]; then
	exit 1
fi
printf '敏感信息检查通过。\n'
