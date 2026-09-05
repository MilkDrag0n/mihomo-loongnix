# Mihomo TUI 控件树与测试基准

本文是 UI 行为的权威测试清单。每个鼠标按钮和键盘触发必须进入同一回调，不允许存在只修改前端布尔值的第二条路径。

## 1. 全局控件树

```text
应用
├─ 顶部五页导航
│  ├─ 1 首页
│  ├─ 2 配置
│  ├─ 3 节点
│  ├─ 4 规则
│  └─ 5 日志
└─ 全局键盘
   ├─ 1..5：切页，并刷新目标页
   ├─ Tab / Shift+Tab：按控件树顺序前进/后退焦点
   ├─ ←→：在顶部导航选择页面，Enter 进入；↑↓ / j k：移动列表或表格
   ├─ Enter / Space：执行当前按钮或表格动作
   ├─ PgUp / PgDn：节点、规则上一页/下一页
   ├─ Home / End：节点、规则第一页/最后一页
   ├─ /：聚焦当前页筛选框
   ├─ r：不在输入框时刷新当前页
   ├─ Esc：退出输入焦点；再次按返回顶部导航
   └─ q：不在输入框时退出
```

验收：`KEY-001`—`KEY-010` 分别执行上述十组操作；切页后旧日志流必须关闭，输入框内的 `j/k/q/r/数字` 必须作为文本输入，不能触发全局动作。

`THEME-001`：背景和正文沿用终端默认色，不铺设固定背景。边框、文字状态、下划线与局部反色区分焦点；80 列 × 24 行时首页状态、操作栏和底部快捷键完整可见。

`KEY-011`：弹窗内 `Tab` 切换按钮、`Esc` 取消并恢复原焦点，`q` 不退出应用；展开的下拉框自行处理按键。

`KEY-012`：输入框将焦点委托给内部编辑器时，`Tab` / `Shift+Tab` 仍能离开和返回输入框。

`layout_test.go` 使用独立 Unix socket 上的假数据服务和模拟终端覆盖五页布局、输入框与弹窗焦点、下拉框快捷键隔离；不会调用正式系统服务。

## 2. 首页

```text
首页
├─ 内核 / TUN / 当前连接三个状态区（GET /v1/status）
│  ├─ systemd service_active
│  ├─ controller_healthy
│  ├─ running（前两者同时为真）
│  ├─ TUN configured/runtime_enabled/interface_present/enabled
│  ├─ active_profile
│  ├─ proxy_port（运行时取 Mihomo `/configs` 的实际 mixed-port）
│  └─ current_group/current_node（取默认代理组的实际 `now`）
├─ 端口输入 / 应用端口
│  ├─ 只接受 1-65535 的十进制数字
│  ├─ Enter 与“应用端口”按钮调用同一 PUT /v1/proxy-port
│  ├─ pending：禁止重复执行，不提前修改首页实际端口
│  ├─ 成功：以后端响应和 Mihomo `/configs` 回读值重绘
│  └─ 失败：恢复旧配置并显示真实错误
├─ 启动代理 / 停止代理
│  ├─ POST /v1/core/start 或 /stop
│  ├─ pending：禁止重复执行，显示“等待后端状态回读”
│  ├─ 成功：使用响应中的 ManagerStatus 重绘
│  └─ 失败：弹出真实错误并重新 GET /v1/status
├─ 开启 TUN / 关闭 TUN
│  ├─ PUT /v1/tun
│  ├─ 请求目标取反 configured，而非取反按钮文字
│  └─ 成功必须经过配置、内核和网卡三重回读
└─ 刷新：GET /v1/status
```

