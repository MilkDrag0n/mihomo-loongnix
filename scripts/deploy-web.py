#!/usr/bin/env python3
"""可选 Web 发布工具；永远不启停 Mihomo 代理双服务。"""
import argparse
import fcntl
import getpass
import hashlib
import json
import os
from pathlib import Path
import pwd
import re
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.request

UNIT = 'mihomo-web.service'
UNIT_FILE = Path('/etc/systemd/system') / UNIT
CONFIG = Path('/etc/mihomo-web/config.json')
STATE = Path('/var/lib/mihomo-web')
RELEASES = Path('/opt/mihomo-web/releases')
CURRENT = RELEASES.parent / 'current'
BINARY = 'mihomo-web-linux-loong64'
SCRIPT_ROOT = Path(__file__).resolve().parent.parent


def run(*args, **kwargs):
    return subprocess.run(args, check=True, text=True, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, **kwargs).stdout.strip()


def digest(path):
    with path.open('rb') as f:
        hasher = hashlib.sha256()
        for block in iter(lambda: f.read(1024 * 1024), b''):
            hasher.update(block)
        return hasher.hexdigest()


def verify_build(build):
    if not re.fullmatch(r'[0-9a-f]{40}', build.name):
        raise ValueError('Web 构建目录必须使用完整提交号')
    manifest = {}
    for line in (build / 'SHA256SUMS').read_text().splitlines():
        match = re.fullmatch(r'([0-9a-f]{64})  (.+)', line)
        if not match:
            raise ValueError('校验清单格式错误')
        value, name = match.groups()
        path = Path(name)
        if path.is_absolute() or '..' in path.parts or name in manifest or name == 'SHA256SUMS':
            raise ValueError('校验清单存在非法路径或重复项目')
        manifest[name] = value
    actual = set()
    for path in build.rglob('*'):
        if path.is_symlink():
            raise ValueError('发布包不可包含符号链接')
        if path.is_file() and path != build / 'SHA256SUMS':
            name = path.relative_to(build).as_posix()
            actual.add(name)
            if manifest.get(name) != digest(path):
                raise ValueError('发布包校验失败：' + name)
    if actual != set(manifest) or not {BINARY, 'BUILD-INFO.txt', 'static/index.html', 'mihomo-web.service'} <= actual:
        raise ValueError('发布包文件不完整')
    for name in actual:
        if name not in (BINARY, 'BUILD-INFO.txt', 'mihomo-web.service') and not name.startswith('static/'):
            raise ValueError('发布包存在未知文件')
    metadata = run('go', 'version', '-m', str(build / BINARY))
    settings = dict(re.findall(r'^\s*build\s+([^=\s]+)=(\S+)', metadata, re.M))
    if any(settings.get(k) != v for k, v in {'GOOS': 'linux', 'GOARCH': 'loong64',
               'vcs.revision': build.name, 'vcs.modified': 'false'}.items()):
        raise ValueError('Web 二进制来源或架构不符')
    return manifest


def service_state():
    output = run('systemctl', 'show', UNIT, '--property=LoadState,ActiveState,UnitFileState,FragmentPath,DropInPaths')
    fields = dict(line.split('=', 1) for line in output.splitlines() if '=' in line)
    if fields.get('LoadState') != 'not-found':
        if fields.get('FragmentPath') != str(UNIT_FILE) or fields.get('DropInPaths'):
            raise ValueError('仅支持本工具的标准 Web unit，不覆盖自定义 unit/drop-in')
        if fields.get('ActiveState') not in ('active', 'inactive', 'failed'):
            raise ValueError('Web 服务正在切换，请稍后重试')
    return fields


def set_current(path):
    CURRENT.parent.mkdir(parents=True, exist_ok=True)
    temporary = CURRENT.with_name('.current-' + str(os.getpid()))
    try:
        temporary.symlink_to(path)
        temporary.replace(CURRENT)
    finally:
        temporary.unlink(missing_ok=True)


def healthy():
    cfg = json.loads(CONFIG.read_text())
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    deadline = time.monotonic() + 12
    while time.monotonic() < deadline:
        try:
            pid = int(run('systemctl', 'show', UNIT, '--property=MainPID', '--value'))
            with opener.open('http://' + cfg['listen'] + '/healthz', timeout=1) as response:
                body = json.load(response)
                if response.status == 200 and body.get('app') == 'mihomo-web' and body.get('pid') == pid and pid > 0:
                    return
        except (OSError, ValueError, subprocess.CalledProcessError):
            pass
        time.sleep(.25)
    raise RuntimeError('Web 本机健康检查未通过')


