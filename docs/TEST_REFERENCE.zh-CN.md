# mihomo-tui 功能与测试参考

本文档描述 Loongnix fork **当前已经实现的行为**，用于功能验收、回归测试和缺陷记录，不代表后续规划。测试结论应以目标机器上的实际版本为准。

## 1. 测试基线

| 项目 | 当前基线 |
|---|---|
| mihomo-tui | `v0.2.1-loongnix.4` |
| 目标系统 | Loongnix GNU/Linux 25.1，LoongArch ABI2，Go 构建目标 `linux/loong64` |
| 管理服务 | `mihomo-tui.service`，root daemon，systemd 开机启动 |
| 旧服务 | `mihomo.service` 已停用并禁用，只作为人工回退保留 |
| 当前监听方式 | mihomo mixed port `7890`，external controller `9090` |
| 当前网络开关 | mihomo 自动启动开启；TUN 关闭；当前用户系统代理偏好关闭 |
| 已验证代理链路 | mixed port 的 HTTP 与 SOCKS5 请求均成功返回 HTTP 204 |
| 已验证恢复行为 | 重启 `mihomo-tui.service` 后 daemon 会按 `auto_start` 恢复 mihomo |

状态标记：

- **已验证**：已在当前 Loongnix 服务器上实际执行过。
- **待验收**：代码已实现，但当前服务器尚未完成完整人工测试。
- **高风险**：会中断代理、修改路由/防火墙或删除数据，测试前必须准备回退方式。
- **已知限制**：当前实现与理想行为存在明确差距，不应误判为测试环境故障。

测试记录中不得粘贴真实订阅 URL、节点凭据、API secret、服务器密码或完整私密配置。订阅名称、节点名称和日志截图也应先检查是否含个人信息。

## 2. 组件与数据流

```text
普通用户 TUI
    │ Unix socket + SO_PEERCRED 身份校验
    ▼
/var/run/mihomo-tui/daemon.sock
    │
    ▼
root mihomo-tui daemon
    ├── 管理 mihomo 子进程
    ├── 生成运行配置并调用 mihomo REST API
    ├── 主动下载、校验和缓存订阅/规则/资源
    └── 将公开配置与私密数据分开落盘
```

默认分体模式下，systemd 只启动管理 daemon；mihomo 是 daemon 管理的子进程。`System.AutoStart=true` 时 daemon 启动后会恢复 mihomo。mihomo 启动失败不会让 IPC daemon 一起退出，以便仍能进入 TUI 排错。

`--standalone` 是一体模式：没有可用 daemon 时由当前 TUI 进程启动嵌入式 daemon；退出 TUI 时会停止本进程启动的嵌入式 daemon。服务器常驻运行应使用 systemd 分体模式。

## 3. 权限模型

root daemon 通过 Unix socket 的调用方 UID/GID 判断权限。安装服务会创建以下两组：

| 身份 | 能力 |
|---|---|
| 未授权用户 | 不能访问 IPC |
| `mihomo-tui` 组 | 只读查看 daemon、mihomo 状态、版本和下载进度；不能读取订阅、配置或 API 凭据 |
| `mihomo-tui-operator` 组 | 具备上面的能力，并可管理订阅、订阅池、规则订阅和外部资源；可通过 TUI 使用 mihomo API 查看节点、连接和日志 |
| root / daemon 所有者 | 完整管理；可改全局配置、启停内核、管理内核版本、关闭 daemon |

当前 operator 用户的关键边界：

- 可以导入、刷新、应用、编辑和删除订阅，也可以管理订阅池。
- 可以导入、刷新和删除规则订阅；可以管理 GeoIP/GeoSite 外部资源。
- 可以用鼠标选择代理节点、测速、查看连接/日志和关闭连接。
- 不能通过全局配置接口修改 TUN、代理模式、端口、自动启动、内置规则、自定义规则等配置。
- 不能启停 mihomo、切换内核版本或关闭 daemon；相应按钮当前仍可能显示，操作后应收到“需要 root 权限”。
- “系统代理”是当前 TUI 用户的本地偏好，不属于 daemon 全局配置，因此普通用户可切换。

