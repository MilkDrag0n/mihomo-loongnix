# Mihomo Web 实施方案

更新：2026-09-06。状态：首版代码与独立测试已完成，正式 Web 尚未安装；验证记录见 docs/WEB_VERIFICATION.zh-CN.md。本文保留原设计作为决策记录，当前接口与命令以 docs/MANAGER_API.zh-CN.md、docs/WEB_API.zh-CN.md、docs/WEB_INTEGRATION.zh-CN.md 为准。

实现收敛：配置采用 root 控制的 JSON，替代拟定环境变量；不解析代理身份头，不使用多余会话签名密钥。摘要第一版不缓存。静态资源、Go 程序和 unit 一起发布。会话改密通过本机维护配置并重启撤销；不增加网页改密入口。生产安装、HTTPS 与真实 Homepage 对接仍需实际部署验收。

## 1. 目标与源码边界

在现有 mihomo-loongnix 仓库的 web/ 目录实现独立网页前端。它与 TUI 共用现有管理器，通过本项目独立的 Web 网关访问 Unix socket。Web 是可选组件，默认关闭，独立安装、构建、测试、启停和升级；不安装或不开启 Web 时，现有 TUI、管理器与代理核心仍能完整工作。TUI 首页提供 Web 开启／关闭操作，由管理器控制独立服务。Homepage 只负责卡片与跳转；本项目不依赖 HoloBot-dashboard、Homepage 服务或它们的源码。

本地仓库：/home/server/projects/mihomo-loongnix
来源：https://github.com/MilkDrag0n/mihomo-loongnix
基础上游：https://github.com/WangZhongDian/mihomo-tui
本次核对提交：e70f636a9848a57e6961880bc66669252f79f7bc
本次正式程序报告：v0.2.1-loongnix.4（程序版本不替代提交/产物校验）。
运行架构：Loongnix 25 / loongarch64。

实施前必须读根 AGENTS.md、docs/MANAGER_API.zh-CN.md、docs/WEB_INTEGRATION.zh-CN.md。后两份文档所说“总面板后端”，在本方案中具体收敛为本项目的独立 Mihomo Web 网关，不是 Homepage。实现时同步更新接入文档，避免保留互相冲突的拓扑。

相关方案：

- /home/server/homepage/PLAN.md
- /home/server/HoloBot-project/HoloBot-dashboard/PLAN.md

Mihomo 唯一常驻源码在服务器本地磁盘，不在 Mac/共享目录再建立第二份常驻仓库。

## 2. 当前已存在的能力

- mihomo-manager.service：本机 HTTP over Unix socket，/run/mihomo-tui/daemon.sock。
- mihomo.service：代理核心；控制接口 127.0.0.1:9090、混合代理端口 127.0.0.1:7890。
- TUI 与管理服务分离；管理器负责配置、核心、TUN、节点、规则和日志。
- 已只读验证 GET /v1/status 成功；当时 core.running=true、controller_healthy=true，存在活动配置与节点，TUN 未生效。
- 只属于 mihomo-tui 组时只读 /v1/status 和 /v1/logging/status；完整操作需要 mihomo-tui-operator。
- /v1 没有异步任务、能力发现、稳定机器错误码或浏览器鉴权。
- 规则当前只读；没有配置正文编辑/导出、任意本地上传、流量/连接管理的 /v1 接口。

