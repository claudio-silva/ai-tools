#!/usr/bin/env node
/**
 * cdp.mjs — minimal Chrome DevTools Protocol CLI for agent-driven browser
 * automation. Zero dependencies (Node >= 22: built-in fetch + WebSocket).
 *
 * Commands:
 *   launch [url] [--name L] [--port N] [--profile dir] [--extension dir] [--app name]
 *   tabs [--name L | --port N]
 *   eval <expr|@file> [--name L | --port N] [--url substr] [--raw]
 *   nav <url> [--name L | --port N]
 *   wait <text> [--name L | --port N] [--url substr] [--timeout S]
 *   shot <path.png> [--name L | --port N] [--url substr] [--full]
 *   cookie <name=value>... --domain <d> [--name L | --port N] [--url substr] [--path /] [--secure]
 *   status
 *   close <name>... | --all [--purge]
 *
 * Instances are registered in $TMPDIR/lite-chrome/instances.json so concurrent
 * agents can run side by side: every launch gets a collision-free name, an
 * auto-allocated debug port, and its own --user-data-dir. Close only the
 * instance names you created.
 */

import { spawn } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';

const USAGE = `Usage: node cdp.mjs <command> [args]

  launch [url] [--name L] [--port N] [--profile dir] [--extension dir] [--app name]
      Launch Chrome with remote debugging on an isolated profile.
      Prints {name, port, profile}. Defaults: name auto-generated, port
      auto-allocated, profile /tmp/lite-chrome-<name>.
  tabs [--name L | --port N]
      List debuggable page targets of an instance.
  eval <expr|@file> [--name L | --port N] [--url substr] [--raw]
      Evaluate JS in a tab (MAIN world, await supported, JSON output).
      Prefix the expression with @ to read it from a file.
  nav <url> [--name L | --port N]
      Navigate a tab and wait for the load event.
  wait <text> [--name L | --port N] [--url substr] [--timeout S]
      Poll until rendered page text contains <text> (default 15s).
  shot <path.png> [--name L | --port N] [--url substr] [--full]
      Screenshot a tab to a PNG file.
  cookie <name=value>... --domain <d> [--name L | --port N] [--url substr]
      Set cookies via Network.setCookie (e.g. inject a session cookie for
      an automated suite). Optional: --path --secure --httpOnly
      --sameSite Lax|Strict|None --expires <epoch>.
  status
      List registered instances: name, port, pid, alive, url.
  close <name>... | --all [--purge]
      Terminate instances you created. --purge also deletes profiles that
      were auto-created under the OS temp dir.`;

const args = process.argv.slice(2);
const cmd = args[0];
const opt = (name) => {
  const i = args.indexOf(name);
  return i !== -1 ? args[i + 1] : undefined;
};
const flag = (name) => args.includes(name);
const OPTVALS = new Set(['--port', '--profile', '--extension', '--app', '--url', '--name', '--timeout', '--domain', '--path', '--expires']);
const positional = () =>
  args.slice(1).filter((a, i) => !a.startsWith('--') && !OPTVALS.has(args[i]));

const REG_DIR = path.join(os.tmpdir(), 'lite-chrome');
const REG_FILE = path.join(REG_DIR, 'instances.json');

function die(msg, code = 1) {
  console.error(msg);
  process.exit(code);
}

// ---------- instance registry (shared, merge-write so agents never clobber) ----------
function loadRegistry() {
  try {
    return JSON.parse(readFileSync(REG_FILE, 'utf8'));
  } catch {
    return { instances: {} };
  }
}

function saveRegistry(mutate) {
  mkdirSync(REG_DIR, { recursive: true });
  const disk = loadRegistry();
  const ret = mutate(disk.instances);
  const tmp = `${REG_FILE}.${process.pid}.tmp`;
  writeFileSync(tmp, JSON.stringify(disk, null, 1));
  renameSync(tmp, REG_FILE);
  return ret;
}

