# Loongnix 25 / LoongArch ABI2

本 fork 以 Loongnix GNU/Linux 25、`loongarch64` 用户态和 Go 的 `linux/loong64` 目标为首要运行环境。

程序启动时会在 Linux 上补齐 `/usr/local/sbin`、`/usr/sbin` 和 `/sbin`。Loongnix 普通用户的默认 `PATH` 不包含这些目录，而 `nft`、`ip` 等 TUN 依赖通常安装在 `/usr/sbin`。

## 构建

```bash
./scripts/install-git-hooks.sh
./scripts/build-loongnix.sh
```

生成文件为 `build/linux/mihomo-tui-linux-loong64`。正式服务的 mixed port 默认 `7890`、可从首页修改，控制端口固定为 `9090`；安装前必须保留现有二进制、unit 和私有状态目录快照。

## 影子验证

只能在独立状态目录中显式启用影子模式；默认使用 `17890` 和 `19090`，不会占用生产端口：

```bash
MIHOMO_TUI_SHADOW=1 \
MIHOMO_TUI_SOCKET=/var/lib/mihomo-tui-shadow/run/daemon.sock \
./mihomo-tui server -d /var/lib/mihomo-tui-shadow
```

需要调整时可设置 `MIHOMO_TUI_SHADOW_MIXED_PORT` 与 `MIHOMO_TUI_SHADOW_CONTROLLER_PORT`。这些覆盖在未设置 `MIHOMO_TUI_SHADOW=1` 时会被忽略，生产服务继续使用持久化 mixed port 与固定的 `9090` 控制端口。影子目录只允许在服务器本机复制，不得放入 Git 仓库。

影子 Manager 使用进程控制器，不应启用 TUN。Socket 必须放在影子状态目录内的专用子目录，不能直接放在 `/tmp` 或 `/run` 根目录。使用 `curl --unix-socket /var/lib/mihomo-tui-shadow/run/daemon.sock` 验证 `/v1/status`、Profile、节点、规则和日志接口；完成后停止影子进程，再切换正式 systemd 双服务。

## 正式切换与回滚

切换前将当前 `mihomo-tui`、`mihomo`、旧 unit 和 `/var/lib/mihomo-tui` 完整复制到仅 root 可读的独立备份目录。然后执行：

```bash
sudo ./mihomo-tui-linux-loong64 install_service
systemctl is-active mihomo-manager mihomo
curl --unix-socket /run/mihomo-tui/daemon.sock http://localhost/v1/status
curl --proxy http://127.0.0.1:7890 http://cp.cloudflare.com/generate_204
```

安装器在停止旧一体服务前会查询实际 Mihomo 控制接口；若原内核健康，双服务切换后自动恢复 `mihomo.service`。不能根据旧 manager 是否 active 猜测内核状态。

回滚时先停止新双服务，将失败现场移到另一个 root 私有目录，再从备份恢复原二进制、旧 unit 和完整状态目录，执行 `systemctl daemon-reload` 后启动旧 `mihomo-tui.service`。不要把备份目录放入仓库，也不要在没有确认具体备份路径时执行覆盖或递归删除。

## 私密数据边界

运行时主配置位于 `/var/lib/mihomo-tui/config.yaml`，其中订阅来源只保存 `secret://...` 引用。真实的 API secret、订阅链接和规则订阅链接位于：

```text
/var/lib/mihomo-tui/secrets/runtime.yaml
```

目录权限为 `0700`，文件权限为 `0600`。订阅正文继续保存在 `/var/lib/mihomo-tui/subscriptions/`，同样不属于 Git 仓库。

不要将 `/var/lib/mihomo-tui`、`~/.config/mihomo-tui`、旧的 `config.yaml` 或 provider 缓存复制进仓库。提交前运行 `make security-check`。