以下所有 /api/v1 路由、/v1/web/*、新增状态字段、Web 端口与网页登录都是拟新增，不得写成已实现。既有 /v1 业务路由继续兼容现有 TUI；新增 Web 控制使用独立接口，不强迫普通状态查询等待 Web 服务。

## 3. 框架和参考项目

| 层 | 选择 | 原因与范围 |
| --- | --- | --- |
| 前端 | Vue 3 + TypeScript + Vite | 与 Zashboard 接近，便于复用代理界面组件 |
| 页面与主题 | Vue Router、Tailwind CSS、daisyUI | 中文、浅深主题、移动端卡片 |
| 请求与状态 | 独立 api 客户端 + Vue composables | 统一 Cookie/错误处理，所有业务状态从后台回读 |
| Web 网关 | 现有 Go module 中独立 cmd/mihomo-web | 原生 loong64 程序，使用 net/http Unix socket Transport |
| 静态资源 | Vite 构建后由 Go 提供 | 运行时无需 Node；独立于 manager.service 生命周期 |
| 测试 | Vue 组件测试 + 浏览器关键流程 + Go httptest | 验证接口与异常语义，不复制业务实现 |

主要参考：https://github.com/Zephyruso/zashboard （MIT）。

如何参考：
1. 在实施时读取选定 tag/commit 的 package.json 与 LICENSE，记录完整提交；版本锁文件跟随所选基线，不机械照搬最新依赖。
2. 按需借用节点组卡片、节点列表、延迟颜色、搜索、移动布局和日志界面。将实际复制文件、原路径、原提交写进 web/THIRD_PARTY_NOTICES.md。
3. 拆掉浏览器直连 Clash API、通过 URL 带 secret、多后端地址设置及云字体依赖。
4. 用本方案的 /api/v1 客户端替换上游直连接口。当前 /v1 的数据结构与 Zashboard 的 Clash API 不同，不能只改 baseURL。
5. 订阅/配置页、TUN 的配置与生效状态、运行控制、网页登录和 Homepage 摘要由本项目新增。
6. 不整包嵌入 Zashboard iframe；不再写一套订阅下载、YAML 生成或系统路由逻辑。

其他资料：

- https://wiki.metacubex.one/api/ ：只在确需新增管理器能力时参考核心接口，不作为浏览器直连接口。
- ../docs/MANAGER_API.zh-CN.md ：当前管理器精确契约。
- ../docs/WEB_INTEGRATION.zh-CN.md ：IPC 权限、超时和原始日志流陷阱。
- https://github.com/Zephyruso/zashboard/blob/main/package.json ：框架参考，具体版本须锁定。

## 4. 网页功能和路由

| 路由 | 页面 | 第一版功能 |
| --- | --- | --- |
| /overview | 概览 | 核心实际状态、配置、节点、代理端口；启停核心；TUN 配置/生效分别展示 |
| /profiles | 配置 | 列表、URL 导入、激活、更新、重命名、删除 |
| /proxies | 节点 | 分组、搜索、当前节点、单节点测速、串行队列的组内测速、节点切换 |
| /rules | 规则 | 生效规则列表、前端筛选和分页，只读 |
| /logs | 日志 | 实时日志、级别过滤、暂停显示、清空显示；独立落盘开关与占用 |
| /login | 登录 | 单管理员会话；未登录保留经过校验的站内返回路径 |

根路径跳转 /overview。所有深层路由刷新能返回 SPA 页面；API 不得被 SPA fallback 吞掉。

交互要求：

- core.running 决定“运行中”，不能看 HTTP 200 或仅 service_active。
- TUN 展示 configured 与 enabled 的差异；停机时开 TUN 只保存期望，不误报生效。
- 延迟 -1 显示失败，-3 未测试；测试中的状态是前端局部状态，不伪装为服务器任务。
- 节点、组、配置操作使用原始完整名字/ID；名称截断只影响视觉。
- 当前管理器以 delayMu 串行执行所有单节点测速，TUI 与网页共用这把锁。第一版组内测速由浏览器逐个调用单节点接口，不新增后台批量任务；网关全局最多放行一个网页测速请求，额外请求返回 409/BUSY，避免多标签重复堆积。取消只停止发送后续节点，已经送入管理器的请求不能承诺取消；页面关闭不继续整组测速。真正并发作为后续独立后端改动。
- 概览约 3 秒刷新，后台标签暂停；其他列表进入页面和操作结束刷新。
- 操作超时显示“结果待确认”，回读实际状态；禁止自动重复提交。
- 当前 API 没有整体 restart：第一版提供启动/停止，不随意拼接重启形成隐藏的网络中断。
- 删除、TUN/端口变更等在产品里说明具体影响，确认后只提交一次；这属于交互设计，不是本次文档任务的审批请求。

第一版不做：内核升级、服务安装卸载、任意配置正文编辑、订阅池编辑、流量和连接页、规则写入。确需增加时先实现 manager /v1 能力并更新契约，再开放 UI。

## 5. 独立连接与部署

浏览器 -> Mihomo Web（网页 + /api/v1） -> 本机 Unix socket -> manager -> core。

拟定默认：

- Web：127.0.0.1:9080；启动前检查占用。
- 生产 socket：/run/mihomo-tui/daemon.sock。
- 浏览器公开地址：MIHOMO_WEB_PUBLIC_URL，通过单独反向代理或 Tunnel 路由配置。
- Homepage 服务端摘要地址：可达本机时 http://127.0.0.1:9080/api/v1/summary。
- 不创建指向 Homepage/HoloBot 的运行依赖，不挂载其文件或共享数据库。
- 网关以独立普通服务账号运行，只获得所需 IPC 组；不需要 root、Docker socket 或正式配置目录权限。
- 网关停止不会停止管理器或核心。生产服务名拟为 mihomo-web.service；独立普通服务账号拟为 mihomo-web，同时加入 mihomo-tui 与 mihomo-tui-operator，才能调用业务接口。安装器负责首次授权，TUI 不安装服务或调整系统权限。
- Web unit 不设置与代理双服务互相联动停止的 Requires/BindsTo/PartOf；仅允许必要启动排序。管理器暂时不可用时网页仍能显示登录页及“管理器不可用”，恢复后重新连接 socket。网关不启动第二个管理器。
- 默认不启用开机自启，不配置 socket activation。首页开关仅控制本次运行，退出 TUI 不关闭 Web；服务器重启后 Web 保持关闭。第一版不提供自启开关，若后续增加必须独立显示和记录，不能混淆运行中与已设自启。

配置模板变量：
MIHOMO_WEB_LISTEN、MIHOMO_WEB_PUBLIC_URL、MIHOMO_WEB_MANAGER_SOCKET、
MIHOMO_WEB_STATE_DIR、MIHOMO_WEB_ADMIN_PASSWORD_HASH、
MIHOMO_WEB_SESSION_SECRET、MIHOMO_WEB_SUMMARY_TOKEN、
MIHOMO_WEB_SUMMARY_SHOW_NODE、MIHOMO_WEB_TRUSTED_PROXIES。

所有路径可配置，模板不带真实凭据。独立测试必须显式给 socket；禁止测试程序缺省落到正式 socket。静态资源使用系统字体或本地授权字体。

### 5.1 TUI 首页与命令入口（拟新增）

首页增加紧凑的“网页访问”状态及“开启 Web／关闭 Web”按钮，不增加第六页，也不改变代理、TUN 开关语义。宽度不足时调整首页布局；在 80×24 终端仍需完整操作，沿用终端默认背景和既有 Tab/Enter 导航，不抢占输入框按键。

| 状态 | 显示与操作 |
| --- | --- |
| 未安装 | 显示“Web 未安装”；禁用开启，提示可选组件安装文档，不自动下载或安装 |
| 已关闭 | “开启 Web”；保留已配置的公开地址，但注明当前不可访问 |
| 开启中／关闭中 | 显示等待状态并禁用重复点击；超时后查询实际状态，不自动重复命令 |
| 运行中 | “关闭 Web”；显示经过校验的公开入口；没有公开地址时注明仅本机可访问，不把服务器 127.0.0.1 当作远程电脑访问地址 |
| 故障 | 显示安全错误摘要；服务仍活动时允许关闭，确认停止后才允许再次开启 |
| 查询失败／后端不支持 | 显示未知／当前版本未提供，不误报关闭；保留其他首页功能 |

同时提供拟新增命令 `mihomo-tui web status`、`mihomo-tui web start`、`mihomo-tui web stop`，与首页调用同一 IPC 客户端及后端接口。沿用现有 socket 选择方式，测试必须显式指定独立 socket。普通操作员无需在 TUI 中输入 sudo；权限不足返回清晰提示。打开网页浏览器不属于此开关，SSH 终端只显示地址。

### 5.2 Web 生命周期接口与权限（管理器拟新增）

生命周期控制只由现有管理器实现小型服务适配器；管理器不导入网页组件、会话模块或静态资源，TUI 不直接执行 systemctl。接口通过管理器 Unix socket 提供，即使 Web 已关闭也可开启。

| 方法与路径 | 请求 | 权限 | 成功响应 |
| --- | --- | --- | --- |
| GET /v1/web/status | 无 | 现有只读 IPC 身份及以上；需明确更新只读白名单 | 200，现有 success/data 外层中的 WebStatus |
| POST /v1/web/start | 无请求体 | IPC operator 或 root | 200，回读后的 WebStatus，running=true |
| POST /v1/web/stop | 无请求体 | IPC operator 或 root | 200，回读后的 WebStatus，确认本服务停止 |

WebStatus 拟定字段：`installed`（服务和发布产物是否齐全）、`configured`（私有配置预检是否通过）、`state`（not_installed/stopped/starting/running/stopping/failed/unknown）、`service_active`、`healthy`、`running`、`public_url`（可空）、`observed_at`（RFC3339 UTC，可空）、`error_code`（可空）、`message`（已筛选的说明）。`running` 必须同时满足服务活动与预期实例的本机健康检查；不得仅因 9080 上有 HTTP 响应就认定启动成功。API 不返回密码哈希、摘要令牌、会话密钥或私有配置路径。

- 使用独立 Web 操作锁，重复 start/stop 幂等；不占用代理配置的 actionMu，不阻塞 TUI 的代理操作。查询不等待 Web 写锁，不把 Web 诊断并入 GET /v1/status 的关键路径。
- 仅允许操作固定的 mihomo-web.service；请求不能传入 unit、命令、路径、监听地址或健康检查 URL。配置由安装阶段放置在 root 控制、服务账号只读的位置；网关状态目录可写但不含可影响 root 执行的配置。
- 开启前验证组件、私有配置、监听绑定和端口；缺组件不安装，缺密码不启动无认证入口。启动后等待自身实例就绪；失败清理本次启动并回读，不影响原来已运行的服务。停止只终止本 Web unit，关闭其日志连接，不操作代理服务、订阅、路由或其他占用端口的程序。
- 生命周期等待窗口拟为 15 秒、TUI/CLI 请求窗口 20 秒；超时返回 504/RESULT_UNKNOWN，先读实际状态。systemd 的启动/停止超时、网关排空期限必须配套，不能把请求断开当作服务操作已取消。
- 错误沿用 success=false/error 文本外层，新增可选 error_code；至少规定 403/FORBIDDEN、409/WEB_NOT_INSTALLED、409/WEB_NOT_CONFIGURED、409/PORT_IN_USE、409/BUSY、500/WEB_START_FAILED、500/WEB_STOP_FAILED、503/WEB_STATUS_UNAVAILABLE、504/RESULT_UNKNOWN。旧客户端忽略新增字段仍可工作；已知未安装查询正常返回 200，而不能读取服务状态时返回 503。
- 浏览器网关白名单不映射 /v1/web/*，网页不能关闭或重新开启自身；摘要令牌也不能调用。网关服务账号的 IPC operator 权限意味着进程本身属于受信任本机调用方，不能把 HTTP 白名单描述为对被攻陷进程的系统级隔离。
- 测试注入假的 Web 服务控制器，不执行生产 systemctl。未配置测试控制器时明确返回不可用，禁止退回正式 unit；仅设置 MIHOMO_TUI_SHADOW=1 不能替代这一防护。

## 6. 拟新增浏览器 API

### 6.1 公共约定

- 前缀 /api/v1；和 manager 的 /v1 分开。
- JSON 查询成功：{"data":...,"request_id":"..."}。
- JSON 错误：{"error":{"code":"UPSTREAM_UNAVAILABLE","message":"管理器暂时不可用","retryable":false},"request_id":"..."}。
- code 枚举至少 UNAUTHORIZED、FORBIDDEN、INVALID_INPUT、NOT_FOUND、CONFLICT、BUSY、UPSTREAM_UNAVAILABLE、UPSTREAM_TIMEOUT、RESULT_UNKNOWN、INTERNAL_ERROR。
- 正常查询 200，配置创建 201；400 参数错误、401 未登录、403 无权、404 资源不存在、409 冲突／忙碌、502 上游错误。504 在查询中为 UPSTREAM_TIMEOUT，在可能已执行的写操作中为 RESULT_UNKNOWN。
- 不根据 manager 中文错误文本做稳定分支；保留必要 HTTP 分类，筛选正文。
- 普通 JSON GET 总超时默认 5 秒；写操作使用独立上游等待窗口（默认 120 秒可配置），不保证上游执行被取消。日志流必须使用独立 HTTP 客户端：连接／响应头等待 5 秒，建立后不设普通请求的总时限。前端、Go 服务和反向代理分别配置查询、写入、流式请求超时，避免外层提前截断。
- 同一浏览器重复点击禁用；网关对配置、核心启停、TUN、端口等变更只放行一个进行中的网页请求，额外请求返回 409/BUSY，不积累隐式写队列。TUI 仍可能并发修改，所以操作后必须回读；超时后显示结果待确认，不能用一次旧状态采样证明操作已结束。
- 请求体默认不超过 64 KiB；enabled 必须是显式 bool，port 为整数 1—65535，拒绝未知字段。
- 组名包括中文、斜线、百分号的 URL 编码必须联调；管理器现有重复解码问题若被触发，修复管理器并加回归用例。

### 6.2 路由映射

| Web 路由 | 管理器现有路由 | 说明 |
| --- | --- | --- |
| GET /api/v1/status | GET /v1/status | 筛选诊断字段 |
| POST /api/v1/core/start | POST /v1/core/start | 无 body |
| POST /api/v1/core/stop | POST /v1/core/stop | 无 body |
| PUT /api/v1/tun | PUT /v1/tun | {"enabled":true} |
| PUT /api/v1/proxy-port | PUT /v1/proxy-port | {"port":7890} |
| GET /api/v1/profiles | GET /v1/profiles | 返回摘要，无订阅正文 |
| POST /api/v1/profiles | POST /v1/profiles | {"name":"示例","url":"https://example.invalid/sub"} |
| POST /api/v1/profiles/{id}/activate | POST /v1/profiles/{id}/activate | 激活不代表已验证代理连通 |
| POST /api/v1/profiles/{id}/update | POST /v1/profiles/{id}/update | 更新失败后回读 |
| PATCH /api/v1/profiles/{id} | PATCH /v1/profiles/{id} | {"name":"新名称"} |
| DELETE /api/v1/profiles/{id} | DELETE /v1/profiles/{id} | 回读剩余列表 |
| GET /api/v1/proxy-groups | GET /v1/proxy-groups | 缺失/null 列表按契约兼容 |
| PUT /api/v1/proxy-groups/{group} | PUT /v1/proxy-groups/{group} | {"name":"完整节点名称"} |
| POST /api/v1/proxy-delay | POST /v1/proxy-delay | {"group":"Auto","name":"完整节点名称"} |
| GET /api/v1/rules | GET /v1/rules | 只读 |
| GET /api/v1/logs/stream | GET /v1/logs/stream | 网关解析并重组标准 SSE |
| GET /api/v1/logging/status | GET /v1/logging/status | 不暴露完整磁盘路径 |
| PUT /api/v1/logging | PUT /v1/logging | {"enabled":true} |

不添加 GET /profiles/{id} 或 GET /proxy-groups/{group} 到上游映射：当前不存在这些单项 GET。禁止通配代理 /api/*、用户指定目标 URL 或用户指定 Unix socket。

### 6.3 网关独有接口

| 接口 | 行为 |
| --- | --- |
| GET /healthz | 只报告 Web 进程可响应，无敏感数据 |
| POST /api/v1/auth/login | 校验管理员密码，创建服务器会话 |
| GET /api/v1/auth/session | 身份、CSRF token、可用权限 |
| POST /api/v1/auth/logout | 撤销会话 |
| POST /api/v1/auth/refresh | 有真实交互时续空闲有效期，不延长绝对期限 |
| GET /api/v1/capabilities | 报告当前网关真正已实现的动作，不能把建议字段当成上游已有能力 |
| GET /api/v1/summary | 只读 Homepage 摘要，独立 token |

登录使用 HttpOnly Cookie、SameSite=Lax、生产 Secure，Cookie 名与 HoloBot 不同且不设跨子域 Domain。写请求校验 CSRF/Origin；限制登录尝试。TLS/Host 信息只信任明确配置的代理。首次安装用一次性本机方式设置密码哈希，初始会话密钥私有保存。

会话采用服务器内存中的随机不透明 ID，不在 Cookie 中存密码或业务状态。拟定绝对有效期 12 小时、空闲有效期 30 分钟；自动轮询与日志心跳不续期，页面经节流的真实交互可通过登录保护的会话刷新接口续空闲期（POST /api/v1/auth/refresh，须校验 CSRF，不能延长绝对期限）。注销撤销当前会话；修改密码或轮换会话密钥撤销所有会话并关闭对应日志连接。网关重启／关闭使所有旧会话失效，凭据重置仅由本机维护入口完成。

摘要令牌与管理员登录独立，验证 scope 只能 GET summary，其他任何路由拒绝。会话和认证响应同样使用 Cache-Control: no-store；会话失效时浏览器停止自动写入，日志断开后 401 不做无限重连。

### 6.4 标准日志事件

上游 /v1/logs/stream 虽有 SSE 响应头，可能是 NDJSON 或 data: 帧。网关按分块缓冲、行解析后输出：

```text
event: log
data: {"level":"info","message":"示例日志","received_at":"2026-09-05T14:00:00Z"}

event: gap
data: {"reason":"upstream_reconnected"}
```

日志流单独配置：每 15 秒输出 SSE 注释心跳并立即 flush；反向代理对该路径关闭缓冲，读取空闲窗口至少 60 秒，Go 服务不可用普通短 WriteTimeout 截断流。网关持续监测会话撤销和到期并主动关闭上下游连接。无日志的长时间空闲不算故障。

浏览器仅展示纯文本；网关解析缓冲和单事件均有上限（拟定 64 KiB，超过时丢弃该事件并报告 gap），浏览器最多约 1000 行缓存。断开浏览器即关闭上游连接，退避重连。无历史补发保证，gap 明确提示可能漏行。清空显示不删除磁盘文件。日志启用不等于已收到数据，应显示 logging.last_error 的安全摘要。

## 7. Homepage 摘要契约

GET /api/v1/summary 接受独立 Authorization: Bearer token。
平铺 JSON（不使用普通 data 外壳）；Content-Type application/json、Cache-Control no-store：

```json
{
  "schema_version": 1,
  "app": "mihomo",
  "state": "healthy",
  "state_label": "运行正常",
  "observed_at": "2026-09-05T14:00:00Z",
  "stale": false,
  "node_label": "示例节点",
  "tun_label": "关闭"
}
```

state：healthy/degraded/stopped/unknown。

- 健康字段描述代理运行／控制面状态，不等同于已验证外网连通。
- 先完善管理器状态契约：新增 `core.state_query_ok`、`core.service_state`（active/inactive/failed/activating/deactivating/unknown）、`tun.observation_ok`、顶层 `observed_at`，保留旧字段。查询失败不能把零值 false 当作确定状态，不解析 detail 的中文文字来分类。`tun.observation_ok` 指已取得当前服务状态所需的观测；确认停机时不要求访问不存在的核心控制接口，服务运行时则必须成功读取运行配置及网卡状态。

| 条件（从上到下判断） | 摘要 state |
| --- | --- |
| 管理器不可读、关键查询失败、缺少新增状态字段或缺少有效采样时间 | unknown |
| 确认 service_state=inactive | stopped；若 TUN 已配置，说明“代理已停止，TUN 待生效” |
| service_state=failed/activating/deactivating，或服务活动但控制接口不健康 | degraded |
| core.running=true，且 TUN 期望与实际不一致 | degraded |
| core.running=true，且必要观测成功、TUN 状态一致 | healthy |
| 其他矛盾或不足以判断的状态 | unknown |

- 管理器返回 observed_at 代表本次完成的观测时间；网关不为缓存或失败采样生成新的成功时间。新字段未支持时只读界面仍可显示已有字段，但 summary 保守返回 unknown，不猜测停止原因。
- observed_at 为 RFC3339 UTC 或 null；没有有效采样时 stale=true、state=unknown。有效采样超过 30 秒同样 stale=true、state=unknown，state_label 显示“数据过期”；不要保留旧的“运行正常”文本。
- 不返回订阅链接、用户身份、日志或原始内部错误。node_label 可配置为“已选择”以隐藏名字。
- 首页请求允许短缓存 3—5 秒以减少重复 IPC，但不得伪造新采样时间。
- Homepage href 为公开地址 + /overview，widget.url 为独立内部地址 + /api/v1/summary。
- Web 关闭时该摘要端点也关闭，Homepage 显示“网页未连接／不可用”，不能据此推断代理已停止。第一版不为关闭后的 Web 另起常驻摘要服务，也不允许首页为了取摘要自动开启 Web。

## 8. 未来目录布局与构建

```text
web/
  PLAN.md
  package.json
  pnpm-lock.yaml
  src/
    pages/
    components/
    api/
    composables/
  public/
  tests/
  THIRD_PARTY_NOTICES.md
cmd/mihomo-web/             Go 网关入口
internal/webgateway/        会话、白名单映射、日志转换、摘要
deploy/web/                未来服务与环境模板
scripts/build-web.sh       独立 Web 正式构建（拟新增）
scripts/test-web.sh        独立 Web 检查（拟新增）
scripts/deploy-web.py      独立 Web 安装／升级／回退（拟新增）
docs/WEB_API.zh-CN.md       浏览器接口契约（拟新增）
```

Go 包放置须兼容现有根 module；不要让 UI 代码进入 manager 业务包。不为 web/ 单独 git init。

### 8.1 独立构建与版本化交付

先验证服务器能完成最小 Vue/Vite 构建，再裁剪参考界面。固定兼容的 Node/pnpm 版本并使用 frozen lockfile；服务器没有已确认的原生 Node，可评估固定镜像的独立构建容器及 LATX。容器架构和构建链必须先实测，不假设官方镜像原生支持 loong64。所有正式源码仍保留在当前服务器仓库，不全局修改代理或系统 Git。

- 保持现有 scripts/build-release.sh 和 scripts/deploy.py 的 TUI／管理器职责；新增 Web 脚本，不把 Web 当成原双服务部署的必选附件。
- Web 正式产物拟放 `$HOME/.local/share/mihomo-loongnix/builds/web/<完整提交>/`；日常检查放 checks/web/。同一提交的 TUI 与 Web 目录互不占用、不覆盖已有归档。构建必须来自干净提交，保存 Go 构建信息、源码提交、依赖锁摘要、Node/pnpm 及构建镜像版本。
- 每个 Web 发布包包含独立 `mihomo-web-linux-loong64`、`static/`、`BUILD-INFO.txt` 和覆盖所有文件的校验清单。静态资源从该发布目录读取，启动验证资源存在及版本匹配；不在普通 Go 编译路径强制 go:embed 尚未生成的 dist，确保没有 Node 或静态产物也能构建和测试 TUI。
- 网关与 manager 通过显式 IPC 契约交互；运行时不导入 TUI 包或依赖 TUI 进程。同一 Go module 下允许 go test ./... 覆盖纯 Go 网关测试，但这些测试不能要求 Node、浏览器、root 或真实服务。

### 8.2 安装、升级与回退

首次可选安装准备普通服务账号、root 控制的 unit 与配置、独立私有状态目录（拟为 /var/lib/mihomo-web）及初始认证；默认安装后保持关闭且不设自启。密码哈希和密钥不通过网页接口设置，不提交仓库。

Web 发布目录拟为 `/opt/mihomo-web/releases/<完整提交>/`，root 控制 `current` 符号链接，unit 固定从 current 启动。进程启动时将发布根解析为不可变的真实目录，避免切换链接后旧进程读取新静态资源。先暂存并核验完整包，然后原子替换 current；按部署前运行状态决定是否重启 Web，原来关闭时部署后仍关闭。静态资源和网关必须一起切换及一起回退。

升级前保存上一个发布指向、Web unit／私有配置／状态与权限、会话失效规则、运行和自启状态。测试失败恢复完整 Web 备份并验证；不删除旧版本及备份。若改变持久状态格式，必须提供可回退迁移，不把旧二进制配新格式数据。部署只允许操作 Web 服务，不停代理双服务；若还需要升级 manager 以支持新 API，单独列为基础后端升级并按原部署流程授权执行。

候选检查分两层：普通 --check 检查发布完整性和接口兼容，不启动正式服务；独立候选实例只使用测试 socket、测试凭据及测试端口。真正生产验证分别检查 Web 进程、监听、静态资源、登录和 manager 只读连通性；manager 不可用应如实降级，不能直接认定 Web 二进制损坏而回退。通过后记录 Web 与 manager 各自提交及兼容范围。

### 8.3 独立测试环境

- 新增 scripts/test-web.sh：前端类型检查、组件测试、浏览器关键流程和 Go 网关契约测试独立执行，CI 单独作业；普通 TUI 构建不依赖网页构建成功。
- 测试状态放 `$HOME/.local/state/mihomo-loongnix-test/web/`，独立凭据、缓存、会话及日志；网关候选端口 19080，前端开发端口 15173，启动前确认空闲，不使用正式 9080。
- 默认对接 Go httptest／假 Unix socket 管理器，使用 testdata 假节点与配置。需要真实独立 manager 时，显式设置专用 socket、MIHOMO_TUI_SHADOW=1、代理端口 17890、控制端口 19090，并清除 MIHOMO_TUI_CORE_SERVICE；不得复用任何正在占用的测试进程或目录。
- 测试模式必须拒绝正式 socket、正式状态目录和生产 Web unit；缺少测试 socket 或服务控制替身就失败，不做生产默认值回退。真实 systemd 集成检查另列部署验收，不能用普通单元测试暗中调用。
- 开发前端由 Vite 将 /api/v1 代理到同一测试网关，不接生产 socket、不为开发开放宽泛 CORS。浏览器端到端测试覆盖 Cookie 与 SSE 实际行为；生产 Secure Cookie 另用隔离 HTTPS 测试入口验证。
- 启停与清理只针对测试工具自己记录的 PID／临时路径。分别验证“Web 关闭代理继续运行”“管理器暂时断开网页可展示故障”“TUI 退出 Web 继续运行”；正式代理数据不复制到测试。

### 8.4 接口文档与兼容规则

文档是实现交付的一部分，不能只留下本方案中的路径表。实施时同步维护三份中文文档：

| 文档 | 必须覆盖 |
| --- | --- |
| docs/MANAGER_API.zh-CN.md | 管理器供 TUI／Web 使用的完整 /v1 契约、新增 Web 生命周期接口、只读／operator 权限、新状态字段、错误码、锁与超时、旧客户端兼容 |
| docs/WEB_API.zh-CN.md（新增） | 浏览器 /api/v1 每条路由及其上游映射、请求和返回类型、可选/null 字段、状态码与机器错误码、Cookie/CSRF/会话刷新与撤销、SSE、限流与结果待确认、summary 和 capabilities |
| docs/WEB_INTEGRATION.zh-CN.md | 可选独立 Web 拓扑、首页开关与 CLI、安装和停用、公开入口与内部摘要地址、独立测试、版本组合和 Web 专用部署／回退 |

每个新增接口给出虚构请求／响应示例和可在独立测试 socket／测试网关执行的命令；说明谁调用、是否改变状态、是否幂等、超时是否可能继续执行。补充 WebStatus、core/TUN 观测字段与错误枚举的契约测试，文档以实际注册路由为准。

旧 manager 不支持 /v1/web/* 时 TUI 显示“不支持 Web 控制”，其他功能仍正常；网关对缺失新状态字段保守降级，缺失写入能力则明确禁用。首个正式 Web 版本记录最低已验证 manager 提交及能力矩阵，后续优先增量扩展字段；不根据产品版本字符串猜测接口存在。本次没有修改现有 API 文档中的“已实现”列表，新增行为须在实现并验证后写入该列表。

## 9. 实施阶段与验收

1. 确认独立构建链可用，固定参考版本；先编写拟新增接口契约、状态表与测试替身。
2. 实现 manager 的 Web 服务适配器、生命周期路由与状态观测字段，补全接口文档；用假服务验证 TUI 首页／CLI 开关及 80×24 布局，旧 manager 保持兼容。
3. 建立独立 Web 构建／测试入口；完成五页假数据界面、Go 网关、登录、只读接口和摘要，只连接测试 socket。
4. 接入写操作、串行测速和标准日志，完善会话撤销、错误映射和接口文档；完成独立浏览器流程。
5. 与 Homepage 分别联调固定数据、真实测试 summary，以及 Web 关闭时摘要不可用的表现；不修改摘要字段、枚举或鉴权约定。若实际修改这些共享契约，同步三份关联 PLAN.md。
6. 准备并验证 Web 专用安装／升级／回退工具。在用户后续授权的部署阶段分别完成所需 manager 升级和可选 Web 安装；Web 默认关闭，随后由用户通过 TUI／CLI 开启。

必须验收：

- TUI 和网页使用同一个 manager，变更互相可见，没有第二份配置。
- core 已停、服务故障、查询失败、controller 不健康、TUN 仅配置未生效都按状态表显示；旧接口缺字段不误报 healthy 或 stopped。
- 没有 Web 产物或 Node 时 TUI 仍可构建、运行；首页开关在 80×24 可操作。Web 未安装、缺凭据、端口占用、查询超时和旧 manager 场景反馈准确。
- 开启／关闭 Web、退出 TUI、Web 升级／回退不停止代理；重复点击不重复启动；生产 Web 服务账号和只读 IPC 组权限需独立验收。
- readonly summary token 不能查询详细节点、配置或操作服务。
- 空 enabled、非法端口、特殊名称、请求超时、重复提交得到正确反馈。
- 订阅输入不存 localStorage、不进日志；HTML 中无核心 secret。
- 日志兼容分块 NDJSON/SSE、缓冲有上限、重连有 gap；空闲超过 60 秒仍保持连接，退出／过期／改密后旧流被关闭。
- 多标签测速不积累无界请求；取消批量后不再提交剩余节点，不声称已取消管理器中的请求。
- Web 发布包校验、静态与网关同版、升级前关闭则升级后仍关闭、失败只回退 Web 均验证；不以模拟检查替代实际 systemd 部署验收。
- Homepage/HoloBot 都停止时此网页仍可独立工作；网页停止代理继续运行。
- 前端构建/类型检查、关键浏览器流程通过。按 AGENTS.md 执行相关 Go 检查、文档检查与部署工具测试；不用真实订阅或正式 TUN 做开发测试。
- 记录 Web 空闲与日志流时 RSS/CPU/文件描述符，不承诺未经测量的内存数字。

完成后交付可选独立 Web 服务、五页网页、TUI 首页开关与 CLI、独立构建／测试／部署工具、完整接口文档、配置模板、来源清单和回退说明。代码实施后以实际接口文档与验证记录为准；正式服务安装和实际系统回退验收单独记录。
