#!/usr/bin/env python3
"""从当前工作区构建并更新现有服务；不备份、不自动回滚。"""
import argparse
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


def install_built(build, with_web):
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


def deploy(build_only=False):
    if os.geteuid() == 0:
        raise RuntimeError('请直接运行 ./scripts/deploy.sh，不要在前面加 sudo；构建完成后会申请权限')
    if service_value(MANAGER, 'LoadState') != 'loaded':
        raise RuntimeError('请先按 README 安装管理器服务')
    with_web = service_value(WEB, 'LoadState') == 'loaded'
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
    subprocess.run(command, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--build-only', action='store_true', help='只构建当前代码，不替换或重启服务')
    parser.add_argument('--install-built', type=Path, help=argparse.SUPPRESS)
    parser.add_argument('--web', action='store_true', help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.install_built:
        if os.geteuid() != 0:
            parser.error('内部安装步骤需要 sudo')
        try:
            install_built(args.install_built, args.web)
        except (Exception, KeyboardInterrupt) as exc:
            report_failure(exc)
            return 1
    else:
        try:
            deploy(args.build_only)
        except (Exception, KeyboardInterrupt) as exc:
            print('操作未完成：' + str(exc), file=sys.stderr)
            return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
