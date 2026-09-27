// Smoke test do build real com Chrome headless e servidor exclusivamente fictício.
// Não usa .env, proxy do Vite, backend, Orthanc ou perfil de navegador existente.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, extname } from 'node:path';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { syntheticDICOM } from './dicom-fixture.mjs';

const id = (n) => `${String(n).padStart(8, '0')}-00000000-00000000-00000000-00000000`;
const study = id(1), series = id(2), instance = id(3);
const studyPath = `/api/studies/${study}/series`;
const instancesPath = `${studyPath}/${series}/instances`;
const dicomPath = `${instancesPath}/${instance}/dicom`;
const calls = [];
const server = createServer(async (req, res) => {
  const path = new URL(req.url, 'http://fixture.invalid').pathname;
  const json = (body) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(body)); };
  res.setHeader('Cache-Control', 'no-store');
  if (path.startsWith('/api/')) calls.push(path);
  if (path === '/api/auth/me') return json({ user: { id: 'synthetic-user', name: 'Usuário fictício', username: 'ficticio', role: 'MEDICO', unit: null, active: true, accessValidUntil: null, lastLoginAt: null } });
  if (path === '/api/studies') return json({ items: [{ orthancStudyId: study, studyInstanceUid: '2.25.111111111', studyDate: '20260920', studyTime: '101112', patientName: 'FICTICIO^VIEWER', patientId: 'SYNTH-VIEWER', accessionNumber: 'SYNTH-ACC', studyDescription: 'Padrão sintético', institutionName: 'Instituição fictícia', modalities: ['OT'], seriesCount: 1 }], limit: 25, offset: 0, hasMore: false, nextOffset: null });
  if (path === studyPath) return json({ items: [{ orthancSeriesId: series, description: 'Padrão sintético', number: '1', modality: 'OT', instanceCount: 1 }] });
  if (path === instancesPath) return json({ items: [{ orthancInstanceId: instance, number: 1 }] });
  if (path === dicomPath) { res.setHeader('Content-Type', 'application/dicom'); return res.end(syntheticDICOM()); }
  if (path.startsWith('/api/')) { res.statusCode = 404; return json({ error: { code: 'NOT_FOUND', message: 'Recurso fictício ausente.' } }); }
  try {
    const asset = path.startsWith('/assets/') && /^\/assets\/[\w.-]+$/.test(path) ? path.slice(1) : 'index.html';
    const types = { '.js': 'text/javascript', '.css': 'text/css', '.wasm': 'application/wasm', '.html': 'text/html' };
    res.setHeader('Content-Type', types[extname(asset)] ?? 'application/octet-stream');
    res.end(await readFile(new URL(`../dist/${asset}`, import.meta.url)));
  } catch { res.statusCode = 404; res.end(); }
});
server.listen(0, '127.0.0.1'); await once(server, 'listening');
const origin = `http://127.0.0.1:${server.address().port}`;
const profile = await mkdtemp(join(tmpdir(), 'pacs-viewer-smoke-'));
const chrome = spawn(process.env.CHROME_BIN ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', [
  '--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--disable-sync',
  '--remote-debugging-port=0', `--user-data-dir=${profile}`, '--window-size=1440,1000',
  '--use-angle=swiftshader', '--enable-unsafe-swiftshader', 'about:blank',
], { stdio: 'ignore' });
let socket;
const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const until = async (fn, timeout = 30000) => {
  const end = Date.now() + timeout;
  while (Date.now() < end) { const result = await fn(); if (result) return result; await pause(100); }
  throw new Error('Tempo excedido no smoke test sintético.');
};
try {
  const port = await until(async () => { try { return (await readFile(join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0]; } catch { return null; } });
  const target = await (await fetch(`http://127.0.0.1:${port}/json/new?${encodeURIComponent('about:blank')}`, { method: 'PUT' })).json();
  socket = new WebSocket(target.webSocketDebuggerUrl); await once(socket, 'open');
  let seq = 0;
  const pending = new Map(), failures = [], network = [];
  socket.addEventListener('message', (event) => {
    const message = JSON.parse(event.data);
    if (message.id) { const p = pending.get(message.id); pending.delete(message.id); if (message.error) p.reject(new Error(message.error.message)); else p.resolve(message.result); }
    if (message.method === 'Runtime.exceptionThrown') failures.push(message.params.exceptionDetails.exception?.description ?? 'Erro JS');
    if (message.method === 'Network.requestWillBeSent') network.push(message.params.request.url);
  });
  const command = (method, params = {}) => new Promise((resolve, reject) => { const id = ++seq; pending.set(id, { resolve, reject }); socket.send(JSON.stringify({ id, method, params })); });
  const evaluate = async (expression) => (await command('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true })).result?.value;
  await command('Runtime.enable'); await command('Network.enable'); await command('Page.enable');
  // Fontes externas já existentes no design não participam deste teste.
  await command('Network.setBlockedURLs', { urls: ['https://*', 'http://fonts.*'] });
  await command('Page.navigate', { url: origin });
  await until(() => evaluate('!!document.querySelector(\'[role="button"]\')'));
  await evaluate('document.querySelector(\'[role="button"]\').click()');
  try {
    await until(() => evaluate('document.body.textContent.includes("Imagem inicial renderizada")'), 45000);
  } catch (error) {
    console.error('Falhas JS no ambiente sintético:', failures);
    console.error('Chamadas fictícias:', calls);
    console.error(await evaluate('document.body.textContent'));
    throw error;
  }
  assert.equal(await evaluate('location.pathname'), `/viewer/${study}`);
  assert.ok(calls.includes(studyPath) && calls.includes(instancesPath) && calls.includes(dicomPath));
  assert.ok(await evaluate('Array.from(document.querySelectorAll("canvas")).some(c => c.width > 0 && c.height > 0)'));
  assert.equal(failures.length, 0);
  assert.deepEqual(network.filter((url) => !url.startsWith(origin) && !url.startsWith('blob:') && !url.startsWith('data:') && !/^https:\/\/fonts\.(googleapis|gstatic)\.com\//.test(url)), []);
  const screenshot = await command('Page.captureScreenshot', { format: 'png' });
  const output = join(tmpdir(), 'pacs-viewer-synthetic.png');
  await writeFile(output, Buffer.from(screenshot.data, 'base64'));
  console.log(`PASS: Worklist → séries → instâncias → DICOM sintético → Cornerstone IMAGE_RENDERED. Screenshot: ${output}`);
} finally {
  socket?.close(); chrome.kill();
  await once(chrome, 'exit').catch(() => {});
  await new Promise((resolve) => server.close(resolve));
  await rm(profile, { recursive: true, force: true });
}