- `HOME-001`：服务 inactive，首页必须显示停止。
- `HOME-002`：伪造 service active、控制接口失败，必须显示“服务活动，控制接口异常”，不能显示运行。
- `HOME-003`：启动成功后核对 systemd、9090 和页面三处一致。
- `HOME-004`：快速连续点击启动，只产生一个在途请求。
- `HOME-005`：TUN 配置为真但内核关闭，显示“已预设，尚未生效”。
- `HOME-006`：TUN 开启成功后 `/configs`、`ip link show mihomo-tui-tun`、首页一致。
- `HOME-007`：制造重载失败，配置和页面均恢复旧值。
- `HOME-008`：首页代理端口与 `/configs` 的 `mixed-port` 一致，当前节点与默认代理组 `/proxies/{group}` 的 `now` 一致。
- `HOME-009`：输入框 Enter 与“应用端口”按钮产生相同 PUT；成功后新端口监听、旧端口释放，Manager 重启后仍保留。
- `HOME-010`：越界值、9090 和已占用端口均拒绝；热重载或回读失败时配置、监听和页面恢复旧端口。

## 3. 配置页

```text
配置
├─ 名称输入
│  ├─ 导入时可选
│  └─ 重命名时为新名称
├─ URL 输入：仅提交给 POST /v1/profiles，成功后清空
├─ 导入
│  ├─ 下载到内存
│  ├─ 识别/转换/验证
│  ├─ 原子写私有缓存与 secrets
│  └─ 首个 Profile 自动成为活动配置
├─ 配置列表（GET /v1/profiles）
│  ├─ 活动标记只取 active 字段
│  └─ 来源只显示脱敏 scheme + host
├─ 激活：POST /v1/profiles/{id}/activate
├─ 更新：POST /v1/profiles/{id}/update
├─ 重命名：PATCH /v1/profiles/{id}
├─ 删除：确认弹窗 → DELETE /v1/profiles/{id}
└─ 刷新：GET /v1/profiles
```

- `PROF-001`：导入合法 provider YAML URL，缓存存在、权限 `0600`。
- `PROF-002`：分别导入完整 YAML、Base64 YAML、URI、Base64 URI。
- `PROF-003`：HTML、空正文、部分无效 URI 全部拒绝，不新增 Profile。
- `PROF-004`：重复 URL 返回冲突，提示使用更新。
- `PROF-005`：激活后生成配置只引用该 Profile，运行内核成功重载。
- `PROF-006`：激活重载失败时活动标记、配置文件和内核恢复原 Profile。
- `PROF-007`：更新失败保留旧缓存哈希和更新时间。
- `PROF-008`：重命名不改变 ID、URL、缓存或活动状态。
- `PROF-009`：删除非活动 Profile 不影响内核。
- `PROF-010`：运行时删除最后一个活动 Profile 被拒绝。
- `PROF-011`：名称、来源和错误弹窗不出现 URL token/query/path。

## 4. 节点页

```text
节点
├─ 代理组下拉：选择组只切换当前数据集，不调用内核变更
├─ 筛选：名称或类型，不改变实际 now
├─ 刷新：GET /v1/proxy-groups
├─ 节点表
│  ├─ “● 当前”只比较组的 now
│  ├─ 行顺序：严格采用 Mihomo all 数组，筛选和测速后都不重排
│  ├─ 终端显示名：地区标识与纯文本节点名使用独立单元；地区单元用双宽空白强制重置终端光标，台湾旗帜使用 TW 回退，后端节点名不变
│  ├─ 方向键/j/k 移动候选焦点，左右键移动表格列
│  ├─ 普通列 Enter 与“使用所选节点”调用同一切换动作
│  └─ 延迟右侧“测速”列
│     ├─ 鼠标单击与键盘 Enter/Space 调用同一 POST /v1/proxy-delay
│     ├─ pending：该节点显示“测试中”，重复点击不重复发请求
│     ├─ 成功：显示 Manager 返回的 Mihomo 实测延迟
│     └─ 失败：恢复测速前延迟，弹出后端错误
├─ 上一页 / 下一页
└─ 使用所选节点
   ├─ PUT /v1/proxy-groups/{group}
   └─ 响应 now 与目标一致后才更新“当前”标记
```