加入 operator 组后需要重新登录，或使用 `newgrp mihomo-tui` 建立包含新组的会话。测试完整管理功能时可运行 `sudo mihomo-tui`，但不要在 root TUI 中测试“系统代理”，否则修改的是 root 的 shell 环境，而不是日常用户环境。

## 4. 全局界面与按键

左侧共有 9 个页面：

1. 首页
2. 代理
3. 订阅
4. 订阅池
5. 连接
6. 规则
7. 日志
8. 资源管理
9. 设置

当前实际按键行为：

| 操作 | 当前行为 |
|---|---|
| `↑` / `↓` + `Enter` | 在左侧菜单移动并打开页面 |
| `Tab` | 从左侧菜单进入右侧页面；表单中切换控件 |
| `Esc` | 从当前控件返回左侧菜单；部分弹窗内表示取消 |
| 鼠标 | 支持点击菜单、页签、按钮和节点卡片 |
| 粘贴 | TUI 已启用 paste；订阅正文应粘贴到专用弹窗 |
| `Ctrl-C` | 当前可靠的 TUI 退出方式 |

**已知限制：** 当前没有全局 `q` 退出处理；`Esc` 只返回导航栏，不会退出。README 中的“`Esc` / `q` 返回/退出”与当前代码不一致。`PgUp` / `PgDn` 也不是全局分页快捷键，只可能由具体控件按其默认行为处理。

## 5. 页面功能明细

### 5.1 首页

首页由 6 个卡片组成：

| 卡片 | 实现细节 | 权限/状态 |
|---|---|---|
| 当前运行订阅池 | 显示订阅池名称、主备/合并模式、活动源、缓存、失败和额度信息；URL 会脱敏 | operator 可查看 |
| 当前节点 | 按 rule/global/direct 模式显示当前节点、类型、策略组和延迟；Fallback/LoadBalance 显示自动切换状态 | operator 可查看 |
| 网络设置 | 在“系统代理”和“TUN”之间切换；系统代理写当前用户环境，TUN 写 daemon 配置 | 系统代理普通用户可用；TUN 需 root |
| 代理模式 | rule、global、direct 三种模式 | 修改需 root；当前只支持鼠标点击 |
| 流量统计 | 读取 mihomo `/traffic` 实时流并显示上传、下载和累计值 | 待核对单位准确性 |
| 内核 | 显示运行状态、PID、运行模式、版本和 GeoIP/GeoSite 状态；可启停内核和下载资源 | 启停需 root；外部资源 operator 可管理 |

内核状态首次在约 1 秒后读取，之后每 3 秒轮询。节点与订阅池信息按默认刷新周期更新。

**待核查：** `/traffic` 返回值与界面换算目前需要对照 mihomo API 再验一次，测试时不要仅凭界面单位做流量计费或性能结论。

### 5.2 代理

已实现：

- 从 mihomo 获取全部代理组和节点，兼容 mihomo `v1.19.28` 起 provider 节点与 `/proxies` 分离的 API。
- 代理组下拉选择；节点卡片显示名称、类型、延迟和选中样式。
- 终端宽高变化时自动计算每行卡片数与分页数量。
- 单节点测速和整组批量测速，超时为 5 秒；再次点击批量测速按钮可中断界面等待。
- 测速完成后按有效延迟升序排列，无效/超时结果排在后面。
- 每 5 秒重新读取代理组。
- 鼠标点击节点信息区会调用 mihomo API 切换该组的节点。

**已知限制：**

- 键盘方向键只移动高亮，`Enter` 当前不会调用节点切换；切换节点应使用鼠标。
- 页面首次打开时默认高亮第一个节点，没有根据代理组的 `now` 字段定位真实当前节点。真实节点应以首页“当前节点”或 mihomo API 为准。
- 节点切换失败只写应用日志，没有页面弹窗。
- 当前没有“自动选择最低延迟节点”的独立动作；批量测速只排序，不自动切换。

### 5.3 订阅

支持的来源：

