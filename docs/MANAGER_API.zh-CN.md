# 管理后端接口文档

本文描述本仓库当前实现的本机管理接口，供 TUI、未来的网页后端以及其他本机客户端使用。它不是 Mihomo 内核自身的 API，也不表示网页端已经实现。网页接入方式见 [网页接入指南](WEB_INTEGRATION.zh-CN.md)。

接口路径前缀为 `/v1`。当前没有接口版本协商、能力发现、异步任务查询或正式的兼容性承诺；接入方应记录所对接的源码提交，接口变更时同步更新本文。

## 1. 连接和权限

### 连接方式

正式 `mihomo-manager.service` 通过 Unix socket 上的 HTTP 提供服务：

```text
/run/mihomo-tui/daemon.sock
```

不监听用于管理的 TCP 端口。请求中的 `http://localhost` 只是 HTTP 地址占位，实际连接由 Unix socket 决定，与 Mihomo 的 `9090` 控制接口不同。

```bash
curl --unix-socket /run/mihomo-tui/daemon.sock http://localhost/v1/status
```

独立测试使用 `MIHOMO_TUI_SOCKET` 指定的绝对路径，配合 `MIHOMO_TUI_SHADOW=1` 和专用状态目录。示例见 [部署与测试说明](LOONGNIX.md)。浏览器不能直接访问服务器的 Unix socket，需要总面板后端转接。

### 权限模型

正式管理器根据连接进程的本机身份及组成员关系鉴权，不使用浏览器 Cookie、JWT 或调用方自行填写的身份请求头。socket 目录为 `root:mihomo-tui`、`0750`，socket 为 `0660`。

| 调用身份 | 本文 `/v1` 接口权限 |
| --- | --- |
| 不具备 socket 访问权限 | 连接可能直接报权限错误，尚未进入 HTTP 层 |
| 仅属于 `mihomo-tui` 组 | 只能 `GET /v1/status`、`GET /v1/logging/status` 和 `GET /v1/web/status` |
| 同时属于 `mihomo-tui`、`mihomo-tui-operator` | 可以调用本文全部 `/v1` 接口，包括启停内核、TUN 和配置管理 |
| root | 可以调用全部已注册接口 |

普通用户运行的独立测试管理器以其所属用户为管理员，与正式 root 管理器的授权场景不同。测试成功不等于已验证生产组权限。

安装器创建权限组，`sudo mihomo-tui grant_operator 用户名` 将指定用户加入两个组。已有登录进程或网页服务进程需要重新建立会话或重启，才能取得新的系统组权限。本文不要求为了开发文档而修改正式服务权限。

## 2. 公共约定

### JSON 与状态码

带请求体的写操作使用 `Content-Type: application/json`。无参数的启停、激活、更新、删除操作不需要请求体。

正常 JSON 响应：

```json
{"success":true,"data":{}}
```

处理器的错误响应：

```json
{"success":false,"error":"代理端口必须在 1-65535 之间"}
```

`data` 可以是对象、数组或字符串；失败时通常省略。可选字段可能不出现，部分集合也可能为 `null`，客户端应兼容。日志流不使用这个 JSON 外层。未知路由等框架级错误可能返回普通文本，不应假定所有失败都能解析成 JSON。

| HTTP 状态 | 含义 |
| --- | --- |
| `200` | 查询或操作处理成功；实际状态仍需读取 `data` |
| `201` | 新配置已导入 |
| `400` | 请求、配置或不支持的操作有误 |
| `403` | IPC 身份无权调用该接口 |
| `404` | 配置、组、节点或路由不存在 |
| `405` | 方法不支持；配置详情处理器的部分方法错误使用 `400` |
| `409` | 状态冲突、重复 URL、端口占用或状态回读不一致 |
| `500` | 持久化、服务控制、配置应用、路由处理等失败 |
| `502` | 下载订阅或请求内核接口失败 |
| `504` | 等待内核、TUN 或端口达到目标状态超时 |

没有独立的机器错误码；不能根据中文错误文字实现稳定的业务分支。错误消息没有统一的全局脱敏保证，网页后端应筛选展示内容，避免原样公开内部路径、订阅地址或诊断信息。