- `NODE-001`：组切换后页码归零，候选恢复为该组实际 `now`。
- `NODE-002`：筛选后页码归零，清空筛选恢复全部节点。
- `NODE-003`：0、1、整页、整页+1 条数据的页码和边界正确。
- `NODE-004`：最后一页 `PgDn` 不越界，第一页 `PgUp` 不越界。
- `NODE-005`：鼠标按钮和表格 Enter 产生相同 PUT 请求。
- `NODE-006`：PUT 成功但回读 `now` 不一致时显示失败，不能移动当前标记。
- `NODE-007`：切换成功后同时核对 Mihomo `/proxies` 和页面。
- `NODE-008`：后端 `all` 依次返回高延迟、低延迟、未测试节点，页面仍保持该顺序；测速和刷新后不变。
- `NODE-009`：点击单行“测速”与在该格按 Enter/Space 产生相同 POST，返回值与 Mihomo 健康检查一致。
- `NODE-010`：测速失败恢复原延迟；快速连点同一节点只有一个在途请求。
- `NODE-011`：地区标识和节点名称必须位于独立表格单元，二者之间的双宽空白会迫使 tcell 在名称前绝对定位光标；台湾旗帜使用 `TW` 回退。初次绘制、选中当前行、切换后重绘新旧两行均不能遮住节点名首字，选择和测速请求仍携带未替换的完整原名。

## 5. 规则页

```text
规则
├─ 筛选：content/type/policy
├─ 刷新：GET /v1/rules
├─ 只读表格
└─ 上一页 / 下一页
```

- `RULE-001`：表格与 Mihomo `/rules` 数量、顺序一致。
- `RULE-002`：按内容、类型、策略筛选均正确且大小写不敏感。
- `RULE-003`：筛选和刷新将页码归零，分页不重复、不遗漏。
- `RULE-004`：页面不存在编辑、禁用、模式切换按钮。

## 6. 日志页

```text
日志
├─ 实时流：进入页面 GET /v1/logs/stream，离开页面取消
├─ 筛选：只影响当前内存视图
├─ 级别：全部/DEBUG/INFO/WARNING/ERROR，只影响视图
├─ 开启/关闭磁盘记录
│  ├─ GET /v1/logging/status 决定按钮文字
│  ├─ PUT /v1/logging 执行
│  └─ 使用响应重绘，不做乐观切换
├─ 暂停/继续显示：仅暂停重绘，实时流仍进入最多 2000 条内存缓冲
├─ 清空界面：只清内存，不删除磁盘文件
├─ 刷新大小：GET /v1/logging/status
└─ 状态栏
   ├─ enabled
   ├─ 当前文件（关闭时为最近文件）及大小
   ├─ 全部受管日志总大小
   └─ 10 MiB × 3 轮转策略
```

- `LOG-001`：首次安装 enabled=false，日志目录没有因 TUI 启动而新增文件。
- `LOG-002`：磁盘记录关闭时，实时日志仍持续显示。
- `LOG-003`：开启后状态来自 GET/PUT 响应，文件大小随实际流量增长。
- `LOG-004`：关闭后文件句柄和专用上游流关闭，大小停止增长。
- `LOG-005`：重启 TUI 不改变开关；重启 Manager 保留最后设置。
- `LOG-006`：达到 10 MiB 后轮转，只存在当前文件和最多 `.1`—`.3`。
- `LOG-007`：状态栏当前大小、总量与 `stat`/`du` 一致。
- `LOG-008`：暂停、筛选、级别和清空都不改变磁盘记录开关。
- `LOG-009`：离开日志页后仅在磁盘记录开启时允许存在 Manager 专用日志流。

## 7. 安全与真机验收

- `SEC-001`：运行 `scripts/check-secrets.sh`，工作区、暂存区和 Git 历史扫描通过。
- `SEC-002`：仓库外 `secrets`、Profile 缓存和日志权限符合 API 文档。
- `SEC-003`：非 operator 用户不能连接可写接口；operator 能操作日常功能但不能安装/替换内核。
- `LIVE-001`：Loongnix 25.1 LoongArch ABI2 构建和启动成功。
- `LIVE-002`：备用状态目录和端口完成影子测试后再切换双服务。
- `LIVE-003`：当前受管 mixed port 的 HTTP/SOCKS、9090 API、节点选择和规则匹配产生真实流量。
- `LIVE-004`：TUN 测试前保持 SSH/Tailscale 回滚会话；启停后管理连接、Docker 和原有防火墙仍正常。
- `LIVE-005`：切换失败时恢复旧二进制、unit、配置和运行状态。
