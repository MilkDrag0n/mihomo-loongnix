# Web 首版验证记录

日期：2026-09-06；环境：server-pc，Loongnix 25 / loongarch64，Go 1.26.1，Node 24.20.0。此记录针对源代码与独立假数据环境，不代表正式 Web 已安装。

- Go 全包测试、go vet 通过；TUI 模拟终端覆盖 80×24，新增 Web 控件未遮挡当前配置。
- Web 前端类型检查、3 个组件交互测试和正式模式静态构建通过。组件验证规则分页／筛选、导入链接清理和特殊代理组名称。
- Go 网关验证 Cookie 登录／注销／空闲过期、生产 Secure Cookie 标志、CSRF／Host、摘要令牌范围、字段／大小校验、路径白名单、写入互斥、状态分类、日志分块与注销关闭上游。
- 单独运行 66 秒静默流测试，收到至少 4 次心跳；普通测试默认跳过这一较长验收。
- Python 全部 26 项测试通过，其中 Web 部署包完整性、篡改、路径穿越、符号链接、自定义 unit、架构与来源、保持关闭回退共 7 项。
- 浏览器实测：登录、切换节点并回读、筛选后逐个测速、导入假配置、规则筛选、实时日志、暂停／清空和退出。
- 桌面默认视口与 390×844 手机视口检查；深浅主题可用，手机文档宽度为 390 像素，没有横向页面溢出；浏览器检查未发现控制台警告或错误。
- pnpm 生产依赖审计未报告已知漏洞（检查当时结果）。
- 假数据预览网关一次空闲采样：RSS 9728 KiB，进程累计 CPU 显示 0.0%，7 个文件描述符；不是峰值或资源承诺。
- 全程未重启正式 mihomo-manager.service／mihomo.service；只读检查两者 active。

可复跑：`./scripts/test-web.sh`、`go test -count=1 ./...`、`go vet ./...`、`MIHOMO_WEB_LONG_STREAM_TEST=1 go test -count=1 ./internal/webgateway -run TestQuietStreamSurvivesMinute -v`。

尚未完成真实环境验收：sudo 正式安装／升级／回退、普通 Web 服务账号权限、实际 HTTPS 反向代理与 Cookie、Homepage 真实 widget、原生 systemd 启停与异常恢复。这些检查需要按部署说明在具体入口和安装版本上执行；不能用单元测试或 --check 代替。
