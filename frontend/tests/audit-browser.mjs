// Synthetic local audit UI, no real database/PACS/accounts.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {readFile,mkdtemp,rm,writeFile} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,extname} from 'node:path';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
let role='ADMIN',fail=false,empty=false,expire=false,delay=0;
const reads=[];
const actor={id:'00000001-0000-4000-8000-000000000001',name:'Operador Sintético',username:'synthetic_admin',kind:'user'};
const target={id:'00000002-0000-4000-8000-000000000001',name:'Alvo Sintético',username:'synthetic_target',kind:'user'};
const event={id:'1',occurredAt:'2026-09-29T12:00:00Z',event:'USER_CREATED',category:'users',actor,target,result:'success',detail:'{"password":"synthetic-sensitive-marker"}',origin:'127.0.0.1'};
const server=createServer(async(req,res)=>{
 const url=new URL(req.url,'http://fixture.invalid');res.setHeader('Cache-Control','no-store');
 const json=(body,status=200)=>{res.statusCode=status;res.setHeader('Content-Type','application/json');res.end(JSON.stringify(body));};
 if(url.pathname==='/api/auth/me')return json({user:{...actor,role,units:[],unit:null,mustChangePassword:false,active:true,accessValidUntil:null,lastLoginAt:null}});
 if(url.pathname==='/api/studies')return json({items:[],limit:25,offset:0,hasMore:false,nextOffset:null});
 if(url.pathname==='/api/admin/users')return json({items:[{...actor,role:'ADMIN'},{...target,role:'MEDICO'}],limit:25,offset:0,hasMore:false,nextOffset:null});
 if(url.pathname==='/api/admin/audit'){
  assert.equal(req.method,'GET');const query=Object.fromEntries(url.searchParams);reads.push(query);
  const lag=delay;const isEmpty=empty;if(lag)await new Promise(r=>setTimeout(r,lag));
  if(expire)return json({error:{code:'UNAUTHENTICATED',message:'Sessão expirada.'}},401);
  if(fail)return json({error:{code:'SERVICE_UNAVAILABLE',message:'Falha sintética.'}},503);
  const offset=Number(query.offset||0);return json({items:isEmpty?[]:[{...event,id:offset?'2':'1',event:offset?'UNIT_CREATED':'USER_CREATED'}],limit:50,offset,hasMore:!isEmpty&&!offset,nextOffset:!isEmpty&&!offset?50:null});
 }
 if(url.pathname.startsWith('/api/'))return json({error:{code:'NOT_FOUND',message:'Fixture ausente.'}},404);
 try{const path=url.pathname;const asset=path.startsWith('/assets/')&&/^\/assets\/[\w.-]+$/.test(path)?path.slice(1):'index.html';res.setHeader('Content-Type',extname(asset)==='.js'?'text/javascript':extname(asset)==='.css'?'text/css':'text/html');res.end(await readFile(new URL('../dist/'+asset,import.meta.url)));}catch{res.statusCode=404;res.end();}
});
server.listen(0,'127.0.0.1');await once(server,'listening');const origin=`http://127.0.0.1:${server.address().port}`;
const profile = await mkdtemp(join(tmpdir(), 'pacs-users-smoke-'));
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
  const browserTarget = await (await fetch(`http://127.0.0.1:${port}/json/new?${encodeURIComponent('about:blank')}`, { method: 'PUT' })).json();
  socket = new WebSocket(browserTarget.webSocketDebuggerUrl); await once(socket, 'open');
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

  await command('Emulation.setDeviceMetricsOverride', { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });

  const click=async label=>{await until(()=>evaluate(`Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()===${JSON.stringify(label)}&&!b.disabled)`));await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(label)}).click()`);};
  const set=async(selector,value)=>{await evaluate(`(()=>{const el=document.querySelector(${JSON.stringify(selector)});Object.getOwnPropertyDescriptor(el.tagName==='SELECT'?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value').set.call(el,${JSON.stringify(value)});el.dispatchEvent(new Event(el.tagName==='SELECT'?'change':'input',{bubbles:true}));})()`);};
  const ready=()=>until(()=>evaluate(`document.querySelector('table')&&!document.querySelector('[aria-busy="true"]')`));
  await command('Page.navigate',{url:origin});await click('Auditoria');await ready();
  assert.equal(await evaluate(`document.querySelector('tbody').textContent.includes('Usuário criado')`),true);
  await evaluate(`document.querySelector('summary').click()`);assert.equal(await evaluate(`document.body.textContent.includes('synthetic-sensitive-marker')`),false);
  assert.equal(await evaluate(`document.body.textContent.includes('Nomes resolvidos pelo cadastro atual.')`),true);
  await click('Próxima');await ready();assert.equal(reads.at(-1).offset,'50');assert.equal(await evaluate(`document.body.textContent.includes('Página 2')`),true);
  await click('Anterior');await ready();assert.equal(reads.at(-1).offset,'0');
  await set('#audit-from','2026-09-01');await set('#audit-to','2026-09-29');await set('#audit-event','USER_CREATED');await set('#audit-category','users');
  await evaluate(`document.querySelector('#audit-actor').focus()`);await set('#audit-actor','synthetic');await click('Operador Sintético / synthetic_admin');
  await evaluate(`document.querySelector('#audit-target').focus()`);await set('#audit-target','synthetic');await click('Alvo Sintético / synthetic_target');
  await click('Aplicar filtros');await ready();assert.equal(reads.at(-1).actorId,actor.id);assert.equal(reads.at(-1).targetUserId,target.id);assert.equal(reads.at(-1).event,'USER_CREATED');assert.equal(reads.at(-1).dateTo,'2026-09-29');
  const before=reads.length;await set('#audit-from','2026-10-01');await click('Aplicar filtros');await until(()=>evaluate(`document.body.textContent.includes('A data inicial deve')`));assert.equal(reads.length,before);
  await click('Limpar filtros');await ready();assert.equal(reads.at(-1).event,undefined);
  const shot=await command('Page.captureScreenshot',{format:'png'});await writeFile(join(tmpdir(),'pacs-audit-synthetic.png'),Buffer.from(shot.data,'base64'));
  fail=true;await click('Atualizar');await until(()=>evaluate(`document.body.textContent.includes('Tentar novamente')`));fail=false;await click('Tentar novamente');await ready();
  empty=true;await click('Atualizar');await ready();assert.equal(await evaluate(`document.body.textContent.includes('Nenhum evento encontrado')`),true);empty=false;
  // A slow old response must not overwrite a later filter request.
  delay=600;empty=true;await click('Atualizar');await until(()=>evaluate(`!!document.querySelector('[aria-busy="true"]')`));await until(()=>reads.length>before);
  await pause(100);delay=0;empty=false;await click('Aplicar filtros');await ready();await pause(700);assert.equal(await evaluate(`document.querySelectorAll('tbody tr').length`),1);
  // Unmount aborts reads; role gates remove Auditoria.
  delay=400;await click('Atualizar');await click('Exames');await pause(500);delay=0;
  for(const value of ['GESTOR','MEDICO']){role=value;await command('Page.navigate',{url:origin});await until(()=>evaluate(`!!document.querySelector('nav')`));assert.equal(await evaluate(`Array.from(document.querySelectorAll('nav button')).some(b=>b.textContent.trim()==='Auditoria')`),false);}
  role='ADMIN';await command('Page.navigate',{url:origin});await click('Auditoria');await ready();expire=true;await click('Atualizar');await until(()=>evaluate(`document.body.textContent.includes('Sua sessão expirou')`));
  assert.equal(failures.length,0,failures.join('\n'));
  console.log('PASS: Auditoria — ADMIN/menu, filtros AND, datas, ator/alvo, paginação, detalhes seguros, retry, vazio, concorrência, unmount e sessão expirada.');
} finally {
 socket?.close();chrome.kill();await once(chrome,'exit').catch(()=>{});server.closeAllConnections();await new Promise(resolve=>server.close(resolve));await rm(profile,{recursive:true,force:true,maxRetries:10,retryDelay:150});
}