- HTTPS/HTTP 订阅 URL。
- 本地文件内容。
- TUI 粘贴的 Clash YAML、Base64 或 URI 列表。
- CLI 标准输入。
- “新建”创建的手动空订阅项。

每个订阅卡片显示名称、脱敏来源、更新时间、额度/到期时间、缓存状态、连续失败次数和最近错误。卡片动作如下：

| 动作 | 行为 |
|---|---|
| 应用 `✓` | 使用本地缓存生成 mihomo 配置并应用 |
| 编辑 `✎` | URL 来源可改名称、URL、拉取网络和 User-Agent；文件/粘贴来源只允许改名称 |
| 刷新 `↻` | daemon 主动下载、校验并原子更新缓存；活动源刷新后重新应用 |
| 删除 `✕` | 删除订阅元数据，并同步修正订阅池成员关系 |

远程订阅的拉取网络支持：直连、本地 mihomo 代理、系统代理。修改 URL 后不会自动下载，旧缓存继续保留，必须手动刷新。

daemon 会保留最后一次成功缓存。远端刷新失败时记录失败时间、错误和连续失败次数，但不应以失败内容覆盖可用缓存。订阅元数据优先解析 `subscription-userinfo` 等响应头，也会尝试从 URI 备注解析额度和到期信息。

页面键盘行为：上下键移动卡片；`Enter` 刷新当前高亮订阅；`Tab` 进入顶部输入框。分页数量随终端高度变化。

### 5.4 订阅池

订阅池把一个或多个订阅来源组合成运行输入。一个订阅同一时间只属于一个订阅池；把成员加入新池会从其他池移出，旧池为空时会自动禁用并标记降级。

支持两种模式：

| 模式 | 行为 |
|---|---|
| 优先级主备（failover） | 成员顺序即优先级，只有活动源进入运行配置；活动源失败时可切换到有可用缓存的备用源 |
| 合并（merge） | 所有具有有效缓存的成员同时进入生成配置；单成员刷新失败不执行主备切换 |

编辑器支持：名称、启用状态、刷新间隔、运行模式、添加/移除成员、上移/下移、手动设为活动源。启用的池至少需要一个成员，刷新间隔必须是正整数秒。

卡片显示：运行状态、模式、活动源、可用缓存数量、成员数、刷新间隔、最近切换时间和原因。可编辑、刷新整个池或删除。

**边界说明：** 订阅池不是“多个完整 Clash 配置”。当前还不能保存并切换多套互相独立的完整 profile（每套独立端口、规则、DNS、TUN 等）；目前管理的是多个订阅来源及其主备/合并关系。

### 5.5 连接

已实现：

- 每 2 秒读取一次当前连接。
- 显示目标主机/端口、累计下载、累计上传、估算上下行速度和代理链路。
- 统计当前活跃连接数量及累计流量。
- 按主机字符串过滤，当前为区分大小写的包含匹配。
- “关闭全部”调用 mihomo 删除全部当前连接。

**已知限制：**

- 当前 API 解析只返回活跃连接，所有记录都标记为 `Active=true`，因此“已关闭”页签不会显示历史连接。
- 表格没有单连接详情弹窗，也没有单连接关闭按钮；当前只能关闭全部。
- 关闭失败没有页面提示。

### 5.6 规则

规则页有 4 个子页签：

1. **规则列表**：读取当前 mihomo 生效规则，显示规则内容、类型和策略；支持对三列进行不区分大小写的过滤和动态分页。
2. **规则订阅**：从 URL 导入 classical/domain/ipcidr 规则集，选择 Auto/DIRECT/REJECT 策略；支持刷新、删除和修改策略。
3. **自定义规则**：添加前置或后置规则，支持上移、下移和删除；禁止 `MATCH`，未带策略时自动追加所选策略。
4. **内置规则**：启用/禁用、编辑、排序、单项恢复或恢复全部默认；最后的 MATCH 规则不能禁用或移动。

权限细节：operator 可导入、刷新和删除规则订阅；“修改规则订阅策略”、自定义规则和内置规则通过全局配置保存，因此需 root。

### 5.7 日志

