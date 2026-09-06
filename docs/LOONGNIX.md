# Loongnix 开发与快速部署

主要目标为 Loongnix GNU/Linux 25 / LoongArch ABI2，Go 构建目标为 `linux/loong64`。全新安装步骤见 [README](../README.md)，协作规则见 [AGENTS.md](../AGENTS.md)。

## 固定目录

server-pc 的唯一开发仓库为 `/home/server/projects/mihomo-loongnix`。正式程序位于 `/usr/local/bin/mihomo-tui`，现有正式内核位于 `/usr/local/bin/mihomo`；以 `systemctl show mihomo.service -p ExecStart` 为实际依据。

构建产物放在 `~/.local/share/mihomo-loongnix/build/current/`，测试状态放在 `~/.local/state/mihomo-loongnix-test/`。共享文件夹不作为常驻开发目录。

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

日常修改后按需测试；不必先提交。直接运行 ./scripts/deploy.sh 构建并更新当前代码，提交与推送另行进行。

## 独立测试环境

测试使用普通用户，保持 TUN 关闭，不连接生产 socket，不复制生产订阅或配置。下面使用全新的专用测试目录；若目录已有数据，应先检查它的配置，不得直接假定它仍是空白环境。

```bash
test_dir="$HOME/.local/state/mihomo-loongnix-test"
install -d -m 0700 "$test_dir" "$test_dir/run" "$test_dir/bin"
# 若测试需要运行内核，使用独立副本。
install -m 0700 /usr/local/bin/mihomo "$test_dir/bin/mihomo"
```

先用 `ss -ltn` 确认 `17890` 和 `19090` 未被占用。在源码目录中，先运行 ./scripts/build-current.sh，再从当前构建启动测试管理器：

```bash
artifact_root="${XDG_DATA_HOME:-$HOME/.local/share}/mihomo-loongnix/build/current"
test_dir="$HOME/.local/state/mihomo-loongnix-test"
env -u MIHOMO_TUI_CORE_SERVICE \
  MIHOMO_TUI_SHADOW=1 \
  MIHOMO_TUI_SHADOW_MIXED_PORT=17890 \
  MIHOMO_TUI_SHADOW_CONTROLLER_PORT=19090 \
  MIHOMO_TUI_SOCKET="$test_dir/run/daemon.sock" \
  "$artifact_root/mihomo-tui" server -d "$test_dir"
```

在另一个终端中显式指定相同 socket，避免 TUI 默认连接正式后台：

```bash
cd "$HOME/projects/mihomo-loongnix"
artifact_root="${XDG_DATA_HOME:-$HOME/.local/share}/mihomo-loongnix/build/current"
test_dir="$HOME/.local/state/mihomo-loongnix-test"
MIHOMO_TUI_SOCKET="$test_dir/run/daemon.sock" \
  "$artifact_root/mihomo-tui" -d "$test_dir"
```

测试混合端口为 `17890`，控制接口为 `127.0.0.1:19090`，数据和 socket 均属于测试目录。可用下列请求检查管理器，不打印订阅内容：

```bash
curl --unix-socket "$HOME/.local/state/mihomo-loongnix-test/run/daemon.sock" http://localhost/v1/status
```

测试样例为 `testdata/profile.example.yaml`，其中节点使用文档示例地址，不提供真实代理连通性。需要验证导入时，可将该样例通过本机临时 HTTP 服务提供给测试管理器。

结束时先在测试 TUI 停止测试内核，再退出 TUI，并在测试管理器终端按 `Ctrl+C`。若使用后台启动，记录 PID，只停止自己的进程，不使用按名称批量终止命令。

## 正式部署

已有管理器服务时，用普通用户执行：

~~~bash
cd "$HOME/projects/mihomo-loongnix"
./scripts/deploy.sh
~~~

不需要提交号，也不必提前构建。允许工作区包含未提交修改；脚本构建的就是当前代码。构建失败不更新任何正式程序。全部构建完成后才提示 sudo 密码，不要在整个命令前加 sudo。

实际步骤：

1. 检查管理器是否已安装，并判断是否安装了可选 Web。
2. 普通用户构建 TUI／管理器；已安装 Web 时也构建网页及 Go 网关。构建输出到固定 build/current 目录，不运行测试或生成版本归档。
3. 提权替换 /usr/local/bin/mihomo-tui。Web 原来运行时先停止 Web，将程序和静态文件更新到 /opt/mihomo-web/runtime/，current 链接指向这里。
4. 仅当 Web 服务文件内容变化时执行 daemon-reload。
5. 重启管理器；Web 原来运行则启动，原来关闭则保持关闭。简单检查服务是否 active。
6. 输出完成；失败直接报告错误和相关服务日志，不自动恢复。

脚本不停止或更新 Mihomo 内核，不覆盖正式订阅、数据或 Web 私有配置，不修改服务自启状态。管理器重启期间 TUI／Web 请求可能短暂失败，已打开的 TUI 可退出重开。无需部署前关闭 TUN。

部署不制作备份、快照，不做外网代理探测、校验清单验证、版本检查或配置前后比较。更新后的启动失败可能留下部分更新及关闭的 Web；根据错误修复源码或环境后重新运行脚本。启动检查通过不等于业务接口或公网访问已验收。

首次运行新脚本会将旧 Web current 链接改到固定 runtime 目录；历史 /opt/mihomo-web/releases、按提交构建和 ~/backups/mihomo-loongnix 均保留，但不再参与日常部署。旧 current-deployment.json 是历史记录，不代表新脚本部署的当前源码。

## 只构建与开发检查

~~~bash
# 自动判断已安装 Web，仅构建，不提权、不更新服务
./scripts/deploy.sh --build-only

# 不依赖已安装服务，单独构建管理器；首次 Web 安装可加 --web
./scripts/build-current.sh
./scripts/build-current.sh --web

# 部署脚本的隔离测试；不是每次部署的前置步骤
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/tests -v
~~~

scripts/build-release.sh 和 scripts/build-web.sh 保留为固定目录构建的便捷入口，不再归档版本。可用 XDG_DATA_HOME 指定当前构建所在用户数据目录；首次 Web 安装使用默认 ~/.local/share 路径。

服务异常时可查看：

~~~bash
systemctl status mihomo-manager.service mihomo-web.service --no-pager
journalctl -u mihomo-manager.service -u mihomo-web.service -n 40 --no-pager
~~~

首次安装管理器按 README；首次可选 Web 安装按网页接入指南。部署脚本不为首次安装自动创建账号、配置、服务。

## 私密数据

正式管理配置位于 `/var/lib/mihomo-tui/config.yaml`，真实订阅链接与 API secret 位于该目录的 `secrets/runtime.yaml`。订阅正文、缓存和日志也属于运行数据。

这些数据以及 `~/.config/mihomo-tui`、测试状态、内核、构建产物和恢复备份均不提交到 Git。暂存后执行 `./scripts/check-secrets.sh`，提交钩子会再次检查。
