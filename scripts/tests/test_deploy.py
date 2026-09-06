"""纯临时文件和模拟系统调用；不会操作本机服务或真实代理。"""
import contextlib
import importlib.util
import io
from pathlib import Path
from types import SimpleNamespace
import subprocess
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / 'deploy.py'
spec = importlib.util.spec_from_file_location('deploy', SCRIPT)
deploy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(deploy)

COMMIT = 'a' * 40
METADATA = '\n'.join('build\t'+k+'='+v for k, v in {
    'vcs.revision': COMMIT, 'vcs.modified': 'false', 'GOOS': 'linux', 'GOARCH': 'loong64'}.items())


def tls_failure():
    return subprocess.CalledProcessError(35, ['curl'], output='000', stderr='SSL handshake failed')


class BuildTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.build = Path(self.temp.name) / COMMIT
        self.build.mkdir()
        (self.build / deploy.BINARY).write_bytes(b'fake executable')
        (self.build / 'BUILD-INFO.txt').write_text('commit='+COMMIT+'\n')
        self.manifest()

    def manifest(self):
        (self.build / 'SHA256SUMS').write_text(''.join(
            deploy.digest(self.build / name)+'  '+name+'\n' for name in [deploy.BINARY, 'BUILD-INFO.txt']))

    def test_valid_build(self):
        with patch.object(deploy, 'run', return_value=METADATA):
            self.assertEqual(deploy.verify_build(self.build), deploy.digest(self.build / deploy.BINARY))

    def test_tampered_binary_is_rejected_before_go(self):
        (self.build / deploy.BINARY).write_bytes(b'changed')
        with patch.object(deploy, 'run') as run, self.assertRaises(RuntimeError):
            deploy.verify_build(self.build)
        run.assert_not_called()

    def test_forged_clean_metadata_file_cannot_hide_dirty_binary(self):
        for metadata in [METADATA.replace('false', 'true'), METADATA.replace(COMMIT, 'b'*40),
                         METADATA.replace('loong64', 'amd64')]:
            with self.subTest(metadata=metadata), patch.object(deploy, 'run', return_value=metadata):
                with self.assertRaises(RuntimeError):
                    deploy.verify_build(self.build)

    def test_manifest_cannot_reference_other_files(self):
        with (self.build / 'SHA256SUMS').open('a') as out:
            out.write('0'*64+'  ../../private\n')
        with self.assertRaises(RuntimeError):
            deploy.verify_build(self.build)

    def test_atomic_replacement_preserves_open_old_binary(self):
        target = self.build / 'installed'
        target.write_bytes(b'old')
        with target.open('rb') as old:
            deploy.replace(self.build / deploy.BINARY, target)
            self.assertEqual(old.read(), b'old')
        self.assertEqual(target.read_bytes(), b'fake executable')
        self.assertEqual(target.stat().st_mode & 0o777, 0o755)


class ProbeTests(unittest.TestCase):
    def scenario(self, results, fail):
        events = []
        args = SimpleNamespace(probe_url='https://example.com/check', probe_status=204)
        with patch.object(deploy, 'run', side_effect=results) as run, patch.object(deploy.time, 'sleep'), contextlib.redirect_stdout(io.StringIO()):
            if fail:
                with self.assertRaises(RuntimeError):
                    deploy.probe(17890, args, events)
            else:
                deploy.probe(17890, args, events)
            self.assertEqual(run.call_count, len(results))
        return events

    def test_transient_failure_retries_both_protocols(self):
        events = self.scenario([tls_failure(), '204', '204'], False)
        self.assertEqual(events[0]['exit_code'], 35)
        self.assertIn('handshake', events[0]['error'])
        self.assertEqual(events[-1]['protocol'], 'socks5h')

    def test_persistent_failure_is_not_ignored(self):
        self.scenario([tls_failure()]*3, True)

    def test_http_success_does_not_hide_socks_failure(self):
        self.scenario(['204']+[tls_failure()]*3, True)

    def test_wrong_status_is_not_success(self):
        self.scenario(['403']*3, True)


class RecoveryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.build = self.base / COMMIT
        self.build.mkdir()
        (self.build / deploy.BINARY).write_bytes(b'new')
        (self.build / 'BUILD-INFO.txt').write_text('metadata')
        self.target = self.base / 'installed'
        self.target.write_bytes(b'old')
        self.core = self.base / 'core'
        self.core.write_bytes(b'core')
        self.args = SimpleNamespace(backup_root=self.base / 'backups')
        self.caller = SimpleNamespace(pw_dir=str(self.base / 'user'), pw_uid=1000, pw_gid=1000)
        self.states = {deploy.CORE: {'ActiveState': 'active'}}
        self.before = {'proxy_port': 17890, 'tun': {}, 'active_profile': None}

    def exercise(self, stage, broken_diagnostics=False):
        calls = []
        old_umask = deploy.os.umask(0o077)
        deploy.os.umask(old_umask)
        self.addCleanup(deploy.os.umask, old_umask)

        def fake_run(*args):
            calls.append(args)
            if args[0] == 'tar' and '-cpf' in args:
                if stage == 'backup':
                    raise RuntimeError('snapshot failed')
                Path(args[args.index('-cpf')+1]).write_bytes(b'archive')
            return ''

        def fake_replace(source):
            self.target.write_bytes(source.read_bytes())

        def fake_rollback(backup, archive, installed, states):
            calls.append(('rollback', installed))
            if installed:
                self.target.write_bytes((backup / 'old-program').read_bytes())
            deploy.resume(states)

        with patch.object(deploy, 'TARGET', self.target), patch.object(deploy, 'run', side_effect=fake_run), \
                patch.object(deploy, 'status', return_value=self.before), \
                patch.object(deploy, 'replace', side_effect=fake_replace), \
                patch.object(deploy, 'rollback', side_effect=fake_rollback), \
                patch.object(deploy, 'validate', side_effect=[RuntimeError('probe failed'), self.before] if stage == 'verify' else [self.before]), \
                contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(RuntimeError):
                if broken_diagnostics:
                    with patch.object(deploy, 'write_json', wraps=deploy.write_json) as write:
                        original = deploy.write_json._mock_wraps
                        def writing(path, value):
                            if path.name == 'failure.json': raise OSError('disk error')
                            return original(path, value)
                        write.side_effect = writing
                        deploy.deploy(self.build, deploy.digest(self.build / deploy.BINARY), self.before,
                                      self.states, self.core, ['fake'], self.args, self.caller, [])
                else:
                    deploy.deploy(self.build, deploy.digest(self.build / deploy.BINARY), self.before,
                                  self.states, self.core, ['fake'], self.args, self.caller, [])
        self.assertEqual(self.target.read_bytes(), b'old')
        self.assertIn(('rollback', stage == 'verify'), calls)
        self.assertIn(('systemctl', 'start', deploy.MANAGER), calls)
        return calls

    def test_backup_failure_never_installs_new_binary(self):
        self.exercise('backup')

    def test_validation_failure_restores_old_binary(self):
        self.exercise('verify')

    def test_diagnostic_write_failure_does_not_prevent_recovery(self):
        self.exercise('verify', broken_diagnostics=True)

    def test_stopped_core_is_not_started_by_resume(self):
        with patch.object(deploy, 'run') as run:
            deploy.resume({deploy.CORE: {'ActiveState': 'inactive'}})
        self.assertEqual([call.args for call in run.call_args_list], [('systemctl', 'start', deploy.MANAGER)])

    def test_real_rollback_preserves_failed_data(self):
        data = self.base / 'state'
        data.mkdir()
        (data / 'new-file').write_text('failed state')
        backup = self.base / 'backup'
        backup.mkdir()
        (backup / 'old-program').write_bytes(b'old')
        def extract(*args):
            if args[0] == 'tar':
                data.mkdir()
                (data / 'old-file').write_text('restored')
            return ''
        with patch.object(deploy, 'DATA', data), patch.object(deploy, 'run', side_effect=extract), patch.object(deploy, 'replace') as replace:
            deploy.rollback(backup, backup / 'archive', True, self.states)
        self.assertEqual((data / 'old-file').read_text(), 'restored')
        self.assertEqual((self.base / 'mihomo-tui.failed-backup/new-file').read_text(), 'failed state')
        replace.assert_called_once_with(backup / 'old-program')


