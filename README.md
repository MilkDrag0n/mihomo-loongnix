# mihomo-tui — Loongnix edition

An independent Mihomo manager and terminal UI for Loongnix 25.1 on LoongArch ABI2. It provides a small, auditable control surface for a headless server: core start/stop, TUN state, URL profiles, proxy-group/node selection, effective rules, and logs.

## Usage

```bash
go test ./...
GOOS=linux GOARCH=loong64 CGO_ENABLED=0 go build -trimpath -o build/mihomo-tui .
sudo ./build/mihomo-tui install_service
```

After installation, add the operator account to the group created by the installer, start a new login session, and run:

```bash
mihomo-tui
```

The manager uses Mihomo's loopback controller on port `9090` and the managed mixed proxy port defaults to `7890`. TUN is off on first install. Disk logging is also off on first install; live log viewing remains available.

For the Loongnix deployment and rollback procedure, see [`docs/LOONGNIX.md`](docs/LOONGNIX.md).

## Keyboard shortcuts

| Key | Action |
| --- | --- |
| `1`–`5` | Switch between Home, Profiles, Nodes, Rules, and Logs |
| `Tab` / `Shift+Tab` | Move focus forward/backward |
| Arrow keys / `j` / `k` | Move through a list; `Left` / `Right` move table columns |
| `Enter` / `Space` | Activate the focused button or select the focused item |
| `PgUp` / `PgDn` | Turn the current list or table page backward/forward |
| `Home` / `End` | Jump to the first/last item |
| `/` | Focus the filter field |
| `Esc` | Cancel text input or close the current dialog |
| `r` | Refresh the current page |
| `q` | Quit when no input field or dialog has focus |

## Mihomo kernel

The kernel is the open-source [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo) project (Clash.Meta). The current Loongnix server deployment uses:

```text
Mihomo Meta v1.19.30 linux loong64
```

This project does not modify or redistribute the Mihomo kernel source. The ABI2-compatible `mihomo` binary is installed separately and is expected at `/var/lib/mihomo-tui/bin/mihomo` or the path configured for the service.

## Requirements

- **Operating system**: Loongnix 25.1 on LoongArch ABI2. The current deployment is verified on Linux `6.6.52-loong64` (`loongarch64`) with glibc `2.41`.
- **Service manager**: systemd is required; the current deployment is verified with systemd `257.7`.
- **Mihomo**: an ABI2-compatible `linux loong64` binary. The tested server runs Mihomo Meta `v1.19.30`; see [Mihomo kernel](#mihomo-kernel).
- **Build from source**: Go `1.26.1` or newer, as declared in [`go.mod`](go.mod). Go is not required when using a prebuilt TUI binary.
- **Privileges**: root is required only for service installation and system-level TUN operations; the TUI itself runs as an unprivileged operator account.

## Architecture

```text
regular-user TUI ── Unix socket ──> root mihomo-manager.service
                                         ├─ controls mihomo.service
                                         ├─ owns private runtime data
                                         └─ proxies Mihomo's loopback API
```

- `mihomo-manager.service` is the privileged management boundary.
- `mihomo.service` runs the Mihomo core separately.
- The TUI runs as an ordinary user and does not call `sudo`, `systemctl`, or write system configuration directly.
- A status is displayed as successful only after the manager executes the operation and reads the state back from systemd, Mihomo, or the host.

## Supported scope

The intentionally minimal UI has five pages:

1. Home — authoritative core/TUN/configuration summary and start/stop controls.
2. Profiles — import a URL, validate and stage it, activate/update/rename/delete profiles, with failure rollback.
3. Nodes — preserve Mihomo's original `all` order, show the actual `now` node, select a node, and test one node's delay.
4. Rules — read-only view of the currently effective rules in `rule` mode.
5. Logs — live Mihomo logs plus an independent, disabled-by-default disk log switch, file size reporting, and 10 MiB rotation with three historical files.

## Documentation

- [中文说明](README-zh.md)
- [Manager API](docs/MANAGER_API.zh-CN.md)
- [UI control tree and test IDs](docs/UI_CONTROL_TREE.zh-CN.md)
- [Test reference](docs/TEST_REFERENCE.zh-CN.md)
- [Loongnix deployment and rollback](docs/LOONGNIX.md)

## License

This project is released under the [MIT License](LICENSE). Mihomo is a separate project with its own license and distribution terms.
