import hashlib
import importlib.util
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('deploy_web', Path(__file__).parents[1] / 'deploy-web.py')
web = importlib.util.module_from_spec(spec)
spec.loader.exec_module(web)

class WebDeploymentTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.build = Path(self.temp.name) / ('a' * 40)
        self.build.mkdir()
        (self.build / 'static').mkdir()
        for name in [web.BINARY, 'BUILD-INFO.txt', 'static/index.html', 'mihomo-web.service']:
            (self.build / name).write_text('fixture')
        self.manifest()
        self.meta = '\n'.join(' build '+k+'='+v for k,v in {'GOOS':'linux','GOARCH':'loong64','vcs.revision':self.build.name,'vcs.modified':'false'}.items())
    def manifest(self):
        (self.build / 'SHA256SUMS').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.relative_to(self.build).as_posix()+'\n' for p in sorted(self.build.rglob('*')) if p.is_file() and p.name!='SHA256SUMS'))
    def test_complete_release(self):
        with patch.object(web,'run',return_value=self.meta):
            self.assertEqual(len(web.verify_build(self.build)),4)
    def test_corruption_missing_and_extra_rejected(self):
        for name in ['static/index.html',web.BINARY,'mihomo-web.service']:
            original=(self.build/name).read_text()
            (self.build/name).write_text('changed')
            with self.assertRaises(ValueError): web.verify_build(self.build)
            (self.build/name).write_text(original)
        (self.build/'extra').write_text('extra')
        with self.assertRaises(ValueError):web.verify_build(self.build)
    def test_manifest_traversal(self):
        with (self.build/'SHA256SUMS').open('a') as f:f.write('0'*64+'  ../outside\n')
        with self.assertRaises(ValueError):web.verify_build(self.build)
    def test_no_symlink_assets(self):
        (self.build/'static/link').symlink_to('/etc/passwd')
        with self.assertRaises(ValueError):web.verify_build(self.build)
    def test_wrong_architecture_or_dirty_binary(self):
        for a,b in [('loong64','amd64'),('modified=false','modified=true'),('revision='+self.build.name,'revision='+'b'*40)]:
            with patch.object(web,'run',return_value=self.meta.replace(a,b)):
                with self.assertRaises(ValueError):web.verify_build(self.build)
    def test_custom_unit_not_overwritten(self):
        with patch.object(web,'run',return_value='LoadState=loaded\nActiveState=active\nFragmentPath=/custom/unit\nDropInPaths='):
            with self.assertRaises(ValueError):web.service_state()
    def test_restore_only_web_and_preserves_closed_state(self):
        root=Path(self.temp.name)
        paths={'UNIT_FILE':root/'unit','CONFIG':root/'config','STATE':root/'state','RELEASES':root/'releases','CURRENT':root/'current'}
        backup=root/'backup';backup.mkdir();paths['RELEASES'].mkdir();old=paths['RELEASES']/('b'*40);old.mkdir()
        import json
        (backup/'before.json').write_text(json.dumps({'service':{'LoadState':'loaded','ActiveState':'inactive','UnitFileState':'disabled'},'current':str(old),'present':{'unit':False,'config':False,'state':False}}))
        from contextlib import ExitStack
        with ExitStack() as stack:
            for name,value in paths.items():stack.enter_context(patch.object(web,name,value))
            call=stack.enter_context(patch.object(web,'run',return_value=''))
            stack.enter_context(patch.object(web.subprocess,'run'))
            web.restore(backup)
            for c in call.call_args_list:
                self.assertNotIn('mihomo.service',c.args)
                self.assertNotIn('mihomo-manager.service',c.args)
                self.assertNotEqual(c.args[:2],('systemctl','start'))
        self.assertEqual(paths['CURRENT'].resolve(),old)