class ExecutionTests(unittest.TestCase):
    def test_read_only_and_already_installed_never_deploy(self):
        with tempfile.TemporaryDirectory() as temp:
            base = Path(temp)
            (base / COMMIT).mkdir()
            states = {deploy.MANAGER: {'ActiveState': 'active', 'MainPID': '41'}, deploy.CORE: {'ActiveState': 'active', 'MainPID': '42'}}
            before = {'core': {'running': True, 'service_active': True},
                      'tun': {'configured': False, 'enabled': False}, 'proxy_port': 17890}
            for check in [True, False]:
                args = SimpleNamespace(commit=COMMIT[:7], build_root=base,
                                       backup_root=base / 'backups', check=check)
                with self.subTest(check=check), patch.object(deploy.os, 'uname', return_value=SimpleNamespace(machine='loongarch64')), \
                        patch.object(deploy, 'prepare_web', return_value=None), \
                        patch.object(deploy, 'verify_build', return_value='expected'), \
                        patch.object(deploy, 'services', return_value=states), \
                        patch.object(deploy, 'service_paths', return_value=(base / 'core', [])), \
                        patch.object(deploy, 'status', return_value=before), \
                        patch.object(deploy, 'digest', return_value='expected'), \
                        patch.object(deploy, 'probe') as probe, patch.object(deploy, 'deploy') as install, \
                        contextlib.redirect_stdout(io.StringIO()):
                    deploy.execute(args, None)
                install.assert_not_called()
                probe.assert_called_once()

    def test_bad_commit_and_insecure_probe_are_rejected(self):
        for args in [['../../main'], ['aaaaaaa', '--probe-url', 'http://example.com'],
                     ['aaaaaaa', '--probe-url', 'https://user:password@example.com'],
                     ['aaaaaaa', '--probe-status', '403']]:
            with self.subTest(args=args), contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                deploy.parse_args(args)


class ValidationTests(unittest.TestCase):
    def setUp(self):
        self.before = {'core': {'running': True, 'service_active': True},
                       'proxy_port': 17890, 'active_profile': {'id': 'demo'}, 'tun': {}}
        self.original = {}
        for unit, binary, argv in [
                (deploy.MANAGER, '/usr/local/bin/mihomo-tui', '/usr/local/bin/mihomo-tui server -d /var/lib/mihomo-tui'),
                (deploy.CORE, '/usr/local/bin/mihomo', '/usr/local/bin/mihomo -d /var/lib/mihomo-tui -f /var/lib/mihomo-tui/mihomo/config.yaml')]:
            self.original[unit] = {
                'ActiveState': 'active', 'UnitFileState': 'enabled',
                'FragmentPath': '/etc/systemd/system/'+unit, 'DropInPaths': '',
                'ExecStart': '{ path='+binary+' ; argv[]='+argv+' ; ignore_errors=no ; '
                             'start_time=[n/a] ; stop_time=[n/a] ; pid=817 ; code=(null) ; status=0/0 }'}
        self.current = {unit: dict(state) for unit, state in self.original.items()}

    def validate(self):
        with patch.object(deploy, 'status', return_value=self.before), \
                patch.object(deploy, 'services', return_value=self.current), \
                patch.object(deploy, 'probe') as probe:
            result = deploy.validate(self.before, self.original, None, [])
        probe.assert_called_once()
        return result

    def test_restart_runtime_metadata_may_change(self):
        for state in self.current.values():
            state['ExecStart'] = state['ExecStart'].replace('start_time=[n/a]', 'start_time=[Sat 2026-09-05 19:56:05 CST]').replace('stop_time=[n/a]', 'stop_time=[Sat 2026-09-05 19:55:59 CST]').replace('pid=817', 'pid=220276').replace('code=(null) ; status=0/0', 'code=exited ; status=0/SUCCESS')
        self.assertEqual(self.validate(), self.before)

    def test_actual_command_changes_are_still_rejected(self):
        original = self.current[deploy.MANAGER]['ExecStart']
        for value in [original.replace('path=/usr/local/bin/mihomo-tui', 'path=/tmp/other'),
                      original.replace('server -d /var/lib/mihomo-tui', 'server -d /tmp/other'),
                      original.replace('ignore_errors=no', 'ignore_errors=yes'), 'unrecognized format']:
            with self.subTest(value=value):
                self.current[deploy.MANAGER]['ExecStart'] = value
                with self.assertRaises(RuntimeError):
                    self.validate()

    def test_service_state_and_unit_changes_are_still_rejected(self):
        for key, value in [('ActiveState', 'inactive'), ('UnitFileState', 'disabled'),
                           ('FragmentPath', '/tmp/other.service'), ('DropInPaths', '/tmp/override.conf')]:
            with self.subTest(key=key):
                saved = self.current[deploy.MANAGER][key]
                self.current[deploy.MANAGER][key] = value
                with self.assertRaises(RuntimeError):
                    self.validate()
                self.current[deploy.MANAGER][key] = saved


