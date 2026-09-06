# Loongnix 开发、部署与回滚

主要目标为 Loongnix GNU/Linux 25 / LoongArch ABI2，Go 构建目标为 `linux/loong64`。全新安装步骤见 [README](../README.md)，协作规则见 [AGENTS.md](../AGENTS.md)。

## 固定目录

server-pc 的唯一开发仓库为 `/home/server/projects/mihomo-loongnix`。正式程序位于 `/usr/local/bin/mihomo-tui`，现有正式内核位于 `/usr/local/bin/mihomo`；以 `systemctl show mihomo.service -p ExecStart` 为实际依据。

构建产物放在 `~/.local/share/mihomo-loongnix/builds/<完整提交号>/`，测试状态放在 `~/.local/state/mihomo-loongnix-test/`。共享文件夹不作为常驻开发目录。

## 获取更新与检查

```bash
cd "$HOME/projects/mihomo-loongnix"
git status --short --branch
git remote -v
# 工作区干净时再获取更新；有修改时先提交或妥善保存。
git pull --ff-only origin main
./scripts/install-git-hooks.sh
go test -count=1 ./...
go vet ./...
git diff --check
```

修改、检查并提交后，执行 `./scripts/build-release.sh`。它将程序、构建来源和校验值放在同一个提交号目录，拒绝脏工作区和覆盖已有产物。未提交代码的编译检查使用 AGENTS.md 中的 checks 目录，不得部署该产物。

## 独立测试环境

测试使用普通用户，保持 TUN 关闭，不连接生产 socket，不复制生产订阅或配置。下面使用全新的专用测试目录；若目录已有数据，应先检查它的配置，不得直接假定它仍是空白环境。

```bash
test_dir="$HOME/.local/state/mihomo-loongnix-test"
install -d -m 0700 "$test_dir" "$test_dir/run" "$test_dir/bin"
# 若测试需要运行内核，使用独立副本。
install -m 0700 /usr/local/bin/mihomo "$test_dir/bin/mihomo"
```

先用 `ss -ltn` 确认 `17890` 和 `19090` 未被占用。在源码目录中，从已提交构建启动测试管理器：

```bash
commit=$(git rev-parse HEAD)
artifact_root="${XDG_DATA_HOME:-$HOME/.local/share}/mihomo-loongnix/builds"
test_dir="$HOME/.local/state/mihomo-loongnix-test"
env -u MIHOMO_TUI_CORE_SERVICE \
  MIHOMO_TUI_SHADOW=1 \
  MIHOMO_TUI_SHADOW_MIXED_PORT=17890 \
  MIHOMO_TUI_SHADOW_CONTROLLER_PORT=19090 \
  MIHOMO_TUI_SOCKET="$test_dir/run/daemon.sock" \
  "$artifact_root/$commit/mihomo-tui-linux-loong64" server -d "$test_dir"
```

在另一个终端中显式指定相同 socket，避免 TUI 默认连接正式后台：

```bash
cd "$HOME/projects/mihomo-loongnix"
commit=$(git rev-parse HEAD)
artifact_root="${XDG_DATA_HOME:-$HOME/.local/share}/mihomo-loongnix/builds"
test_dir="$HOME/.local/state/mihomo-loongnix-test"
MIHOMO_TUI_SOCKET="$test_dir/run/daemon.sock" \
  "$artifact_root/$commit/mihomo-tui-linux-loong64" -d "$test_dir"
```

测试混合端口为 `17890`，控制接口为 `127.0.0.1:19090`，数据和 socket 均属于测试目录。可用下列请求检查管理器，不打印订阅内容：

```bash
curl --unix-socket "$HOME/.local/state/mihomo-loongnix-test/run/daemon.sock" http://localhost/v1/status
```

测试样例为 `testdata/profile.example.yaml`，其中节点使用文档示例地址，不提供真实代理连通性。需要验证导入时，可将该样例通过本机临时 HTTP 服务提供给测试管理器。

