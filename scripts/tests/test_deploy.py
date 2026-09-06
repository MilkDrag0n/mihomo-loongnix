"""使用临时文件及模拟服务，不触碰正式程序。"""
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('quick_deploy', Path(__file__).parents[1] / 'deploy.py')
deploy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(deploy)


class BuildTests(unittest.TestCase):
    def test_failed_build_never_requests_sudo(self):
        with patch.object(deploy.os, 'geteuid', return_value=1000), \
             patch.object(deploy, 'service_value', side_effect=['loaded', 'loaded']), \
             patch.object(deploy.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, ['build'])) as run:
            with self.assertRaises(subprocess.CalledProcessError):
                deploy.deploy()
        self.assertEqual(run.call_count, 1)
        self.assertEqual(run.call_args.args[0][-1], '--web')

    def test_uninstalled_web_is_skipped_and_build_only_never_installs(self):
        with patch.object(deploy.os, 'geteuid', return_value=1000), \
             patch.object(deploy, 'service_value', side_effect=['loaded', 'not-found']), \
             patch.object(deploy.subprocess, 'run') as run:
            deploy.deploy(build_only=True)
        self.assertEqual(run.call_count, 1)
        self.assertNotIn('--web', run.call_args.args[0])

    def test_sudo_occurs_only_after_successful_build(self):
        with patch.object(deploy.os, 'geteuid', return_value=1000), \
             patch.object(deploy, 'service_value', side_effect=['loaded', 'loaded']), \
             patch.object(deploy.subprocess, 'run') as run:
            deploy.deploy()
        commands = [c.args[0] for c in run.call_args_list]
        self.assertEqual(len(commands), 2)
        self.assertNotEqual(commands[0][0], 'sudo')
        self.assertEqual(commands[1][0], 'sudo')
        self.assertIn('--web', commands[1])


class ServiceTests(unittest.TestCase):
    def exercise(self, web_running, unit_changed=False, with_web=True):
        with patch.object(deploy, 'service_value', return_value='active' if web_running else 'inactive'), \
             patch.object(deploy, 'replace_binary'), \
             patch.object(deploy, 'install_web_files', return_value=unit_changed) as files, \
             patch.object(deploy, 'check_started'), patch.object(deploy, 'run') as run:
            deploy.install_built(Path('/fake/build'), with_web)
        commands = [c.args for c in run.call_args_list]
        for command in commands:
            self.assertNotIn('mihomo.service', command)
            self.assertNotIn('enable', command)
            self.assertNotIn('disable', command)
        if not with_web:
            files.assert_not_called()
        return commands

    def test_active_web_restarts_without_reloading_unchanged_unit(self):
        commands = self.exercise(True)
        self.assertEqual(commands, [('systemctl', 'stop', deploy.WEB),
                                    ('systemctl', 'restart', deploy.MANAGER),
                                    ('systemctl', 'start', deploy.WEB)])

    def test_closed_web_remains_closed(self):
        self.assertEqual(self.exercise(False), [('systemctl', 'restart', deploy.MANAGER)])

    def test_without_web_only_manager_restarts(self):
        self.assertEqual(self.exercise(False, with_web=False), [('systemctl', 'restart', deploy.MANAGER)])

    def test_changed_unit_reloads_before_starting(self):
        commands = self.exercise(True, unit_changed=True)
        self.assertLess(commands.index(('systemctl', 'daemon-reload')),
                        commands.index(('systemctl', 'start', deploy.WEB)))

    def test_start_failure_raises_without_rollback(self):
        with patch.object(deploy, 'replace_binary') as replace, \
             patch.object(deploy, 'run', side_effect=subprocess.CalledProcessError(1, ['restart'])) as run:
            with self.assertRaises(subprocess.CalledProcessError):
                deploy.install_built(Path('/fake'), False)
        self.assertEqual(replace.call_count, 1)
        self.assertEqual(run.call_count, 1)


class FilesTests(unittest.TestCase):
    def test_running_binary_can_be_replaced_without_overwriting_open_inode(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source, target = root / 'new', root / 'installed'
            source.write_bytes(b'new')
            target.write_bytes(b'old')
            with target.open('rb') as old:
                deploy.replace_binary(source, target)
                self.assertEqual(old.read(), b'old')
            self.assertEqual(target.read_bytes(), b'new')
            self.assertEqual(target.stat().st_mode & 0o777, 0o755)

    def test_migrate_current_to_fixed_runtime_and_detect_unit_changes(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            build, runtime = root / 'build', root / 'runtime'
            build.mkdir()
            (build / 'static').mkdir()
            (build / 'static/index.html').write_text('new page')
            (build / 'mihomo-web').write_bytes(b'new web')
            (build / 'mihomo-web.service').write_text('same unit')
            unit = root / 'unit'
            unit.write_text('same unit')
            old = root / 'old-release'
            old.mkdir()
            (old / 'kept').write_text('historical')
            current = root / 'current'
            current.symlink_to(old)
            config = root / 'config'
            config.write_text('private')
            with patch.object(deploy, 'WEB_RUNTIME', runtime), \
                 patch.object(deploy, 'WEB_CURRENT', current), patch.object(deploy, 'WEB_UNIT', unit):
                self.assertFalse(deploy.install_web_files(build))
                (runtime / 'static/stale.js').write_text('old asset')
                (build / 'mihomo-web.service').write_text('changed unit')
                self.assertTrue(deploy.install_web_files(build))
            self.assertEqual(current.resolve(), runtime)
            self.assertEqual((runtime / 'mihomo-web-linux-loong64').stat().st_mode & 0o777, 0o755)
            self.assertFalse((runtime / 'static/stale.js').exists())
            self.assertEqual((old / 'kept').read_text(), 'historical')
            self.assertEqual(config.read_text(), 'private')


if __name__ == '__main__':
    unittest.main()
