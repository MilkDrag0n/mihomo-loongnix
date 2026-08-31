# 测试参考入口

精简版 TUI 的完整测试基准已经拆分为两份权威文档：

- [UI 控件树与逐项测试](UI_CONTROL_TREE.zh-CN.md)：每个页面、按钮、鼠标/键盘动作、后端调用、成功来源、失败表现和测试编号。
- [Manager API](MANAGER_API.zh-CN.md)：接口、状态来源、回滚、安全边界、Profile 格式和日志语义。

旧版九页界面、系统代理、订阅池、连接管理、测速、资源管理、规则编辑和内核版本 UI 已删除，不再作为测试对象。

最低验收命令：

```bash
go test ./... -count=1
go test -race ./mihomotui ./mihomotui/ui
./scripts/check-secrets.sh
GOOS=linux GOARCH=loong64 go build -o build/mihomo-tui-linux-loong64 .
```

真机验收必须在 Loongnix 25.1 LoongArch ABI2 上执行控制树中的全部 `LIVE-*` 项目，并保存 systemd、Mihomo API、TUN 网卡与日志文件的实际证据。
