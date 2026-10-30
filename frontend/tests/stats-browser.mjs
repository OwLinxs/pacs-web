// Synthetic statistics/browser test: no real backend, database or PACS.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, extname } from 'node:path';
import { spawn } from 'node:child_process';
import { once } from 'node:events';

let role = 'GESTOR', failExams = false, failAll = false;
const requests = [];
const days = (from, n) => Array.from({ length: n }, (_, i) => { const d = new Date(`${from}T00:00:00Z`); d.setUTCDate(d.getUTCDate() + i); return d.toISOString().slice(0, 10); });
function stats(period, date) {
  const week = days('2026-10-26', 7), month = days('2026-10-01', 31);
  const labels = period === 'day' ? Array.from({ length: 24 }, (_, h) => String(h).padStart(2, '0')) : period === 'week' ? week : month;
  const buckets = labels.map((label, i) => ({ label, count: period === 'day' ? (i >= 7 && i <= 22 ? (i * 7) % 11 + 1 : 0) : (i * 13) % 17 + 4 }));
  const total = buckets.reduce((s, b) => s + b.count, 0);
  return {
    period, from: period === 'day' ? date : labels[0], to: period === 'day' ? date : labels.at(-1), previousFrom: '2026-10-19', previousTo: '2026-10-25',
    exams: failExams ? null : { total, previousTotal: Math.round(total * 0.9), truncated: false, withoutTime: 2, buckets, byHour: Array.from({ length: 24 }, (_, h) => (h >= 7 && h <= 22 ? (h * 5) % 13 + 2 : 0)) },
    ...(failExams ? { examsError: 'Estatísticas de exames indisponíveis. Verifique a conexão com o PACS.' } : {}),
    archive: failExams ? null : { studies: 48213, series: 61022, instances: 130455, diskMB: 812345 },
    usage: { logins: 41, loginFailures: 3, activeUsers: 17, exports: 6, previousLogins: 38, previousActiveUsers: 17, previousExports: 0 },
  };
}
const server = createServer(async (req, res) => {
  const url = new URL(req.url, 'http://fixture.invalid'); res.setHeader('Cache-Control', 'no-store');
  const json = (body, status = 200) => { res.statusCode = status; res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(body)); };
  if (url.pathname === '/api/auth/me') { res.setHeader('Set-Cookie', 'pacs_csrf=synthetic; Path=/; SameSite=Lax'); return json({ user: { id: 'synthetic', name: 'Operador Sintético', username: 'operator', role, unit: null, units: [], mustChangePassword: false, active: true, accessValidUntil: null, lastLoginAt: null } }); }
  if (url.pathname === '/api/studies') return json({ items: [], limit: 25, offset: 0, hasMore: false, nextOffset: null });
  if (url.pathname === '/api/stats') {
    requests.push(Object.fromEntries(url.searchParams));
    if (failAll) return json({ error: { code: 'SERVICE_UNAVAILABLE', message: 'Falha sintética.' } }, 503);
    return json(stats(url.searchParams.get('period'), url.searchParams.get('date')));
  }
  if (url.pathname.startsWith('/api/')) return json({ error: { code: 'NOT_FOUND', message: 'Fixture ausente.' } }, 404);
  try { const path = url.pathname; const asset = path.startsWith('/assets/') && /^\/assets\/[\w.-]+$/.test(path) ? path.slice(1) : 'index.html'; res.setHeader('Content-Type', ({ '.js': 'text/javascript', '.css': 'text/css', '.wasm': 'application/wasm', '.html': 'text/html' })[extname(asset)] ?? 'application/octet-stream'); res.end(await readFile(new URL(`../dist/${asset}`, import.meta.url))); } catch { res.statusCode = 404; res.end(); }
});
server.listen(0, '127.0.0.1'); await once(server, 'listening');
const origin = `http://127.0.0.1:${server.address().port}`;
const profile = await mkdtemp(join(tmpdir(), 'pacs-stats-smoke-'));
const chrome = spawn(process.env.CHROME_BIN ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', [
  '--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--disable-sync',
  '--remote-debugging-port=0', `--user-data-dir=${profile}`, '--window-size=1440,1000', 'about:blank',
], { stdio: 'ignore' });
let socket;
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const until = async (fn, timeout = 30000) => { const end = Date.now() + timeout; while (Date.now() < end) { const r = await fn(); if (r) return r; await pause(100); } throw new Error('Tempo excedido no teste sintético de estatísticas.'); };
try {
  const port = await until(async () => { try { return (await readFile(join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0]; } catch { return null; } });
  const target = await (await fetch(`http://127.0.0.1:${port}/json/new?${encodeURIComponent('about:blank')}`, { method: 'PUT' })).json();
  socket = new WebSocket(target.webSocketDebuggerUrl); await once(socket, 'open');
  let seq = 0; const pending = new Map(), failures = [];
  socket.addEventListener('message', (event) => { const m = JSON.parse(event.data); if (m.id) { const p = pending.get(m.id); pending.delete(m.id); if (m.error) p.reject(new Error(m.error.message)); else p.resolve(m.result); } if (m.method === 'Runtime.exceptionThrown') failures.push(m.params.exceptionDetails.exception?.description ?? 'Erro JS'); });
  const command = (method, params = {}) => new Promise((resolve, reject) => { const id = ++seq; pending.set(id, { resolve, reject }); socket.send(JSON.stringify({ id, method, params })); });
  const evaluate = async (expression) => { const r = await command('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }); if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? 'Falha na expressão'); return r.result?.value; };
  await command('Runtime.enable'); await command('Page.enable');
  await command('Network.enable'); await command('Network.setBlockedURLs', { urls: ['https://*', 'http://fonts.*'] });
  await command('Emulation.setDeviceMetricsOverride', { width: 1440, height: 1100, deviceScaleFactor: 1, mobile: false });
  const text = () => evaluate('document.body.textContent');
  const click = async (label) => { await until(() => evaluate(`Array.from(document.querySelectorAll('button')).some(b=>(b.textContent.trim()===${JSON.stringify(label)}||b.getAttribute('aria-label')===${JSON.stringify(label)})&&!b.disabled)`)); await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(label)}||b.getAttribute('aria-label')===${JSON.stringify(label)}).click()`); };

  await command('Page.navigate', { url: origin }); await click('Estatísticas');
  await until(async () => (await text()).includes('Exames por hora'));
  assert.equal(requests.at(-1).period, 'day');
  const t = await text();
  for (const expected of ['Exames realizados', 'Usuários ativos', '17', 'igual ao dia anterior', '3 falha(s)', 'sem registros no dia anterior', 'Acervo do PACS', '48.213', '793,3 GB', '2 exame(s) sem horário válido']) assert.ok(t.includes(expected), `faltou ${expected}`);
  assert.equal(await evaluate(`document.querySelectorAll('[role="list"] [role="listitem"]').length`), 24);
  // Tooltip por hover mostra hora e contagem.
  const bar = await evaluate(`(()=>{const r=document.querySelectorAll('[role="listitem"]')[8].getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height-10};})()`);
  await command('Input.dispatchMouseEvent', { type: 'mouseMoved', x: bar.x, y: bar.y });
  await until(async () => (await evaluate(`document.querySelector('figure [role="status"]')?.textContent ?? ''`)).includes('08:00–08:59'));
  assert.equal(await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.getAttribute('aria-label')==='Próximo período').disabled`), true, 'não avança além de hoje');

  await click('Semana'); await until(async () => (await text()).includes('Horário de pico'));
  assert.equal(requests.at(-1).period, 'week');
  assert.equal(await evaluate(`document.querySelectorAll('figure')[0].querySelectorAll('[role="listitem"]').length`), 7);
  assert.ok((await text()).includes('seg 26'));
  const shot = await command('Page.captureScreenshot', { format: 'png' }); await writeFile(join(tmpdir(), 'pacs-stats-week.png'), Buffer.from(shot.data, 'base64'));

  await click('Mês'); await until(async () => (await text()).includes('outubro de 2026') || (await text()).includes('de 2026'));
  assert.equal(await evaluate(`document.querySelectorAll('figure')[0].querySelectorAll('[role="listitem"]').length`), 31);
  const monthShot = await command('Page.captureScreenshot', { format: 'png' }); await writeFile(join(tmpdir(), 'pacs-stats-month.png'), Buffer.from(monthShot.data, 'base64'));
  const before = requests.length; await click('Período anterior'); await until(() => requests.length > before);
  assert.ok(requests.at(-1).date < requests[before - 1].date, 'navegou para o período anterior');
  await click('Hoje'); await until(() => requests.at(-1).date === requests[0].date);

  failExams = true; await click('Atualizar'); await until(async () => (await text()).includes('Verifique a conexão com o PACS'));
  assert.ok((await text()).includes('Usuários ativos') && (await text()).includes('41'), 'uso continua visível sem PACS');
  failExams = false; failAll = true; await click('Atualizar'); await until(async () => (await text()).includes('Não foi possível carregar as estatísticas'));
  failAll = false; await click('Tentar novamente'); await until(async () => (await text()).includes('Horário de pico') || (await text()).includes('Exames por'));

  role = 'MEDICO'; await command('Page.navigate', { url: origin }); await until(() => evaluate(`!!document.querySelector('nav')`));
  assert.equal(await evaluate(`Array.from(document.querySelectorAll('nav button')).some(b=>b.textContent.includes('Estatísticas'))`), false, 'MEDICO não vê o menu');
  assert.equal(failures.length, 0, failures.join('\n'));
  console.log('PASS: Estatísticas — dia/semana/mês, navegação, tooltip por hover, tabela, falha parcial do PACS, retry e menu por perfil.');
} finally {
  socket?.close(); chrome.kill(); await once(chrome, 'exit').catch(() => {}); server.closeAllConnections(); await new Promise((resolve) => server.close(resolve)); await rm(profile, { recursive: true, force: true, maxRetries: 10, retryDelay: 150 });
}