### 请求校验、并发和超时

- 请求应显式提供本文列出的字段。当前解码器会忽略未知字段，也没有统一的请求体大小限制；网页入口需要自行限制请求大小并校验字段。独立网关现已实现这些入口校验，见 [Web API](WEB_API.zh-CN.md)。
- 特别是 `enabled`：当前缺失时会按 Go 的零值 `false` 处理，网页入口应拒绝 `{}`，避免误关功能。
- 无分页、筛选或排序查询参数；配置、节点、规则的显示筛选和分页由客户端完成。
- 多数状态修改共用互斥锁；节点测速使用独立锁。不要同时发起多次配置/TUN/端口变更。
- 当前 TUI 普通请求总超时为 60 秒。启动内核的语义校验最多 45 秒，后续启动等待窗口为 12 秒；排队和网络请求还会增加耗时。这不是服务端统一的 60 秒执行上限。
- 客户端超时或断线不代表后端操作已取消。修改请求失败或超时后先重新查询实际状态，不盲目重试导入、切换或删除。
- 回滚是后端尽力恢复的行为，部分恢复错误未向调用方完整汇报，不能保证收到失败后所有状态都完全不变。

## 3. 接口总表

下表的返回类型均指成功响应的 `data`，日志流除外。

| 方法 | 路径 | 请求体 / 查询参数 | 成功状态与返回 |
| --- | --- | --- | --- |
| GET | `/v1/status` | 无 | `200`，ManagerStatus |
| POST | `/v1/core/start` | 无 | `200`，ManagerStatus |
| POST | `/v1/core/stop` | 无 | `200`，ManagerStatus |
| PUT | `/v1/tun` | `{"enabled":true}` | `200`，ManagerStatus |
| PUT | `/v1/proxy-port` | `{"port":7890}` | `200`，ManagerStatus |
| GET | `/v1/profiles` | 无 | `200`，ProfileSummary 数组 |
| POST | `/v1/profiles` | `{"name":"示例","url":"https://example.com/profile.yaml"}` | `201`，ProfileSummary |
| POST | `/v1/profiles/{id}/activate` | 无 | `200`，ProfileSummary |
| POST | `/v1/profiles/{id}/update` | 无 | `200`，ProfileSummary |
| PATCH | `/v1/profiles/{id}` | `{"name":"新名称"}` | `200`，ProfileSummary |
| DELETE | `/v1/profiles/{id}` | 无 | `200`，剩余 ProfileSummary 数组 |
| GET | `/v1/proxy-groups` | 无 | `200`，ProxyGroup 数组 |
| PUT | `/v1/proxy-groups/{group}` | `{"name":"节点完整名称"}` | `200`，更新后的 ProxyGroup |
| POST | `/v1/proxy-delay` | `{"group":"Auto","name":"节点完整名称"}` | `200`，ProxyDelayResponse |
| GET | `/v1/rules` | 无 | `200`，Rule 数组 |
| GET | `/v1/logs/stream` | 可选 `level=info` | `200`，实时日志流 |
| GET | `/v1/logging/status` | 无 | `200`，LoggingStatus |
| PUT | `/v1/logging` | `{"enabled":true}` | `200`，LoggingStatus |

路径中的 `{id}`、`{group}` 是占位符。使用列表返回的原始 ID 或完整组名，并对路径参数做 URL 编码；节点名放在 JSON 请求体中，不能用界面裁剪、去旗帜后的显示名称替代。组名仅解码一次；路径参数必须先 URL 编码，中文、斜线和字面百分号已纳入回归检查。

不存在 `GET /v1/profiles/{id}` 或 `GET /v1/proxy-groups/{group}` 单项详情接口；需要从列表查找。

## 4. 状态、内核、TUN 和端口

### ManagerStatus

示例数据均为虚构：

```json
{
  "success": true,
  "data": {
    "core": {
      "service_active": true,
      "controller_healthy": true,
      "running": true,
      "pid": 1234,
      "detail": "active"
    },
    "tun": {
      "configured": false,
      "runtime_enabled": false,
      "interface_present": false,
      "enabled": false,
      "interface": "mihomo-tui-tun"
    },
    "active_profile": {
      "id": "example-profile-id",
      "name": "示例配置",
      "source": "https://example.com",
      "updated_at": "2026-09-05 12:00",
      "active": true
    },
    "proxy_port": 7890,
    "current_group": "Auto",
    "current_node": "示例节点"
  }
}
```

