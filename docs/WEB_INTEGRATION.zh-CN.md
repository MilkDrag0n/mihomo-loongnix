# 可选网页与服务器首页接入

网页源码和独立 Go 网关已实现；正式安装和 HTTPS 入口需要按下文配置。Web 默认关闭，TUI／管理器／代理不依赖网页进程，关闭网页不会停止代理。Homepage 只放跳转卡片和只读摘要；HoloBot 与本项目互不依赖。

```mermaid
flowchart LR
  TUI[TUI 首页 / CLI] -->|本机 IPC；含 Web 开关| Manager[管理器]
  Browser[浏览器五页界面] -->|同源 HTTPS / Cookie| Web[独立 Go 网关]
  Homepage[Homepage 服务端] -->|只读令牌 / summary| Web
  Web -->|本机 Unix socket| Manager
  Manager --> Core[Mihomo 内核]
  Manager -->|仅启停固定 unit| Web
```

业务逻辑仍在 manager：网页不读取正式订阅文件，不直接使用核心 secret，不连接 9090。网关普通账号属于 mihomo-tui 和 mihomo-tui-operator，因此可执行本机代理管理；HTTP 层支持密码认证或外部认证，并保留接口白名单。此账号自身是受信任操作员，不能把网页路由白名单当作被攻陷进程的系统权限隔离。

## 目录与构建环境

| 用途 | 路径 |
| --- | --- |
| 网页源码 | web/，Vue 3、TypeScript、Vite、Tailwind CSS、daisyUI |
| 独立网关 | cmd/mihomo-web/、internal/webgateway/ |
| 可选配置类型 | internal/webconfig/；无前端依赖 |
| Web 正式构建 | ~/.local/share/mihomo-loongnix/builds/web/<完整提交>/ |
| 日常检查 | ~/.local/share/mihomo-loongnix/checks/web/ |
| 测试私有状态 | ~/.local/state/mihomo-loongnix-test/web/ |
| Web 安装 | /opt/mihomo-web/releases/<完整提交>/，current 指向当前版本 |
| 私有配置 | /etc/mihomo-web/config.json，root:mihomo-web，0640 |
| 独立状态目录 | /var/lib/mihomo-web，mihomo-web，0700 |

构建需要 Go 1.26.1、Node >=22.12、pnpm 10.34.5、Python 3.8+。server-pc 已验证原生 Node 24.20.0（Node unofficial-builds 的 linux-loong64），工具链在用户的 ~/.local/share/mihomo-loongnix/toolchains/，不修改系统 Node。安装依赖用 frozen lockfile；esbuild 使用已下载的原生可选包，安装脚本明确禁用，构建已实际验证。

```bash
./scripts/test-web.sh
./scripts/build-web.sh
```

正式构建要求干净提交，记录完整提交、Go 元信息、Node/pnpm、依赖锁摘要，归档独立程序、static/、unit 和校验清单。同一提交不能覆盖归档。TUI 构建不要求 Node、静态资源或网页运行；同一 Go module 的纯 Go 测试仍会覆盖网关。

## 完全隔离的预览与测试

```bash
. ./scripts/web-toolchain.sh
MIHOMO_WEB_DIST="$HOME/.local/share/mihomo-loongnix/checks/web/static" pnpm --dir web build
python3 scripts/preview-web.py
```

预览在 127.0.0.1:19080 使用假管理器和测试密码 `preview-only-mihomo`，终端会打印地址；Ctrl+C 只清理本次测试进程和状态。它不连接生产 socket，不操作 systemd，不包含真实订阅。远程电脑可用 SSH 转发同名本地端口，再访问本地地址：

```bash
ssh -N -L 19080:127.0.0.1:19080 server-pc
```

开发热更新另用 15173，Vite 将 API 转发至测试网关。生产 Cookie 的 HTTPS 场景由隔离 HTTPS 入口验收；HTTP 测试通过 test_mode 显式启用，且必须使用独立 socket。管理器影子模式或非 root 环境缺少 Web 控制替身时返回不可用，不会默认执行生产 systemctl。