// ---------- instance resolution ----------
function pidAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

// Liveness: pid check when known; port probe covers pid-less (`open -na`) launches.
async function instAlive(i) {
  if (i.pid && pidAlive(i.pid)) return true;
  try {
    await fetch(`http://localhost:${i.port}/json/version`, { signal: AbortSignal.timeout(1500) });
    return true;
  } catch {
    return false;
  }
}

async function resolveInstance() {
  const explicitPort = opt('--port') || process.env.CDP_PORT;
  if (explicitPort) return { port: Number(explicitPort) };
  const name = opt('--name');
  const reg = loadRegistry().instances;
  if (name) {
    const inst = reg[name];
    if (!inst) die(`No instance named "${name}". Known: ${Object.keys(reg).join(', ') || '(none)'}`);
    return inst;
  }
  const alive = [];
  for (const [n, i] of Object.entries(reg)) {
    if (await instAlive(i)) alive.push([n, i]);
  }
  if (alive.length === 1) return alive[0][1];
  if (alive.length === 0) die('No running instances. Start one: node cdp.mjs launch <url>');
  die(`Multiple instances running: ${alive.map(([n]) => n).join(', ')}. Pass --name.`);
}

// ---------- CDP plumbing ----------
async function targets(port) {
  try {
    return await (await fetch(`http://localhost:${port}/json`)).json();
  } catch {
    die(`No Chrome debug port on ${port}. Run: node cdp.mjs launch <url>`);
  }
}

async function pickTarget(port) {
  const list = await targets(port);
  const sel = opt('--url');
  const pages = list.filter((t) => t.type === 'page');
  const page = sel
    ? pages.find((t) => t.url.includes(sel) || t.title.includes(sel))
    : pages.find((t) => !t.url.startsWith('chrome://')) || pages[0];
  if (!page) die(sel ? `No tab matching "${sel}"` : 'No debuggable tab found');
  return page;
}

function connect(wsUrl) {
  const ws = new WebSocket(wsUrl);
  let id = 0;
  const pending = new Map();
  const listeners = new Map();
  ws.onmessage = (e) => {
    const msg = JSON.parse(e.data);
    if (msg.id && pending.has(msg.id)) {
      pending.get(msg.id)(msg);
      pending.delete(msg.id);
    } else if (msg.method && listeners.has(msg.method)) {
      listeners.get(msg.method)(msg.params);
    }
  };
  const ready = new Promise((res, rej) => {
    ws.onopen = res;
    ws.onerror = () => rej(new Error('WebSocket connection failed'));
  });
  const send = (method, params = {}) =>
    new Promise((resolve, reject) => {
      const msgId = ++id;
      pending.set(msgId, (msg) =>
        msg.error ? reject(new Error(`${method}: ${msg.error.message}`)) : resolve(msg.result)
      );
      ws.send(JSON.stringify({ id: msgId, method, params }));
    });
  const once = (method) => new Promise((resolve) => listeners.set(method, resolve));
  return { ready, send, once, close: () => ws.close() };
}

async function evalIn(page, expression) {
  const cdp = connect(page.webSocketDebuggerUrl);
  await cdp.ready;
  try {
    return await cdp.send('Runtime.evaluate', {
      expression,
      returnByValue: true,
      awaitPromise: true,
      userGesture: true,
    });
  } finally {
    cdp.close();
  }
}

// ---------- commands ----------
async function freePort() {
  return new Promise((res, rej) => {
    const s = net.createServer();
    s.once('error', rej);
    s.listen(0, '127.0.0.1', () => {
      const p = s.address().port;
      s.close(() => res(p));
    });
  });
}

