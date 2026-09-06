#!/usr/bin/env python3
"""仅用于可选 Web 首次安装；日常更新运行 ./scripts/deploy.sh。"""
import argparse
import getpass
import json
import os
from pathlib import Path
import pwd
import secrets
import subprocess
import sys
import tempfile

from deploy import install_web_files, run, service_value, WEB

CONFIG = Path('/etc/mihomo-web/config.json')
STATE = Path('/var/lib/mihomo-web')


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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--install', action='store_true', required=True, help='首次安装')
    parser.add_argument('--public-url', required=True, help='HTTPS 公开地址')
    parser.add_argument('--auth-mode', choices=['password', 'external'], default='password')
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise ValueError('首次安装需要 sudo；构建请用普通用户执行 ./scripts/build-web.sh')
    if CONFIG.exists() or service_value(WEB, 'LoadState') != 'not-found':
        raise ValueError('Web 已安装；日常更新直接运行 ./scripts/deploy.sh')
    caller = pwd.getpwnam(os.environ.get('SUDO_USER') or getpass.getuser())
    build = Path(caller.pw_dir) / '.local/share/mihomo-loongnix/build/current'
    cfg = {'listen': '127.0.0.1:9080', 'public_url': args.public_url.rstrip('/'),
           'manager_socket': '/run/mihomo-tui/daemon.sock', 'auth_mode': args.auth_mode,
           'password_hash': password_hash(build / 'mihomo-web', args.auth_mode),
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
    install_web_files(build)
    print('重载新安装的 Web 服务配置……', flush=True)
    run('systemctl', 'daemon-reload')
    run('systemctl', '--no-reload', 'disable', WEB)
    print('Web 已安装并保持关闭；使用 mihomo-tui web start 开启。')


if __name__ == '__main__':
    try:
        main()
    except (Exception, KeyboardInterrupt) as exc:
        print('安装未完成：' + str(exc) + '；未执行自动恢复。', file=sys.stderr)
        sys.exit(1)