| 字段 | 类型与含义 |
| --- | --- |
| `core.state_query_ok` | 布尔；系统服务状态查询是否成功，失败不能当成已停止 |
| `core.service_state` | active/inactive/failed/activating/deactivating/unknown；系统状态或独立进程状态 |
| `observed_at` | RFC3339 UTC 字符串；本次状态观测完成时间 |
| `tun.observation_ok` | 布尔；当前服务状态所需的 TUN 观测是否成功，停机无需查询不存在的控制接口 |
| `core.service_active` | 布尔；正式环境来自 systemd，独立测试来自受管进程状态 |
| `core.controller_healthy` | 布尔；内核 `/version` 请求是否成功 |
| `core.running` | 布尔；上面两个条件同时成立 |
| `core.pid` | 整数；进程 ID，停止时一般为 0 |
| `core.detail` | 可选字符串；服务状态或诊断文字 |
| `tun.configured` | 布尔；持久配置期望是否启用 TUN |
| `tun.runtime_enabled` | 布尔；内核运行配置是否启用 TUN |
| `tun.interface_present` | 布尔；主机是否存在项目 TUN 网卡 |
| `tun.enabled` | 布尔；内核运行、运行配置开启、网卡存在三个条件同时成立 |
| `tun.interface` | 字符串；项目网卡名 |
| `active_profile` | 可选 ProfileSummary；没有活动配置时省略 |
| `proxy_port` | 整数；内核可用时优先取运行 mixed-port，否则返回受管端口 |
| `current_group` | 可选字符串；配置中的默认代理组 |
| `current_node` | 可选字符串；从该组实际 `now` 读取的节点，无法取得时省略 |

`GET /v1/status` 在内核停止或服务查询失败时也可能返回 `200`、`success=true`。这只表示状态查询已返回，不表示内核健康。网页必须检查各状态字段。

### 启停内核

`POST /v1/core/start` 生成配置，调用实际内核的 `-t` 做最终语义校验，再执行必要的 TUN 预检、启动并等待服务和控制接口健康。首次校验可能下载缺失的地理数据。参数或配置错误一般返回 `400`，服务启动失败返回 `500`，健康等待超时返回 `504`。

`POST /v1/core/stop` 停止内核并按需清理项目 TUN 路由。重复停止可成功返回停止状态；内核已停止但路由清理失败时仍会返回 `500`。该接口不会退出管理服务。

### TUN

`PUT /v1/tun` 的 `enabled` 必须由客户端显式提供布尔值。内核运行时，后台修改配置、调整路由、重载并等待状态；活动服务的控制接口不健康时可能返回 `409`。

**内核停止时，开启操作只保存期望配置，不会自动启动内核。** 因而成功响应可能是 `configured=true`、`enabled=false`。提交值与已有配置相同时会直接返回当前状态，不保证主动修复运行状态偏差。

### 混合代理端口

`PUT /v1/proxy-port` 的 `port` 是 `1`—`65535` 的整数，不可等于控制端口或使用不可用端口。正式控制端口固定为 `9090`，混合端口默认 `7890`。

内核停止时保存下次使用的端口；运行时热重载并回读实际 mixed-port。范围错误返回 `400`，控制端口冲突或占用返回 `409`，应用失败返回 `500`，回读超时返回 `504`。新端口将影响使用旧代理端口的客户端。

测试模式端口由环境变量强制覆盖，不能假定在测试模式下调用此接口就能改变覆盖后的端口。

## 5. 配置管理

### ProfileSummary

| 字段 | 类型与含义 |
| --- | --- |
| `id` | 字符串；后端生成的配置标识，后续操作使用它 |
| `name` | 字符串；显示名称 |
| `source` | 字符串；URL 配置只显示协议与主机，不含凭据、路径、查询或片段；本地迁移配置可能返回文字标签 |
| `updated_at` | 可选字符串；服务端本地时间，格式 `YYYY-MM-DD HH:mm`，不含时区，不能当作精确 UTC 时间 |
| `active` | 布尔；是否为当前活动配置 |

