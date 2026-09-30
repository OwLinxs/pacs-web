import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import ts from 'typescript';
async function load(path){const source=await readFile(new URL(path,import.meta.url),'utf8');const {outputText}=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}});return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);}
const {api,ApiError}=await load('../src/api/client.ts');
const {eventLabels,eventLabel,auditDate,resultLabel,detailLabel}=await load('../src/audit/model.ts');
test('Audit authenticated read-only API: filters, pagination, abort and allowlist',async t=>{
 const signal=new AbortController().signal;
 t.mock.method(globalThis,'fetch',async(path,options)=>{
  const url=new URL(path,'http://fixture.invalid');assert.equal(url.pathname,'/api/admin/audit');assert.equal(url.searchParams.get('limit'),'50');assert.equal(url.searchParams.get('offset'),'50');assert.equal(url.searchParams.get('event'),'USER_CREATED');assert.equal(url.searchParams.get('category'),'users');assert.equal(url.searchParams.has('secret'),false);assert.equal(options.method,'GET');assert.equal(options.credentials,'same-origin');assert.equal(options.signal,signal);assert.equal(options.body,undefined);
  return Response.json({items:[],limit:50,offset:50,hasMore:false,nextOffset:null});
 });
 await api.getAudit({dateFrom:'2026-09-01',dateTo:'2026-09-29',event:'USER_CREATED',category:'users',actorId:'00000001-0000-4000-8000-000000000001',targetUserId:'00000002-0000-4000-8000-000000000001',offset:50,secret:'synthetic'},signal);
});
test('Audit labels, timezone, safe details and no arbitrary JSON rendering',()=>{
 assert.equal(Object.keys(eventLabels).length,17);assert.equal(eventLabel('USER_CREATED'),'Usuário criado');assert.equal(eventLabel('unknown'),'Evento histórico não reconhecido');
 assert.match(auditDate('2026-09-29T02:30:00Z'),/28\/09\/2026.*23:30:00/);assert.equal(auditDate('invalid'),'—');assert.equal(resultLabel('failure'),'Falha');
 assert.equal(detailLabel('PACS/Orthanc: connected'),'Conexão estabelecida.');assert.equal(detailLabel('{"password":"synthetic"}'),'Sem contexto adicional disponível.');assert.equal(detailLabel('target_user_id=arbitrary'),'Sem contexto adicional disponível.');
});
test('Audit permission/session/errors remain distinguishable',async t=>{
 let status=403;t.mock.method(globalThis,'fetch',async()=>Response.json({error:{code:status===403?'FORBIDDEN':'UNAUTHENTICATED',message:'Erro sintético.'}},{status}));
 await assert.rejects(api.getAudit({},new AbortController().signal),e=>e instanceof ApiError&&e.status===403);status=401;await assert.rejects(api.getAudit({},new AbortController().signal),e=>e instanceof ApiError&&e.status===401);
});
