"""Exercise the template's embedded Python against a fake HTTP transport."""
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


class TemplateTest(unittest.TestCase):
    def run_template(self, fail_setup=False):
        source = Path(__file__).with_name('boat-template.sh').read_text().split("<<'PY'\n", 1)[1].rsplit('\nPY', 1)[0]
        calls = []

        def request(req, **kwargs):
            path = req.full_url.split('/api/v1')[-1]
            calls.append((req.method, path))
            body = json.loads(req.data) if req.data and req.method != 'PUT' else {}
            if req.method == 'DELETE':
                self.assertEqual(req.get_header('X-ascii-confirm-delete'), path.rsplit('/',1)[-1])
                reply = {}
            elif path == '/environments':
                reply = {'environments':[{'name':'hev-template-fake-template','id':'env_fake'}]}
            elif path == '/environments/env_fake':
                flags=json.loads(req.data)
                for flag in ['safeForThirdParties','passGithub','passSecrets','passSandboxCredentials','passAgentsCredentials']:
                    self.assertFalse(flags[flag])
                self.assertEqual(flags['envContents'],'')
                self.assertEqual(flags['secretFiles'],[])
                reply = {}
            elif path == '/sandboxes':
                self.assertNotIn('noEnv',body)
                self.assertEqual(body['environment'],'hev-template-fake-template')
                reply = {'sandbox': {'id': 'bx_fake'}}
            elif path == '/sandboxes/bx_fake':
                reply = {'sandbox': {'state': 'ready'}}
            elif path.endswith('/usage'):
                reply = {'seconds': 1, 'dollars': 0.00001}
            elif path.endswith('/files/upload-url'):
                reply = {'url': 'https://upload.invalid/binary'}
            elif req.method == 'PUT':
                reply = {}
            elif path.endswith('/commands'):
                command = body['command']
                self.assertIn('ps -p $$ -o lstart=', command)
                self.assertIn('apt-get install -y procps', command)
                self.assertIn('hev --help >/dev/null', command)
                self.assertNotIn('hev --version', command)
                self.assertIn('EnvironmentFile=/run/hev-capture.env', command)
                self.assertIn('ConditionPathExists=/run/hev-capture.env', command)
                reply = {'exitCode': 1 if fail_setup else 0, 'stderr': 'synthetic setup failure'}
            elif path == '/saved-snapshots':
                reply = {}
            elif path == '/saved-snapshots/fake-template':
                reply = {'savedSnapshot': {'status': 'ready'}}
            else:
                self.fail('unexpected request: ' + path)
            return io.BytesIO(json.dumps(reply).encode())

        with tempfile.NamedTemporaryFile() as binary:
            binary.write(b'fake executable')
            binary.flush()
            with patch('sys.argv', ['template', 'fake-template', binary.name]), patch.dict('os.environ', {'BOAT_API_KEY': 'fake-key'}), patch('urllib.request.urlopen', side_effect=request):
                if fail_setup:
                    with self.assertRaisesRegex(RuntimeError, 'Template command failed'):
                        exec(compile(source, 'boat-template', 'exec'), {})
                else:
                    exec(compile(source, 'boat-template', 'exec'), {})
        self.assertEqual(calls[-2:], [('DELETE', '/sandboxes/bx_fake'), ('DELETE','/environments/env_fake')])
        self.assertEqual(('/saved-snapshots' in [p for _, p in calls]), not fail_setup)

    def test_template_success(self):
        self.run_template()

    def test_template_setup_failure_deletes_vm(self):
        self.run_template(fail_setup=True)


if __name__ == '__main__':
    unittest.main()
