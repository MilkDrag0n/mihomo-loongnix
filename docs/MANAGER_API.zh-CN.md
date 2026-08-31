# Mihomo Manager API

## 1. 边界与连接

`mihomo-manager.service` 以 root 运行，通过 `/run/mihomo-tui/daemon.sock` 提供 HTTP API。普通 TUI 用户必须属于 `mihomo-tui-operator` 组。Socket 使用 peer credentials 鉴权，不监听 TCP，也不向 TUI 返回 Mihomo secret。

TUI 只能调用 `/v1/*`。`mihomo.service` 独立运行 Mihomo，默认 mixed port 为 `127.0.0.1:7890`，external controller 为 `127.0.0.1:9090`。

生产 Manager 持久化受管 mixed port，默认 `7890`；控制端口固定为 `9090`。仅在显式设置 `MIHOMO_TUI_SHADOW=1` 时，独立状态目录使用隔离的 `17890/19090` 影子端口；详见 [Loongnix 部署说明](LOONGNIX.md)。

所有非流式响应使用：

```json
{"success": true, "data": {}}
```

失败响应使用：

```json
{"success": false, "error": "已脱敏的错误信息"}
```

常用状态码：`400` 请求/配置错误，`403` IPC 权限不足，`404` 资源不存在，`409` 实际状态冲突，`502` Mihomo 接口失败，`504` 状态回读超时。

## 2. 状态来源

- `core.running = systemd ActiveState=active && GET /version 成功`。
- `tun.enabled = core.running && GET /configs 的 tun.enable=true && mihomo-tui-tun 网卡存在`。
- `active_profile` 来自 Manager 持久配置中的活动 Profile。
- `proxy_port` 优先来自 Mihomo `/configs` 的实际 `mixed-port`，内核不可用时返回 Manager 将使用的受管端口。
- `current_group` 是运行配置的默认代理组；`current_node` 来自该组 `/proxies/{group}` 的实际 `now`。
- 代理组和节点来自 Mihomo `/proxies`；节点严格保留组 `all` 字段的原始顺序，不按名称或延迟重排。节点成功选择以重新读取的 `now` 为准。
- 规则来自 Mihomo `/rules`，不是本地配置推测值。
- 磁盘日志状态和大小来自 Manager 文件句柄及文件系统统计。

## 3. 接口

### 状态与内核

- `GET /v1/status`：返回 `core`、`tun`、`active_profile`、`proxy_port`、`current_group` 和 `current_node`。
- `PUT /v1/proxy-port`，请求 `{"port":7890}`：校验范围、控制端口冲突及本机占用，原子保存并生成配置；内核运行时热重载，只有 `/configs` 回读的 `mixed-port` 一致才成功，失败自动恢复旧端口。
- `POST /v1/core/start`：生成配置，使用实际 Mihomo 二进制执行 `-t` 语义校验，执行 TUN 安全预检、启动 `mihomo.service`，等待 systemd 与控制接口同时健康。首次校验可能需要下载缺失的 GeoIP 数据，因此客户端允许最长 60 秒响应时间。
- `POST /v1/core/stop`：停止 `mihomo.service`、清理项目 TUN 路由并回读停止状态。重复停止是幂等操作。
- `PUT /v1/tun`，请求 `{"enabled":true}`：提交配置、重载内核、检查运行配置与网卡；任一步失败自动恢复原配置和路由。

内核服务 active 但控制接口不健康时，`service_active=true`、`controller_healthy=false`、`running=false`，UI 必须显示异常而不是“已启动”。

### Profiles

- `GET /v1/profiles`：返回脱敏后的 Profile 列表。
- `POST /v1/profiles`：请求 `{"name":"可选名称","url":"https://..."}`，只接受 HTTP/HTTPS URL。
- `POST /v1/profiles/{id}/activate`：生成并应用该 Profile；失败恢复上一个 Profile。
- `POST /v1/profiles/{id}/update`：重新下载并验证；活动 Profile 应用失败时恢复旧缓存和配置。
- `PATCH /v1/profiles/{id}`：请求 `{"name":"新名称"}`。
- `DELETE /v1/profiles/{id}`：删除配置；内核运行时禁止删除最后一个活动 Profile。

导入先下载到内存并识别格式，再转换和结构验证并原子写入私有缓存。配置激活时，已停止的内核通过 `mihomo -t` 做最终语义校验；运行中的内核以 reload 接口是否接受及其后状态回读作为语义判定，避免测试进程与运行内核争用同一个 `cache.db`。支持：

- 带 `proxies` 的 Clash/Mihomo YAML 或 provider YAML；
- Base64 编码的上述 YAML；
- Base64 或明文 URI 节点列表；
- URI 转换支持 SS、VMess、VLESS、Trojan、Hysteria/Hysteria2、TUIC、SOCKS5。

只要其中一行 URI 不可解析，整个导入失败且不保存部分节点。

### 节点与规则

- `GET /v1/proxy-groups`：返回组名、类型、实际 `now` 和节点。
- `PUT /v1/proxy-groups/{group}`：请求 `{"name":"节点名"}`；发送选择后重新读取整个组，只有 `now` 一致才成功。
- `POST /v1/proxy-delay`：请求 `{"group":"代理组","name":"节点名"}`；Manager 先核对组和节点，再调用 Mihomo 实际节点健康检查，返回 `{"name":"...","delay":73}`。内核未运行返回 `409`，节点不存在返回 `404`，测速请求失败返回 `502`。
- `GET /v1/rules`：返回实际生效规则；筛选和分页仅在 TUI 内完成，不修改规则。

### 实时与磁盘日志

- `GET /v1/logs/stream`：转发 Mihomo 实时日志流；兼容当前内核的逐行 JSON 和部分版本使用的 `data:` SSE 格式。请求结束或离开日志页时立即取消上游连接。
- `GET /v1/logging/status`：返回 `enabled`、`current_file`、`current_file_bytes`、`total_bytes`、`max_file_bytes`、`max_backups` 和最近错误。
- `PUT /v1/logging`：请求 `{"enabled":true}`，启用或关闭 Mihomo 运行日志落盘。

磁盘日志初始默认关闭，设置持久化。启用后写入 `mihomo-runtime.log`，达到 10 MiB 后轮转，保留 `.1`—`.3` 三个历史分卷。关闭会取消专用上游日志订阅并关闭文件句柄，但不删除已有文件。Manager 自身诊断只进入 journald。

## 4. 权限与私密数据

- Profile URL 和 Mihomo API secret 位于运行目录的 `secrets/runtime.yaml`，权限 `0600`。
- Profile 正文位于 `subscriptions/cache/`，权限 `0600`。
- 受管日志目录权限 `0750`，文件 `0640`；TUI 通过 API 查看，不直接写文件。
- 列表接口只返回订阅地址的 scheme 和 host，不返回 userinfo、path、query 或 fragment。
- 安装/卸载、内核文件替换不属于日常 Manager API，仍需显式 root 命令。
