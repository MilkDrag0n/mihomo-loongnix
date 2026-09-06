# 开发约定

## 工作目录与源码

- server-pc 唯一常驻开发仓库：/home/server/projects/mihomo-loongnix，通过 SSH 在服务器本地磁盘开发。共享目录只交换文件，不保存第二份开发仓库。
- 主仓库：https://github.com/MilkDrag0n/mihomo-loongnix，基于 WangZhongDian/mihomo-tui。origin 指向用户仓库；upstream 只用于读取原作者代码。
- 开始前检查 git status --short --branch，保留用户修改；不强制重置、清空工作区或强推 main。
- 较大修改使用短期分支，完成后合并 main；普通小修改可以在 main 提交。
- 程序可能来自未提交工作区，不能只凭目录名、页面数量或内嵌提交号判断实际源码。

## 日常开发与部署

流程：修改 → 按需测试 → 按用户授权运行 ./scripts/deploy.sh。提交与推送单独进行，不是部署前提。

- 部署当前目录中的代码，包括未提交修改；不要求版本号、干净工作区或按提交归档。
- ./scripts/deploy.sh 使用普通用户构建 TUI／管理器；已安装 Web 时也构建 Web。全部构建成功后才申请 sudo 替换程序。
- ./scripts/deploy.sh --build-only 只构建，不替换程序或重启服务。
- 构建失败直接停止；更新或启动失败直接报错并输出相关服务日志。不制作部署备份、快照，不自动回滚，不做外网代理探测或配置前后比较。
- 重启管理器；Web 原来运行就重启，原来关闭就保持关闭。日常脚本不更新或启停 Mihomo 内核，不覆盖订阅、运行数据、Web 域名或认证配置，不改变服务自启设置。
- Web unit 内容变化时才执行 daemon-reload。不要为每次更新无条件重载系统服务。
- 普通代码、文档修改不等于正式部署授权；未获授权时使用独立测试和 --build-only。
- 首次安装另见 README 和 docs/WEB_INTEGRATION.zh-CN.md；scripts/deploy-web.py 只负责首次可选 Web 安装。
- 不自动删除历史备份、旧按提交构建或 /opt/mihomo-web/releases/；它们不再参与日常部署。源码中不保留另一套旧部署流程。

## 固定目录

| 用途 | server-pc 路径 |
| --- | --- |
| 源码 | /home/server/projects/mihomo-loongnix |
| 当前构建 | ~/.local/share/mihomo-loongnix/build/current/ |
| 独立检查产物 | ~/.local/share/mihomo-loongnix/checks/ |
| 测试状态 | ~/.local/state/mihomo-loongnix-test/ |
| TUI／管理器 | /usr/local/bin/mihomo-tui |
| 正式 Mihomo 内核 | /usr/local/bin/mihomo，以实际服务启动参数为准 |
| 全新安装默认内核 | /var/lib/mihomo-tui/bin/mihomo |
| 正式管理器数据 | /var/lib/mihomo-tui |
| 用户配置 | ~/.config/mihomo-tui |
| Web 固定运行目录 | /opt/mihomo-web/runtime/，current 链接指向这里 |
| Web 私有配置 | /etc/mihomo-web/config.json |
| Web 状态 | /var/lib/mihomo-web |
| 历史备份 | ~/backups/mihomo-loongnix/ |

首次使用新日常脚本时，将 Web current 从旧版本目录指向固定 runtime，不删除旧版本目录。不要手动覆盖旧版本归档或把它当成新运行目录。

## 检查与测试

- 按改动选择检查，不把所有测试放到每次部署中。Go 修改运行适当的 go test、go vet；部署修改运行 PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/tests -v；Web 使用 ./scripts/test-web.sh 独立验证。
- 仅格式化本次修改的文件，检查 git diff --check。
- 普通测试不操作正式服务、不连接生产 socket、不复制真实订阅；使用 testdata 假数据。
- 测试管理器设置 MIHOMO_TUI_SHADOW=1，测试混合端口 17890、控制端口 19090；清除 MIHOMO_TUI_CORE_SERVICE，显式指定独立 MIHOMO_TUI_SOCKET。启动前确认端口未占用。
- Web 预览使用独立测试 socket、状态和 19080 端口。测试与日常构建均用普通用户。
- 只停止自己创建的测试进程，不使用宽泛 pkill。具体步骤见 docs/LOONGNIX.md。
- 部署成功提示只代表启动检查通过，不等于业务接口、公网入口或真实代理已经验收；报告实际验证范围。

## 提交与资料保护

- 提交前显式暂存本次文件、读取暂存差异，并运行 ./scripts/check-secrets.sh；可用 ./scripts/install-git-hooks.sh 启用钩子。部署不重复执行这些提交检查。
- 使用现有 Git 身份，不伪造身份，不向 upstream 推送。服务器无推送凭据时，可导出 Git bundle，经 Mac 临时 bare 仓库转送，提交对象不变；完成后清理临时副本，服务器同步 origin/main。
- 订阅、密钥、运行配置、真实日志、内核、构建和备份不提交到 Git；截图和输出避免暴露真实凭据。
- 不修改全局代理、系统 Git 配置或其他项目环境来解决本项目问题。
- README 仅维护中文；部署规则变化时同步 README、docs/LOONGNIX.md、docs/WEB_INTEGRATION.zh-CN.md，避免文档继续要求旧提交号或自动回滚。

## 界面与接口

- TUI 背景及正文使用终端默认色，不铺固定背景；保留五页导航，80 列 × 24 行必须可操作。
- 管理逻辑在后端；TUI 和 Web 只负责交互，不各自实现内核、配置、路由控制。
- 路由、权限、字段、错误码或日志格式变化时同步 docs/MANAGER_API.zh-CN.md 和 docs/WEB_API.zh-CN.md，按实际实现写文档。
- Web 生命周期独立，TUI 首页／CLI 通过 /v1/web/* 控制；关闭 Web 不停止代理。
- 已有外部访问保护的部署可选择 external 免 Web 密码；不推断其他部署也应关闭密码。现有免密码配置在日常更新中原样保留。