已实现：

- 通过 mihomo `/logs` SSE 流实时读取日志。
- 支持全部、DEBUG、INFO、WARNING、ERROR 级别过滤。
- UI 每 200 ms 批量刷新，单批最多按 100 条组装。
- 内存最多保留 2000 条，超过后从最旧记录开始丢弃。
- “清空”只清空当前 TUI 内存和视图，不删除 daemon/mihomo 的磁盘日志。

**已知限制：** 当前没有暂停/继续按钮；README 中“支持过滤与暂停”的“暂停”尚未实现。日志流断开后也没有自动重连逻辑。

### 5.8 资源管理

资源管理分为两个页签。

“内核管理”支持：

- 读取和刷新 mihomo Release 版本列表。
- 显示发布日期、预发布、下载状态、当前使用版本和手动导入标记。
- 下载版本并显示进度，下载后手动切换。
- 切换已下载版本；删除未启用版本，删除前二次确认。
- 更新到最新稳定版。
- 扫描手动放置的、已解压的 mihomo 可执行文件；拒绝符号链接并校验实际版本。

以上内核写操作只允许 root。下载失败时会展示手工下载地址和放置路径。

“外部资源”支持分别管理 GeoIP 和 GeoSite：

- 查看缺失、无效或可用状态、大小、路径、更新时间和最近问题。
- 编辑并保存下载 URL，保存 URL 不会立即下载。
- 强制更新、扫描手动放置文件、恢复默认 URL。
- 拒绝空文件、符号链接和可被组/其他用户写入的不安全文件。

operator 可管理外部资源。资源下载为网络操作，测试时应允许失败并验证手动导入提示，不应把上游暂时不可达误判为 TUI 逻辑失败。

### 5.9 设置

设置修改均为自动保存，没有统一“保存”按钮。输入框在失焦时提交；复选框和下拉框在值变化时提交。保存失败后控件应回滚到当前生效值。

“系统设置”：

- 开机启动：控制 daemon 启动时是否自动恢复 mihomo。
- 系统代理：当前用户本地偏好，写 shell 环境和客户端偏好文件。
- 虚拟网卡模式：控制 TUN。
- 语言：简体中文/English；当前只保存配置，界面文本尚未完整国际化。
- 应用日志级别：DEBUG/INFO/WARN/ERROR。
- 日志目录：可修改。
- 工作目录：只读显示 daemon 返回的配置目录。

“mihomo 设置”：

- HTTP、SOCKS5、mixed、redir、TProxy 端口。
- 允许局域网、IPv6、统一延迟、自动透明代理。
- mihomo 日志级别。
- 默认代理策略。
- 延迟测试 URL。

除了当前用户“系统代理”外，上述全局设置都需要 root。配置已成功落盘但运行时应用失败时，当前界面可能只写日志而不弹窗；测试应同时检查服务日志和实际端口。

“关于”显示应用版本、Go 版本和仓库地址。“关闭后台服务并退出”需要 root；operator 点击时 TUI 仍会退出，但 daemon 会因权限拒绝而继续运行。

## 6. 命令行功能

| 命令 | 功能 | 权限/风险 |
|---|---|---|
| `mihomo-tui` | 连接 daemon 并启动 TUI | operator 推荐 |
| `mihomo-tui --standalone` | 无 daemon 时启动嵌入式服务 | 仅单用户/调试 |
| `mihomo-tui server [-d DIR]` | 前台运行 IPC daemon | systemd 下由 root 使用 |
| `mihomo-tui subscription import --url URL` | 导入远程订阅 | operator；URL 视为敏感信息 |
| `... --file PATH` | 读取文件正文并导入 | operator |
| `... --stdin` | 从标准输入导入 | operator；优先于把正文写临时文件 |
| `... --name NAME` | 指定订阅显示名称 | 可选 |
| `... --via-local-proxy` | 后续刷新通过本地 mihomo HTTP/mixed 端口 | 仅 URL 来源 |
| `sudo mihomo-tui install_service` | 安装二进制和 systemd 服务 | 高风险，覆盖安装 |
| `sudo mihomo-tui uninstall` | 卸载 systemd 服务 | 高风险 |
| `sudo mihomo-tui grant_operator USER` | 将用户加入 IPC 授权组 | 需重新登录生效 |
| `sudo mihomo-tui cleanup` | 清理项目系统代理和 TUN 环境 | 高风险 |
| `mihomo-tui tun_diagnose` | 输出 TUN dry-run 计划 | 只读 |
| `mihomo-tui tun_debug` | 输出 TUN 预检、冲突和拟执行命令 | 只读 |
| `sudo mihomo-tui tun_debug --apply` | 清理并重建本项目 TUN 路由/防火墙 | 极高风险 |
| `mihomo-tui version` | 输出版本 | 只读 |

