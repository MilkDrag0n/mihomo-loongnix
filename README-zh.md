# mihomo-tui — Loongnix 版

面向 Loongnix 25.1 / LoongArch ABI2 的 Mihomo 本机管理器与终端界面。本项目只提供服务器上最基本、可审计的控制能力：代理内核启停、TUN 状态、配置链接、代理组与节点选择、生效规则和日志。

## 使用方式

```bash
go test ./...
GOOS=linux GOARCH=loong64 CGO_ENABLED=0 go build -trimpath -o build/mihomo-tui .
sudo ./build/mihomo-tui install_service
```

安装程序会创建独立的 `mihomo-manager.service` 和 `mihomo.service`，并建立本机读取/操作权限组。将操作用户加入相应组后重新登录，再运行：

```bash
mihomo-tui
```

Manager 使用 Mihomo loopback 控制端口 `9090`，受管混合代理端口默认为 `7890`。首次安装时 TUN 和磁盘日志均关闭，但不影响日志页查看实时日志。

Loongnix 的影子部署、正式切换和回滚见 [`docs/LOONGNIX.md`](docs/LOONGNIX.md)。

## 页面快捷键

| 按键 | 操作 |
| --- | --- |
| `1`—`5` | 在首页、配置、节点、规则、日志之间切换 |
| `Tab` / `Shift+Tab` | 向前/向后切换焦点 |
| 方向键 / `j` / `k` | 移动列表项；`左` / `右` 移动表格列 |
| `Enter` / `Space` | 执行当前焦点按钮或选择当前项 |
| `PgUp` / `PgDn` | 当前列表或表格向前/向后翻页 |
| `Home` / `End` | 跳到首项/末项 |
| `/` | 聚焦筛选框 |
| `Esc` | 取消输入或关闭当前弹窗 |
| `r` | 刷新当前页面 |
| `q` | 没有输入框或弹窗占用焦点时退出 |

## Mihomo 内核

本项目使用开源的 [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo)（Clash.Meta）作为代理内核。当前 Loongnix 服务器实际运行版本为：

```text
Mihomo Meta v1.19.30 linux loong64
```

Mihomo 内核源码不在本项目中修改或重新分发。请单独安装兼容 ABI2 的 `mihomo` 二进制，默认路径为 `/var/lib/mihomo-tui/bin/mihomo`，也可以使用服务配置的其他路径。

## 系统要求

- **操作系统**：Loongnix 25.1 / LoongArch ABI2。当前部署已在 Linux `6.6.52-loong64`（`loongarch64`）和 glibc `2.41` 上验证。
- **服务管理器**：必须使用 systemd；当前部署已在 systemd `257.7` 上验证。
- **Mihomo 内核**：兼容 ABI2 的 `linux loong64` 二进制；服务器当前运行 Mihomo Meta `v1.19.30`，详见 [Mihomo 内核](#mihomo-内核)。
- **从源码构建**：Go `1.26.1` 或更高版本，以 [`go.mod`](go.mod) 声明为准。使用预编译 TUI 时不需要安装 Go。
- **权限**：仅安装服务和执行系统级 TUN 操作需要 root；日常 TUI 使用普通操作用户运行。

## 架构

```text
普通用户 TUI ── Unix socket ──> root: mihomo-manager.service
                                      ├─ 控制 mihomo.service
                                      ├─ 管理受限运行目录
                                      └─ 代理 Mihomo loopback 控制接口
```

- `mihomo-manager.service` 是唯一的特权管理边界。
- `mihomo.service` 独立运行 Mihomo 内核。
- TUI 以普通用户运行，不调用 `sudo`、`systemctl`，也不直接写系统配置。
- 所有写操作都经过“请求 → pending → Manager 执行 → 状态回读 → 成功/失败”流程，不用前端假状态代替内核状态。

## 功能范围

当前 TUI 保留五个页面：

1. **首页**：显示 Manager、systemd、Mihomo 控制接口和 TUN 的真实状态，提供代理启动/停止及 TUN 操作。
2. **配置**：导入配置链接，验证并暂存；支持激活、更新、重命名、删除，失败时回滚到原配置。
3. **节点**：严格保持 Mihomo `all` 字段原始顺序，显示代理组实际 `now` 节点，支持节点选择和单节点测速。
4. **规则**：只读显示当前生效规则，固定使用 `rule` 模式，不编辑规则。
5. **日志**：查看 Mihomo 实时日志；磁盘记录独立控制，默认关闭，显示文件大小，单文件达到 10 MiB 后轮转并保留 3 个历史分卷。

鼠标点击和键盘操作走同一套 Manager 请求路径。节点选择和测速始终使用后端完整原始节点名；界面显示不会改变配置内容。

## 文档

- [English README](README.md)
- [Manager API](docs/MANAGER_API.zh-CN.md)
- [TUI 控件树与测试编号](docs/UI_CONTROL_TREE.zh-CN.md)
- [测试参考](docs/TEST_REFERENCE.zh-CN.md)
- [Loongnix 部署与回滚](docs/LOONGNIX.md)

## 许可证

本项目采用 [MIT License](LICENSE)。Mihomo 是独立项目，遵循其自身许可证和分发条款。
