import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';
const source = await readFile(new URL('../src/api/client.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } });
const { api, ApiError } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
const id='00000001-0000-4000-8000-000000000001';

test('Unidades GET lista ativa/admin com paginação e cancelamento',async t=>{
 const signal=new AbortController().signal;
 const calls=[];
 t.mock.method(globalThis,'fetch',async(path,options)=>{calls.push(path);assert.equal(options.method,'GET');assert.equal(options.credentials,'same-origin');assert.equal(options.signal,signal);return Response.json({items:[],hasMore:false,nextOffset:null,limit:50,offset:0});});
 await api.getUnits(false,0,signal);await api.getUnits(true,50,signal);
 assert.deepEqual(calls,['/api/units?includeInactive=false&limit=50&offset=0','/api/units?includeInactive=true&limit=50&offset=50']);
});
test('Unidades POST/PATCH usam sessão/CSRF e somente campos permitidos',async t=>{
 globalThis.document={cookie:'pacs_csrf=synthetic-csrf'};
 t.after(()=>{delete globalThis.document;});
 const calls=[];
 t.mock.method(globalThis,'fetch',async(path,options)=>{assert.equal(options.headers['X-CSRF-Token'],'synthetic-csrf');assert.equal(options.credentials,'same-origin');calls.push([path,options.method,JSON.parse(options.body)]);return Response.json({id,name:'Unidade Sintética',active:true});});
 const signal=new AbortController().signal;
 await api.createUnit('Unidade Sintética',signal);await api.updateUnit(id,{name:'Editada Sintética'},signal);await api.updateUnit(id,{active:false},signal);await api.updateUnit(id,{active:true},signal);
 assert.deepEqual(calls.map(c=>c[1]),['POST','PATCH','PATCH','PATCH']);assert.deepEqual(calls[2],[`/api/admin/units/${id}`,'PATCH',{active:false}]);
 await assert.rejects(api.updateUnit('../arbitrary',{active:false},signal));assert.equal(calls.length,4);
});
test('Unidades conflito e sessão expirada são identificáveis, sem retry de escrita arbitrário',async t=>{
 globalThis.document={cookie:'pacs_csrf=synthetic-csrf'};t.after(()=>{delete globalThis.document;});
 let status=409;
 const fetch=t.mock.method(globalThis,'fetch',async()=>Response.json({error:{code:status===409?'CONFLICT':'UNAUTHENTICATED',message:'Erro sintético sanitizado.'}},{status}));
 await assert.rejects(api.createUnit('Sintética',new AbortController().signal),e=>e instanceof ApiError&&e.status===409);
 status=401;await assert.rejects(api.getUnits(true,0,new AbortController().signal),e=>e instanceof ApiError&&e.status===401);assert.equal(fetch.mock.callCount(),2);
});