Loongnix 上程序启动时会补齐 `/usr/local/sbin`、`/usr/sbin` 和 `/sbin`，确保普通登录环境也能找到 `ip`、`nft`、`iptables` 等管理工具。

## 7. TUN 安全边界

TUN 会修改主机路由和防火墙，必须从服务器本地控制台或另一个不依赖该 TUN 的管理通道测试。仅有一条 SSH 会话时不要直接启用。

当前保护措施：

- 启动内核前安装 IPv4 回包保护，降低 Docker、SSH、VPN 和 Tailscale 回包被 TUN 重定向的风险。
- 使用项目专用策略路由表、规则优先级、fwmark 和 nftables/iptables 链。
- 清理只针对记录为本项目所有的状态，不应清空全机 nftables 规则。
- 检测其他活跃 TUN 默认路由；发现 Clash Verge、其他 mihomo 或 sing-box 冲突时拒绝并发启用。
- `tun_diagnose` 和不带 `--apply` 的 `tun_debug` 只读。

**已知限制：** 回包保护当前只覆盖 IPv4。检测到 IPv6 默认路由时会告警，但不能保证 IPv6 管理连接安全。当前服务器基线保持 TUN 关闭，尚未完成生产环境破坏性验收。

## 8. 敏感信息与仓库边界

运行时目录不属于 Git 仓库：

| 路径 | 内容 | 期望权限 |
|---|---|---|
| `/var/lib/mihomo-tui/` | root daemon 工作目录 | `0700` |
| `/var/lib/mihomo-tui/config.yaml` | 可分享主配置；订阅/规则 URL 仅保存 `secret://...` 引用，API secret 为空 | `0600` |
| `/var/lib/mihomo-tui/secrets/` | 私密目录 | `0700` |
| `/var/lib/mihomo-tui/secrets/runtime.yaml` | API secret、真实订阅 URL、真实规则订阅 URL | `0600` |
| `/var/lib/mihomo-tui/subscriptions/` | 已校验的订阅正文缓存 | `0700`，内部文件 `0600` |
| `~/.config/mihomo-tui/` | 当前 TUI 用户的本地偏好和日志 | 不得提交 |

仓库的 `.gitignore` 已覆盖 `secrets/`、`subscriptions/`、`proxy_providers/`、`.env*`、`config.yaml`、`runtime.yaml`、订阅文件、私有/本地 YAML、日志、socket、PID 和构建产物。

提交前执行：

```bash
make security-check
go test ./...
git diff --cached --check
```

`make security-check` 会检查被跟踪的敏感路径、私钥和常见明文凭据；安装 `gitleaks` 后会追加全仓历史扫描。没有安装 gitleaks 时，内置扫描仍会运行，但测试报告必须注明“gitleaks 未执行”。仓库已提供 pre-commit hook 安装脚本和 CI 扫描。

不要用 `cat`、`sed` 或调试日志把 `runtime.yaml` 正文输出到测试报告。权限测试只使用 `stat`，内容隔离优先由自动化测试验证。

## 9. 推荐测试分级

| 级别 | 范围 | 示例 |
|---|---|---|
| L0 只读 | 不改变配置和网络 | 版本、服务状态、页面浏览、TUN dry-run、安全扫描 |
| L1 可回滚数据 | 修改 TUI 管理数据，不中断服务 | 导入测试订阅、规则、创建测试池、日志过滤 |
| L2 服务变更 | 可能短暂中断代理 | 应用订阅、改端口、启停内核、切换内核版本 |
| L3 网络变更 | 可能中断 SSH/Tailscale/Docker | TUN 开关、`tun_debug --apply`、cleanup |