列表不返回完整订阅链接、节点凭据或配置正文，也没有导出正文接口。

### 导入、激活与更新

`POST /v1/profiles` 要求 `url` 为 HTTP/HTTPS URL，`name` 可省略。后端下载、识别和结构校验后保存私有缓存；首份配置自动成为活动配置，后续导入不会自动替换活动配置。已存在的相同 URL 返回 `409`，下载失败返回 `502`。

支持带非空 `proxies` 列表的 Clash/Mihomo YAML、provider YAML、它们的 Base64 形式，以及明文或 Base64 URI 节点列表。URI 转换覆盖 SS、VMess、VLESS、Trojan、Hysteria、Hysteria2、TUIC、SOCKS5；具体传输参数仍受转换器和实际内核版本限制。任一 URI 行解析失败时，整个导入失败。

这不是任意完整 Mihomo 配置文件的无损导入接口：仅有 `proxy-providers` 而没有内联 `proxies` 的文件不满足当前结构检查，生成运行配置时还会使用本项目的受管配置逻辑。

`POST /v1/profiles/{id}/activate` 切换活动配置并生成运行配置；内核健康运行时调用 reload。**当前实现不会在内核停止时额外执行 `mihomo -t`，也没有在配置 reload 成功后完整回读所有运行字段。** 最终语义校验在启动内核时执行，不能把“激活成功”当作“代理连通已验证”。

`POST /v1/profiles/{id}/update` 重新下载并更新缓存；若目标为活动配置，则生成和应用运行配置。失败时尝试恢复旧缓存和配置，客户端仍应重新查询活动状态。

### 重命名与删除

`PATCH /v1/profiles/{id}` 要求非空 `name`，重名或空名称返回 `400`。

`DELETE /v1/profiles/{id}` 返回剩余配置数组。内核健康运行时禁止删除最后一个活动配置，返回 `409`。删除活动配置且还有其他配置时，后端会调整活动项并尝试应用；客户端应重新读取列表和状态，而不是自行推算下一项。

## 6. 代理组、节点与规则

### ProxyGroup 与 ProxyNode

```json
{
  "success": true,
  "data": [
    {
      "name": "Auto",
      "type": "Selector",
      "now": "示例节点",
      "nodes": [{"name":"示例节点","type":"socks5","delay":73}]
    }
  ]
}
```

组包含 `name`、`type`、`now` 和 `nodes`；每个节点包含 `name`、`type`、`delay`。组类型来自内核，客户端不要依赖固定大小写。节点顺序保留内核组的原始顺序，`now` 才是当前节点；某些组类型会由内核自动选择节点。

`delay` 单位为毫秒：正数为测量值，`-1` 表示失败/超时，`-3` 表示未测试。`-2` 是代码中为“测试中”保留的显示状态，当前测速接口不是异步任务，不会据此提供任务轮询。

`GET /v1/proxy-groups` 在内核不可用时一般返回 `502`。

`PUT /v1/proxy-groups/{group}` 接受节点完整原始名称。选择后回读组列表，只有 `now` 一致才成功；实际未切到目标可返回 `409`，组不存在可返回 `404`，内核拒绝选择或回读失败返回 `502`。并非每一种内核组类型都支持手动切换。

### ProxyDelayResponse

```json
{"success":true,"data":{"name":"示例节点","delay":73}}
```

`POST /v1/proxy-delay` 必须提供 `group` 和 `name`。使用后台配置的测速地址，单次内核测速参数为 5000 毫秒；调用方不能通过此接口传入任意测速 URL 或超时。内核不健康返回 `409`，组或节点不存在返回 `404`，测速请求失败返回 `502`；正常响应中的非正测量值会转换为 `-1`。

### Rule

```json
{"success":true,"data":[{"content":"example.com","type":"Domain","policy":"Auto"}]}
```

`content` 是内核规则的 payload，不一定是完整的逗号分隔规则文本；`type` 是规则类型，`policy` 是目标策略。当前接口只读，来自内核实际规则列表；内核不可用一般返回 `502`。没有添加、删除、排序规则的公共接口。

