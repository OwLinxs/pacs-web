import assert from 'node:assert/strict';
import {writeFile} from 'node:fs/promises';import{join}from'node:path';import{tmpdir}from'node:os';import{PDFDocument}from'pdf-lib';
export async function testViewerExport({evaluate,until,pause,command,audits,setAuditStatus}) {
 const click=async label=>{await until(()=>evaluate(`Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()===${JSON.stringify(label)}&&!b.disabled)`));await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(label)}).click()`)};
 await click('1x2');await until(()=>evaluate(`document.querySelectorAll('[data-viewport]').length===2 && !!document.querySelectorAll('[data-viewport]')[1].querySelector('canvas')?.width`));await evaluate(`document.querySelectorAll('[data-viewport]')[1].click()`);
 await evaluate(`(()=>{window.exportBlobs=[];window.exportTexts=[];window.exportRevoked=0;window.originalCreate=URL.createObjectURL;window.originalRevoke=URL.revokeObjectURL;window.originalClick=HTMLAnchorElement.prototype.click;window.originalText=CanvasRenderingContext2D.prototype.fillText;URL.createObjectURL=function(blob){const url=window.originalCreate.call(URL,blob);window.exportBlobs.push({blob,url});return url;};URL.revokeObjectURL=function(url){window.exportRevoked++;return window.originalRevoke.call(URL,url)};HTMLAnchorElement.prototype.click=function(){window.exportBlobs.at(-1).name=this.download;};CanvasRenderingContext2D.prototype.fillText=function(text,...args){window.exportTexts.push(text);return window.originalText.call(this,text,...args)};})()`);
 const state=()=>evaluate(`(()=>{const panes=Array.from(document.querySelectorAll('[data-viewport]'));return panes.map(p=>{const c=p.querySelector('canvas');return {active:p.dataset.active,canvas:c?.toDataURL(),counter:p.querySelector('[role=status]')?.textContent}})})()`) ;
 for(const format of ['PNG','JPEG','PDF'])for(const identified of [true,false]) {
  const before=await state();await click('Exportar');await until(()=>evaluate(`!!document.querySelector('dialog[open]')`));
  await evaluate(`(()=>{const s=document.querySelector('#export-format');Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value').set.call(s,'${format}');s.dispatchEvent(new Event('change',{bubbles:true}));})()`);
  await evaluate(`document.querySelectorAll('dialog input[type=radio]')[${identified?0:1}].click()`);
  if(!identified)await evaluate(`document.querySelector('dialog input[type=checkbox]').click()`)
  await evaluate(`window.exportTexts=[]`);const n=await evaluate(`window.exportBlobs.length`);await click('Gerar arquivo');await until(()=>evaluate(`window.exportBlobs.length>${n}`));await until(()=>evaluate(`document.querySelector('dialog').textContent.includes('Arquivo gerado')`));
  const texts=await evaluate(`window.exportTexts`);if(identified){for(const value of ['FICTICIO VIEWER','SYNTH-VIEWER','29/09/2026','OT','PADRAO SINTETICO','INSTITUICAO TESTE'])assert.ok(texts.some(t=>t.includes(value)),`missing synthetic field ${value}`)}else assert.deepEqual(texts,[]);
  const file=await evaluate(`(async()=>{const item=window.exportBlobs.at(-1);const data=new Uint8Array(await item.blob.arrayBuffer());let s='';for(const b of data)s+=String.fromCharCode(b);return {name:item.name,type:item.blob.type,base64:btoa(s)}})()`);
  assert.match(file.name,/^pacs-imagem-\d{8}-\d{6}\.(png|jpg|pdf)$/);
  const bytes=Buffer.from(file.base64,'base64');
  if(format==='PDF'){const doc=await PDFDocument.load(bytes);assert.equal(doc.getPageCount(),1);assert.equal(doc.getTitle(),undefined);assert.equal(doc.getAuthor(),undefined)}
  else {const dims=await evaluate(`(async()=>{const b=window.exportBlobs.at(-1).blob;const img=await createImageBitmap(b);const c=document.querySelector('[data-active=true] canvas');const result={width:img.width,height:img.height,sourceWidth:c.width,sourceHeight:c.height};img.close();return result})()`);if(!identified){assert.equal(dims.width,dims.sourceWidth);assert.equal(dims.height,dims.sourceHeight)}else assert.ok(dims.height>dims.sourceHeight)}
  if(format==='PNG'&&!identified){const pixelMatch=await evaluate(`(async()=>{const img=await createImageBitmap(window.exportBlobs.at(-1).blob),c=document.createElement('canvas');c.width=img.width;c.height=img.height;c.getContext('2d').drawImage(img,0,0);img.close();return c.toDataURL()===document.querySelector('[data-active=true] canvas').toDataURL()})()`);assert.equal(pixelMatch,true,'PNG must exactly preserve rendered viewport pixels')}
  for(const forbidden of ['SYNTH-ACC','2.25.111111111','2.25.222222222'])assert.equal(bytes.includes(Buffer.from(forbidden)),false);
  await writeFile(join(tmpdir(),`pacs-export-synthetic-${identified?'identified':'plain'}.${format==='JPEG'?'jpg':format.toLowerCase()}`),bytes);
  await click('Fechar');assert.deepEqual(await state(),before,'export mutated viewport pixels/stack/layout');
 }
 await pause(1200);assert.equal(await evaluate(`window.exportRevoked`),6);assert.equal(audits.length,6);for(const a of audits)assert.deepEqual(Object.keys(a).sort(),['format','identified']);
 setAuditStatus(503);await click('Exportar');await evaluate(`document.querySelectorAll('dialog input[type=radio]')[0].click()`);await click('Gerar arquivo');await until(()=>evaluate(`document.querySelector('dialog').textContent.includes('auditoria não pôde ser confirmada')`));await click('Fechar');setAuditStatus(204);
 await pause(1200);assert.equal(await evaluate(`window.exportRevoked`),7);assert.equal(audits.length,7);

 await click('1x1');
 await evaluate(`URL.createObjectURL=window.originalCreate;URL.revokeObjectURL=window.originalRevoke;HTMLAnchorElement.prototype.click=window.originalClick;CanvasRenderingContext2D.prototype.fillText=window.originalText;delete window.exportBlobs;delete window.exportTexts;`);
 console.log('PASS: V5 PNG/JPEG/PDF identified/plain, synthetic metadata, active canvas pixel fidelity, viewport state and URL cleanup.');
}