L2/L3 测试前至少准备：本地控制台、第二条 SSH 会话、当前配置备份、`ip rule`/`ip route`/`nft list ruleset` 快照，以及明确的回退命令。备份文件必须保存在仓库外并设为 `0600` 或目录 `0700`。

## 10. 核心测试用例

### 10.1 环境与服务

| ID | 级别 | 步骤 | 预期结果 |
|---|---|---|---|
| ENV-001 | L0 | 执行 `mihomo-tui version`、`uname -m` | 版本为基线版本；系统为 LoongArch |
| ENV-002 | L0 | 检查 `systemctl is-active/is-enabled mihomo-tui.service` | 均为成功；旧 `mihomo.service` 不活跃且未启用 |
| ENV-003 | L0 | operator 启动 TUI | 能连接 IPC、读取配置并显示 9 个页面；无权限用户被明确拒绝 |
| ENV-004 | L2 | 保持 `auto_start=true`，重启 `mihomo-tui.service` | daemon 恢复 mihomo；7890/9090 重新监听；首次 provider 就绪允许约 3–10 秒 |
| ENV-005 | L0 | 通过 HTTP 和 SOCKS5 分别请求 204 测试地址 | 两种方式都成功；不在报告中记录节点信息 |

### 10.2 导航与显示

| ID | 级别 | 步骤 | 预期结果 |
|---|---|---|---|
| UI-001 | L0 | 用方向键、Enter、Tab、Esc 遍历全部页面 | 焦点可进入页面并返回导航；无崩溃或卡死 |
| UI-002 | L0 | 缩放终端到窄、宽、矮三种尺寸 | 代理、订阅、规则卡片重新分页；控件不互相覆盖 |
| UI-003 | L0 | 按 `q`、Esc、Ctrl-C | `q` 不退出；Esc 返回导航；Ctrl-C 退出，符合当前已知行为 |
| HOME-001 | L0 | 查看首页 6 个卡片 | 状态、PID、版本、活动池、真实当前节点与开关状态一致 |

### 10.3 订阅与订阅池

测试应使用专用测试订阅，不得把真实 URL 写入脚本、Issue 或 Git。测试结束后删除测试项。

| ID | 级别 | 步骤 | 预期结果 |
|---|---|---|---|
| SUB-001 | L1 | 导入一个有效 HTTPS URL | 生成稳定 ID 和私有缓存；卡片 URL 脱敏；额度可解析则显示 |
| SUB-002 | L1 | 分别从文件、粘贴和 stdin 导入有效内容 | 均成功缓存；文件/粘贴项编辑器只允许改名称 |
| SUB-003 | L1 | 导入无效 URL 格式、垃圾正文和空正文 | 明确拒绝，不创建可用缓存，不覆盖已有配置 |
| SUB-004 | L1 | 修改远程 URL、User-Agent、拉取网络 | 保存后旧缓存仍在；手动刷新后才替换内容 |
| SUB-005 | L1 | 先成功刷新，再令远端失败并刷新 | 记录错误和失败次数；保留最后成功缓存；错误文本不泄露 URL 查询参数 |
| SUB-006 | L2 | 应用一个有效订阅 | 生成配置，mihomo reload/restart 成功；代理仍能请求 204 |
| SUB-007 | L1 | 删除池内订阅 | 订阅池成员同步更新；活动源不存在时自动选择剩余成员或禁用空池 |
| POOL-001 | L1 | 创建含两个缓存成员的 failover 池，调整顺序和活动源 | 顺序、活动源、间隔持久化；只有活动源进入运行配置 |
| POOL-002 | L1/L2 | 令 failover 活动源刷新失败 | 切换到有缓存的备用源；记录切换时间、原因和健康状态 |
| POOL-003 | L1/L2 | 创建 merge 池并应用 | 所有有效缓存成员同时进入生成配置；缺缓存成员不破坏其余成员 |
| POOL-004 | L1 | 将已有成员加入另一个池 | 成员从原池移出，不出现重复归属；原池为空则自动禁用 |

