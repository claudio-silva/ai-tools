"""Linux remote job controller. Receives a JSON request on stdin; no shell prompt interpolation."""
import base64
import fcntl
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import time


def atomic(path, data):
    temp = path.with_suffix('.tmp')
    temp.write_text(json.dumps(data), encoding='utf-8')
    temp.replace(path)


def identity(pid):
    try:
        stat = Path('/proc/%d/stat' % pid).read_text().rsplit(')', 1)[1].split()
        return [Path('/proc/sys/kernel/random/boot_id').read_text().strip(), stat[19]] if stat[0] != 'Z' else None
    except (OSError, IndexError):
        return None


def command(args):
    try:
        p = subprocess.run(args, capture_output=True, text=True, timeout=10)
        return {'code': p.returncode, 'stdout': p.stdout.strip(), 'stderr': p.stderr.strip()}
    except (OSError, subprocess.TimeoutExpired) as e:
        return {'code': -1, 'stderr': str(e), 'stdout': ''}


def persistence():
    manager = command(['systemctl', '--user', 'is-system-running'])
    linger = command(['loginctl', 'show-user', str(os.getuid()), '-p', 'Linger', '--value'])
    return {'user_manager': manager['stdout'], 'linger': linger['stdout'],
            'systemd_ready': bool(shutil.which('systemd-run')) and manager['stdout'] in ('running', 'degraded') and linger['stdout'] == 'yes'}


def probe():
    models = []
    cache = Path(os.environ.get('CODEX_HOME', str(Path.home() / '.codex'))) / 'models_cache.json'
    try:
        data = json.loads(cache.read_text())
        for m in data.get('models', []):
            models.append({k: m[k] for k in ('slug', 'display_name', 'supported_reasoning_levels') if k in m})
    except (OSError, ValueError, TypeError):
        pass
    return {'platform': sys.platform, 'uid': os.getuid(), 'home': str(Path.home()),
            'codex': command(['codex', '--version']), 'auth': command(['codex', 'login', 'status']),
            'persistence': persistence(), 'cached_models': models,
            'models_notice': 'Cached metadata is advisory, not a live entitlement check.'}


def jobpath(job):
    if not re.fullmatch(r'[a-z0-9][a-z0-9-]{7,79}', job):
        raise ValueError('Invalid job ID')
    base = Path.home() / '.local/state/codex-remote/jobs'
    base.mkdir(parents=True, exist_ok=True, mode=0o700)
    return base / job


def status(folder):
    state = json.loads((folder / 'state.json').read_text())
    if state['status'] == 'running' and identity(state['pid']) != state['identity']:
        state = dict(state, status='lost', note='Worker absent; reconcile effects before resubmission.')
    if state['status'] == 'queued' and time.time() - state['created_at'] > 30:
        state = dict(state, status='launch_unknown', note='Worker has not acknowledged launch; inspect worker.log and backend.')
    return state


def launch(req):
    folder = jobpath(req['job'])
    folder.mkdir(mode=0o700, exist_ok=True)
    with (folder / 'launch.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        spec = {k: req[k] for k in ('job', 'cwd', 'model', 'effort', 'sandbox', 'max_seconds', 'prompt', 'backend')}
        if (folder / 'request.json').exists():
            if json.loads((folder / 'request.json').read_text()) != spec:
                raise ValueError('Job ID already exists with a different request')
            return status(folder) if (folder / 'state.json').exists() else {'job': req['job'], 'status': 'launch_unknown'}
        if not os.path.isabs(req['cwd']) or not Path(req['cwd']).is_dir():
            raise ValueError('cwd must be an existing remote absolute directory')
        if not shutil.which('codex'):
            raise ValueError('Codex is not on the remote PATH')
        available = persistence()
        backend = req['backend']
        if backend == 'auto':
            backend = 'systemd' if available['systemd_ready'] else 'detached'
        if backend == 'systemd' and not available['systemd_ready']:
            raise ValueError('systemd backend requires a user manager and linger; no settings changed')
        atomic(folder / 'request.json', spec)
        (folder / 'worker.py').write_text(req['worker_source'], encoding='utf-8')
        state = {'job': req['job'], 'status': 'queued', 'created_at': time.time(), 'backend': backend,
                 'directory': str(folder), 'model': req['model'], 'effort': req['effort'],
                 'sandbox': req['sandbox'], 'uid': os.getuid(), 'max_seconds': req['max_seconds']}
        if backend == 'detached':
            state['persistence_note'] = 'Detached from SSH; logout cleanup, reboot and OOM remain possible.'
        atomic(folder / 'state.json', state)
        argv = [sys.executable, str(folder / 'worker.py'), '--run', str(folder)]
        try:
            with (folder / 'worker.log').open('ab') as log:
                if backend == 'systemd':
                    unit = 'codex-remote-' + req['job']
                    p = subprocess.run(['systemd-run', '--user', '--quiet', '--collect', '--unit', unit,
                         '--property=KillMode=control-group', '--property=RuntimeMaxSec=%ds' % (req['max_seconds'] + 30),
                         '--setenv=PATH=' + os.environ.get('PATH', ''), *argv],
                         stdin=subprocess.DEVNULL, stdout=log, stderr=log, timeout=15)
                    if p.returncode:
                        raise RuntimeError('systemd-run failed; see worker.log')
                else:
                    subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                                     start_new_session=True, close_fds=True)
        except Exception as e:
            atomic(folder / 'state.json', dict(state, status='launch_failed', error=str(e)))
        return status(folder)


