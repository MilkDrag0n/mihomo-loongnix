# Loongnix 25 / LoongArch ABI2

本 fork 以 Loongnix GNU/Linux 25、`loongarch64` 用户态和 Go 的 `linux/loong64` 目标为首要运行环境。

## 构建

```bash
./scripts/install-git-hooks.sh
./scripts/build-loongnix.sh
```

生成文件为 `build/linux/mihomo-tui-linux-loong64`。安装服务前先保留现有 Mihomo 配置，并确认 7890–7894、9090 端口没有与旧服务冲突。

## 私密数据边界

运行时主配置位于 `/var/lib/mihomo-tui/config.yaml`，其中订阅来源只保存 `secret://...` 引用。真实的 API secret、订阅链接和规则订阅链接位于：

```text
/var/lib/mihomo-tui/secrets/runtime.yaml
```

目录权限为 `0700`，文件权限为 `0600`。订阅正文继续保存在 `/var/lib/mihomo-tui/subscriptions/`，同样不属于 Git 仓库。

不要将 `/var/lib/mihomo-tui`、`~/.config/mihomo-tui`、旧的 `config.yaml` 或 provider 缓存复制进仓库。提交前运行 `make security-check`。