def snapshot(backup, before):
    meta = {'service': before, 'current': str(CURRENT.resolve()) if CURRENT.is_symlink() else None,
            'present': {}, 'created_at': time.strftime('%Y-%m-%dT%H:%M:%S%z')}
    for key, path in [('unit', UNIT_FILE), ('config', CONFIG), ('state', STATE)]:
        meta['present'][key] = path.exists()
        if path.exists():
            run('cp', '-a', '--', str(path), str(backup / key))
    (backup / 'before.json').write_text(json.dumps(meta, indent=2))
    return meta


def restore(backup):
    meta = json.loads((backup / 'before.json').read_text())
    if UNIT_FILE.exists():
        run('systemctl', 'stop', UNIT)
    for key, path in [('unit', UNIT_FILE), ('config', CONFIG), ('state', STATE)]:
        if path.is_symlink():
            raise ValueError('拒绝替换符号链接：' + str(path))
        if path.exists():
            # 保留失败状态，不覆盖唯一一份资料。
            failed = backup / ('failed-' + key + '-' + str(time.time_ns()))
            run('cp', '-a', '--', str(path), str(failed))
            if path.is_dir():
                shutil.rmtree(path)
            else:
                path.unlink()
        if meta['present'][key]:
            path.parent.mkdir(parents=True, exist_ok=True)
            run('cp', '-a', '--', str(backup / key), str(path))
    old = meta['current']
    if old:
        old_path = Path(old)
        if old_path.parent != RELEASES or not old_path.is_dir():
            raise ValueError('备份的旧发布路径无效')
        set_current(old_path)
    else:
        CURRENT.unlink(missing_ok=True)
    run('systemctl', 'daemon-reload')
    if meta['service'].get('LoadState') != 'not-found':
        enabled = meta['service'].get('UnitFileState') == 'enabled'
        run('systemctl', 'enable' if enabled else 'disable', UNIT)
        if meta['service'].get('ActiveState') == 'active':
            run('systemctl', 'start', UNIT)
            healthy()
    else:
        (Path('/etc/systemd/system/multi-user.target.wants') / UNIT).unlink(missing_ok=True)
    return meta