async function cmdLaunch() {
  const url = positional()[0] || 'about:blank';
  let name = opt('--name') || `chrome-${Math.random().toString(36).slice(2, 8)}`;
  // Never overwrite a live registry entry — suffix colliding names instead.
  const known = loadRegistry().instances;
  if (known[name] && (await instAlive(known[name]))) {
    let k = 1;
    while (known[`${name}-${k}`] && (await instAlive(known[`${name}-${k}`]))) k++;
    name = `${name}-${k}`;
  }
  const profile = opt('--profile') || `/tmp/lite-chrome-${name}`;
  const app = opt('--app') || process.env.CDP_APP || 'Google Chrome';
  const ext = opt('--extension');
  const port = Number(opt('--port')) || (await freePort());

  const chromeArgs = [
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${profile}`,
    '--no-first-run',
    '--no-default-browser-check',
  ];
  if (ext) chromeArgs.push(`--load-extension=${ext}`);
  chromeArgs.push(url);

  // Spawn the binary directly so we know the pid (needed for close/status).
  // Fall back to `open -na` when the bundle path is unknown (pid then unknown).
  const bin = `/Applications/${app}.app/Contents/MacOS/${app}`;
  let pid = null;
  if (existsSync(bin)) {
    const child = spawn(bin, chromeArgs, { detached: true, stdio: 'ignore' });
    child.unref();
    pid = child.pid;
  } else {
    spawn('open', ['-na', app, '--args', ...chromeArgs], { stdio: 'ignore' }).unref();
  }

  for (let i = 0; i < 40; i++) {
    try {
      const v = await (await fetch(`http://localhost:${port}/json/version`)).json();
      saveRegistry((instances) => {
        instances[name] = { name, port, pid, profile, url, app, autoProfile: !opt('--profile'), started: new Date().toISOString() };
      });
      console.log(JSON.stringify({ ok: true, name, port, pid, profile, browser: v.Browser, webSocketDebuggerUrl: v.webSocketDebuggerUrl }, null, 2));
      return;
    } catch {
      await new Promise((r) => setTimeout(r, 500));
    }
  }
  die(`Chrome did not open a debug port on ${port} within 20s`);
}

async function cmdTabs() {
  const inst = await resolveInstance();
  const list = await targets(inst.port);
  console.log(JSON.stringify(list.filter((t) => t.type === 'page').map((t) => ({ id: t.id, title: t.title, url: t.url })), null, 2));
}

async function cmdEval() {
  let expr = positional()[0];
  if (!expr) die('eval requires an expression or @file');
  if (expr.startsWith('@')) expr = readFileSync(expr.slice(1), 'utf8');
  const inst = await resolveInstance();
  const res = await evalIn(await pickTarget(inst.port), expr);
  if (res.exceptionDetails) {
    console.error('EXCEPTION:', JSON.stringify(res.exceptionDetails, null, 2).slice(0, 4000));
    process.exitCode = 1;
  } else if (flag('--raw')) {
    console.log(JSON.stringify(res.result));
  } else {
    const v = res.result;
    console.log(v.value !== undefined ? JSON.stringify(v.value, null, 2) : v.description ?? v.type);
  }
}

async function cmdNav() {
  const url = positional()[0];
  if (!url) die('nav requires a url');
  const inst = await resolveInstance();
  const page = await pickTarget(inst.port);
  const cdp = connect(page.webSocketDebuggerUrl);
  await cdp.ready;
  try {
    await cdp.send('Page.enable');
    const loaded = cdp.once('Page.loadEventFired');
    await cdp.send('Page.navigate', { url });
    await Promise.race([loaded, new Promise((r) => setTimeout(r, 15000))]);
    const cur = await cdp.send('Runtime.evaluate', { expression: 'location.href', returnByValue: true });
    console.log(JSON.stringify({ ok: true, url: cur.result.value }));
  } finally {
    cdp.close();
  }
}

