# 网页接口文档

本文对应 `cmd/mihomo-web` 和 `internal/webgateway` 的已实现接口。正式安装为可选步骤；存在源码不表示服务器已经部署 Web。管理器 Unix socket 接口另见 [管理器契约](MANAGER_API.zh-CN.md)。

## 连接、登录与权限

浏览器与网关同源访问 `/api/v1`，网关通过私有配置指定的 Unix socket 访问 `/v1`。正式 Web 只监听回环地址，由 HTTPS 入口转发；生产 Cookie 带 Secure、HttpOnly、SameSite=Lax、Path=/，不设置跨子域 Domain，名称为 `mihomo_web_session`。

浏览器必须使用配置中的公开 Host；网关不采信调用者提供的 Forwarded 或 X-Forwarded-* 来决定身份、来源或限流。所有非 GET 浏览器请求要求 `Content-Type: application/json`，Origin 必须与公开入口完全相同；登录之外还要求 `X-CSRF-Token`。无参数 POST 发送 `{}`。不开启跨域 CORS。密码至少 12 字节，使用 PBKDF2-SHA256（600000 次、随机盐）验证，密码哈希只在服务器私有配置保存。

服务器内存会话绝对期限 12 小时，空闲期限 30 分钟。普通查询、自动轮询和日志心跳不续期；前端真实操作最多每分钟调用一次 refresh。重启／关闭网关使会话失效。修改密码通过本机维护配置后重启网关完成，所有旧会话和日志连接随之撤销。单实例最多 16 个有效会话；登录总量最多每分钟 10 次，同时只验证一个密码。此限制不依赖可伪造的代理来源 IP，多人共享入口会共用额度。

所有接口默认 `Cache-Control: no-store`。使用独立摘要 Bearer 令牌的请求即使同时携带管理员 Cookie，也不能访问其他浏览器 API。网页不暴露 `/v1/web/*`，不能关闭或启动自身。

## 响应与错误

正常响应（查询 200，创建配置沿用上游 201）：

```json
{"data":{"enabled":false},"request_id":"示例请求标识"}
```

错误响应：

```json
{"error":{"code":"RESULT_UNKNOWN","message":"结果待确认，请刷新实际状态，不要重复提交","retryable":false},"request_id":"示例请求标识"}
```

| 状态码 | code | 含义 |
| --- | --- | --- |
| 400 | INVALID_INPUT | 参数缺失、类型错误、未知字段、多余 JSON、正文超过 64 KiB 或不支持的查询参数 |
| 401 | UNAUTHORIZED | 未登录、会话失效、密码或摘要令牌错误 |
| 403 | FORBIDDEN | Host、Origin、CSRF、身份或令牌范围不匹配 |
| 404 | NOT_FOUND | 路由／方法或上游资源不存在 |
| 405 | INVALID_INPUT | 登录、会话、日志、健康检查等明确路由使用了错误方法 |
| 409 | BUSY / CONFLICT | 网页已有操作，或管理器报告状态冲突 |
| 429 | RATE_LIMITED | 登录验证繁忙、尝试频率或会话数量达到上限 |
| 502 | UPSTREAM_UNAVAILABLE | 管理器连接失败、返回格式异常或未分类上游失败 |
| 504 | UPSTREAM_TIMEOUT / RESULT_UNKNOWN | 查询超时；写请求可能已经执行、需要回读确认 |

不向浏览器原样返回上游错误正文。机器分支依赖 code，不解析中文文字。错误中的 retryable 当前始终为 false，前端不自动重复写请求。上游失败也可能已部分改变状态。

普通 JSON 查询等待 5 秒；写操作等待 120 秒。网关只放行一个常规写操作和一个测速操作，超额立即返回 409/BUSY，不排隐式写队列。TUI 仍可能持有管理器锁；浏览器断开或请求超时不能保证上游任务已取消，网关不提供任务 ID、任务进度或跨客户端幂等键。

## 浏览器认证与能力接口

| 方法与路径 | 请求 | data |
| --- | --- | --- |
| POST /api/v1/auth/login | `{"password":"演示密码，不是真实凭据"}`；仍要求 Origin 和 JSON | user、csrf_token；同时设置 Cookie |
| GET /api/v1/auth/session | 登录 Cookie | user、csrf_token、permissions（read/operate） |
| POST /api/v1/auth/refresh | `{}`、Cookie、CSRF | `{"refreshed":true}`；只延长空闲期 |
| POST /api/v1/auth/logout | `{}`、Cookie、CSRF | `{"logged_out":true}`；撤销当前会话，关闭其日志连接 |
| GET /api/v1/capabilities | 登录 Cookie | schema_version、actions、web_lifecycle=false、delay_concurrency=1 |
| GET /healthz | 无认证，仅本服务健康信号 | 直接返回 `{"app":"mihomo-web","pid":1234}`，无 data 外层 |

capabilities 表示当前网关实现的映射，不证明旧 manager 的每个动作都存在。没有浏览器密码修改、账号管理或自启设置接口。`/healthz` 只用于本机验证 Web 进程，不表示代理内核或外网健康。

## 代理业务接口

下表字段类型和业务行为沿用 [管理器接口](MANAGER_API.zh-CN.md)，返回值不再套管理器的 success 外层。数组按客户端兼容空数组／null；节点与规则数据不分页，浏览器本地筛选。

