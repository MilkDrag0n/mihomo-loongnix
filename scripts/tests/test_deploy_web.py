"""首次 Web 安装的密码分支；不安装服务。"""
import importlib.util
from pathlib import Path
import sys
import unittest
from unittest.mock import patch

scripts = Path(__file__).parents[1]
sys.path.insert(0, str(scripts))
spec = importlib.util.spec_from_file_location('install_web', scripts / 'deploy-web.py')
web = importlib.util.module_from_spec(spec)
spec.loader.exec_module(web)
sys.path.remove(str(scripts))


class PasswordTests(unittest.TestCase):
    def test_external_never_prompts_or_hashes(self):
        with patch.object(web.getpass, 'getpass') as prompt, patch.object(web.subprocess, 'run') as run:
            self.assertEqual(web.password_hash(Path('/fake'), 'external'), '')
        prompt.assert_not_called()
        run.assert_not_called()

    def test_invalid_password_does_not_run_binary(self):
        with patch.object(web.getpass, 'getpass', return_value='short'), patch.object(web.subprocess, 'run') as run:
            with self.assertRaises(ValueError):
                web.password_hash(Path('/fake'), 'password')
        run.assert_not_called()


if __name__ == '__main__':
    unittest.main()