结束时先在测试 TUI 停止测试内核，再退出 TUI，并在测试管理器终端按 `Ctrl+C`。若使用后台启动，记录 PID，只停止自己的进程，不使用按名称批量终止命令。

## 正式部署

首次安装仍使用 README 中的安装命令。已有双服务部署统一使用仓库里的 `scripts/deploy.py`，不再为每次升级生成写死版本的脚本。部署工具使用 Python 3.8+ 标准库，另需 Go（读取实际二进制构建信息）、curl、GNU tar（支持 ACL 与扩展属性）和 systemd。

### Web 随统一入口升级

已安装 Web 的服务器，日常只执行一次 deploy.py。先从同一干净提交分别运行 ./scripts/build-release.sh 与 ./scripts/build-web.sh，准备两份构建，再执行下面的统一部署命令。

工具先核验 Web 包；正式 sudo 部署还会预检私有配置，缺包或配置错误会在代理维护前失败。然后依次执行管理器阶段和 Web 阶段，保留 Web 的域名、认证方式、开关和自启设置。未安装 Web 时自动跳过，不要求 Node 或 Web 构建。

两阶段分别备份、分别恢复，不是跨组件的整体回滚。Web 阶段失败时，已成功的管理器升级保留；修复错误后重跑同一命令即可，管理器相同构建不再重启。Web 的备份路径由子工具输出，默认位于调用用户的 ~/backups/mihomo-loongnix；主工具 --backup-root 仅作用于管理器备份。

--web-build-root 可指定 Web 包目录，默认是 --build-root 下的 web/。--skip-web 可明确仅更新管理器。--check 检查两份发布包但不读取 root 私有 Web 配置、不执行部署。首次可选 Web 安装及单独回退见网页接入指南。

### 日常升级流程

先在普通用户下完成修改、测试与提交，然后构建：

```bash
cd "$HOME/projects/mihomo-loongnix"
./scripts/build-release.sh
commit=$(git rev-parse HEAD)
python3 scripts/deploy.py "$commit" --check
sudo python3 scripts/deploy.py "$commit"
```

已有该提交的构建时，直接复用，不重复运行构建脚本。部署接受完整提交号或至少 7 位的唯一缩写，例如 `133d11e`；缩写匹配多个构建时会拒绝，要求使用完整提交号。`--check` 只检查，不停止服务、不写正式配置，也不代表已经完成 root 私有数据备份。正式部署需要一次 `sudo`。

默认读取调用用户的 `~/.local/share/mihomo-loongnix/builds/<提交号>/`，即使通过 `sudo` 执行，也通过 `SUDO_USER` 找到原用户目录。若构建时设置了自定义 `XDG_DATA_HOME`，显式指定对应根目录：

```bash
sudo python3 scripts/deploy.py "$commit" --build-root /absolute/path/to/builds
```

备份默认放在调用用户的 `~/backups/mihomo-loongnix/`，可用 `--backup-root /absolute/path/to/backups` 更改；不能放进源码或正式数据目录。每次升级生成独立的私有目录，不覆盖以前的备份。

### 部署步骤与边界

1. 校验构建清单和实际二进制中的提交号、目标平台、`vcs.modified=false`。只有本仓库 `build-release.sh` 生成的完整产物目录可以部署，不能直接传入单个下载文件。
2. 检查服务启动路径、当前状态、配置、端口与代理连通性；root 部署同时核对运行进程与磁盘程序的校验值。已安装相同管理器构建时跳过管理器替换，仍继续处理已安装的 Web。
3. 获取部署互斥锁，避免两次升级并发修改服务；将新旧程序暂存并校验。
4. 暂停双服务，对完整运行数据、现用 TUI、现用 Mihomo、unit 及其覆盖配置制作保留权限、ACL 与扩展属性的快照，并与源文件比较验证。
5. 原子替换 `/usr/local/bin/mihomo-tui`，恢复管理器及内核此前的运行状态；内核原本关闭时仍保持关闭。不会更换 Mihomo 内核或重写 unit，也不会修改开机启用状态。
6. 验证服务、控制接口、活动配置 ID、代理端口与 TUN 状态。原内核运行时，HTTP 和 SOCKS 都必须通过 HTTPS 请求验证。失败时先保存诊断，再恢复旧程序与数据并复查。
7. 保存提交号、程序与内核校验值、前后状态、连接检查、日志及备份位置到私有目录，同时更新 `~/.local/share/mihomo-loongnix/current-deployment.json`。

