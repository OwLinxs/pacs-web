// API administrativa sintética + Chrome temporário; não usa backend/.env/banco real.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, extname } from 'node:path';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
let role='ADMIN',expire=false,fail=false;
let next=1;const units=[];const writes=[];
const server=createServer(async(req,res)=>{
 const url=new URL(req.url,'http://fixture.invalid');
 res.setHeader('Cache-Control','no-store');
 const json=(body,status=200)=>{res.statusCode=status;res.setHeader('Content-Type','application/json');res.end(JSON.stringify(body));};
 if(url.pathname==='/api/auth/me'){res.setHeader('Set-Cookie','pacs_csrf=synthetic-csrf; Path=/; SameSite=Lax');return json({user:{id:'synthetic-admin',name:'Administrador Sintético',username:'synthetic',role,unit:null,active:true,accessValidUntil:null,lastLoginAt:null}});}
 if(url.pathname==='/api/studies')return json({items:[],limit:25,offset:0,hasMore:false,nextOffset:null});
 if(url.pathname==='/api/units'){
  if(expire)return json({error:{code:'UNAUTHENTICATED',message:'Sessão expirada.'}},401);
  if(fail)return json({error:{code:'SERVICE_UNAVAILABLE',message:'Unidades indisponíveis.'}},503);
  assert.equal(url.searchParams.get('includeInactive'),'true');
  return json({items:units,limit:50,offset:0,hasMore:false,nextOffset:null});
 }
 if(url.pathname.startsWith('/api/admin/units')){
  if(req.headers['x-csrf-token']!=='synthetic-csrf')return json({error:{code:'CSRF_INVALID',message:'Token inválido.'}},403);
  let raw='';for await(const chunk of req)raw+=chunk;
  const body=JSON.parse(raw);const id=url.pathname.split('/').at(-1);
  if(body.name&&units.some(u=>u.id!==id&&u.name.toLowerCase()===body.name.toLowerCase()))return json({error:{code:'CONFLICT',message:'Já existe uma unidade com esse nome, inclusive entre as inativas.'}},409);
  writes.push({method:req.method,body});
  if(req.method==='POST'){const u={id:`0000000${next++}-0000-4000-8000-000000000001`,name:body.name,active:true,createdAt:'2026-09-28T12:00:00Z',updatedAt:'2026-09-28T12:00:00Z'};units.push(u);return json(u,201);}
  const u=units.find(u=>u.id===id);if(!u)return json({error:{code:'NOT_FOUND',message:'Unidade ausente.'}},404);
  Object.assign(u,body);return json(u);
 }
 if(url.pathname.startsWith('/api/'))return json({error:{code:'NOT_FOUND',message:'Fixture ausente.'}},404);
 try{const path=url.pathname;const asset=path.startsWith('/assets/')&&/^\/assets\/[\w.-]+$/.test(path)?path.slice(1):'index.html';res.setHeader('Content-Type',({'.js':'text/javascript','.css':'text/css','.wasm':'application/wasm','.html':'text/html'})[extname(asset)]??'application/octet-stream');res.end(await readFile(new URL(`../dist/${asset}`,import.meta.url)));}catch{res.statusCode=404;res.end();}
});
server.listen(0, '127.0.0.1'); await once(server, 'listening');
const origin = `http://127.0.0.1:${server.address().port}`;
const profile = await mkdtemp(join(tmpdir(), 'pacs-units-smoke-'));
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

  await command('Emulation.setDeviceMetricsOverride', { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });

  let acceptDialog=true;
  socket.addEventListener('message',event=>{const message=JSON.parse(event.data);if(message.method==='Page.javascriptDialogOpening')void command('Page.handleJavaScriptDialog',{accept:acceptDialog});});
  const click=async label=>{await until(()=>evaluate(`Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()===${JSON.stringify(label)}&&!b.disabled)`));await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(label)}).click()`);};
  const text=()=>evaluate('document.body.textContent');
  const setName=async value=>{await until(()=>evaluate(`!!document.querySelector('#unit-name:not(:disabled)')`));return evaluate(`(()=>{const el=document.querySelector('#unit-name');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(el,${JSON.stringify(value)});el.dispatchEvent(new Event('input',{bubbles:true}));})()`);};
  const ready=()=>until(()=>evaluate(`!document.querySelector('[aria-busy="true"]') && Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()==='Nova unidade'&&!b.disabled)`));
  await command('Page.navigate',{url:origin});
  await until(()=>evaluate(`Array.from(document.querySelectorAll('nav button')).some(b=>b.textContent==='Unidades')`));
  await click('Unidades');await ready();assert.ok((await text()).includes('Nenhuma unidade cadastrada.'));
  await click('Nova unidade');await setName('   ');await click('Salvar unidade');await until(()=>evaluate(`document.querySelector('[role="alert"]')?.textContent.includes('Informe um nome')`));assert.equal(writes.length,0);
  await setName('  Unidade Sintética  ');await click('Salvar unidade');await until(()=>evaluate(`document.querySelector('tbody')?.textContent.includes('Unidade Sintética')`));await ready();
  assert.equal(units[0].name,'Unidade Sintética');assert.ok(units[0].active);
  await click('Editar');await setName('Unidade Editada Sintética');await click('Salvar unidade');await until(()=>units[0].name==='Unidade Editada Sintética');await ready();
  await click('Nova unidade');await setName('unidade editada sintética');await click('Salvar unidade');await until(()=>evaluate(`document.querySelector('[role="alert"]')?.textContent.includes('Já existe')`));await click('Cancelar');
  acceptDialog=false;await click('Desativar');assert.equal(units[0].active,true);
  acceptDialog=true;await click('Desativar');await until(()=>units[0].active===false);await ready();assert.ok((await text()).includes('Inativa'));
  await click('Reativar');await until(()=>units[0].active===true);await ready();assert.ok((await text()).includes('Ativa'));
  const screenshot=await command('Page.captureScreenshot',{format:'png'});await writeFile(join(tmpdir(),'pacs-units-synthetic.png'),Buffer.from(screenshot.data,'base64'));
  assert.deepEqual(writes.map(w=>w.method),['POST','PATCH','PATCH','PATCH']);
  // Falha de listagem e retry ao entrar na tela.
  await click('Exames');fail=true;await click('Unidades');await until(()=>evaluate('document.body.textContent.includes("Tentar novamente")'));fail=false;await click('Tentar novamente');await ready();
  await click('Exames');expire=true;await click('Unidades');await until(()=>evaluate('document.body.textContent.includes("Sua sessão expirou")'));expire=false;
  for(const nextRole of ['GESTOR','MEDICO']){
   role=nextRole;await command('Page.navigate',{url:origin});await until(()=>evaluate(`!!document.querySelector('nav')`));
   assert.equal(await evaluate(`Array.from(document.querySelectorAll('nav button')).some(b=>b.textContent==='Unidades')`),false);
   assert.equal(await evaluate(`Array.from(document.querySelectorAll('button')).some(b=>b.textContent==='Nova unidade')`),false);
  }
  assert.equal(failures.length,0,failures.join('\n'));
  console.log('PASS: Administração Unidades — criar, editar, validar, conflito, cancelar desativação, desativar/reativar, retry, expiração e controles somente ADMIN.');
} finally {
 socket?.close();chrome.kill();await once(chrome,'exit').catch(()=>{});server.closeAllConnections();await new Promise(resolve=>server.close(resolve));await rm(profile,{recursive:true,force:true});
}
