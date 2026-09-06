#!/usr/bin/env python3
"""从当前工作区构建并更新现有服务；不备份、不自动回滚。"""
import argparse
import getpass
import json
import pwd
import secrets
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
MANAGER = 'mihomo-manager.service'
WEB = 'mihomo-web.service'
TARGET = Path('/usr/local/bin/mihomo-tui')
WEB_RUNTIME = Path('/opt/mihomo-web/runtime')
WEB_CURRENT = Path('/opt/mihomo-web/current')
WEB_UNIT = Path('/etc/systemd/system/mihomo-web.service')
CONFIG = Path('/etc/mihomo-web/config.json')
STATE = Path('/var/lib/mihomo-web')


def run(*command, capture=False):
    return subprocess.run(command, check=True, text=True,
                          stdout=subprocess.PIPE if capture else None,
                          timeout=120).stdout


def service_value(unit, field):
    return run('systemctl', 'show', unit, '--property=' + field, '--value', capture=True).strip()


def replace_binary(source, target):
    # 正在运行的旧程序仍可读，避免覆盖可执行文件触发 Text file busy。
    fd, name = tempfile.mkstemp(prefix='.' + target.name + '-', dir=target.parent)
    os.close(fd)
    temporary = Path(name)
    try:
        shutil.copyfile(source, temporary)
        temporary.chmod(0o755)
        os.replace(temporary, target)
    finally:
        temporary.unlink(missing_ok=True)


def install_web_files(build):
    WEB_RUNTIME.mkdir(parents=True, exist_ok=True, mode=0o755)
    WEB_RUNTIME.parent.chmod(0o755)
    replace_binary(build / 'mihomo-web', WEB_RUNTIME / 'mihomo-web-linux-loong64')
    static = WEB_RUNTIME / 'static'
    if static.exists():
        shutil.rmtree(static)
    shutil.copytree(build / 'static', static)
    for path in [WEB_RUNTIME, *static.rglob('*'), static]:
        path.chmod(0o755 if path.is_dir() else 0o644)
    # 首次日常更新把旧 current 链接改到固定 runtime；历史版本不删除。
    link = WEB_CURRENT.with_name('.current-' + str(os.getpid()))
    try:
        link.symlink_to(WEB_RUNTIME)
        link.replace(WEB_CURRENT)
    finally:
        link.unlink(missing_ok=True)
    template = build / 'mihomo-web.service'
    changed = not WEB_UNIT.exists() or WEB_UNIT.read_bytes() != template.read_bytes()
    if changed:
        shutil.copyfile(template, WEB_UNIT)
        WEB_UNIT.chmod(0o644)
    return changed


def password_hash(binary, mode):
    if mode == 'external':
        return ''
    password = getpass.getpass('设置 Web 管理员密码（12—1024 字节）：')
    if not 12 <= len(password.encode('utf-8')) <= 1024:
        raise ValueError('密码必须为 12—1024 字节')
    if password != getpass.getpass('再次输入密码：'):
        raise ValueError('两次密码不同')
    return subprocess.run([str(binary), '--hash-password'], input=password + '\n',
                          text=True, stdout=subprocess.PIPE, check=True, timeout=30).stdout.strip()


def initialize_web(build, public_url, auth_mode):
    if CONFIG.exists() or service_value(WEB, 'LoadState') != 'not-found':
        raise ValueError('Web 已安装；日常更新不需要 --install-web')
    cfg = {'listen': '127.0.0.1:9080', 'public_url': public_url.rstrip('/'),
           'manager_socket': '/run/mihomo-tui/daemon.sock', 'auth_mode': auth_mode,
           'password_hash': password_hash(build / 'mihomo-web', auth_mode),
           'summary_token': secrets.token_hex(32), 'show_node': False, 'test_mode': False}
    with tempfile.TemporaryDirectory(prefix='mihomo-web-install-') as temp:
        candidate = Path(temp) / 'config.json'
        candidate.write_text(json.dumps(cfg))
        run(str(build / 'mihomo-web'), '--config', str(candidate), '--static', str(build / 'static'), '--check')
    try:
        account = pwd.getpwnam('mihomo-web')
    except KeyError:
        run('useradd', '--system', '--user-group', '--home-dir', str(STATE),
            '--shell', '/usr/sbin/nologin', 'mihomo-web')
        account = pwd.getpwnam('mihomo-web')
    run('usermod', '-aG', 'mihomo-tui,mihomo-tui-operator', 'mihomo-web')
    CONFIG.parent.mkdir(parents=True, exist_ok=True)
    os.chown(CONFIG.parent, 0, account.pw_gid)
    CONFIG.parent.chmod(0o750)
    CONFIG.write_text(json.dumps(cfg, indent=2))
    os.chown(CONFIG, 0, account.pw_gid)
    CONFIG.chmod(0o640)
    STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
    os.chown(STATE, account.pw_uid, account.pw_gid)
    STATE.chmod(0o700)