## 7. 日志

### 实时日志流

`GET /v1/logs/stream?level=info` 将 `level` 原样传给内核；当前 Manager 不统一校验级别。省略时使用上游的默认行为。

成功响应头包括 `Content-Type: text/event-stream` 和 `Cache-Control: no-cache`，但响应体是**内核原始字节流**，不是 Manager 统一生成的标准 SSE。实际可能是逐行 JSON：

```json
{"type":"info","payload":"示例日志"}
```

管理器取得内核成功响应后会立即刷新响应头，不再额外等待正文；内核自己的 HTTP 日志接口仍可能直到第一条匹配日志才响应。网页网关应独立建立下游 SSE 并保持心跳，不能把这一等待等同于代理故障。

部分上游版本使用带 `data:` 前缀的 SSE。不能仅凭响应头就让浏览器 `EventSource` 直接消费，网页后端应兼容解析后再输出自己的标准流格式。

```bash
manager_socket="$HOME/.local/state/mihomo-loongnix-test/run/daemon.sock"
curl -N --unix-socket "$manager_socket" 'http://localhost/v1/logs/stream?level=info'
```

该示例要求测试内核已启动；未启动时通常返回 `502`。流没有统一 JSON 外层、历史回放、事件 ID 或断点续传机制。断开请求会取消上游连接；流开始后的失败可能只表现为断流，无法再改变已经发送的 HTTP 状态。

日志正文不保证已脱敏，可能含访问目标或配置诊断。页面展示应按普通文本处理，并控制可见缓存长度。

### LoggingStatus

```json
{
  "success": true,
  "data": {
    "enabled": false,
    "current_file_bytes": 0,
    "total_bytes": 0,
    "max_file_bytes": 10485760,
    "max_backups": 3
  }
}
```

| 字段 | 类型与含义 |
| --- | --- |
| `enabled` | 布尔；磁盘记录开关 |
| `current_file` | 可选字符串；当前或最近的日志文件名 |
| `current_file_bytes` | 整数；该文件字节数 |
| `total_bytes` | 整数；受管日志及轮转分卷总字节数 |
| `max_file_bytes` | 整数；单文件轮转上限，默认 10485760 |
| `max_backups` | 整数；历史分卷数，默认 3 |
| `last_error` | 可选字符串；最近记录错误 |

`PUT /v1/logging` 持久化磁盘记录开关。`enabled=true` 表示记录器启用，不保证此时已经从内核收到日志；还应看 `last_error`。关闭会结束专用订阅并关闭文件，但保留已有日志。

实时日志与磁盘开关独立，关闭落盘仍可查看实时流。默认文件为 `mihomo-runtime.log`，轮转分卷为 `.1`—`.3`。当前接口不能下载历史文件、清空磁盘日志或修改轮转参数。

## 8. 调用示例

先按 [独立测试说明](LOONGNIX.md) 启动测试管理器。以下写请求仅对指定的测试 socket 执行；示例 URL 不提供真实订阅服务，需替换为本机提供的假数据链接。

```bash
manager_socket="$HOME/.local/state/mihomo-loongnix-test/run/daemon.sock"

# 查询状态和配置列表
curl --unix-socket "$manager_socket" http://localhost/v1/status
curl --unix-socket "$manager_socket" http://localhost/v1/profiles

# 导入配置，保存返回 data.id 以供后续操作
curl --unix-socket "$manager_socket" -X POST http://localhost/v1/profiles \
  -H 'Content-Type: application/json' \
  --data '{"name":"测试配置","url":"http://127.0.0.1:18080/profile.example.yaml"}'

# 将下面的 ID 替换为导入返回的 ID
profile_id='replace-with-returned-id'
curl --unix-socket "$manager_socket" -X PATCH "http://localhost/v1/profiles/$profile_id" \
  -H 'Content-Type: application/json' --data '{"name":"重新命名的测试配置"}'

# 测试配置存在且需要验证实际内核运行时再执行
curl --max-time 65 --unix-socket "$manager_socket" -X POST http://localhost/v1/core/start
curl --unix-socket "$manager_socket" -X POST http://localhost/v1/core/stop
```

