"""Offline behavioral tests: no SSH, model calls or user configuration changes."""
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('worker', Path(__file__).with_name('worker.py'))
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)

FAKE = '''#!/usr/bin/env python3
import json, pathlib, sys, time
text = sys.stdin.read()
if text.startswith('sleep'):
    time.sleep(20)
if text == 'turn-failure':
    print(json.dumps({'type':'turn.failed'}))
else:
    print(json.dumps({'type':'thread.started','thread_id':'fake-thread'}))
    print(json.dumps({'type':'turn.completed','usage':{'input_tokens':1}}))
    pathlib.Path(sys.argv[sys.argv.index('--output-last-message')+1]).write_text(text)
'''


class Jobs(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.home = Path(self.temp.name)
        binary = self.home / 'codex'
        binary.write_text(FAKE)
        binary.chmod(0o700)
        self.env = patch.dict(os.environ, {'HOME': str(self.home), 'PATH': str(self.home) + os.pathsep + os.environ['PATH']})
        self.env.start()
        self.req = dict(action='start', job='job-test12345', cwd=str(self.home), model='gpt-5.6-luna', effort='medium',
                        sandbox='read-only', max_seconds=10, prompt='literal $(touch BAD) `no` "quotes"\nPortuguês',
                        backend='detached', worker_source=Path(w.__file__).read_text())

    def tearDown(self):
        self.env.stop()
        self.temp.cleanup()

    def finish(self):
        deadline = time.monotonic() + 8
        while time.monotonic() < deadline:
            state = w.status(w.jobpath(self.req['job']))
            if state['status'] not in ('running', 'queued'):
                return state
            time.sleep(.1)
        self.fail('Job did not finish')

    def test_result_and_idempotency(self):
        w.launch(self.req)
        state = self.finish()
        self.assertEqual(state['status'], 'completed')
        self.assertEqual(state['thread_id'], 'fake-thread')
        result = w.main(dict(action='result', job=self.req['job']))
        self.assertEqual(result['text'], self.req['prompt'])
        self.assertFalse((self.home / 'BAD').exists())
        self.assertEqual(w.launch(self.req)['pid'], state['pid'])
        with self.assertRaises(ValueError):
            w.launch(dict(self.req, prompt='different'))

    def test_cancel(self):
        self.req['prompt'] = 'sleep'
        w.launch(self.req)
        w.main(dict(action='cancel', job=self.req['job']))
        self.assertEqual(self.finish()['status'], 'cancelled')

    def test_timeout(self):
        self.req.update(prompt='sleep', max_seconds=1)
        w.launch(self.req)
        self.assertEqual(self.finish()['status'], 'timed_out')

    def test_failed_event_despite_zero_exit(self):
        self.req['prompt'] = 'turn-failure'
        w.launch(self.req)
        self.assertEqual(self.finish()['status'], 'failed')

    def test_invalid_id(self):
        with self.assertRaises(ValueError):
            w.jobpath('../../other')

    def test_recycled_or_absent_pid(self):
        folder = w.jobpath('job-test12345')
        folder.mkdir()
        w.atomic(folder / 'state.json', dict(status='running', pid=os.getpid(), identity=['wrong-boot', 'wrong-start']))
        self.assertEqual(w.status(folder)['status'], 'lost')


if __name__ == '__main__':
    unittest.main()
