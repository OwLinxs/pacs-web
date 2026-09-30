import test from 'node:test';import assert from 'node:assert/strict';import{readFile}from'node:fs/promises';import ts from'typescript';
async function load(file){const {outputText}=ts.transpileModule(await readFile(new URL(file,import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}});return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)}
const {patientName,exportFields,exportFilename,mayExportWithoutIdentification}=await load('../src/viewer/exportModel.ts');const {api}=await load('../src/api/client.ts');
test('export metadata PN, missing tags, dates and neutral filenames',()=>{
 assert.equal(patientName('TESTE^PACIENTE^^DR^JR=ALTERNATIVO'),'TESTE PACIENTE DR JR');assert.equal(patientName('=TESTE^IDEOGRAFICO'),'TESTE IDEOGRAFICO');
 const m={patientName:'PACIENTE^TESTE',patientId:'TESTE001',studyDate:'20260929',modality:'OT',studyDescription:'PADRAO SINTETICO',institution:'INSTITUICAO TESTE',seriesNumber:'2',instanceNumber:'5',index:0,total:3,burnedIn:'NO'};
 assert.deepEqual(exportFields(m).map(p=>p[1]),['PACIENTE TESTE','TESTE001','29/09/2026','OT','PADRAO SINTETICO','INSTITUICAO TESTE','2','1 / 3']);
 assert.equal(exportFields({...m,studyDate:'20260230',patientName:''})[2][1],'—');assert.equal(exportFields({...m,patientName:''})[0][1],'—');
 for(const f of ['PNG','JPEG','PDF'])assert.match(exportFilename(f,new Date(2026,8,29,14,25,30)),/^pacs-imagem-20260929-142530\.(png|jpg|pdf)$/);
 assert.equal(mayExportWithoutIdentification(m),true);for(const burnedIn of ['YES','', 'UNKNOWN'])assert.equal(mayExportWithoutIdentification({...m,burnedIn}),false);
});
test('export audit sends only format and identification, never actor or image metadata',async t=>{
 globalThis.document={cookie:'pacs_csrf=synthetic'};t.after(()=>delete globalThis.document);
 t.mock.method(globalThis,'fetch',async(path,options)=>{assert.equal(path,'/api/viewer/exports');assert.equal(options.method,'POST');assert.equal(options.credentials,'same-origin');assert.deepEqual(JSON.parse(options.body),{format:'PNG',identified:false});return new Response(null,{status:204})});
 await api.recordViewerExport('PNG',false,new AbortController().signal);
});
