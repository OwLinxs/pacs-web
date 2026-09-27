import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { testViewerAnnotations } from './viewer-annotations-browser.mjs';

export async function testViewerV3({ evaluate, command, key, choose, pause, until, calls, id }) {
  const button = async (text) => evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(text)}).click()`);
  const text = (index) => evaluate(`document.querySelector('[data-viewport="image-${index}"]')?.textContent || ''`);
  const wait = (index, value) => until(async () => (await text(index)).includes(value));
  const activate = async (index) => {
    const p = await evaluate(`(()=>{const r=document.querySelector('[data-viewport="image-${index}"]').getBoundingClientRect();return {x:r.x+8,y:r.y+8}})()`);
    await command('Input.dispatchMouseEvent', {type:'mousePressed', ...p, button:'left',buttons:1,clickCount:1});
    await command('Input.dispatchMouseEvent', {type:'mouseReleased', ...p, button:'left',buttons:0,clickCount:1});
    await until(()=>evaluate(`document.querySelector('[data-viewport="image-${index}"]').dataset.active==='true'`));
  };
  const hash = (index) => evaluate(`(()=>{const c=document.querySelector('[data-viewport="image-${index}"] canvas'); const data=c.getContext('2d').getImageData(0,0,c.width,c.height).data;let h=0;for(let i=0;i<data.length;i+=256)h=(h*31+data[i])|0;return h})()`);
  await button('1x2');
  assert.equal(await evaluate("document.querySelectorAll('[data-viewport]').length"),2);
  await wait(0,'Imagem 1 / 3');
  await activate(1); await choose(1); await wait(1,'Imagem 1 / 2');
  await key('ArrowRight'); await wait(1,'Imagem 2 / 2'); await wait(0,'Imagem 1 / 3');
  await key('ArrowDown'); await pause(100); await wait(1,'Imagem 2 / 2');
  await key('ArrowLeft'); await wait(1,'Imagem 1 / 2');
  const inactivePoint = await evaluate("(()=>{const r=document.querySelector('[data-viewport=\"image-0\"]').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
  await command('Input.dispatchMouseEvent',{type:'mouseWheel',...inactivePoint,deltaX:0,deltaY:100});
  await pause(120); await wait(0,'Imagem 1 / 3'); await wait(1,'Imagem 1 / 2');
  const before0 = await hash(0), before1 = await hash(1);
  await button('Invert'); await pause(150);
  assert.equal(await hash(0),before0); assert.notEqual(await hash(1),before1);
  await button('Reset'); await pause(150); assert.equal(await hash(1),before1);
  await choose(3); await wait(1,'Não foi possível carregar'); await wait(0,'Imagem 1 / 3');
  await choose(1); await wait(1,'Imagem 1 / 2');
  await button('2x2');
  assert.equal(await evaluate("document.querySelectorAll('[data-viewport]').length"),4);
  await activate(2); await choose(0); await wait(2,'Imagem 1 / 3');
  await key('ArrowRight'); await wait(2,'Imagem 2 / 3'); await wait(0,'Imagem 1 / 3');
  await activate(3); await choose(1); await wait(3,'Imagem 1 / 2');
  const screenshot = await command('Page.captureScreenshot', { format: 'png' });
  await writeFile(join(tmpdir(), 'pacs-viewer-v3-layouts-synthetic.png'), Buffer.from(screenshot.data, 'base64'));
  await button('Play'); await wait(3,'Imagem 2 / 2'); await wait(3,'Imagem 1 / 2');
  await button('Pause'); await pause(200); const stopped=await text(3); await pause(350); assert.equal(await text(3),stopped);
  await button('Play'); await choose(0); await wait(3,'Imagem 1 / 3');
  assert.equal((await text(3)).includes('Cine'),false);
  await button('Play'); await button('1x1'); await pause(500);
  assert.equal(await evaluate("document.querySelectorAll('[data-viewport]').length"),1);
  await wait(0,'Imagem 1 / 3');
  assert.equal((await text(0)).includes('Cine'),false);
  for (let i=0;i<3;i++) { await button('2x2'); await button('1x1'); }
  await pause(250);
  assert.equal(await evaluate("document.querySelectorAll('main .svg-layer').length"),1);
  await testViewerAnnotations({ evaluate, command, key, choose, pause, until });
  // Thumbnails não requisitam outras instâncias da série B além das usadas explicitamente acima.
  assert.ok(await evaluate("document.querySelector('[data-thumbnail=ready]')!==null"));
  assert.ok(await evaluate("document.querySelector('[data-thumbnail=placeholder], [data-thumbnail=error]')!==null"));
  // Card fora da área visível não é carregado até rolar o painel.
  const before = calls.filter(path=>path.includes(`/${id(7)}/`) && path.endsWith('/dicom')).length;
  await evaluate("document.querySelector('aside').scrollTop=10000");
  await until(()=>evaluate("Array.from(document.querySelectorAll('aside [data-thumbnail]')).at(-1).dataset.thumbnail==='ready'"));
  assert.ok(calls.filter(path=>path.includes(`/${id(7)}/`) && path.endsWith('/dicom')).length >= before);
  await evaluate("document.querySelector('aside').scrollTop=0");
  console.log('PASS: V3 layouts, stacks independentes, viewport ativo, Invert/Reset isolados, Cine/loop/cleanup, thumbnails e falhas localizadas.');
}
