"""Local transport tests; no SSH or model calls."""
import base64
import contextlib
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('remote', Path(__file__).with_name('remote.py'))
r = importlib.util.module_from_spec(spec)
spec.loader.exec_module(r)


class Transport(unittest.TestCase):
    def test_large_unicode_result_exact_and_not_in_output(self):
        content = ('é🐍 literal $(do-not-execute)\n' * 12000).encode()
        def rpc(host, request):
            if request['action'] == 'status':
                return {'status': 'completed', 'job': 'job-test123'}
            start = request['offset']
            chunk = content[start:start + 100000]
            return {'data_base64': base64.b64encode(chunk).decode(), 'more': start + len(chunk) < len(content)}
        with tempfile.TemporaryDirectory() as folder, patch.object(r, 'rpc', side_effect=rpc):
            target = Path(folder) / 'result.txt'
            result = r.save_result('devbox', 'job-test123', str(target))
            self.assertEqual(target.read_bytes(), content)
            self.assertEqual(result['sha256'], hashlib.sha256(content).hexdigest())
            self.assertNotIn('text', result)
            with self.assertRaises(FileExistsError):
                r.save_result('devbox', 'job-test123', str(target))

    def test_prompt_passed_to_transport_without_stdout_echo(self):
        text = 'PRIVATE PAYLOAD 🐍\n' * 12000
        with tempfile.TemporaryDirectory() as folder:
            prompt = Path(folder) / 'task.txt'
            prompt.write_text(text)
            out, err = io.StringIO(), io.StringIO()
            args = ['remote.py', 'start', '--host', 'devbox', '--cwd', '/tmp', '--model', 'sol',
                    '--effort', 'high', '--prompt-file', str(prompt), '--job', 'job-test123']
            with patch('sys.argv', args), patch.object(r, 'rpc', return_value={'status': 'queued'}) as rpc, contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
                self.assertEqual(r.main(), 0)
                self.assertEqual(rpc.call_args.args[1]['prompt'], text)
                self.assertNotIn('PRIVATE PAYLOAD', out.getvalue() + err.getvalue())

    def test_status_only_does_not_fetch_result(self):
        args = ['remote.py', 'wait', '--host', 'devbox', '--job', 'job-test123', '--status-only']
        with patch('sys.argv', args), patch.object(r, 'rpc', return_value={'status': 'completed'}) as rpc, contextlib.redirect_stdout(io.StringIO()):
            r.main()
            self.assertEqual(rpc.call_count, 1)
            self.assertEqual(rpc.call_args.args[1]['action'], 'status')


if __name__ == '__main__':
    unittest.main()
