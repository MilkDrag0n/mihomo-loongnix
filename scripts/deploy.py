#!/usr/bin/env python3
"""升级现有 Loongnix 双服务部署；不用于首次安装或替换 Mihomo 内核。"""
import argparse
import datetime
import fcntl
import hashlib
import http.client
import json
import os
from pathlib import Path
import pwd
import re
import shutil
import socket
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlsplit

TARGET = Path('/usr/local/bin/mihomo-tui')
DATA = Path('/var/lib/mihomo-tui')
SOCKET = '/run/mihomo-tui/daemon.sock'
MANAGER, CORE = 'mihomo-manager.service', 'mihomo.service'
BINARY = 'mihomo-tui-linux-loong64'


def run(*args):
    return subprocess.run(args, check=True, text=True, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, timeout=120).stdout


def digest(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')


def verify_build(build):
    """同时核对两份清单校验值与二进制中的真实构建信息。"""
    commit = build.name
    if not re.fullmatch(r'[0-9a-f]{40}', commit):
        raise RuntimeError('构建目录名称必须是完整提交号')
    entries = {}
    for line in (build / 'SHA256SUMS').read_text().splitlines():
        match = re.fullmatch(r'([0-9a-f]{64})  (mihomo-tui-linux-loong64|BUILD-INFO.txt)', line)
        if not match or match[2] in entries:
            raise RuntimeError('SHA256SUMS 格式不正确或存在重复条目')
        entries[match[2]] = match[1]
    if set(entries) != {BINARY, 'BUILD-INFO.txt'}:
        raise RuntimeError('构建校验清单不完整')
    for name, expected in entries.items():
        if digest(build / name) != expected:
            raise RuntimeError(f'构建文件校验失败：{name}')
    metadata = run('go', 'version', '-m', str(build / BINARY))
    settings = dict(re.findall(r'^\s*build\s+([^=\s]+)=(\S+)\s*$', metadata, re.M))
    if any(settings.get(k) != v for k, v in {
            'vcs.revision': commit, 'vcs.modified': 'false',
            'GOOS': 'linux', 'GOARCH': 'loong64'}.items()):
        raise RuntimeError('二进制提交、平台或干净构建标记不符')
    if f'commit={commit}' not in (build / 'BUILD-INFO.txt').read_text().splitlines():
        raise RuntimeError('构建说明中的提交号不符')
    return entries[BINARY]


class UnixHTTP(http.client.HTTPConnection):
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(5)
        self.sock.connect(SOCKET)


def status():
    c = UnixHTTP('localhost')
    try:
        c.request('GET', '/v1/status')
        response = c.getresponse()
        obj = json.loads(response.read())
        if response.status != 200 or not obj.get('success'):
            raise RuntimeError('管理器状态接口异常')
        return obj['data']
    finally:
        c.close()


def services():
    result = {}
    for unit in [MANAGER, CORE]:
        text = run('systemctl', 'show', unit, '-p', 'ActiveState', '-p', 'UnitFileState',
                   '-p', 'ExecStart', '-p', 'FragmentPath', '-p', 'DropInPaths', '-p', 'MainPID')
        result[unit] = dict(line.split('=', 1) for line in text.splitlines() if '=' in line)
    return result


def service_paths(states):
    """只支持项目标准双服务布局；不猜测自定义 unit 中数据和程序的位置。"""
    expected = f'argv[]={TARGET} server -d {DATA} ;'
    if expected not in states[MANAGER]['ExecStart']:
        raise RuntimeError('管理器启动路径不是标准布局，请先核对 unit')
    match = re.search(r'path=([^;]+?)\s*;', states[CORE]['ExecStart'])
    if not match:
        raise RuntimeError('无法确认实际 Mihomo 内核路径')
    core = Path(match[1])
    if f'argv[]={core} -d {DATA} -f {DATA}/mihomo/config.yaml ;' not in states[CORE]['ExecStart']:
        raise RuntimeError('内核启动参数不是标准布局，请先核对 unit')
    paths = [DATA, TARGET, core]
    for state in states.values():
        paths.append(Path(state['FragmentPath']))
        paths.extend(Path(p) for p in state['DropInPaths'].split())
    for path in paths:
        if not path.is_absolute() or path.is_symlink() or not path.exists():
            raise RuntimeError(f'部署路径不存在或为符号链接，需先核对：{path}')
    return core, list(dict.fromkeys(str(p).lstrip('/') for p in paths))


def probe(port, args, events):
    for scheme in ['http', 'socks5h']:
        for attempt in range(1, 4):
            try:
                code = run('curl', '--silent', '--show-error', '--connect-timeout', '8',
                           '--max-time', '20', '--noproxy', '', '--proxy',
                           f'{scheme}://127.0.0.1:{port}', '--output', '/dev/null',
                           '--write-out', '%{http_code}', args.probe_url)
                error = '' if code == str(args.probe_status) else f'HTTP 状态码 {code}'
                exit_code = 0
            except subprocess.CalledProcessError as exc:
                code, exit_code = exc.stdout or '', exc.returncode
                error = (exc.stderr or str(exc)).strip()
            events.append(dict(protocol=scheme, attempt=attempt, http_status=code,
                               exit_code=exit_code, error=error))
            if not error:
                break
            print(f'{scheme} 检查 {attempt}/3 未通过：{error}', flush=True)
            if attempt < 3:
                time.sleep(3 * attempt)
        else:
            raise RuntimeError(f'{scheme} 代理连续三次验证失败：{error}')


def comparable_status(value):
    return (value['proxy_port'], (value.get('active_profile') or {}).get('id'),
            tuple(value['tun'].get(k) for k in
                  ['configured', 'enabled', 'runtime_enabled', 'interface_present']))


def validate(before, original_states, args, events):
    want_running = original_states[CORE]['ActiveState'] == 'active'
    last = None
    for _ in range(30):
        try:
            now = status()
            if now['core']['running'] == want_running and now['core']['service_active'] == want_running:
                break
        except Exception as exc:
            last = exc
        time.sleep(1)
    else:
        raise RuntimeError('管理器或内核未恢复到升级前状态') from last
    current = services()
    for unit in [MANAGER, CORE]:
        for key in ['ActiveState', 'UnitFileState', 'ExecStart', 'FragmentPath', 'DropInPaths']:
            if current[unit][key] != original_states[unit][key]:
                raise RuntimeError(f'{unit} 的 {key} 与升级前不一致')
    if comparable_status(now) != comparable_status(before):
        raise RuntimeError('配置、端口或 TUN 状态与升级前不一致')
    if want_running:
        probe(now['proxy_port'], args, events)
    return now


def replace(source, target=TARGET):
    fd, name = tempfile.mkstemp(prefix='.mihomo-install-', dir=target.parent)
    temp = Path(name)
    try:
        with os.fdopen(fd, 'wb') as out, open(source, 'rb') as src:
            shutil.copyfileobj(src, out)
            out.flush()
            os.fsync(out.fileno())
        os.chmod(temp, 0o755)
        os.replace(temp, target)
    finally:
        temp.unlink(missing_ok=True)


def resume(states):
    # 内核原本关闭时保持关闭；不修改开机启用状态。
    if states[CORE]['ActiveState'] == 'active':
        run('systemctl', 'start', CORE)
    run('systemctl', 'start', MANAGER)


def diagnostics(backup, exc, events, name='failure'):
    # 保存错误本身失败时，仍必须继续回滚。
    try:
        write_json(backup / (name + '.json'), dict(error=str(exc),
                   stderr=getattr(exc, 'stderr', None), proxy_checks=events))
        (backup / ('journal-' + name + '.txt')).write_text(run(
            'journalctl', '-u', MANAGER, '-u', CORE, '-n', '150', '--no-pager'))
    except Exception:
        print('部分诊断文件未能保存，继续恢复服务。', file=sys.stderr)


def rollback(backup, archive, installed, states):
    if installed:
        run('systemctl', 'stop', MANAGER, CORE)
        # 留存失败现场；同一父目录内重命名，不删除新版本写入的数据。
        failed = DATA.with_name('mihomo-tui.failed-' + backup.name)
        DATA.rename(failed)
        run('tar', '--acls', '--xattrs', '-xpf', str(archive), '-C', '/', str(DATA).lstrip('/'))
        replace(backup / 'old-program')
        (backup / 'failed-state-location.txt').write_text(str(failed) + '\n')
    resume(states)


def deploy(build, expected, before, states, core, files, args, caller, events):
    os.umask(0o077)
    args.backup_root.mkdir(parents=True, exist_ok=True, mode=0o700)
    stamp = datetime.datetime.now().strftime('%Y%m%d-%H%M%S')
    backup = Path(tempfile.mkdtemp(prefix=stamp+'-deploy-'+build.name[:7]+'-', dir=args.backup_root))
    archive = backup / 'before-deploy.tar'
    old_hash = digest(TARGET)
    core_hash = digest(core)
    write_json(backup / 'services-before.json', states)
    write_json(backup / 'status-before.json', before)
    (backup / 'core-version.txt').write_text(run(str(core), '-v'))
    shutil.copy2(TARGET, backup / 'old-program')
    shutil.copy2(build / BINARY, backup / 'new-program')
    shutil.copy2(build / 'BUILD-INFO.txt', backup / 'new-build.txt')
    if digest(backup / 'old-program') != old_hash or digest(backup / 'new-program') != expected:
        raise RuntimeError('暂存程序校验失败，尚未停止服务')
    record = Path(caller.pw_dir) / '.local/share/mihomo-loongnix/current-deployment.json'
    if record.exists():
        shutil.copy2(record, backup / 'previous-deployment.json')
    stopped = installed = False
    try:
        if comparable_status(status()) != comparable_status(before):
            raise RuntimeError('预检查后运行配置发生变化，请重新检查')
        if digest(TARGET) != old_hash or digest(core) != core_hash:
            raise RuntimeError('正式程序在预检查后发生变化')
        print('开始维护：暂停双服务并制作完整快照。', flush=True)
        stopped = True
        run('systemctl', 'stop', MANAGER, CORE)
        run('tar', '--acls', '--xattrs', '-cpf', str(archive), '-C', '/', *files)
        run('tar', '--acls', '--xattrs', '-df', str(archive), '-C', '/')
        (backup / 'SHA256SUMS').write_text(digest(archive) + '  before-deploy.tar\n')
        installed = True
        replace(backup / 'new-program')
        if digest(TARGET) != expected:
            raise RuntimeError('安装后的程序校验失败')
        resume(states)
        after = validate(before, states, args, events)
        if digest(core) != core_hash:
            raise RuntimeError('Mihomo 内核意外变化')
        write_json(backup / 'status-after.json', after)
        write_json(backup / 'proxy-checks.json', events)
        (backup / 'journal-after.txt').write_text(run('journalctl', '-u', MANAGER, '-u', CORE, '-n', '100', '--no-pager'))
        receipt = dict(recorded_at=datetime.datetime.now().astimezone().isoformat(),
                       status='deployed-and-verified', commit=build.name, binary_sha256=expected,
                       previous_binary_sha256=old_hash, core_sha256=core_hash, backup=str(backup),
                       proxy_checked=states[CORE]['ActiveState'] == 'active')
        write_json(backup / 'deployment.json', receipt)
        record.parent.mkdir(parents=True, exist_ok=True)
        fd, name = tempfile.mkstemp(prefix='.deployment-', dir=record.parent)
        try:
            with os.fdopen(fd, 'w') as out:
                json.dump(receipt, out, ensure_ascii=False, indent=2)
            os.chown(name, caller.pw_uid, caller.pw_gid)
            os.replace(name, record)
        finally:
            Path(name).unlink(missing_ok=True)
    except BaseException as exc:
        diagnostics(backup, exc, events)
        print(f'升级未完成，恢复备份：{backup}', file=sys.stderr, flush=True)
        if stopped:
            try:
                rollback(backup, archive, installed, states)
                if digest(TARGET) != old_hash or digest(core) != core_hash:
                    raise RuntimeError('恢复后的程序或内核校验不一致')
                validate(before, states, args, events)
                print('原程序、服务状态与连通性已验证恢复。', file=sys.stderr)
            except BaseException as recovery_error:
                print(f'自动恢复未验证成功，请依据备份人工恢复：{backup}', file=sys.stderr)
                diagnostics(backup, recovery_error, events, name='recovery-failure')
                raise RuntimeError('升级失败且自动恢复未验证成功') from recovery_error
        raise
    print(f'部署完成：{build.name}\n恢复备份：{backup}', flush=True)


def parse_args(argv=None):
    caller = pwd.getpwnam(os.environ['SUDO_USER']) if os.environ.get('SUDO_USER') else pwd.getpwuid(os.getuid())
    home = Path(caller.pw_dir)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('commit', help='目标构建的完整提交号或唯一缩写（至少 7 位）')
    parser.add_argument('--check', action='store_true', help='仅检查，不停止服务或写入运行数据')
    parser.add_argument('--build-root', type=Path, default=home / '.local/share/mihomo-loongnix/builds', help='构建产物根目录')
    parser.add_argument('--backup-root', type=Path, default=home / 'backups/mihomo-loongnix', help='私有备份根目录')
    parser.add_argument('--probe-url', default='https://cp.cloudflare.com/generate_204', help='HTTPS 连通性检查地址')
    parser.add_argument('--probe-status', type=int, default=204, help='检查地址预期的 HTTP 状态码')
    args = parser.parse_args(argv)
    if not re.fullmatch(r'[0-9a-f]{7,40}', args.commit):
        parser.error('提交号必须是 7 至 40 位小写十六进制字符')
    url = urlsplit(args.probe_url)
    if url.scheme != 'https' or not url.hostname or url.username or url.password or not 200 <= args.probe_status < 300:
        parser.error('检查地址必须是无账号密码的 HTTPS URL，预期状态码必须为 2xx')
    args.build_root = args.build_root.expanduser().resolve()
    args.backup_root = args.backup_root.expanduser().resolve()
    return args, caller


def execute(args, caller):
    if os.uname().machine not in ['loongarch64', 'loong64']:
        raise RuntimeError('本部署脚本目前仅支持 LoongArch Linux 服务器')
    for parent in [DATA, Path(__file__).resolve().parents[1]]:
        if args.backup_root == parent or parent in args.backup_root.parents:
            raise RuntimeError('备份目录必须位于正式数据和源码目录之外')
    matches = [p for p in args.build_root.glob(args.commit+'*') if re.fullmatch(r'[0-9a-f]{40}', p.name) and p.is_dir()]
    if len(matches) != 1:
        raise RuntimeError('找不到唯一的已构建提交，请先构建或使用完整提交号')
    build = matches[0]
    expected = verify_build(build)
    states = services()
    if states[MANAGER]['ActiveState'] != 'active' or states[CORE]['ActiveState'] not in ['active', 'inactive']:
        raise RuntimeError('要求管理器运行且内核处于稳定的运行或关闭状态')
    core, files = service_paths(states)
    if not args.check:
        # root 部署时同时核对正在运行的进程，避免磁盘文件已换、进程仍是旧版。
        for unit, binary in [(MANAGER, TARGET), (CORE, core)]:
            if states[unit]['ActiveState'] == 'active':
                process = Path('/proc') / states[unit]['MainPID'] / 'exe'
                if digest(process) != digest(binary):
                    raise RuntimeError(f'{unit} 的运行程序与磁盘文件不一致，请先核对来源')
    before = status()
    running = states[CORE]['ActiveState'] == 'active'
    if before['core']['running'] != running or before['core']['service_active'] != running:
        raise RuntimeError('当前内核或控制接口异常，请先处理现有故障')
    if before['tun']['configured'] or before['tun']['enabled']:
        raise RuntimeError('请先通过 TUI 关闭 TUN，再执行标准升级')
    events = []
    if running:
        probe(before['proxy_port'], args, events)
    if args.check:
        print('预检查通过；尚未制作正式备份，未修改程序或服务。')
        return
    if digest(TARGET) == expected:
        print('已安装相同构建且检查通过，无需重复部署。')
        return
    deploy(build, expected, before, states, core, files, args, caller, events)


def main(argv=None):
    args, caller = parse_args(argv)
    if args.check:
        execute(args, caller)
        return
    if os.geteuid() != 0:
        raise RuntimeError('部署需要 sudo；只读预检查可使用 --check')
    # 阻止两个部署进程同时修改服务和快照。
    fd = os.open('/run/lock/mihomo-loongnix-deploy.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError('已有部署任务运行，请等待其完成')
        execute(args, caller)


if __name__ == '__main__':
    try:
        main()
    except (Exception, KeyboardInterrupt) as exc:
        print(f'错误：{exc}', file=sys.stderr)
        if getattr(exc, 'stderr', None):
            print(exc.stderr.strip(), file=sys.stderr)
        sys.exit(1)