### 10.4 代理、连接和日志

| ID | 级别 | 步骤 | 预期结果 |
|---|---|---|---|
| PROXY-001 | L0 | 进入代理页并切换代理组 | 组和节点在 5 秒内显示；provider 节点不为空 |
| PROXY-002 | L1 | 执行单节点和整组测速 | 显示测试中/延迟/超时状态，结果按延迟排序；中断按钮可停止等待 |
| PROXY-003 | L1 | 用鼠标选择节点，再到首页确认 | mihomo 的真实当前节点改变，首页显示一致 |
| PROXY-004 | L0 | 用键盘移动高亮并按 Enter | 当前不会切换节点；按已知限制记录，不作为环境故障 |
| CONN-001 | L0 | 产生 HTTP/SOCKS 流量，查看连接页 | 2 秒内出现目标、流量、速度和链路；过滤可用 |
| CONN-002 | L1 | 点击关闭全部 | 当前连接被清空或重新建立；失败时查应用日志 |
| CONN-003 | L0 | 打开“已关闭”页签 | 当前应为空，记录为尚未实现历史连接 |
| LOG-001 | L0 | 产生代理流量，切换日志级别 | 实时日志到达；级别过滤正确；URL/凭据不应明文出现 |
| LOG-002 | L0 | 点击清空后继续产生流量 | 仅当前视图被清空，新日志继续出现；磁盘日志不被删除 |

### 10.5 规则与设置

| ID | 级别 | 步骤 | 预期结果 |
|---|---|---|---|
| RULE-001 | L0 | 查看并过滤当前生效规则 | 内容、类型、策略和总数正确；大小写不影响过滤 |
| RULE-002 | L1 | operator 导入、刷新和删除测试规则订阅 | 缓存和状态正常；失败 URL 被脱敏 |
| RULE-003 | L1/L2 | root 修改规则订阅策略 | Auto/DIRECT/REJECT 保存并进入生成配置 |
| RULE-004 | L1/L2 | root 添加前置/后置自定义规则并排序、删除 | 顺序持久化；MATCH 和无逗号格式被拒绝 |
| RULE-005 | L1/L2 | root 禁用、编辑、排序和恢复内置规则 | 操作持久化；最后 MATCH 不能禁用或移动 |
| SET-001 | L1 | root 修改一个无风险字段，失焦后重新进入页面 | 自动保存；重新读取值一致 |
| SET-002 | L1 | 输入非法端口或空测试 URL | 保存失败并回滚显示；daemon 配置不改变 |
| SET-003 | L2 | 修改 mixed port 后检查监听和代理 | 新端口实际监听、旧端口释放；HTTP/SOCKS 请求成功；再恢复基线 |
| PERM-001 | L0 | operator 尝试启停内核或改全局设置 | IPC 返回需要 root；配置和进程不变 |
| PERM-002 | L0 | 只读组用户尝试读取配置或订阅 | 被拒绝；仍可查看不含敏感信息的 daemon/mihomo 状态 |

### 10.6 资源、安全与 TUN

