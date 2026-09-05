# 开发约定

## 唯一开发环境

- 主仓库为 https://github.com/MilkDrag0n/mihomo-loongnix，基于 WangZhongDian/mihomo-tui。
- server-pc 上唯一常驻开发仓库：`/home/server/projects/mihomo-loongnix`。通过 SSH 在服务器本地磁盘工作；共享文件夹只交换交付文件，不保存第二份常驻开发仓库。
- `origin` 指向 MilkDrag0n/mihomo-loongnix；`upstream` 指向 WangZhongDian/mihomo-tui，仅用于读取和比较，不向原作者仓库推送。
- 主分支为 `main`。普通小改动可按用户授权在 main 提交；较大功能使用短期分支，完成后合并回 main。
- 先检查工作区、远程地址、当前提交与正在运行的程序；不能仅凭目录名、页面数量或二进制里的旧提交号判断版本。`vcs.modified=true` 表示构建包含未提交内容，不能据此确认完整源码来源。
- 本文件随源码提交，不得重新把 `AGENTS.md` 加入忽略规则。

## 目录与环境边界

| 用途 | server-pc 路径 |
| --- | --- |
| 开发源码 | `/home/server/projects/mihomo-loongnix` |
| 可部署构建 | `/home/server/.local/share/mihomo-loongnix/builds/<完整提交号>/` |
| 日常检查构建 | `/home/server/.local/share/mihomo-loongnix/checks/`，不可部署 |
| 独立测试状态 | `/home/server/.local/state/mihomo-loongnix-test/` |
| 正式 TUI / 管理器 | `/usr/local/bin/mihomo-tui` |
| 现有正式 Mihomo | `/usr/local/bin/mihomo`，以 systemd ExecStart 为准 |
| 全新安装默认内核 | `/var/lib/mihomo-tui/bin/mihomo` |
| 正式运行数据 | `/var/lib/mihomo-tui` |
| 用户侧配置 | `/home/server/.config/mihomo-tui` |
| 恢复备份 | `/home/server/backups/mihomo-loongnix/<日期时间>/`，权限 0700 |

不得将源码中的 `bin/` 或共享目录当作正式内核位置。不要修改全局代理、系统 Git 配置或其他项目的开发环境来解决本仓库的问题。

## 修改与验证

1. 开始前检查 `git status --short --branch`。保留用户改动，不使用清空工作区或强制重置来跳过冲突。
2. 只格式化本次修改的 Go 文件，避免无关的全仓格式化。
3. 在服务器执行适合本次修改的检查：

   ```bash
   go test -count=1 ./...
   go vet ./...
   git diff --check
   ```

4. 编译检查产物放在仓库外：

   ```bash
   check_dir="$HOME/.local/share/mihomo-loongnix/checks"
   mkdir -p "$check_dir"
   CGO_ENABLED=0 GOOS=linux GOARCH=loong64 go build -trimpath -o "$check_dir/mihomo-tui" .
   ```

5. 暂存预期文件后执行 `./scripts/check-secrets.sh`，该扫描器检查 Git 索引。通过 `./scripts/install-git-hooks.sh` 启用提交钩子。
6. 文档修改检查路径、链接、命令与实际实现。公开使用说明只维护中文 `README.md`，不再维护另一份 README。

## 独立测试

- 普通测试不得重启正式服务、修改正式端口、导入真实订阅或启用 TUN。需要部署或实际系统操作时遵循用户明确授权；开发或文档修改本身不等于部署授权。
- 使用专用状态目录、专用 socket；明确设置 `MIHOMO_TUI_SHADOW=1`，测试混合端口 `17890`，控制端口 `19090`。先确认端口未占用。
- 测试命令清除 `MIHOMO_TUI_CORE_SERVICE`，防止测试管理器控制正式 systemd 内核；TUI 也必须显式指定测试 socket，否则默认可能连到正式后台。
- 使用 `testdata/` 中的假数据。测试状态不得从正式目录直接复制，以免带入真实订阅、自动启动或 TUN 设置。
- 测试启动和清理方式见 [docs/LOONGNIX.md](docs/LOONGNIX.md)。记录自己启动的 PID，只停止自己的测试进程，禁止用宽泛的 `pkill mihomo`。

## 提交、构建与推送

- 流程：修改 → 检查 → 提交 → 从干净提交构建 → 按授权部署验证。
- 提交前读取暂存差异，保证只包含当前任务内容。推送只针对自己的 `origin`，不得强推 main。
- 使用已配置的 Git 身份或已连接的 GitHub 身份，不伪造身份，不将访问令牌写入远程 URL、脚本或文档。
- 有服务器 Git 推送凭据时正常提交并推送。当前服务器没有推送凭据，可将服务器已验证的提交导出为 Git bundle，通过已登录 GitHub 的 Mac 临时转送，保持提交号和文件树不变。Mac 仅转送提交对象，不修改代码，完成后清理临时副本。推送后服务器获取 origin/main，核对提交号并保持一致。
- 若使用 GitHub 连接器提交，从服务器暂存内容创建一次完整提交，获取远程提交后逐文件核对；不要留下服务器与 GitHub 两份不同的“实际源码”。
- 正式构建使用 `./scripts/build-release.sh`，要求工作区干净，产物按完整提交号归档。不得部署标记为 `vcs.modified=true` 的程序。
- 每次部署额外记录日期、源码提交、程序校验值、内核版本与校验值、服务状态和回滚路径。`mihomo-tui version` 中的产品版本不能替代提交号。

## 部署与回滚

- 正式服务为 `mihomo-manager.service` 和 `mihomo.service`，不是旧的一体式 `mihomo-tui.service`。
- 部署前备份正式程序、实际使用的内核、unit 文件及完整 `/var/lib/mihomo-tui`，同时记录服务启用和运行状态。root 私有文件无法读取时，明确指出备份覆盖范围，不能宣称已完整备份或继续部署。
- 安装器仍存在“先停止服务、后检查内核”的失败恢复缺口；升级前先验证所有依赖和路径，准备回滚命令。
- 回滚恢复本次部署前保存的二进制、数据及双服务配置，恢复记录中的启用和运行状态。不要照搬启动旧 `mihomo-tui.service` 的命令。
- 验证不仅看界面：检查 systemd、控制接口、实际代理连通性及日志；无法验证的项目需明确说明。

## 清理与敏感信息

- 删除旧目录或二进制前确认未被使用；先备份，再验证归档内容与源文件一致，最后清理明确的目标路径。
- 保留集中恢复备份，不再散落 `final`、`next`、`ui-fix` 等无源码对应的正式候选程序。
- 订阅链接及正文、API secret、运行配置、私钥、日志、内核和备份不进入 Git。截图与输出避免暴露真实订阅凭据。
- 未获授权不要删除恢复备份或正式运行数据。迁移备份是历史资料，不是第二个开发目录。

## 多前端与接口文档

- 管理逻辑放在后端，TUI 和未来网页只负责交互，不各自实现一套内核、配置或路由控制。
- 修改路由、权限、请求字段、返回结构、错误码或日志格式时，同步更新 `docs/MANAGER_API.zh-CN.md`；网页接入边界变化时同步更新 `docs/WEB_INTEGRATION.zh-CN.md`。
- 文档以已注册路由和实际处理器为准，区分当前行为与未来设计。不把未注册的旧处理函数、未实现的网页能力或未经验证的回滚保证写成现有功能。