async function cmdWait() {
  const text = positional()[0];
  if (!text) die('wait requires text');
  const timeout = Number(opt('--timeout') || 15);
  const inst = await resolveInstance();
  const page = await pickTarget(inst.port);
  const needle = JSON.stringify(text);
  const t0 = Date.now();
  while ((Date.now() - t0) / 1000 < timeout) {
    const res = await evalIn(page, `document.body && document.body.innerText.includes(${needle})`);
    if (res.result?.value === true) {
      console.log(JSON.stringify({ ok: true, found: text, url: page.url }));
      return;
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  console.log(JSON.stringify({ ok: false, found: false, text, timeout, url: page.url }));
  process.exitCode = 2;
}

async function cmdShot() {
  const out = positional()[0];
  if (!out) die('shot requires an output path');
  const inst = await resolveInstance();
  const page = await pickTarget(inst.port);
  const cdp = connect(page.webSocketDebuggerUrl);
  await cdp.ready;
  try {
    const res = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: flag('--full') });
    writeFileSync(out, Buffer.from(res.data, 'base64'));
    console.log(JSON.stringify({ ok: true, path: out }));
  } finally {
    cdp.close();
  }
}

async function cmdCookie() {
  const pairs = positional();
  const domain = opt('--domain');
  if (!pairs.length || !domain) die('cookie requires <name=value>... and --domain <d>');
  const inst = await resolveInstance();
  const page = await pickTarget(inst.port);
  const cdp = connect(page.webSocketDebuggerUrl);
  await cdp.ready;
  try {
    const results = [];
    for (const p of pairs) {
      const eq = p.indexOf('=');
      if (eq < 1) die(`malformed cookie "${p}" — expected name=value`);
      const params = {
        name: p.slice(0, eq),
        value: p.slice(eq + 1),
        domain,
        path: opt('--path') || '/',
        secure: flag('--secure'),
        httpOnly: flag('--httpOnly'),
      };
      const sameSite = opt('--sameSite');
      if (sameSite) params.sameSite = sameSite;
      const exp = Number(opt('--expires'));
      if (exp) params.expires = exp;
      const r = await cdp.send('Network.setCookie', params);
      results.push({ name: params.name, success: r.success === true });
    }
    console.log(JSON.stringify({ ok: true, cookies: results }));
  } finally {
    cdp.close();
  }
}

async function cmdStatus() {
  const reg = loadRegistry().instances;
  const rows = [];
  for (const [name, i] of Object.entries(reg)) {
    const alive = await instAlive(i);
    let url = null;
    if (alive) {
      try {
        const list = await targets(i.port);
        url = (list.find((t) => t.type === 'page' && !t.url.startsWith('chrome://')) || list[0])?.url;
      } catch {}
    }
    rows.push({ name, port: i.port, pid: i.pid, alive, profile: i.profile, url });
  }
  console.log(JSON.stringify(rows, null, 2));
}

async function cmdClose() {
  const names = positional();
  const purge = flag('--purge');
  const reg = loadRegistry().instances;
  const targets_ = flag('--all') ? Object.keys(reg) : names;
  if (!targets_.length) die('close requires <name>... or --all');
  const closed = [];
  for (const n of targets_) {
    const i = reg[n];
    if (!i) continue;
    if (i.pid && pidAlive(i.pid)) {
      try {
        process.kill(i.pid, 'SIGTERM');
      } catch {}
      for (let w = 0; w < 30 && pidAlive(i.pid); w++) {
        await new Promise((r) => setTimeout(r, 100));
      }
    }
    if (purge && i.autoProfile && existsSync(i.profile)) rmSync(i.profile, { recursive: true, force: true });
    closed.push(n);
  }
  saveRegistry((instances) => targets_.forEach((n) => delete instances[n]));
  console.log(JSON.stringify({ ok: true, closed }));
}

const commands = { launch: cmdLaunch, tabs: cmdTabs, eval: cmdEval, nav: cmdNav, wait: cmdWait, shot: cmdShot, cookie: cmdCookie, status: cmdStatus, close: cmdClose };
if (!cmd || !commands[cmd]) {
  console.log(USAGE);
  process.exit(cmd ? 1 : 0);
}
commands[cmd]().catch((e) => die(e.message));