## 9. 兼容接口与维护入口

当前路由额外保留 `GET /api/v1/ping`、`GET /api/v1/daemon/info` 和 `GET /api/v1/daemon/config-dir`，供本机兼容和诊断使用。部分源码中仍有旧处理函数或客户端方法，但未注册到当前路由，不能据此认为接口存在。新网页优先使用本文 `/v1` 接口。

安装/卸载服务、授权用户、替换内核二进制不属于本文日常管理 API。管理器本身不提供网页登录、CORS 或远程监听；可选独立 Web 网关实现单管理员会话，详见 [网页接入](WEB_INTEGRATION.zh-CN.md)。

文档核对来源：

- [路由注册与响应封装](../mihomotui/daemon.go)
- [IPC 权限](../mihomotui/ipc_auth.go)
- [状态、内核、TUN、端口、节点和日志接口](../mihomotui/manager_api.go)
- [配置处理](../mihomotui/manager_profiles.go)
- [配置格式转换](../mihomotui/profile_converter.go)
- [Go 客户端封装](../mihomotui/manager_client.go)
- [节点与规则结构](../mihomotui/mihomo_api_models.go)
- [磁盘日志实现](../mihomotui/manager_logging.go)

## 10. 可选 Web 生命周期

以下路由由管理器提供，即使 Web 关闭仍可调用。它们不操作核心、订阅、端口或 TUN，普通根状态查询也不等待网页健康检查。

| 方法与路径 | 权限与请求 | 结果 |
| --- | --- | --- |
| GET /v1/web/status | socket 只读组及以上；无参数 | 200，WebStatus；组件未安装是有效状态 |
| POST /v1/web/start | operator/root；无参数 | 200，已确认 running=true |
| POST /v1/web/stop | operator/root；无参数 | 200，已确认服务停止 |

WebStatus 示例（虚构）：

```json
{"success":true,"data":{"installed":true,"configured":true,"state":"stopped","service_active":false,"healthy":false,"running":false,"public_url":"https://mihomo.example.invalid","observed_at":"2026-09-06T00:00:00Z"}}
```

installed 表示 unit 和程序／静态产物存在；configured 表示正式私有配置有效。state 枚举 not_installed/stopped/starting/running/stopping/failed/unknown。service_active 为 systemd 实际状态，healthy 要求本机健康响应的 app 与 PID 匹配正在运行的 Web unit，running 同时要求服务活动和健康。public_url 可空，error_code/message 可选，不返回配置路径、密码哈希或令牌。生产网页配置不允许 test_mode；支持 auth_mode=password/external，external 配置不需要密码哈希。该字段要求升级到支持外部认证的 manager，旧版本不能识别。

错误仍保留 `success=false,error`，增加可选 `error_code`。403 由 IPC 鉴权返回；409 的 code 为 WEB_NOT_INSTALLED、WEB_NOT_CONFIGURED、PORT_IN_USE、BUSY；500 为 WEB_START_FAILED/WEB_STOP_FAILED；503/WEB_STATUS_UNAVAILABLE 表示无法查询或此环境没有控制器；504/RESULT_UNKNOWN 表示超时待确认。错误后回读状态，不能断言操作未执行。

仅操作固定 mihomo-web.service，请求不能指定服务、命令、健康 URL 或路径。独立 Web 锁不占用代理 actionMu，并与 Web 部署工具共享文件锁；已有操作立即返回 BUSY。正常 start/stop 重复调用幂等。启动前验证配置、产物、端口，失败只尝试停止本次启动的 Web unit；停止／清理本身失败时不保证完全恢复，保留状态待确认。

查询窗口 3 秒，操作窗口 15 秒，启动失败清理额外最多 3 秒；TUI／CLI 请求窗口 20 秒。退出 TUI 不关闭 Web。首页与 `mihomo-tui web status|start|stop` 使用相同接口，均不安装组件或设自启。

影子测试模式或普通用户运行的管理器未注入假控制器时，路由返回 503，绝不退回生产 systemctl。生产安装需要升级此管理器并安装可选组件；版本号不替代接口探测。旧管理器返回 404 时，新 TUI 提示不支持，其他功能继续工作。