def main():
    caller = pwd.getpwnam(os.environ.get('SUDO_USER') or getpass.getuser())
    home = Path(caller.pw_dir)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('commit', nargs='?', help='完整提交或至少 7 位前缀')
    parser.add_argument('--check', action='store_true', help='只检查发布包；不启动服务')
    parser.add_argument('--install', action='store_true', help='首次可选安装，安装后保持关闭')
    parser.add_argument('--public-url', help='首次安装的 HTTPS 公开入口')
    parser.add_argument('--rollback', type=Path, help='恢复本工具的备份目录')
    parser.add_argument('--build-root', type=Path, default=home / '.local/share/mihomo-loongnix/builds/web')
    args = parser.parse_args()
    if args.rollback:
        if args.commit or args.install or args.check:
            raise ValueError('回退不能与部署选项组合')
    else:
        if not args.commit or not re.fullmatch(r'[0-9a-f]{7,40}', args.commit):
            raise ValueError('需要至少 7 位提交号')
        matches = [p for p in args.build_root.glob(args.commit + '*') if p.is_dir()]
        if len(matches) != 1:
            raise ValueError('Web 构建提交不存在或不唯一')
        build = matches[0]
        verify_build(build)
        if args.check:
            print('Web 发布包校验通过；未检查 root 私有配置，未安装或启动服务。')
            return
    if os.geteuid() != 0:
        raise ValueError('安装、部署或回退需要 sudo；可先使用 --check')
    lock = open('/run/lock/mihomo-web.lock', 'a')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    os.umask(0o077)
    if args.rollback:
        restore(args.rollback.resolve())
        print('Web 已恢复；代理双服务未改动。')
        return
    before = service_state()
    for path in [CONFIG, STATE, UNIT_FILE, RELEASES]:
        if path.is_symlink():
            raise ValueError('不覆盖自定义符号链接：' + str(path))
    if CURRENT.exists() and not CURRENT.is_symlink():
        raise ValueError('current 必须为本工具的符号链接')
    installed = before.get('LoadState') != 'not-found'
    if args.install == installed:
        raise ValueError('首次安装使用 --install；已有安装请直接指定提交升级')
    if args.install and any(p.exists() for p in (CONFIG, STATE, UNIT_FILE, CURRENT)):
        raise ValueError('检测到残留 Web 文件，请先核对；不覆盖未知安装')
    if installed and not CURRENT.is_symlink():
        raise ValueError('旧发布记录不完整')
    config_text = None
    if args.install:
        from urllib.parse import urlsplit
        public = urlsplit(args.public_url or '')
        if public.scheme != 'https' or not public.hostname or public.username or public.query or public.fragment or public.path not in ('', '/'):
            raise ValueError('首次安装需 --public-url https://实际域名')
        password = getpass.getpass('设置 Web 管理员密码（至少 12 字节）：')
        if password != getpass.getpass('再次输入密码：'):
            raise ValueError('两次密码不同')
        password_hash = run(str(build / BINARY), '--hash-password', input=password + '\n')
        del password
        import secrets
        config_text = json.dumps({'listen': '127.0.0.1:9080', 'public_url': args.public_url.rstrip('/'),
              'manager_socket': '/run/mihomo-tui/daemon.sock', 'password_hash': password_hash,
              'summary_token': secrets.token_hex(32), 'show_node': False, 'test_mode': False}, indent=2)
    # 验证候选配置与静态资源后才进入维护；不连接正式 manager。
    with tempfile.TemporaryDirectory(prefix='mihomo-web-preflight-') as tmp:
        candidate = Path(tmp) / 'config.json'
        candidate.write_text(config_text if config_text else CONFIG.read_text())
        run(str(build / BINARY), '--config', str(candidate), '--static', str(build / 'static'), '--check')
    RELEASES.mkdir(parents=True, exist_ok=True)
    RELEASES.parent.chmod(0o755)
    RELEASES.chmod(0o755)
    destination = RELEASES / build.name
    if destination.exists():
        verify_build(destination)
        if (destination / 'SHA256SUMS').read_bytes() != (build / 'SHA256SUMS').read_bytes():
            raise ValueError('已归档的相同提交产物不同')
    else:
        staging = Path(tempfile.mkdtemp(prefix='.stage-', dir=RELEASES))
        try:
            shutil.copytree(build, staging, dirs_exist_ok=True)
            staging.rename(destination)
        finally:
            if staging.exists():
                shutil.rmtree(staging)
    verify_build(destination)
    for path in [destination, *destination.rglob('*')]:
        os.chown(path, 0, 0)
        path.chmod(0o755 if path.is_dir() or path.name == BINARY else 0o644)
    # 安装流程固定模板；候选文件已校验，但未重启任何正式服务。
    backup_root = home / 'backups/mihomo-loongnix'
    backup_root.mkdir(parents=True, exist_ok=True)
    backup = Path(tempfile.mkdtemp(prefix=time.strftime('%Y%m%d-%H%M%S-web-') + build.name[:7] + '-', dir=backup_root))
    if before.get('ActiveState') == 'active':
        run('systemctl', 'stop', UNIT)
    try:
        meta = snapshot(backup, before)
    except BaseException:
        if before.get('ActiveState') == 'active':
            run('systemctl', 'start', UNIT)
        raise
    try:
        if args.install:
            try:
                run('getent', 'passwd', 'mihomo-web')
            except subprocess.CalledProcessError:
                run('useradd', '--system', '--user-group', '--home-dir', str(STATE), '--shell', '/usr/sbin/nologin', 'mihomo-web')
            run('usermod', '-aG', 'mihomo-tui,mihomo-tui-operator', 'mihomo-web')
            account = pwd.getpwnam('mihomo-web')
            CONFIG.parent.mkdir(parents=True, exist_ok=True)
            os.chown(CONFIG.parent, 0, account.pw_gid)
            CONFIG.parent.chmod(0o750)
            CONFIG.write_text(config_text)
            os.chown(CONFIG, 0, account.pw_gid)
            CONFIG.chmod(0o640)
            STATE.mkdir(mode=0o700)
            os.chown(STATE, account.pw_uid, account.pw_gid)
        shutil.copyfile(destination / 'mihomo-web.service', UNIT_FILE)
        UNIT_FILE.chmod(0o644)
        set_current(destination)
        run('systemctl', 'daemon-reload')
        if args.install:
            run('systemctl', 'disable', UNIT)
        if before.get('ActiveState') == 'active':
            run('systemctl', 'start', UNIT)
            healthy()
        receipt = {'commit': build.name, 'backup': str(backup), 'previous': meta['current'],
                   'active': before.get('ActiveState') == 'active', 'binary_sha256': digest(destination / BINARY)}
        (backup / 'deployment.json').write_text(json.dumps(receipt, indent=2))
    except BaseException as error:
        (backup / 'failure.txt').write_text(str(error))
        print('Web 部署失败，正在恢复。备份：' + str(backup), file=sys.stderr)
        try:
            restore(backup)
        except BaseException as recovery:
            (backup / 'recovery-failure.txt').write_text(str(recovery))
            raise RuntimeError('Web 自动恢复未验证成功，请保留备份：' + str(backup)) from error
        raise RuntimeError('Web 已恢复原状态；本次部署未完成：' + str(error)) from error
    print('Web 发布完成：' + build.name)
    print('恢复备份：' + str(backup))
    print('保持关闭，可通过 TUI 首页或 mihomo-tui web start 开启。' if not receipt['active'] else 'Web 已重新启动并通过本机健康检查。')


if __name__ == '__main__':
    try:
        main()
    except (Exception, KeyboardInterrupt) as exc:
        print('错误：' + str(exc), file=sys.stderr)
        sys.exit(1)