| 方法与路径 | 请求 | data 与说明 |
| --- | --- | --- |
| GET /api/v1/status | 无 | ManagerStatus；移除 core.detail、tun.interface 等诊断字段，保留观测字段 |
| POST /api/v1/core/start | `{}` | ManagerStatus；启动后回读 |
| POST /api/v1/core/stop | `{}` | ManagerStatus；只停止代理核心 |
| PUT /api/v1/tun | `{"enabled":true}` | ManagerStatus；停止时只保存期望 |
| PUT /api/v1/proxy-port | `{"port":17890}` | ManagerStatus；整数 1—65535 |
| GET /api/v1/profiles | 无 | ProfileSummary 数组 |
| POST /api/v1/profiles | `{"name":"测试配置","url":"https://example.invalid/sub"}` | ProfileSummary；name 可空，URL 必须 HTTP/HTTPS |
| POST /api/v1/profiles/{id}/activate | `{}` | ProfileSummary；成功不代表已验证外网连通 |
| POST /api/v1/profiles/{id}/update | `{}` | ProfileSummary；失败后仍回读 |
| PATCH /api/v1/profiles/{id} | `{"name":"新名称"}` | ProfileSummary；非空名称 |
| DELETE /api/v1/profiles/{id} | `{}` | 剩余 ProfileSummary 数组 |
| GET /api/v1/proxy-groups | 无 | ProxyGroup 数组 |
| PUT /api/v1/proxy-groups/{group} | `{"name":"完整节点名称"}` | ProxyGroup；所有组并非均支持手动选择 |
| POST /api/v1/proxy-delay | `{"group":"Auto","name":"完整节点名称"}` | name、delay（毫秒）；失败 -1，未测试 -3 |
| GET /api/v1/rules | 无 | Rule 数组；只读 |
| GET /api/v1/logging/status | 无 | enabled、current_file_bytes、total_bytes、max_file_bytes、max_backups、has_error |
| PUT /api/v1/logging | `{"enabled":true}` | 同上；只控制落盘，错误信息转成 has_error，不返回磁盘路径 |

除日志级别外不接受任意查询参数。enabled 必须显式提供 bool；含未知字段和多个 JSON 对象会拒绝。配置 ID 使用管理器生成值，不允许斜线等路径内容；组名使用原始完整字符串并作为一个路径参数 encodeURIComponent 编码，支持中文字面、斜线和百分号。不得用展示截断后的名称调用。

没有 GET profiles/{id}、GET proxy-groups/{group}、任意代理目标 URL 或用户指定 Unix socket 接口。订阅链接是受登录保护的导入字段，允许调用管理器进行 HTTP/HTTPS 下载，不是任意浏览器代理转发入口；管理员可以导入本机／内网订阅，第一版不提供多租户或不可信订阅用户角色。

## 日志流

`GET /api/v1/logs/stream?level=info` 要求登录 Cookie；level 可选 debug/info/warning/error/silent，默认 info。成功为标准 SSE，不套 data 外层：

```text
event: log
data: {"level":"info","message":"演示日志","received_at":"2026-09-06T00:00:00Z"}

event: gap
data: {"reason":"upstream_reconnect_required"}
```

网关兼容逐行 JSON 和单行 data: SSE，缓冲跨传输分块；不支持任意复杂的多行 SSE 数据协议。单行解析上限 64 KiB，超长事件丢弃并发送 gap。日志 message 不保证脱敏，浏览器按纯文本显示；最多缓存 1000 条，不使用 HTML 注入。

建立连接及响应头等待 5 秒，流建立后无普通查询的总超时。每 15 秒发送注释心跳，立即刷新输出；单次写入最多 10 秒，慢客户端会断开。反向代理必须禁用该路径缓冲，读取空闲窗口至少 60 秒。会话到期／注销、浏览器断开或网关停机，都会关闭上游流。

gap.reason 为 event_too_large、upstream_disconnected 或 upstream_reconnect_required，表示可能漏行，没有历史补发、事件 ID 或断点续传。前端关闭失效流，以 1—30 秒退避重新连接；重新检查会话失败就停止。隐藏标签暂停订阅，返回时提示间断。暂停／清空显示不删除磁盘日志，也不停止落盘。

## Homepage 摘要

`GET /api/v1/summary` 只接受 `Authorization: Bearer <独立摘要令牌>`。成功直接返回平铺对象：

```json
{"schema_version":1,"app":"mihomo","state":"healthy","state_label":"运行正常","observed_at":"2026-09-06T00:00:00Z","stale":false,"node_label":"已选择","tun_label":"关闭"}
```

state 为 healthy/degraded/stopped/unknown。必须有有效采样和 core.state_query_ok 才分类：确认 inactive 为 stopped；failed／切换中、控制接口不健康或 TUN 期望不一致为 degraded；内核运行且 TUN 观测正常、一致为 healthy。没有新字段、查询失败、采样超过 30 秒或未来异常时间都为 unknown。observed_at 可为 null；无时间或过期时 stale=true。正常返回的查询失败信号可以为 unknown、stale=false，表示“新鲜的未知状态”，不伪造正常状态。

默认 show_node=false，仅返回“已选择”；不带订阅、用户身份、日志和内部错误。第一版每次摘要请求只读查询管理器，不提供缓存，Homepage 建议每 10 秒请求。Web 关闭时摘要端点也关闭；Homepage 应显示网页不可用，而不是代理已停止。

## 独立测试调用

先按接入指南启动假数据预览。以下地址仅对应测试端口，不用于正式账户：

```bash
# 提示输入演示密码，以 Cookie 文件保存本次测试会话；文件不要提交仓库。
umask 077
curl -sS http://127.0.0.1:19080/healthz
```

登录、CSRF、业务请求、特殊名称、摘要令牌隔离、超时和流关闭的可执行用例见 `internal/webgateway/server_test.go`。正式密码、Cookie 和令牌不应放入命令行历史；网页登录优先通过浏览器。
