import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import ts from 'typescript';
const source=await readFile(new URL('../src/api/client.ts',import.meta.url),'utf8');
const {outputText}=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}});
const {api,ApiError}=await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
const id='00000001-0000-4000-8000-000000000001';
test('Users explicit role endpoints, immutable fields and same-origin CSRF',async t=>{
 globalThis.document={cookie:'pacs_csrf=synthetic'};t.after(()=>delete globalThis.document);
 const calls=[],signal=new AbortController().signal;
 t.mock.method(globalThis,'fetch',async(path,options)=>{assert.equal(options.credentials,'same-origin');assert.equal(options.signal,signal);if(options.method!=='GET')assert.equal(options.headers['X-CSRF-Token'],'synthetic');calls.push([path,options]);return new Response(null,{status:204});});
 const input={name:'Pessoa Sintética',username:'synthetic',email:'',unitIds:[id],validity:'3m',initialPassword:'synthetic-only-password',role:'ADMIN'};
 await api.createUser('MEDICO',input,signal);await api.createUser('GESTOR',input,signal);
 assert.equal(calls[0][0],'/api/admin/users/medicos');assert.equal(calls[1][0],'/api/admin/users/gestores');assert.ok(!('role' in JSON.parse(calls[0][1].body)));
 await api.editUser(id,{...input},signal);assert.deepEqual(Object.keys(JSON.parse(calls[2][1].body)),['name','email','unitIds']);
 await api.setUserActive(id,false,signal);await api.renewUser(id,'1y',signal);await api.resetUserPassword(id,'synthetic-only-reset',signal);await api.changePassword('synthetic-current','synthetic-next-password',signal);
 assert.equal(calls.at(-1)[0],'/api/auth/change-password');assert.deepEqual(JSON.parse(calls[4][1].body),{validity:'1y'});
 await assert.rejects(api.editUser('../bad',input,signal));assert.equal(calls.length,7);
});
test('Users search/filter/pagination cancellation and sanitized errors',async t=>{
 const signal=new AbortController().signal;let status=200;
 t.mock.method(globalThis,'fetch',async(path,options)=>{assert.equal(options.signal,signal);const u=new URL(path,'http://fixture.invalid');assert.equal(u.searchParams.get('search'),'Sintético');assert.equal(u.searchParams.get('offset'),'25');return Response.json(status===200?{items:[]}:{error:{code:status===401?'UNAUTHENTICATED':'PASSWORD_CHANGE_REQUIRED',message:'Mensagem sanitizada.'}},{status});});
 const q={search:'Sintético',role:'MEDICO',status:'expired',unitId:id,offset:25};await api.getUsers(q,signal);
 for(status of [401,403])await assert.rejects(api.getUsers(q,signal),e=>e instanceof ApiError&&e.status===status);
});
