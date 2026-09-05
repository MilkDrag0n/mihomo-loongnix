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

正式服务是相互独立的 `mihomo-manager.service` 与 `mihomo.service`。文档修改、普通构建和测试不需要部署。

需要部署时，先确认用户授权，检查内核能执行、路径有效，并记录以下信息到仓库外的私有备份目录：

- 当前两项服务的运行状态、开机启用状态、实际启动命令。
- 当前 TUI 与内核二进制、SHA-256、内核版本。
- 两份 systemd unit 文件和完整 `/var/lib/mihomo-tui` 状态数据。
- 待部署源码提交号、构建目录及 `SHA256SUMS`。

备份目录权限设为 `0700`。运行数据含订阅和密钥，需要 root 权限才能完整备份；无权读取时不要声称已有完整部署备份。为保证一致性，状态快照应在受控维护窗口完成，或使用文件系统快照。

核对 `BUILD-INFO.txt` 中的提交号及 `vcs.modified=false`，校验构建文件后，再从对应产物执行：

```bash
sudo /absolute/path/to/build/mihomo-tui-linux-loong64 install_service
systemctl status mihomo-manager.service mihomo.service
```

上述路径必须替换为已经验证的构建文件。安装器目前在完成所有检查前可能停止管理服务，失败后不一定自动恢复；执行前必须准备好回滚资料。

部署后验证管理器接口、内核状态、原端口上的代理连通性与日志。首次安装未导入配置时，内核保持停止是正常状态；升级则应与部署前记录的状态比较。记录部署日期、提交号、二进制校验值与验证结果，不能仅记录产品版本号。

## 回滚

按本次部署前的记录恢复，不凭目录名选择所谓 final 或 next 版本：

1. 停止新版本的管理器和内核，将失败现场另存到私有目录。
2. 从已校验备份恢复 TUI、实际使用的内核、两份 unit 和运行数据，保留原有属主与权限。
3. 执行 `sudo systemctl daemon-reload`，恢复此前记录的服务启用状态。
4. 启动原管理服务，并按照记录恢复内核是否运行；验证端口、配置和代理连通性。

当前双服务部署的回滚不应启动已废弃的 `mihomo-tui.service`。除非备份明确来自一体式旧部署，否则不要套用旧服务名。

## 私密数据

正式管理配置位于 `/var/lib/mihomo-tui/config.yaml`，真实订阅链接与 API secret 位于该目录的 `secrets/runtime.yaml`。订阅正文、缓存和日志也属于运行数据。

这些数据以及 `~/.config/mihomo-tui`、测试状态、内核、构建产物和恢复备份均不提交到 Git。暂存后执行 `./scripts/check-secrets.sh`，提交钩子会再次检查。
