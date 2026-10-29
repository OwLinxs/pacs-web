// Synthetic administration/browser test: no real backend, database or PACS.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, extname } from 'node:path';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { strToU8, zipSync } from 'fflate';
const workbook = sheetData => zipSync({
 '[Content_Types].xml': strToU8('<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>'),
 '_rels/.rels': strToU8('<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>'),
 'xl/workbook.xml': strToU8('<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Entrada" sheetId="1" r:id="rId1"/></sheets></workbook>'),
 'xl/_rels/workbook.xml.rels': strToU8('<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>'),
 'xl/worksheets/sheet1.xml': strToU8(`<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>${sheetData}</sheetData></worksheet>`),
});
const xlsx = workbook('<row r="1"><c r="A1" t="inlineStr"><is><t>QTD</t></is></c><c r="B1" t="inlineStr"><is><t>Nome</t></is></c></row><row r="2"><c r="A2"><v>1</v></c><c r="B2" t="inlineStr"><is><t> EXEMPLO   A </t></is></c></row><row r="3"><c r="A3"><v>2</v></c><c r="B3" t="inlineStr"><is><t>exemplo a</t></is></c></row><row r="4"><c r="A4"><v>3</v></c></row>');
// Somente nomes, sem cabeçalho; "Synthetic Medico" colide com usuário já criado no teste.
const namesOnly = workbook(['Exemplo Alfa', 'Synthetic Medico', 'exemplo alfa'].map((name, i) => `<row r="${i + 1}"><c r="A${i + 1}" t="inlineStr"><is><t>${name}</t></is></c></row>`).join(''));
let role='ADMIN',mustChange=false,authenticated=true,expire=false,fail=false;
const units=[1,2].map(n=>({id:`0000000${n}-0000-4000-8000-000000000001`,name:`Unidade Sintética ${n}`,active:true}));
let next=3;const users=[],writes=[],reads=[];
const server=createServer(async(req,res)=>{
 const url=new URL(req.url,'http://fixture.invalid');res.setHeader('Cache-Control','no-store');
 const json=(body,status=200)=>{res.statusCode=status;res.setHeader('Content-Type','application/json');res.end(JSON.stringify(body));};
 if(url.pathname==='/api/auth/me'){res.setHeader('Set-Cookie','pacs_csrf=synthetic; Path=/; SameSite=Lax');if(!authenticated)return json({error:{code:'UNAUTHENTICATED',message:'Sessão expirada.'}},401);return json({user:{id:'synthetic',name:'Operador Sintético',username:'operator',role,unit:null,units:[],mustChangePassword:mustChange,active:true,accessValidUntil:null,lastLoginAt:null}});}
 if(url.pathname==='/api/studies'){reads.push('studies');return json({items:[],limit:25,offset:0,hasMore:false,nextOffset:null});}
 if(url.pathname==='/api/units')return json({items:units,limit:50,offset:0,hasMore:false,nextOffset:null});
 if(url.pathname==='/api/admin/users'&&req.method==='GET'){
  reads.push(Object.fromEntries(url.searchParams));if(expire)return json({error:{code:'UNAUTHENTICATED',message:'Sessão expirada.'}},401);if(fail)return json({error:{code:'SERVICE_UNAVAILABLE',message:'Falha sintética sanitizada.'}},503);
  let items=users.filter(u=>role==='ADMIN'||u.role==='MEDICO');const search=url.searchParams.get('search');if(search)items=items.filter(u=>(u.name+u.username).toLowerCase().includes(search.toLowerCase()));
  const filterRole=url.searchParams.get('role');if(filterRole)items=items.filter(u=>u.role===filterRole);
  return json({items,limit:25,offset:0,hasMore:false,nextOffset:null});
 }
 if(url.pathname.startsWith('/api/admin/users/')||url.pathname==='/api/auth/change-password'){
  assert.equal(req.headers['x-csrf-token'],'synthetic');let raw='';for await(const c of req)raw+=c;const body=JSON.parse(raw);writes.push({path:url.pathname,method:req.method,body});
  if(url.pathname==='/api/auth/change-password'){mustChange=false;authenticated=false;res.statusCode=204;return res.end();}
  const id=url.pathname.split('/')[4];
  if(['medicos','gestores'].includes(id)){
   if(users.some(u=>u.username===body.username))return json({error:{code:'CONFLICT',message:'Nome de usuário já cadastrado.'}},409);
   const u={id:`0000000${next++}-0000-4000-8000-000000000001`,name:body.name,username:body.username,email:body.email,role:id==='medicos'?'MEDICO':'GESTOR',units:units.filter(u=>body.unitIds.includes(u.id)),active:true,status:'active',accessValidUntil:'2027-01-31',lastLoginAt:null,mustChangePassword:true};users.push(u);return json(u,201);
  }
  const u=users.find(u=>u.id===id);assert.ok(u);
  if(req.method==='PATCH'){u.name=body.name;u.email=body.email;u.units=units.filter(v=>body.unitIds.includes(v.id));}
  if(url.pathname.endsWith('/active')){u.active=body.active;u.status=u.active?'active':'inactive';}
  if(url.pathname.endsWith('/renew'))u.accessValidUntil=body.validity==='unlimited'?null:'2027-04-30';
  return json(u);
 }
 if(url.pathname.startsWith('/api/'))return json({error:{code:'NOT_FOUND',message:'Fixture ausente.'}},404);
 try{const path=url.pathname;const asset=path.startsWith('/assets/')&&/^\/assets\/[\w.-]+$/.test(path)?path.slice(1):'index.html';res.setHeader('Content-Type',({'.js':'text/javascript','.css':'text/css','.wasm':'application/wasm','.html':'text/html'})[extname(asset)]??'application/octet-stream');res.end(await readFile(new URL(`../dist/${asset}`,import.meta.url)));}catch{res.statusCode=404;res.end();}
});
server.listen(0, '127.0.0.1'); await once(server, 'listening');
const origin = `http://127.0.0.1:${server.address().port}`;
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
  socket.addEventListener('message',event=>{const m=JSON.parse(event.data);if(m.method==='Page.javascriptDialogOpening')void command('Page.handleJavaScriptDialog',{accept:acceptDialog});});
  const click=async label=>{await until(()=>evaluate(`Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()===${JSON.stringify(label)}&&!b.disabled)`));await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(label)}).click()`);};
  const set=async(selector,value)=>{await until(()=>evaluate(`!!document.querySelector(${JSON.stringify(selector)})`));await evaluate(`(()=>{const el=document.querySelector(${JSON.stringify(selector)});Object.getOwnPropertyDescriptor(el.tagName==='SELECT'?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value').set.call(el,${JSON.stringify(value)});el.dispatchEvent(new Event(el.tagName==='SELECT'?'change':'input',{bubbles:true}));})()`);};
  const ready=()=>until(()=>evaluate(`!document.querySelector('[aria-busy="true"]')&&!document.querySelector('[role="dialog"]')`));
  const open=async()=>{await command('Page.navigate',{url:origin});await click('Usuários');await ready();};
  await open();await click('Importar médicos');
  await set('#import-unit',units[0].id);
  const upload = async (bytes,filename) => evaluate(`(()=>{const input=document.querySelector('#import-file');const transfer=new DataTransfer();transfer.items.add(new File([Uint8Array.from(atob(${JSON.stringify(Buffer.from(bytes).toString('base64'))}),c=>c.charCodeAt(0))],${JSON.stringify(filename)}));Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'files').set.call(input,transfer.files);input.dispatchEvent(new Event('change',{bubbles:true}));})()`);
  await upload(xlsx,'synthetic.xlsx');
  await until(()=>evaluate(`document.body.textContent.includes('Válidos: 1')&&document.body.textContent.includes('Duplicados: 1')&&document.body.textContent.includes('Inválidos: 1')`));
  assert.equal(await evaluate(`document.querySelector('[role="dialog"]')?.textContent.includes('Unidade Sintética 1')`),true);
  assert.equal(writes.length,0);
  await upload(strToU8('inválido'),'synthetic.xlsx');await until(()=>evaluate(`document.querySelector('[role="dialog"] [role="alert"]')?.textContent.includes('inválido')`));
  await upload(xlsx,'synthetic.xls');await until(()=>evaluate(`document.querySelector('[role="dialog"] [role="alert"]')?.textContent.includes('Selecione um arquivo .xlsx')`));
  await click('Fechar');await click('Novo médico');
  await set('#user-name','Médico Sintético');await set('#user-username','SYNTHETIC_MEDICO');await set('#user-password','synthetic-initial-password');
  await evaluate(`document.querySelectorAll('[role="dialog"] input[type="checkbox"]').forEach(el=>el.click())`);
  await click('Criar usuário');await until(()=>users.length===1);await ready();assert.equal(users[0].units.length,2);assert.equal(users[0].username,'synthetic_medico');assert.equal(users[0].role,'MEDICO');
  await click('Editar');await set('#user-name','Médico Editado Sintético');assert.equal(await evaluate(`!!document.querySelector('#user-username')`),false);await click('Salvar');await until(()=>users[0].name.includes('Editado'));await ready();
  await click('Renovar acesso');await set('#user-validity','unlimited');await click('Salvar');await until(()=>users[0].accessValidUntil===null);await ready();
  await click('Redefinir senha');await set('#user-password','synthetic-reset-password');await click('Salvar');await ready();assert.ok(writes.some(w=>w.path.endsWith('/reset-password')));
  acceptDialog=false;await click('Desativar');await pause(150);assert.equal(users[0].active,true);acceptDialog=true;await click('Desativar');await until(()=>!users[0].active);await ready();await click('Reativar');await until(()=>users[0].active);await ready();
  await click('Novo gestor');await set('#user-name','Gestor Sintético');await set('#user-username','synthetic_gestor');await set('#user-password','synthetic-initial-password');await evaluate(`document.querySelector('[role="dialog"] input[type="checkbox"]').click()`);await click('Criar usuário');await until(()=>users.length===2);await ready();assert.equal(users[1].role,'GESTOR');
  await set('#users-search','Editado');await until(()=>reads.some(q=>q.search==='Editado'));await until(()=>evaluate(`document.querySelectorAll('tbody tr').length===1`));await set('#users-search','');await until(()=>evaluate(`document.querySelectorAll('tbody tr').length===2`));
  await set('#users-role','GESTOR');await until(()=>reads.some(q=>q.role==='GESTOR'));await until(()=>evaluate(`document.querySelector('tbody')?.textContent.includes('synthetic_gestor')&&!document.querySelector('tbody')?.textContent.includes('synthetic_medico')`));await set('#users-role','');await ready();
  const screenshot=await command('Page.captureScreenshot',{format:'png'});await writeFile(join(tmpdir(),'pacs-users-synthetic.png'),Buffer.from(screenshot.data,'base64'));
  fail=true;await click('Atualizar');await until(()=>evaluate(`document.body.textContent.includes('Tentar novamente')`));fail=false;await click('Tentar novamente');await until(()=>evaluate(`document.querySelectorAll('tbody tr').length===2`));
  await click('Importar médicos');await set('#import-unit',units[0].id);await upload(namesOnly,'nomes.xlsx');
  await until(()=>evaluate(`document.body.textContent.includes('sem cabeçalho')&&document.body.textContent.includes('Válidos: 2')&&document.body.textContent.includes('Duplicados: 1')`));
  assert.equal(await evaluate(`document.querySelector('[aria-label="Usuário da linha 1"]').value`),'exemplo_alfa');
  const writesBefore=writes.length;await set('#import-password','synthetic-batch-password');await set('#import-password-confirm','outra-senha-sintetica');await click('Criar 2 médico(s)');
  await until(()=>evaluate(`document.querySelector('[role="dialog"] [role="alert"]')?.textContent.includes('confirmação')`));assert.equal(writes.length,writesBefore);
  await set('#import-password-confirm','synthetic-batch-password');await click('Criar 2 médico(s)');
  await until(()=>evaluate(`document.body.textContent.includes('Usuário já cadastrado; ajuste')`));
  assert.equal(users.length,3);assert.equal(users[2].username,'exemplo_alfa');assert.equal(users[2].role,'MEDICO');
  const batch=writes.slice(writesBefore);assert.equal(batch.length,2);assert.ok(batch.every(w=>w.path==='/api/admin/users/medicos'&&w.body.initialPassword==='synthetic-batch-password'&&w.body.validity==='3m'&&w.body.unitIds.length===1&&w.body.unitIds[0]===units[0].id));
  assert.equal(await evaluate(`document.querySelector('#import-password').value`),'');
  await set('[aria-label="Usuário da linha 2"]','synthetic_medico2');await set('#import-password','synthetic-batch-password');await set('#import-password-confirm','synthetic-batch-password');await click('Criar 1 médico(s)');
  await until(()=>users.length===4);assert.equal(users[3].username,'synthetic_medico2');await until(()=>evaluate(`document.body.textContent.includes('Criados: 2')`));
  await click('Fechar');await ready();await until(()=>evaluate(`document.querySelector('tbody')?.textContent.includes('synthetic_medico2')`));
  role='GESTOR';await open();assert.equal(await evaluate(`Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()==='Novo gestor')`),false);assert.equal(await evaluate(`Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()==='Importar médicos')`),true);assert.equal(await evaluate(`document.querySelector('tbody')?.textContent.includes('synthetic_gestor')`),false);
  expire=true;await click('Atualizar');await until(()=>evaluate(`document.body.textContent.includes('Sua sessão expirou')`));expire=false;
  role='MEDICO';mustChange=true;const before=reads.filter(r=>r==='studies').length;await command('Page.navigate',{url:origin+'/viewer/00000001-00000001-00000001-00000001-00000001'});await until(()=>evaluate(`document.body.textContent.includes('Troca obrigatória de senha')`));assert.equal(reads.filter(r=>r==='studies').length,before);assert.equal(await evaluate(`!!document.querySelector('nav')`),false);
  await set('#password-current','synthetic-initial-password');await set('#password-new','synthetic-personal-password');await set('#password-confirm','different-synthetic-password');await click('Alterar senha');await until(()=>evaluate(`!!document.querySelector('[role="alert"]')`));assert.equal(writes.filter(w=>w.path==='/api/auth/change-password').length,0);
  await set('#password-confirm','synthetic-personal-password');await click('Alterar senha');await until(()=>evaluate(`!document.body.textContent.includes('Troca obrigatória de senha')`));assert.equal(writes.filter(w=>w.path==='/api/auth/change-password').length,1);
  authenticated=true;mustChange=false;await command('Page.navigate',{url:origin});await until(()=>evaluate(`!!document.querySelector('nav')`));assert.equal(await evaluate(`Array.from(document.querySelectorAll('nav button')).some(b=>b.textContent==='Usuários')`),false);
  assert.equal(failures.length,0,failures.join('\n'));
  console.log('PASS: Usuários — importação sem cabeçalho com criação em lote, criação MEDICO/GESTOR, edição, unidades, validade, reset, ativação, filtros, retry, perfis, expiração e troca obrigatória antes do Viewer.');
} finally {
 socket?.close();chrome.kill();await once(chrome,'exit').catch(()=>{});server.closeAllConnections();await new Promise(resolve=>server.close(resolve));await rm(profile,{recursive:true,force:true,maxRetries:10,retryDelay:150});
}