| ID | 级别 | 步骤 | 预期结果 |
|---|---|---|---|
| RES-001 | L1 | 保存一个测试外部资源 URL，再恢复默认 | URL 单项更新，不影响另一资源；恢复默认不立即下载 |
| RES-002 | L1 | 扫描空文件、符号链接和安全的手动文件 | 前两者被拒绝；合规非空普通文件被识别并收紧权限 |
| RES-003 | L2 | root 下载一个非活动内核、切换后再切回 | 进度可见；版本校验正确；切换后 core 可启动并代理 204 |
| SEC-001 | L0 | 执行 `stat` 检查运行目录，不读取正文 | 目录/文件权限符合第 8 节 |
| SEC-002 | L0 | 执行 `make security-check`、`go test ./...` | 全部通过；报告注明 gitleaks 是否安装 |
| SEC-003 | L0 | 检查 Git 跟踪文件和暂存区 | 没有 config、runtime、订阅正文、私钥或真实凭据 |
| SEC-004 | L1 | 导入带查询 token 的测试 URL后检查公开配置和 UI/错误日志 | 公开配置只留 `secret://` 引用；界面和错误不显示 token |
| TUN-001 | L0 | 运行 `tun_diagnose` 和 `tun_debug` | 只输出出口、冲突、IPv6 风险和拟执行命令；路由/防火墙哈希不变 |
| TUN-002 | L3 | 具备本地控制台时 root 开启 TUN | 无冲突才启用；代理、SSH、Tailscale、Docker 回包均正常；项目规则可识别 |
| TUN-003 | L3 | 关闭 TUN 或执行项目 cleanup | 只清理本项目规则；原有系统/VPN/Docker 规则仍在；管理连接保持可用 |

## 11. 基线检查命令

以下命令不会读取私密文件正文：

```bash
mihomo-tui version
uname -m
systemctl is-active mihomo-tui.service
systemctl is-enabled mihomo-tui.service
systemctl is-active mihomo.service || true
systemctl is-enabled mihomo.service || true
ss -lntp | grep -E ':(7890|9090)\b'
curl --max-time 15 -x http://127.0.0.1:7890 -o /dev/null -sS -w 'HTTP %{http_code}\n' http://cp.cloudflare.com/generate_204
curl --max-time 15 --socks5-hostname 127.0.0.1:7890 -o /dev/null -sS -w 'SOCKS %{http_code}\n' http://cp.cloudflare.com/generate_204
sudo stat -c '%a %U:%G %n' /var/lib/mihomo-tui /var/lib/mihomo-tui/config.yaml /var/lib/mihomo-tui/secrets /var/lib/mihomo-tui/secrets/runtime.yaml /var/lib/mihomo-tui/subscriptions
mihomo-tui tun_debug
make security-check
go test ./...
```

mihomo/provider 第一次启动可能需要约 3–10 秒才完成健康检查。启动后立即请求若得到临时 502，应在 10 秒内重试；持续失败才判为缺陷。

## 12. 当前验收结论与已知缺口

当前可以作为基础代理管理器使用的部分：daemon/systemd、LoongArch ABI2 构建、mihomo 自动恢复、mixed port HTTP/SOCKS 代理、订阅主动缓存、订阅池、鼠标节点选择、实时规则/连接/日志读取、资源管理，以及私密数据独立落盘。

在宣布“完整 TUI 可用”前仍需处理或明确接受以下缺口：

1. 普通 operator 看得到启停、TUN、代理模式和设置控件，但这些全局操作需要 root；界面没有统一禁用或权限提示。
2. 代理页键盘 Enter 不切换节点，初始高亮也不代表 mihomo 的真实当前节点。
3. 连接页没有已关闭历史、单连接详情和单连接关闭。
4. 日志页没有暂停和断线重连。
5. 尚未实现多套完整 profile 的保存与切换；订阅池只解决多个订阅来源的主备/合并。
6. 首页实时流量的单位换算仍需与当前 mihomo API 对照验证。
7. TUN 仅做过代码与 dry-run 级安全验证，生产 Loongnix 主机上的破坏性验收未完成，IPv6 回包保护也不完整。
8. provider 初次启动存在几秒准备期，短暂 502 属允许的过渡状态。

## 13. 测试记录模板

```text
测试 ID：
版本 / commit：
系统 / 架构：
执行用户与 IPC 角色：
前置条件：
实际步骤：
预期结果：
实际结果：
结论：通过 / 失败 / 阻塞 / 已知限制
日志位置：仅写路径或已脱敏摘要，不粘贴私密正文
回退动作：
备注：
```

缺陷报告至少包含测试 ID、版本、执行身份、页面/命令、可重复步骤和脱敏日志。涉及 TUN 的报告还应包含启用前后的 `ip rule`、相关路由表及**仅本项目 nftables/iptables 链**，不要直接公开完整防火墙规则集。