## 正式安装（默认关闭）

先按 [原部署流程](LOONGNIX.md) 安装支持 /v1/web/* 的 manager／TUI。这属于独立的基础后端升级，按原规则备份和验证；Web 专用部署工具不会更新它。

准备实际 HTTPS 域名或私有 HTTPS 入口，将其转发到服务器 127.0.0.1:9080。转发保留公开 Host；日志路径禁用缓冲、允许 SSE 心跳，不使用整体短请求时限。第一版采用独立站点根路径，不支持挂在 /mihomo/ 等子路径。浏览器公开地址与 Homepage 内部 API 地址分开配置。

```bash
commit=$(git rev-parse HEAD)
python3 scripts/deploy-web.py "$commit" --check
# 将域名替换为你已配置的实际 HTTPS 地址。
sudo python3 scripts/deploy-web.py "$commit" --install --public-url https://mihomo.example.invalid
```

默认 password 模式交互设置管理员密码；external 模式不询问、不生成 Web 密码。两种模式都自动生成独立摘要令牌，只写服务器私有配置。安装后 Web 保持关闭、不设自启。先前存在自定义 unit/drop-in、安装残留或符号链接目录时工具会拒绝覆盖，需要先核对。

在 TUI 首页选择“开启 Web”，或：

```bash
mihomo-tui web status
mihomo-tui web start
mihomo-tui web stop
```

这控制网页服务，不是打开电脑上的浏览器。退出 TUI 后 Web 可继续运行；服务器重启后默认不自启。未安装时首页提示“Web 未安装”，不会自动安装；Web 故障不阻止其他五页功能。

## 已有外部访问保护：不设置 Web 密码

已使用 Cloudflare Zero Trust / Access 保护该站点时，首次安装显式选择 external：

```bash
commit=$(git rev-parse HEAD)
sudo python3 scripts/deploy-web.py "$commit" --install \
  --public-url https://mihomo.example.invalid --auth-mode external
```

将示例域名替换为实际 HTTPS 地址。此模式不提示 Web 管理员密码，配置中的 password_hash 为空。通过外部网关后，网页直接进入控制台，不显示本地密码登录与退出按钮；Cloudflare 身份的退出由外部网关处理。现有 password 安装升级时仍保留原配置，不会自动变成 external；--auth-mode 只用于首次安装。

external 明确把访问者认证交给前置网关，本项目不再次校验 Access JWT，不从可伪造的身份请求头认定用户；所有能够到达本机源站的访问者均视为受信任操作员。Web 继续只监听回环地址，保留 HTTPS 公开 Host、同源 Origin、会话 Cookie、CSRF 和只读摘要令牌隔离。应用内会话用于请求校验与日志生命周期，不代表另一层用户登录。

管理器与 Web 必须都支持 auth_mode 配置。首次使用 external 前，从同一提交构建两份程序，并按原流程升级 manager／TUI；旧 manager 不认识该字段，不能只更新 Web 再使用首页开关。

## 复用 Cloudflare Tunnel

现有隧道由 Cloudflare 管理时，在隧道的公开应用路由中新增一条，保留其他应用的路由。主机网络模式下的 cloudflared 可连接本机 Web 监听地址：

| 路由字段 | 本项目设置 |
| --- | --- |
| 公开主机名 | 用户实际指定的专用域名，不带路径 |
| 源站服务 | HTTP，127.0.0.1:9080 |
| HTTP Host Header | 与 public_url 中的公开主机名一致；如有非默认公开端口，也带上端口 |
| 路径 | 留空，使用站点根路径 |
| 禁用分块编码 | 保持关闭，允许流式日志 |

public_url 仍为浏览器访问的 HTTPS 地址；本机源站使用 HTTP，不给 Web 额外配置自签名 HTTPS。网页默认关闭时，入口暂时不可用是预期行为。只发布正式安装后的 9080 服务，不把假数据预览端口 19080 配成公开路由。字段定义见 [Cloudflare 源站参数](https://developers.cloudflare.com/tunnel/advanced/origin-parameters/) 和 [公开应用路由](https://developers.cloudflare.com/tunnel/routing/)。

首次安装后依次验证：本机 Web 健康、公开地址登录、概览读取真实状态、日志保持一分钟、TUI 关闭／开启 Web；关闭网页期间单独确认代理仍正常。若入口已有 Cloudflare Access 策略，保留该策略，实际浏览器登录和 Homepage 服务端访问分别验收。

## 配置字段

JSON 拒绝未知字段；模板不含真实凭据。初始安装器生成全部必需字段，不直接照抄无效示例值：

| 字段 | 含义 |
| --- | --- |
| listen | 必须为明确回环 IP 和端口，默认 127.0.0.1:9080 |
| public_url | 正式 HTTPS 地址，无用户凭据、查询、片段和子路径 |
| manager_socket | 显式绝对路径，生产为 /run/mihomo-tui/daemon.sock |
| auth_mode | password 或 external；省略时兼容旧配置，使用 password |
| password_hash | password 模式需要 PBKDF2-SHA256 哈希；external 必须为空，不保存 Web 密码 |
| summary_token | 至少 32 字符的随机独立令牌 |
| show_node | 是否向摘要返回真实节点名，默认 false |
| test_mode | 仅测试允许回环 HTTP，不能使用生产 socket |

配置通过 `--config` 指定；第一版使用受控 JSON，替代方案阶段拟定的一组环境变量，不解析不必要的代理身份头，也不设置未使用的签名密钥。修改密码后重启 Web 以撤销旧会话；可以通过发布程序的 `--hash-password` 从标准输入生成哈希，避免在命令参数中放密码。更改配置时保留备份并校验 JSON、权限和公开入口。

## 独立升级与回退

```bash
./scripts/build-web.sh
commit=$(git rev-parse HEAD)
python3 scripts/deploy-web.py "$commit" --check
sudo python3 scripts/deploy-web.py "$commit"
# 恢复路径使用本次部署实际输出值。
sudo python3 scripts/deploy-web.py --rollback /home/server/backups/mihomo-loongnix/实际-web-备份目录
```

工具先核验文件清单与程序来源、候选配置和静态资源，再按原 Web 运行状态停止 Web 并备份 unit、私有配置、状态、发布指向；代理双服务完全不参与。发布包整体原子切换 current，旧进程不跨版本读取资源。原来关闭的 Web 升级后仍关闭，原来运行则重新启动并检查本机健康。失败恢复旧配置、发布指向和运行／自启状态，失败数据另存备份。CLI 与管理器启停共享 Web 部署锁，避免同时操作。

备份在集中 backups/mihomo-loongnix/ 下，root 私有。会话在 Web 重启后失效。首次失败可能保留新建的服务账号及未使用发布目录，便于检查，不影响代理；不自动删除历史备份。工具不处理自定义 unit、强制断电、磁盘损坏等所有恢复情况。--check 和模拟测试不代表真实 root 安装、HTTPS 登录及回退已验收；本机健康只验证 Web 进程，代理健康单独看状态。

## Homepage 连接

- href：实际公开地址 + /overview，供浏览器点击。
- widget.url：Homepage 服务端可达的 /api/v1/summary；同机 host 网络可用 http://127.0.0.1:9080/api/v1/summary。
- Authorization：独立 Bearer 摘要令牌，在 Homepage 私有环境中保存，不放 URL、浏览器或 Git。
- widget 每 10 秒刷新；平铺 JSON 字段与 [Web API](WEB_API.zh-CN.md#homepage-摘要) 一致。没有跨项目数据库、源码或运行目录挂载。
- Web 关闭后摘要也关闭，卡片显示网页不可用，不表示代理停止。HoloBot／Homepage 停止不影响此网页。

当前已验证假数据下的登录、页面交互、节点切换／测速、日志、手机布局及 Go/Python 契约测试。首次生产安装、公开入口、真实服务账号、跨项目真实摘要与回退需要部署后验收，不能从假数据预览推定成功。
