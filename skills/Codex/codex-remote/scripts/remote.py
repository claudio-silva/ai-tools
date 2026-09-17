"""Dispatch and inspect remote Codex jobs using OpenSSH and the Python standard library."""
import argparse
import json
from pathlib import Path
import re
import shlex
import subprocess
import sys
import time
import uuid

MODELS = {'luna': 'gpt-5.6-luna', 'terra': 'gpt-5.6-terra', 'sol': 'gpt-5.6-sol', 'astra': 'gpt-6-astra'}
ACTIVE = ('queued', 'running', 'launch_unknown')


def rpc(host, request):
    if not re.fullmatch(r'[A-Za-z0-9_][A-Za-z0-9_.@:\[\]-]*', host):
        raise ValueError('Use an SSH alias or user@host, without spaces, options or shell syntax')
    source = Path(__file__).with_name('worker.py').read_text(encoding='utf-8')
    if request['action'] == 'start':
        request = dict(request, worker_source=source)
    # OpenSSH invokes a remote shell; quote the whole login-shell command as one argument.
    inner = 'python3 -c ' + shlex.quote(source)
    cmd = ['ssh', '-T', '-a', '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10',
           '-o', 'ServerAliveInterval=15', '-o', 'ServerAliveCountMax=2', host,
           'sh -lc ' + shlex.quote(inner)]
    try:
        p = subprocess.run(cmd, input=json.dumps(request), text=True, capture_output=True, timeout=50)
    except subprocess.TimeoutExpired as e:
        raise RuntimeError('SSH timed out; job state is UNKNOWN. Query the same job ID before retrying.') from e
    try:
        value = json.loads(p.stdout)
    except ValueError as e:
        raise RuntimeError('SSH response unavailable; job state is UNKNOWN. ' + p.stderr[-2000:]) from e
    if p.returncode or 'error' in value:
        raise RuntimeError(json.dumps(value) + '\n' + p.stderr[-2000:])
    return dict(value, host=host)


def parser():
    p = argparse.ArgumentParser(description=__doc__)
    sub = p.add_subparsers(dest='action', required=True)
    for name in ('probe', 'start', 'status', 'wait', 'result', 'logs', 'cancel'):
        q = sub.add_parser(name)
        q.add_argument('--host', required=True)
        if name != 'probe':
            q.add_argument('--job', required=name != 'start')
        if name in ('wait', 'start'):
            q.add_argument('--wait-seconds', type=int, default=45)
        if name == 'start':
            q.add_argument('--cwd', required=True)
            q.add_argument('--model', required=True, help='luna, terra, sol, astra, or an explicit model ID')
            q.add_argument('--effort', required=True, choices=('none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra'))
            q.add_argument('--prompt-file', required=True, help='UTF-8 file; - reads stdin')
            q.add_argument('--mode', choices=('sync', 'async'), default='async')
            q.add_argument('--backend', choices=('auto', 'systemd', 'detached'), default='auto')
            q.add_argument('--sandbox', choices=('read-only', 'workspace-write', 'danger-full-access'), default='read-only')
            q.add_argument('--allow-full-access', action='store_true')
            q.add_argument('--max-seconds', type=int, default=3600)
        if name in ('logs', 'result'):
            q.add_argument('--offset', type=int, default=0)
            q.add_argument('--limit', type=int, default=16000)
        if name == 'logs':
            q.add_argument('--file', choices=('events.jsonl', 'stderr.log', 'worker.log'), default='events.jsonl')
    return p


def main():
    p = parser()
    args = p.parse_args()
    req = vars(args).copy()
    host = req.pop('host')
    wait_seconds = req.pop('wait_seconds', 0)
    if not 0 <= wait_seconds <= 45:
        p.error('--wait-seconds must be between 0 and 45; repeat bounded waits for longer work')
    if req.get('offset', 0) < 0 or req.get('limit', 1) <= 0:
        p.error('offset must be nonnegative and limit positive')
    mode = req.pop('mode', None)
    if args.action == 'start':
        if args.max_seconds <= 0:
            p.error('--max-seconds must be positive')
        if args.sandbox == 'danger-full-access' and not args.allow_full_access:
            p.error('Full access requires --allow-full-access and a mandate authorizing it')
        req.pop('allow_full_access')
        req['model'] = MODELS.get(args.model.lower(), args.model)
        req['prompt'] = sys.stdin.read() if args.prompt_file == '-' else Path(args.prompt_file).read_text(encoding='utf-8')
        req.pop('prompt_file')
        if not req['prompt'].strip():
            p.error('Prompt cannot be empty')
        req['job'] = args.job or ('job-' + uuid.uuid4().hex)
        print(json.dumps({'host': host, 'job': req['job'], 'notice': 'Retain this ID even if dispatch loses its response.'}), file=sys.stderr, flush=True)
    if args.action == 'wait':
        req['action'] = 'status'
    result = rpc(host, req)
    if args.action == 'wait' or mode == 'sync':
        deadline = time.monotonic() + wait_seconds
        while result.get('status') in ACTIVE and time.monotonic() < deadline:
            time.sleep(min(2, max(0, deadline - time.monotonic())))
            result = rpc(host, {'action': 'status', 'job': req['job']})
        if result.get('status') not in ACTIVE:
            result = rpc(host, {'action': 'result', 'job': req['job']})
    print(json.dumps(result, indent=2, ensure_ascii=False))
    return 1 if result.get('status') in ('failed', 'lost', 'timed_out', 'cancelled', 'launch_failed') else 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (RuntimeError, ValueError, OSError) as exc:
        print(json.dumps({'error': str(exc)}), file=sys.stderr)
        sys.exit(1)
