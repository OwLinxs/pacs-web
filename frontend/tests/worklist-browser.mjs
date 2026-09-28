// Chrome isolado + API sintética; não utiliza backend, .env ou Orthanc.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, extname } from 'node:path';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
const id = n => `${String(n).padStart(8,'0')}-00000000-00000000-00000000-00000000`;
const requests = [];
let mode = 'ok', delay = 0, slowTerm = '', disconnected = 0;
const item = { orthancStudyId:id(1),studyInstanceUid:'',studyDate:'20260928',studyTime:'143259.123',patientName:'FICTICIO^WORKLIST',patientId:'',accessionNumber:'',studyDescription:'',institutionName:'',modalities:['SR','CT','CT'],seriesCount:2 };
const server = createServer(async(req,res) => {
 const url=new URL(req.url,'http://fixture.invalid');
 res.setHeader('Cache-Control','no-store');
 const json=(body,status=200)=>{res.statusCode=status;res.setHeader('Content-Type','application/json');res.end(JSON.stringify(body));};
 if(url.pathname==='/api/auth/me')return json({user:{id:'synthetic-user',name:'Usuário fictício',username:'synthetic',role:'MEDICO',unit:null,active:true,accessValidUntil:null,lastLoginAt:null}});
 if(url.pathname==='/api/studies'){
  const params=Object.fromEntries(url.searchParams);requests.push(params);
  const currentMode=mode;
  const slow=slowTerm && params.patientName===slowTerm;
  res.on('close',()=>{if(!res.writableEnded)disconnected++;});
  await new Promise(resolve=>setTimeout(resolve,slow?1000:delay));
  if(currentMode==='expired')return json({error:{code:'UNAUTHENTICATED',message:'Sessão expirada.'}},401);
  if(currentMode==='error')return json({error:{code:'PACS_UNAVAILABLE',message:'Não foi possível consultar os exames no PACS.'}},502);
  const offset=Number(params.offset||0),limit=Number(params.limit||25);
  return json({items:currentMode==='empty'?[]:[{...item,patientName:slow?'FICTICIO^OBSOLETO':item.patientName}],limit,offset,hasMore:currentMode!=='empty'&&offset===0,nextOffset:currentMode!=='empty'&&offset===0?limit:null});
 }
 if(url.pathname===`/api/studies/${id(1)}/series`)return json({items:[]});
 if(url.pathname.startsWith('/api/'))return json({error:{code:'NOT_FOUND',message:'Fixture ausente.'}},404);
 try {
  const path=url.pathname;
  const asset=path.startsWith('/assets/')&&/^\/assets\/[\w.-]+$/.test(path)?path.slice(1):'index.html';
  res.setHeader('Content-Type',({'.js':'text/javascript','.css':'text/css','.wasm':'application/wasm','.html':'text/html'})[extname(asset)]??'application/octet-stream');
  res.end(await readFile(new URL(`../dist/${asset}`,import.meta.url)));
 }catch{res.statusCode=404;res.end();}
});
server.listen(0, '127.0.0.1'); await once(server, 'listening');
const origin = `http://127.0.0.1:${server.address().port}`;
const profile = await mkdtemp(join(tmpdir(), 'pacs-worklist-smoke-'));
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
  delay=250;
  await command('Page.navigate',{url:origin});
  await until(()=>evaluate('document.body.textContent.includes("Carregando…")'));
  const ready=()=>until(()=>evaluate(`document.querySelector('[aria-label="Abrir estudo"]')?.getAttribute("aria-disabled")==="false"`));
  await ready();delay=0;
  const click=async label=>{await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(label)}).click()`);};
  const value=label=>evaluate(`document.querySelector('[aria-label="${label}"]').value`);
  const set=async(label,value)=>{await evaluate(`(()=>{const el=document.querySelector('[aria-label="${label}"]');Object.getOwnPropertyDescriptor(el.tagName==='SELECT'?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value').set.call(el,${JSON.stringify(value)});el.dispatchEvent(new Event(el.tagName==='SELECT'?'change':'input',{bubbles:true}));})()`);};
  const queried=async(fn)=>{await until(()=>fn(requests.at(-1)));await ready();};
  const body=()=>evaluate('document.body.textContent');
  assert.equal(requests[0].sort,'dateDesc');assert.equal(requests[0].limit,'25');assert.equal(requests[0].offset,'0');
  assert.ok((await body()).includes('28/09/2026'));assert.ok((await body()).includes('14:32'));assert.ok((await body()).includes('CT / SR'));
  assert.equal(await evaluate(`Array.from(document.querySelector('[aria-label="Abrir estudo"]').children).filter(e=>e.textContent==="—").length`),4,"tags ausentes neutras");
  // Mudança do seletor não gera busca mágica: filtros independentes e AND visível.
  await set('Pesquisar exames','FICTICIO*');await queried(q=>q.patientName==='FICTICIO*');
  await set('Tipo de busca','patientId');assert.equal(await value('Pesquisar exames'),'');
  await set('Pesquisar exames','SYNTH-ID');await queried(q=>q.patientId==='SYNTH-ID');assert.equal(requests.at(-1).patientName,'FICTICIO*');
  await click('Filtros avançados');
  await set('Filtro Accession Number','SYNTH-ACC');await set('Filtro Descrição do estudo','FICTICIO*');await set('Filtro Instituição','FICTICIA');
  await queried(q=>q.institutionName==='FICTICIA'&&q.accessionNumber==='SYNTH-ACC'&&q.studyDescription==='FICTICIO*');
  const local=async ago=>evaluate(`(()=>{const d=new Date();d.setDate(d.getDate()-${ago});return d.getFullYear()+'-'+String(d.getMonth()+1).padStart(2,'0')+'-'+String(d.getDate()).padStart(2,'0')})()`);
  for(const [label,ago,end]of [['Hoje',0,0],['Ontem',1,1],['Últimos 7 dias',6,0],['Últimos 30 dias',29,0]]){
   await click(label);const from=await local(ago),to=await local(end);await queried(q=>q.dateFrom===from&&q.dateTo===to);
  }
  await click('Personalizado');await set('Data inicial','2099-01-01');
  await until(()=>evaluate('document.body.textContent.includes("A data inicial deve ser anterior")'));
  const beforeInvalid=requests.length;await pause(500);assert.equal(requests.length,beforeInvalid);
  await set('Data inicial','2026-09-01');await set('Data final','2026-09-28');await queried(q=>q.dateFrom==='2026-09-01'&&q.dateTo==='2026-09-28');
  await set('Modalidade','NEW');await queried(q=>q.modality==='NEW');
  await set('Ordenação','dateAsc');await queried(q=>q.sort==='dateAsc');
  await set('Estudos por página','50');await queried(q=>q.limit==='50'&&q.offset==='0');
  await click('Próxima');await queried(q=>q.offset==='50');assert.ok((await body()).includes('Página 2'));assert.ok((await body()).includes('Fim dos resultados.'));
  await click('Anterior');await queried(q=>q.offset==='0');
  await click('Próxima');await queried(q=>q.offset==='50');
  // Navegação por teclado sem PHI no endereço e retorno preserva estado/offset.
  await evaluate(`document.querySelector('[aria-label="Abrir estudo"]').focus()`);
  await command('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});
  await command('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});
  await until(()=>evaluate('location.pathname.startsWith("/viewer/")'));
  assert.equal(await evaluate('location.search'),'');
  await evaluate('history.back()');await ready();
  assert.equal(await value('Pesquisar exames'),'SYNTH-ID');assert.equal(await value('Modalidade'),'NEW');assert.equal(await value('Ordenação'),'dateAsc');assert.equal(requests.at(-1).offset,'50');assert.equal(requests.at(-1).limit,'50');
  // Atualizar mantém tabela enquanto consulta e volta à primeira página.
  delay=600;await click('Atualizar');
  await until(()=>evaluate('document.body.textContent.includes("Atualizando…")'));
  assert.equal(await evaluate(`document.querySelectorAll('[aria-label="Abrir estudo"]').length`),1);
  await queried(q=>q.offset==='0');delay=0;
  await click('Limpar filtros');await queried(q=>q.sort==='dateDesc'&&q.limit==='25'&&!q.modality&&!q.patientId&&!q.patientName);
  // Debounce, cancelamento e resultado antigo não sobrescreve novo.
  slowTerm='SLOW-SYNTH';await set('Pesquisar exames',slowTerm);await until(()=>requests.at(-1)?.patientName===slowTerm);
  await set('Pesquisar exames','NEW-SYNTH');await queried(q=>q.patientName==='NEW-SYNTH');await pause(1100);
  assert.ok(!(await body()).includes('OBSOLETO'));assert.ok(disconnected>0,'request anterior cancelado');
  const beforeRapid=requests.length;await set('Pesquisar exames','A');await set('Pesquisar exames','AB');await set('Pesquisar exames','ABC');await queried(q=>q.patientName==='ABC');assert.equal(requests.length,beforeRapid+1);
  mode='empty';await click('Atualizar');await until(()=>evaluate('document.body.textContent.includes("Nenhum estudo encontrado para os filtros selecionados.")'));
  mode='error';await click('Atualizar');await until(()=>evaluate('document.body.textContent.includes("Tentar novamente")'));
  assert.ok(!(await body()).includes('orthanc'));mode='ok';await click('Tentar novamente');await ready();
  // Unmount: cancela request sem receber atualização tardia.
  slowTerm='UNMOUNT-SYNTH';await set('Pesquisar exames',slowTerm);await until(()=>requests.at(-1)?.patientName===slowTerm);
  const beforeUnmount=disconnected;
  await evaluate(`history.pushState(null,'','/viewer/${id(1)}');window.dispatchEvent(new PopStateEvent('popstate'))`);
  await until(()=>disconnected>beforeUnmount);await pause(1100);assert.ok(await evaluate('location.pathname.startsWith("/viewer/")'));
  await evaluate(`history.pushState(null,'','/');window.dispatchEvent(new PopStateEvent('popstate'))`);await ready();
  const shot=await command('Page.captureScreenshot',{format:'png'});await writeFile(join(tmpdir(),'pacs-worklist-synthetic.png'),Buffer.from(shot.data,'base64'));
  mode='expired';await click('Atualizar');await until(()=>evaluate('document.body.textContent.toLowerCase().includes("sessão expirou")'));
  const stopped=requests.length;await pause(800);assert.equal(requests.length,stopped,'sem polling após expiração');
  assert.equal(failures.length,0,failures.join('\n'));assert.ok(network.filter(url=>url.startsWith('http')).every(url=>url.startsWith(origin)||url.includes('fonts.')));

  console.log('PASS: Worklist V2 — busca, AND, períodos, validação, modalidade, ordem, paginação, refresh, navegação, tags ausentes, concorrência, unmount e sessão.');
} finally {
 socket?.close();chrome.kill();await once(chrome,'exit').catch(()=>{});server.closeAllConnections();await new Promise(resolve=>server.close(resolve));await rm(profile,{recursive:true,force:true});
}
