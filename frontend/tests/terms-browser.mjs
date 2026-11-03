// Synthetic terms/privacy browser test: no real backend, database or PACS.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, extname } from 'node:path';
import { spawn } from 'node:child_process';
import { once } from 'node:events';

const VERSION = '2026-10-31-rascunho';
let authenticated = false, termsRequired = true, studiesTermsBlocked = false;
const accepts = [];
const doc = { version: VERSION, title: 'Termo de responsabilidade e sigilo', draft: true, sections: [
  { heading: 'Finalidade', paragraphs: ['Texto sintético de finalidade.'] },
  { heading: 'Compromissos', paragraphs: ['Compromisso sintético um.', 'Compromisso sintético dois.'] },
] };
const server = createServer(async (req, res) => {
  const url = new URL(req.url, 'http://fixture.invalid'); res.setHeader('Cache-Control', 'no-store');
  const json = (body, status = 200) => { res.statusCode = status; res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(body)); };
  if (url.pathname === '/api/auth/me') {
    res.setHeader('Set-Cookie', 'pacs_csrf=synthetic; Path=/; SameSite=Lax');
    if (!authenticated) return json({ error: { code: 'UNAUTHENTICATED', message: 'Sessão inválida ou expirada.' } }, 401);
    return json({ user: { id: 'synthetic', name: 'Médico Sintético', username: 'medico', role: 'MEDICO', unit: null, units: [], mustChangePassword: false, termsRequired, active: true, accessValidUntil: null, lastLoginAt: null } });
  }
  if (url.pathname === '/api/terms') return json(doc);
  if (url.pathname === '/api/terms/accept' && req.method === 'POST') {
    let body = ''; for await (const chunk of req) body += chunk;
    accepts.push({ body: JSON.parse(body), csrf: req.headers['x-csrf-token'] });
    termsRequired = false; res.statusCode = 204; return res.end();
  }
  if (url.pathname === '/api/studies') {
    if (studiesTermsBlocked) return json({ error: { code: 'TERMS_REQUIRED', message: 'Aceite o termo de responsabilidade e sigilo antes de continuar.' } }, 403);
    return json({ items: [], limit: 25, offset: 0, hasMore: false, nextOffset: null });
  }
  if (url.pathname.startsWith('/api/')) return json({ error: { code: 'NOT_FOUND', message: 'Fixture ausente.' } }, 404);
  try { const path = url.pathname; const asset = path.startsWith('/assets/') && /^\/assets\/[\w.-]+$/.test(path) ? path.slice(1) : 'index.html'; res.setHeader('Content-Type', ({ '.js': 'text/javascript', '.css': 'text/css', '.wasm': 'application/wasm', '.html': 'text/html' })[extname(asset)] ?? 'application/octet-stream'); res.end(await readFile(new URL(`../dist/${asset}`, import.meta.url))); } catch { res.statusCode = 404; res.end(); }
});
server.listen(0, '127.0.0.1'); await once(server, 'listening');
const origin = `http://127.0.0.1:${server.address().port}`;
const profile = await mkdtemp(join(tmpdir(), 'pacs-terms-smoke-'));
const chrome = spawn(process.env.CHROME_BIN ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', [
  '--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--disable-sync',
  '--remote-debugging-port=0', `--user-data-dir=${profile}`, '--window-size=1440,1000', 'about:blank',
], { stdio: 'ignore' });
let socket;
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const until = async (fn, timeout = 30000) => { const end = Date.now() + timeout; while (Date.now() < end) { const r = await fn(); if (r) return r; await pause(100); } throw new Error('Tempo excedido no teste sintético do termo.'); };
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
  const text = () => evaluate('document.body.textContent');
  // Procura e clica na mesma avaliação: evita corrida com re-render entre as duas etapas.
  const click = async (label) => { await until(() => evaluate(`(()=>{const b=Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(label)}&&!b.disabled);if(b)b.click();return !!b;})()`)); };
  const enabled = (label) => evaluate(`!Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(label)}).disabled`);
  const dialogOpen = () => evaluate(`!!document.querySelector('dialog[open]')`);

  // Login: aviso de LGPD e de privacidade antes de autenticar.
  await command('Page.navigate', { url: origin });
  await until(async () => (await text()).includes('Dados protegidos pela LGPD'));
  await click('Aviso de privacidade'); await until(dialogOpen);
  for (const expected of ['Base legal', 'Encarregado de dados', 'Rascunho']) assert.ok((await text()).includes(expected), `faltou ${expected}`);
  await command('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 });
  await until(async () => !(await dialogOpen()));

  // Sessão com termo pendente: tela de aceite antes do sistema.
  authenticated = true; await command('Page.navigate', { url: origin });
  await until(async () => (await text()).includes('Compromisso sintético dois.'));
  assert.ok((await text()).includes(`Versão ${VERSION}`), 'versão e rascunho visíveis');
  assert.equal(await evaluate(`!!document.querySelector('nav')`), false, 'sistema bloqueado antes do aceite');
  assert.equal(await enabled('Li e concordo'), false, 'aceite exige marcar a leitura');
  await click('Ler o aviso de privacidade'); await until(dialogOpen); await click('Fechar'); await until(async () => !(await dialogOpen()));
  await evaluate(`document.querySelector('input[type="checkbox"]').click()`);
  await until(() => enabled('Li e concordo'));
  const shot = await command('Page.captureScreenshot', { format: 'png' }); await writeFile(join(tmpdir(), 'pacs-terms.png'), Buffer.from(shot.data, 'base64'));
  await click('Li e concordo');
  await until(async () => (await text()).includes('Exames') && await evaluate(`!!document.querySelector('nav')`));
  assert.deepEqual(accepts.map(a => a.body), [{ version: VERSION }]);
  assert.equal(accepts[0].csrf, 'synthetic', 'aceite envia CSRF');

  // Nova versão publicada no meio da sessão: API recusa e o app volta ao termo.
  await until(async () => (await text()).includes('estudo(s) nesta página'));
  termsRequired = true; studiesTermsBlocked = true;
  await click('Atualizar');
  await until(async () => (await text()).includes('Li o termo e compreendi'));
  assert.equal(failures.length, 0, failures.join('\n'));
  console.log('PASS: Termo e privacidade — aviso no login, bloqueio antes do aceite, leitura obrigatória, CSRF, liberação e nova versão no meio da sessão.');
} finally {
  socket?.close(); chrome.kill(); await once(chrome, 'exit').catch(() => {}); server.closeAllConnections(); await new Promise((resolve) => server.close(resolve)); await rm(profile, { recursive: true, force: true, maxRetries: 10, retryDelay: 150 });
}