def run(folder):
    signal.signal(signal.SIGHUP, signal.SIG_IGN)
    req = json.loads((folder / 'request.json').read_text())
    state = json.loads((folder / 'state.json').read_text())
    state.update(status='running', pid=os.getpid(), identity=identity(os.getpid()), started_at=time.time())
    atomic(folder / 'state.json', state)
    child = None
    interrupted = [False]
    signal.signal(signal.SIGTERM, lambda *_: interrupted.__setitem__(0, True))
    signal.signal(signal.SIGINT, lambda *_: interrupted.__setitem__(0, True))
    try:
        args = ['codex', '-a', 'never', 'exec', '--json', '--color', 'never', '--skip-git-repo-check',
                '--sandbox', req['sandbox'], '--model', req['model'], '-c',
                'model_reasoning_effort=' + json.dumps(req['effort']), '--cd', req['cwd'],
                '--output-last-message', str(folder / 'result.txt'), '-']
        (folder / 'prompt.txt').write_text(req['prompt'], encoding='utf-8')
        with (folder / 'prompt.txt').open('rb') as prompt, (folder / 'events.jsonl').open('wb') as events, (folder / 'stderr.log').open('wb') as err:
            child = subprocess.Popen(args, stdin=prompt, stdout=events, stderr=err, start_new_session=True)
            deadline = time.monotonic() + req['max_seconds']
            terminal = None
            while child.poll() is None:
                if interrupted[0] or (folder / 'cancel.request').exists():
                    terminal = 'cancelled'
                elif time.monotonic() >= deadline:
                    terminal = 'timed_out'
                if terminal:
                    try:
                        os.killpg(child.pid, signal.SIGTERM)
                    except ProcessLookupError:
                        pass
                    try:
                        child.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        pass
                    # Also stop remaining ordinary descendants if the leader exited first.
                    try:
                        os.killpg(child.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    child.wait()
                    break
                time.sleep(0.25)
        failed_event = False
        for line in (folder / 'events.jsonl').open(encoding='utf-8', errors='replace'):
            try:
                event = json.loads(line)
                if event.get('type') == 'thread.started':
                    state['thread_id'] = event.get('thread_id')
                if event.get('type') == 'turn.failed':
                    failed_event = True
                if event.get('type') == 'turn.completed':
                    state['usage'] = event.get('usage')
            except ValueError:
                pass
        state.update(status=terminal or ('completed' if child.returncode == 0 and not failed_event else 'failed'), exit_code=child.returncode)
    except Exception as e:
        state.update(status='failed', error=str(e))
    finally:
        if child and child.poll() is None:
            try:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait()
            except ProcessLookupError:
                pass
        state['finished_at'] = time.time()
        atomic(folder / 'state.json', state)


def main(req):
    if sys.platform != 'linux':
        raise ValueError('This backend currently supports Linux SSH targets only')
    action = req['action']
    if action == 'probe':
        return probe()
    if action == 'start':
        return launch(req)
    folder = jobpath(req['job'])
    current = status(folder)
    if action == 'cancel':
        if current['status'] in ('queued', 'running', 'launch_unknown'):
            (folder / 'cancel.request').touch(mode=0o600)
        return dict(current, cancellation_requested=(folder / 'cancel.request').exists())
    if action in ('result', 'logs'):
        name = 'result.txt' if action == 'result' else req.get('file', 'events.jsonl')
        if name not in ('result.txt', 'events.jsonl', 'stderr.log', 'worker.log'):
            raise ValueError('Invalid output file')
        try:
            with (folder / name).open('rb') as stream:
                stream.seek(req.get('offset', 0))
                data = stream.read(min(req.get('limit', 16000), 100000))
                next_offset = stream.tell()
                more = bool(stream.read(1))
            current.update(file=name, next_offset=next_offset, more=more)
            if req.get('encoding') == 'base64':
                current['data_base64'] = base64.b64encode(data).decode('ascii')
            else:
                current['text'] = data.decode('utf-8', errors='replace')
        except FileNotFoundError:
            current.update(file=name, text='', more=False)
    return current


if __name__ == '__main__':
    os.umask(0o077)
    if len(sys.argv) == 3 and sys.argv[1] == '--run':
        run(Path(sys.argv[2]))
    else:
        try:
            print(json.dumps(main(json.load(sys.stdin))))
        except Exception as exc:
            print(json.dumps({'error': str(exc)}))
            sys.exit(1)