def check_started(unit):
    if service_value(unit, 'ActiveState') != 'active':
        raise RuntimeError(unit + ' 未启动成功')


def report_failure(exc):
    print('更新失败：' + str(exc) + '。未执行自动恢复。', file=sys.stderr, flush=True)
    # 打印本次相关服务的日志，不读取订阅或配置文件。
    try:
        run('journalctl', '-u', MANAGER, '-u', WEB, '-n', '40', '--no-pager')
    except (OSError, subprocess.SubprocessError):
        pass


def install_built(build, with_web, public_url=None, auth_mode='password'):
    if public_url:
        initialize_web(build, public_url, auth_mode)
        with_web = True
    web_running = with_web and service_value(WEB, 'ActiveState') == 'active'
    print('构建已完成，开始替换程序。', flush=True)
    if web_running:
        run('systemctl', 'stop', WEB)
    replace_binary(build / 'mihomo-tui', TARGET)
    if with_web:
        if install_web_files(build):
            print('Web 服务文件有变化，重载服务配置……', flush=True)
            run('systemctl', 'daemon-reload')
        else:
            print('Web 服务文件未变化，跳过重载。', flush=True)
    if public_url:
        run('systemctl', '--no-reload', 'disable', WEB)
    print('重启管理器……', flush=True)
    run('systemctl', 'restart', MANAGER)
    check_started(MANAGER)
    if web_running:
        print('启动 Web……', flush=True)
        run('systemctl', 'start', WEB)
        check_started(WEB)
    elif with_web:
        print('Web 已更新，保持关闭。', flush=True)
    print('更新完成。', flush=True)


def deploy(build_only=False, public_url=None, auth_mode='password'):
    if os.geteuid() == 0:
        raise RuntimeError('请直接运行 ./scripts/deploy.sh，不要在前面加 sudo；构建完成后会申请权限')
    if service_value(MANAGER, 'LoadState') != 'loaded':
        raise RuntimeError('请先按 README 安装管理器服务')
    web_installed = service_value(WEB, 'LoadState') == 'loaded'
    if public_url and web_installed:
        raise RuntimeError('Web 已安装；直接运行 ./scripts/deploy.sh 更新')
    with_web = web_installed or bool(public_url)
    command = [str(ROOT / 'scripts/build-current.sh')]
    if with_web:
        command.append('--web')
    # 所有构建成功后才启动提权安装；构建失败不接触正式服务。
    subprocess.run(command, cwd=ROOT, check=True)
    build = Path(os.environ.get('XDG_DATA_HOME', str(Path.home() / '.local/share'))) / 'mihomo-loongnix/build/current'
    if build_only:
        print('仅构建完成，未更新服务。', flush=True)
        return
    command = ['sudo', sys.executable, str(Path(__file__).resolve()),
               '--install-built', str(build.resolve())]
    if with_web:
        command.append('--web')
    if public_url:
        command.extend(['--install-web', '--public-url', public_url, '--auth-mode', auth_mode])
    subprocess.run(command, check=True)


def main():
    parser = argparse.ArgumentParser(prog='./scripts/deploy.sh', description=__doc__)
    parser.add_argument('--build-only', action='store_true', help='只构建当前代码，不替换或重启服务')
    parser.add_argument('--install-web', action='store_true', help='同时首次安装可选 Web，默认保持关闭')
    parser.add_argument('--public-url', help='首次 Web 安装的 HTTPS 公开地址')
    parser.add_argument('--auth-mode', choices=['password', 'external'], help='首次 Web 安装认证方式，默认 password')
    parser.add_argument('--install-built', type=Path, help=argparse.SUPPRESS)
    parser.add_argument('--web', action='store_true', help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.install_web != bool(args.public_url):
        parser.error('--install-web 与 --public-url 必须一起使用')
    if args.auth_mode and not args.install_web:
        parser.error('--auth-mode 只用于首次安装；日常更新保留现有认证配置')
    if args.install_built:
        if os.geteuid() != 0:
            parser.error('内部安装步骤需要 sudo')
        try:
            install_built(args.install_built, args.web, args.public_url, args.auth_mode or 'password')
        except (Exception, KeyboardInterrupt) as exc:
            report_failure(exc)
            return 1
    else:
        try:
            deploy(args.build_only, args.public_url, args.auth_mode or 'password')
        except (Exception, KeyboardInterrupt) as exc:
            print('操作未完成：' + str(exc), file=sys.stderr)
            return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
