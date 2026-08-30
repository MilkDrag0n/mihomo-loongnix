
.PHONY: all windows linux linux-loong64 macos test security-check

all: windows linux macos

windows:
	GOOS=windows go build -o build/windows/mihomo-tui.exe -ldflags="-s -w" -trimpath main.go

linux:
	GOOS=linux go build -o build/linux/mihomo-tui -ldflags="-s -w" -trimpath main.go

linux-loong64:
	CGO_ENABLED=0 GOOS=linux GOARCH=loong64 go build -o build/linux/mihomo-tui-linux-loong64 -ldflags="-s -w" -trimpath .

macos:
	GOOS=darwin go build -o build/macos/mihomo-tui -ldflags="-s -w" -trimpath main.go

test:
	go test ./...

security-check:
	./scripts/check-secrets.sh
