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
import { testViewerTools } from './viewer-tools-browser.mjs';
import { testViewerV3 } from './viewer-v3-browser.mjs';

const id = (n) => `${String(n).padStart(8, '0')}-00000000-00000000-00000000-00000000`;
const study = id(1), series = id(2), instance = id(3);
const studyPath = `/api/studies/${study}/series`;
const instancesPath = `${studyPath}/${series}/instances`;
const dicomPath = `${instancesPath}/${instance}/dicom`;
const calls = [];
let failImage = false;
let expireImage = false;
let expireThumbnail = false;
const server = createServer(async (req, res) => {
  const path = new URL(req.url, 'http://fixture.invalid').pathname;
  const json = (body) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(body)); };
  res.setHeader('Cache-Control', 'no-store');
  if (path.startsWith('/api/')) calls.push(path);
  if (path === '/api/auth/me') return json({ user: { id: 'synthetic-user', name: 'Usuário fictício', username: 'ficticio', role: 'MEDICO', unit: null, active: true, accessValidUntil: null, lastLoginAt: null } });
  if (path === '/api/studies') return json({ items: [{ orthancStudyId: study, studyInstanceUid: '2.25.111111111', studyDate: '20260920', studyTime: '101112', patientName: 'FICTICIO^VIEWER', patientId: 'SYNTH-VIEWER', accessionNumber: 'SYNTH-ACC', studyDescription: 'Padrão sintético', institutionName: 'Instituição fictícia', modalities: ['OT'], seriesCount: 1 }], limit: 25, offset: 0, hasMore: false, nextOffset: null });
  if (path === studyPath) return json({ items: [
    { orthancSeriesId: series, description: 'Série sintética A', number: '1', modality: 'OT', instanceCount: 3 },
    { orthancSeriesId: id(4), description: 'Série sintética B', number: '2', modality: 'OT', instanceCount: 2 },
    { orthancSeriesId: id(5), description: '', number: '3', modality: 'OT', instanceCount: 0 },
    { orthancSeriesId: id(6), description: 'Série com falha fictícia', number: '4', modality: 'OT', instanceCount: 1 },
    { orthancSeriesId: id(7), description: 'Série lenta fictícia', number: '5', modality: 'OT', instanceCount: 1 },
  ] });
  if (path === instancesPath) return json({ items: [3, 30, 31].map((n, i) => ({ orthancInstanceId: id(n), number: i + 1 })) });
  if (path === `${studyPath}/${id(4)}/instances`) return json({ items: [40, 41].map((n, i) => ({ orthancInstanceId: id(n), number: i + 1 })) });
  if (path === `${studyPath}/${id(5)}/instances`) return json({ items: [] });
  if (path === `${studyPath}/${id(6)}/instances`) { res.statusCode = 502; return json({ error: { code: 'VIEWER_UNAVAILABLE', message: 'Série indisponível.' } }); }
  if (path === `${studyPath}/${id(7)}/instances`) { await new Promise((resolve) => setTimeout(resolve, 300)); return json({ items: [{ orthancInstanceId: id(70), number: 1 }] }); }
  if (path.endsWith(`/${id(30)}/dicom`) && (failImage || expireImage)) { res.statusCode = expireImage ? 401 : 502; return json({ error: { code: 'FIXTURE_FAILURE', message: 'Falha sintética.' } }); }
  if (expireThumbnail && path.endsWith(`/${id(40)}/dicom`)) { res.statusCode = 401; return json({ error: { code: 'SESSION_EXPIRED', message: 'Sessão expirada.' } }); }
  if (path.endsWith('/dicom') && path.startsWith(studyPath)) { res.setHeader('Content-Type', 'application/dicom'); return res.end(syntheticDICOM({ calibrated: !path.includes(`/${id(4)}/`) })); }
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
  const command = (method, params = {}) => new Promise((resolve, reject) => { const id = ++seq; pending.set(id, { resolve, reject: (error) => reject(new Error(`${method}: ${error.message}`)) }); socket.send(JSON.stringify({ id, method, params })); });
  const evaluate = async (expression) => { const result = await command('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }); if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description ?? 'Falha na expressão do teste'); return result.result?.value; };
  await command('Runtime.enable'); await command('Network.enable'); await command('Page.enable');
  // Fontes externas já existentes no design não participam deste teste.
  await command('Network.setBlockedURLs', { urls: ['https://*', 'http://fonts.*'] });
  await command('Emulation.setDeviceMetricsOverride', { width: 1440, height: 600, deviceScaleFactor: 1, mobile: false });
  await command('Page.navigate', { url: origin });
  await until(() => evaluate('!!document.querySelector(\'[role="button"]\')'));
  await evaluate('document.querySelector(\'[role="button"]\').click()');
  try {
    await until(() => evaluate('document.body.textContent.includes("Imagem 1 / 3")'), 45000);
  } catch (error) {
    console.error('Falhas JS no ambiente sintético:', failures);
    console.error('Chamadas fictícias:', calls);
    console.error(await evaluate('document.body.textContent'));
    throw error;
  }
  await pause(250);
  assert.equal(calls.some(path=>path.includes(`/${id(7)}/`) && path.endsWith('/dicom')),false,'card fora da área visível não baixa DICOM');
  await evaluate("document.querySelector('aside').scrollTop=10000");
  await until(()=>evaluate("Array.from(document.querySelectorAll('aside [data-thumbnail]')).at(-1).dataset.thumbnail==='ready'"));
  assert.equal(calls.filter(path=>path.includes(`/${id(7)}/`) && path.endsWith('/dicom')).length,1,'uma instância representativa ao tornar card visível');
  await evaluate("document.querySelector('aside').scrollTop=0");
  await command('Emulation.setDeviceMetricsOverride', { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });
  await pause(200);
  assert.equal(await evaluate('location.pathname'), `/viewer/${study}`);
  assert.ok(calls.includes(studyPath) && calls.includes(instancesPath) && calls.includes(dicomPath));
  assert.ok(await evaluate('Array.from(document.querySelectorAll("canvas")).some(c => c.width > 0 && c.height > 0)'));
  const count = async (text) => { try { await until(() => evaluate(`document.body.textContent.includes(${JSON.stringify(text)})`)); } catch (error) { console.error('Estado sintético:', text, await evaluate('document.body.textContent'), failures, calls.slice(-5)); throw error; } };
  // Eventos reais do Chrome: não dispara KeyboardEvent artificial no <main>.
  const key = async (key) => {
    const code = { ArrowLeft: 37, ArrowUp: 38, ArrowRight: 39, ArrowDown: 40, Delete: 46, Backspace: 8 }[key];
    await command('Input.dispatchKeyEvent', { type: 'keyDown', key, code: key, windowsVirtualKeyCode: code });
    await command('Input.dispatchKeyEvent', { type: 'keyUp', key, code: key, windowsVirtualKeyCode: code });
  };
  const choose = async (index) => { await evaluate(`document.querySelectorAll('aside button')[${index}].click()`); };
  const files = () => calls.filter((path) => path.endsWith('/dicom'));
  assert.ok(files().every(path => !path.includes(`/${id(30)}/`) && !path.includes(`/${id(31)}/`) && !path.includes(`/${id(41)}/`)), 'miniaturas usam somente primeira instância');
  await evaluate("window.fixtureEngine = document.querySelector('main canvas')");
  await testViewerTools({ evaluate, command, key, count, choose, pause, until });
  const loadedAfterTools = files().length;
  // Reproduz perda de foco do main que não era coberta no V1.
  await evaluate("document.querySelector('aside button').focus()");
  await key('ArrowRight'); await count('Imagem 2 / 3');
  assert.equal(files().length, loadedAfterTools + 1);
  const wheelPoint = await evaluate("(() => {const r=document.querySelector('main canvas').getBoundingClientRect(); return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
  await command('Input.dispatchMouseEvent', { type:'mouseWheel', ...wheelPoint, deltaX:0, deltaY:100 });
  await count('Imagem 3 / 3');
  await key('ArrowDown'); await pause(100); await count('Imagem 3 / 3');
  await key('ArrowLeft'); await count('Imagem 2 / 3');
  await key('ArrowUp'); await count('Imagem 1 / 3');
  await key('ArrowLeft'); await pause(100); await count('Imagem 1 / 3');
  await key('ArrowDown'); await count('Imagem 2 / 3');
  await key('ArrowUp'); await count('Imagem 1 / 3');
  await choose(1); await count('Imagem 1 / 2');
  assert.equal(await evaluate("document.querySelectorAll('aside button')[1].getAttribute('aria-pressed')"), 'true');
  assert.equal(await evaluate("window.fixtureEngine === document.querySelector('main canvas')"), true, 'engine/viewport deve ser reutilizado');
  assert.ok(files().filter((path) => path.includes(`/${id(4)}/`)).every((path) => path.endsWith(`/${id(40)}/dicom`)), 'sem prefetch da segunda imagem B');
  await choose(2); await count('Série sem imagens');
  await choose(3); await count('Não foi possível carregar esta série ou imagem.');
  await choose(4); await choose(0); await count('Imagem 1 / 3');
  await pause(400);
  assert.equal(await evaluate("document.querySelector('[data-active=true]').textContent.includes('Série sintética A')"), true, 'seleção obsoleta não substitui viewport');
  assert.equal(await evaluate("document.querySelectorAll('aside button')[0].getAttribute('aria-pressed')"), 'true');
  await testViewerV3({ evaluate, command, key, count, choose, pause, until, calls, id });
  const reopen = async () => {
    await evaluate("document.querySelector('header button').click()");
    await until(() => evaluate("!!document.querySelector('[role=button]')"));
    await evaluate("document.querySelector('[role=button]').click()");
    await count('Imagem 1 / 3');
  };
  await reopen();
  // Falha de um arquivo não bloqueia a navegação para a próxima instância.
  failImage = true;
  await key('ArrowRight'); await count('Não foi possível carregar esta série ou imagem.');
  await key('ArrowRight'); await count('Imagem 3 / 3');
  failImage = false;
  for (const tag of ['input', 'textarea', 'select', 'div']) {
    await evaluate(`{ const input = document.createElement('${tag}'); input.id='fixture-editable'; if ('${tag}' === 'div') input.contentEditable='true'; document.querySelector('main').append(input); input.focus(); }`);
    await key('ArrowLeft'); await pause(100); await count('Imagem 3 / 3');
    await evaluate("document.querySelector('#fixture-editable').remove()");
  }
  assert.equal(failures.length, 0);
  assert.deepEqual(network.filter((url) => !url.startsWith(origin) && !url.startsWith('blob:') && !url.startsWith('data:') && !/^https:\/\/fonts\.(googleapis|gstatic)\.com\//.test(url)), []);
  const screenshot = await command('Page.captureScreenshot', { format: 'png' });
  const output = join(tmpdir(), 'pacs-viewer-synthetic.png');
  await writeFile(output, Buffer.from(screenshot.data, 'base64'));
  await choose(1); await count('Imagem 1 / 2');
  await choose(0); await count('Imagem 1 / 3');
  await reopen();
  expireImage = true;
  await evaluate("Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()==='Play').click()");
  await count('Sua sessão expirou por segurança.');
  const stoppedRequests = calls.length; await pause(400); assert.equal(calls.length, stoppedRequests, 'expiração durante Cine encerra requests');
  expireImage = false; expireThumbnail = true;
  await command('Page.navigate', { url: origin });
  await until(()=>evaluate("!!document.querySelector('[role=button]')"));
  await evaluate("document.querySelector('[role=button]').click()");
  await count('Sua sessão expirou por segurança.');
  const stoppedThumbnailRequests=calls.length; await pause(400); assert.equal(calls.length,stoppedThumbnailRequests,'expiração de thumbnail encerra operações');
  assert.equal(failures.length, 0);
  console.log(`PASS: Viewer V3 + regressão V2 — stack sob demanda, setas/scroll, contador, troca de série, vazio, falha e seleção obsoleta. Screenshot: ${output}`);
} finally {
  socket?.close(); chrome.kill();
  await once(chrome, 'exit').catch(() => {});
  await new Promise((resolve) => server.close(resolve));
  await rm(profile, { recursive: true, force: true });
}
