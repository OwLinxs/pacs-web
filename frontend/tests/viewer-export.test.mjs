import test from 'node:test';import assert from 'node:assert/strict';import{readFile}from'node:fs/promises';import ts from'typescript';
async function load(file){const {outputText}=ts.transpileModule(await readFile(new URL(file,import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}});return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)}
const {patientName,exportOverlay,overlayDate,overlayTime,exportFilename,mayExportWithoutIdentification}=await load('../src/viewer/exportModel.ts');const {api}=await load('../src/api/client.ts');
test('export metadata PN, missing tags, dates and neutral filenames',()=>{
 assert.equal(patientName('TESTE^PACIENTE^^DR^JR=ALTERNATIVO'),'TESTE PACIENTE DR JR');assert.equal(patientName('=TESTE^IDEOGRAFICO'),'TESTE IDEOGRAFICO');
 const m={patientName:'PACIENTE^TESTE',patientId:'TESTE001',patientBirthDate:'20000101',patientSex:'O',patientAge:'026Y',studyDate:'20260929',acquisitionTime:'120047.123',institution:'INSTITUICAO TESTE',station:'ESTACAO TESTE',seriesNumber:'2',laterality:'',viewPosition:'PA',kvp:'93',exposure:'4',filter:'',calibration:'Detector',index:0,total:3,burnedIn:'NO'};
 const view={windowWidth:4096,windowCenter:2048.4,zoom:0.234,mmPerPixel:0.5};
 assert.deepEqual(exportOverlay(m,view,700,800),{
  topLeft:['TESTE001','PACIENTE TESTE','026Y/O/01-jan-2000'],
  topRight:['29-set-2026','Acq: 12:00:47','Se: 2','Im: 1/3','Lat/Pos: PA'],
  bottomLeft:['KVP: 93 kV','Exp: 4 mAs','Filter: —','Zoom: 0,23x','W:4096 L:2048'],
  bottomRight:['ESTACAO TESTE','INSTITUICAO TESTE','DFOV: 35,0 x 40,0 cm','Calibration: Detector'],
 });
 // Ausências: linhas sem rótulo somem; rótulos mostram travessão; nada é inventado.
 const empty={...m,patientName:'',patientId:'',patientBirthDate:'',patientSex:'',patientAge:'',studyDate:'20260230',acquisitionTime:'',institution:'',station:'',seriesNumber:'',viewPosition:'',kvp:'',exposure:'',calibration:''};
 assert.deepEqual(exportOverlay(empty,{windowWidth:null,windowCenter:null,zoom:null,mmPerPixel:null},10,10),{
  topLeft:[],topRight:['Im: 1/3','Lat/Pos: —'],bottomLeft:['KVP: —','Exp: —','Filter: —','Zoom: —','W:— L:—'],bottomRight:[]});
 assert.equal(exportOverlay({...empty,patientSex:'F'},view,1,1).topLeft[0],'—/F/—');
 assert.equal(overlayDate('19650211'),'11-fev-1965');assert.equal(overlayTime('2560'),'');assert.equal(overlayTime('0930'),'09:30');
 for(const f of ['PNG','JPEG','PDF'])assert.match(exportFilename(f,new Date(2026,8,29,14,25,30)),/^pacs-imagem-20260929-142530\.(png|jpg|pdf)$/);
 assert.equal(mayExportWithoutIdentification(m),true);for(const burnedIn of ['YES','', 'UNKNOWN'])assert.equal(mayExportWithoutIdentification({...m,burnedIn}),false);
});
test('export audit sends only format and identification, never actor or image metadata',async t=>{
 globalThis.document={cookie:'pacs_csrf=synthetic'};t.after(()=>delete globalThis.document);
 t.mock.method(globalThis,'fetch',async(path,options)=>{assert.equal(path,'/api/viewer/exports');assert.equal(options.method,'POST');assert.equal(options.credentials,'same-origin');assert.deepEqual(JSON.parse(options.body),{format:'PNG',identified:false});return new Response(null,{status:204})});
 await api.recordViewerExport('PNG',false,new AbortController().signal);
});