if __name__ == '__main__':
    unittest.main()


class UnifiedDeploymentTests(unittest.TestCase):
    def args(self, check=False, skip=False):
        return SimpleNamespace(check=check, skip_web=skip, web_build_root=Path('/fake/builds/web'))

    def test_absent_web_is_optional(self):
        with patch.object(deploy, 'run', return_value='LoadState=not-found'), patch.object(deploy.subprocess, 'run') as child:
            self.assertIsNone(deploy.prepare_web(Path('/fake/' + COMMIT), self.args()))
        child.assert_not_called()

    def test_skip_web_does_not_query_service(self):
        with patch.object(deploy, 'run') as run:
            self.assertIsNone(deploy.prepare_web(Path('/fake/' + COMMIT), self.args(skip=True)))
        run.assert_not_called()

    def test_preflight_uses_exact_commit_and_read_only_mode(self):
        for check, option in [(True, '--check'), (False, '--preflight')]:
            with self.subTest(check=check), patch.object(deploy, 'run', return_value='LoadState=loaded'), patch.object(deploy.subprocess, 'run') as child:
                command = deploy.prepare_web(Path('/fake/' + COMMIT), self.args(check=check))
                self.assertEqual(command[2], COMMIT)
                child.assert_called_once_with(command + [option], check=True)

    def test_missing_web_package_aborts_preflight(self):
        with patch.object(deploy, 'run', return_value='LoadState=loaded'), patch.object(deploy.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, ['web'])):
            with self.assertRaises(subprocess.CalledProcessError):
                deploy.prepare_web(Path('/fake/' + COMMIT), self.args())

    def test_bad_unit_is_not_silently_skipped(self):
        with patch.object(deploy, 'run', return_value='LoadState=error'), patch.object(deploy.subprocess, 'run') as child:
            with self.assertRaises(RuntimeError):
                deploy.prepare_web(Path('/fake/' + COMMIT), self.args())
        child.assert_not_called()

    def test_web_failure_reports_partial_success(self):
        with patch.object(deploy.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, ['web'])):
            with self.assertRaisesRegex(RuntimeError, 'TUI／管理器已处于目标版本'):
                deploy.finish_web(['web'])

    def test_web_build_root_follows_custom_build_root(self):
        args, _ = deploy.parse_args([COMMIT, '--build-root', '/fake/builds'])
        self.assertEqual(args.web_build_root, Path('/fake/builds/web'))

    def test_same_manager_still_updates_web_and_check_never_updates(self):
        with tempfile.TemporaryDirectory() as temp:
            base = Path(temp)
            (base / COMMIT).mkdir()
            states = {deploy.MANAGER: {'ActiveState': 'active', 'MainPID': '41'}, deploy.CORE: {'ActiveState': 'active', 'MainPID': '42'}}
            before = {'core': {'running': True, 'service_active': True},
                      'tun': {'configured': False, 'enabled': False}, 'proxy_port': 17890}
            for check in [True, False]:
                args = SimpleNamespace(commit=COMMIT, build_root=base, backup_root=base / 'backup', check=check)
                with patch.object(deploy.os, 'uname', return_value=SimpleNamespace(machine='loongarch64')), \
                     patch.object(deploy, 'verify_build', return_value='same'), \
                     patch.object(deploy, 'prepare_web', return_value=['web']), \
                     patch.object(deploy, 'services', return_value=states), \
                     patch.object(deploy, 'service_paths', return_value=(base / 'core', [])), \
                     patch.object(deploy, 'status', return_value=before), \
                     patch.object(deploy, 'digest', return_value='same'), \
                     patch.object(deploy, 'probe'), patch.object(deploy, 'deploy') as install, \
                     patch.object(deploy, 'finish_web') as finish:
                    deploy.execute(args, None)
                install.assert_not_called()
                if check:
                    finish.assert_not_called()
                else:
                    finish.assert_called_once_with(['web'])