目前支持 LoongArch Linux 上项目的标准双服务布局：管理器位于 `/usr/local/bin/mihomo-tui`，数据位于 `/var/lib/mihomo-tui`，socket 位于 `/run/mihomo-tui/daemon.sock`。内核实际路径从 `mihomo.service` 读取。自定义启动参数、符号链接部署、管理器未运行或服务处于异常状态时会拒绝升级。

TUN 必须提前在 TUI 中关闭；脚本不会自动修改路由来绕过该条件。普通代码或文档修改不代表授权部署。升级会短暂停止代理，已打开的 TUI 需要退出后重开。强制杀进程、断电、磁盘损坏等情况下不能保证自动回滚，须保留备份用于人工恢复。

### 连通性检查与诊断

默认检查地址为 `https://cp.cloudflare.com/generate_204`，预期返回 `204`。HTTP 和 SOCKS 分别最多尝试 3 次，失败后间隔 3 秒、6 秒；每次连接超时 8 秒、总超时 20 秒。始终校验 TLS，不能通过跳过证书验证或仅检查端口来判定成功。

如果该站点在你的网络中持续不可用，可为预检查和部署指定同一个可访问的 HTTPS 地址及预期的 2xx 状态码：

```bash
python3 scripts/deploy.py "$commit" --check --probe-url https://www.gstatic.com/generate_204 --probe-status 204
sudo python3 scripts/deploy.py "$commit" --probe-url https://www.gstatic.com/generate_204 --probe-status 204
```

备份目录中的 `proxy-checks.json` 保存成功升级的检查记录；失败时先保存 `failure.json`、`journal-failure.txt`。自动恢复失败另存 `recovery-failure.json`，不覆盖最初错误。不要把含真实节点、订阅或日志的诊断文件直接提交到 GitHub。

测试命令（临时文件与模拟系统调用，不操作正式服务）：

```bash
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/tests -v
```

## 回滚

普通部署失败时脚本会自动回滚；没有经过完整验证时会明确报告失败，不会声称恢复成功。失败现场保存在 `/var/lib/mihomo-tui.failed-<本次备份目录名>`，具体位置记录于备份目录的 `failed-state-location.txt`，避免删除新版本写入的数据。

需要人工回滚时，先核对本次备份的 `SHA256SUMS`。`before-deploy.tar` 包含带属主、权限、ACL 和扩展属性的完整快照；`old-program` 是旧 TUI 副本，`services-before.json` 记录原服务状态。按照记录恢复，不凭目录名选择所谓 final 或 next 版本：

1. 停止新版本的管理器和内核，将失败现场另存到私有目录。
2. 从已校验备份恢复 TUI、实际使用的内核、两份 unit 和运行数据，保留原有属主与权限。
3. 执行 `sudo systemctl daemon-reload`，恢复此前记录的服务启用状态。
4. 启动原管理服务，并按照记录恢复内核是否运行；验证端口、配置和代理连通性。

当前双服务部署的回滚不应启动已废弃的 `mihomo-tui.service`。除非备份明确来自一体式旧部署，否则不要套用旧服务名。

## 私密数据

正式管理配置位于 `/var/lib/mihomo-tui/config.yaml`，真实订阅链接与 API secret 位于该目录的 `secrets/runtime.yaml`。订阅正文、缓存和日志也属于运行数据。

这些数据以及 `~/.config/mihomo-tui`、测试状态、内核、构建产物和恢复备份均不提交到 Git。暂存后执行 `./scripts/check-secrets.sh`，提交钩子会再次检查。
