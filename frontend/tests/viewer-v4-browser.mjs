import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

// Build real, APIs públicas/eventos DOM e gestos CDP. Nenhum hook em produção.
export async function testViewerV4({ evaluate, command, key, choose, pause, until, calls }) {
  const button = async (label) => {
    await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.getAttribute('aria-label')===${JSON.stringify(label)} || b.textContent.trim()===${JSON.stringify(label)}).click()`);
    await pause(100);
  };
  const text = index => evaluate(`document.querySelector('[data-viewport="image-${index}"]').textContent`);
  const wait = (index, label) => until(async()=> (await text(index)).includes(label));
  const point = (index,x,y) => evaluate(`(()=>{const r=document.querySelector('[data-viewport="image-${index}"]').getBoundingClientRect();return {x:r.x+r.width*${x},y:r.y+r.height*${y}}})()`);
  const mouse = (type,p,pressed=false) => command('Input.dispatchMouseEvent',{type,...p,button:pressed||type==='mouseReleased'?'left':'none',buttons:pressed?1:0,clickCount:type==='mouseMoved'?0:1});
  const activate = async index => { const p=await point(index,.02,.04);await mouse('mousePressed',p,true);await mouse('mouseReleased',p);await pause(80); };
  const drag = async (index,tool,a,b) => {
    await button(tool);const start=await point(index,...a),end=await point(index,...b);
    await mouse('mouseMoved',start);await mouse('mousePressed',start,true);await pause(210);
    for(let i=1;i<=6;i++){await mouse('mouseMoved',{x:start.x+(end.x-start.x)*i/6,y:start.y+(end.y-start.y)*i/6},true);await pause(20);}
    await mouse('mouseReleased',end);await pause(300);
  };
  const hash = index => evaluate(`(()=>{const c=document.querySelector('[data-viewport="image-${index}"] canvas');const pixels=c.getContext('2d').getImageData(0,0,c.width,c.height).data;let h=0;for(let i=0;i<pixels.length;i+=64)h=(h*31+pixels[i])|0;return h})()`);
  const svg = index => evaluate(`document.querySelector('[data-viewport="image-${index}"] .svg-layer')?.textContent || ''`);
  const pressed = label => evaluate(`document.querySelector('[aria-label="${label}"]').getAttribute('aria-pressed')`);
  const maximize = async index => { await evaluate(`document.querySelector('[data-viewport="image-${index}"] button[aria-label="Maximizar viewport"]').click()`);await pause(250); };
  const camera = index => evaluate(`window.v4Camera[${index}]`);

  await key('ArrowRight');await wait(0,'Imagem 2 / 3');
  await button('1x2');await wait(0,'Imagem 2 / 3');await wait(1,'Imagem 1 / 2');
  await activate(1); assert.equal(await pressed('Invert'), 'false', 'auto-layout inicia Invert OFF'); await activate(0);
  for (let repeat = 0; repeat < 3; repeat++) {
  await button('1x1'); await button('1x2'); await wait(1,'Imagem 1 / 2'); await pause(120);
  const square=await evaluate(`(()=>{const c=document.querySelector('[data-viewport="image-0"] canvas'),d=c.getContext('2d').getImageData(0,0,c.width,c.height).data;let minX=c.width,minY=c.height,maxX=0,maxY=0;for(let y=0;y<c.height;y++)for(let x=0;x<c.width;x++){if(d[4*(y*c.width+x)]>8){minX=Math.min(minX,x);maxX=Math.max(maxX,x);minY=Math.min(minY,y);maxY=Math.max(maxY,y);}}return {width:maxX-minX,height:maxY-minY}})()`);
  assert.ok(Math.abs(square.width-square.height)<4,`resize 1x1→1x2 mantém imagem inicialmente ajustada inteira: ${JSON.stringify(square)}`);
  }
  await button('2x2');await wait(2,'Série sem imagens');await wait(3,'Não foi possível carregar');
  await activate(2);await choose(4);await wait(2,'Imagem 1 / 1');
  await button('1x2');await wait(0,'Imagem 2 / 3');await wait(1,'Imagem 1 / 2');
  await button('2x2');await wait(2,'Imagem 1 / 1');await wait(3,'Não foi possível carregar');
  assert.ok((await text(2)).includes('Série lenta fictícia'));
  await activate(2);await button('Reset');
  await evaluate(`{window.v4Camera={};window.v4VOI={};document.querySelectorAll('[data-viewport]').forEach((pane,index)=>{const e=pane.querySelector('[data-viewport-uid]');e.addEventListener('CORNERSTONE_CAMERA_MODIFIED',ev=>window.v4Camera[index]=ev.detail.camera);e.addEventListener('CORNERSTONE_VOI_MODIFIED',ev=>window.v4VOI[index]=ev.detail.range);});window.v4Canvases=Array.from(document.querySelectorAll('main canvas'));}`);
  await button('Reset');
  const original=await hash(2), untouched=await hash(0);
  assert.notEqual(untouched,0,'viewport preservado precisa manter pixels, não apenas contador');
  await button('Rotate Right');assert.notEqual(await hash(2),original);assert.equal(await hash(0),untouched);
  assert.equal(await pressed('Rotate Right'),null,'ação não é ferramenta ativa');
  assert.equal(Math.round((await camera(2)).rotation)%360,90);
  await button('Rotate Left');assert.equal(await hash(2),original);
  for(let n=0;n<4;n++) await button('Rotate Right');
  assert.equal(await hash(2),original,'quatro rotações completam volta');
  await button('Flip Horizontal');assert.equal(await pressed('Flip Horizontal'),'true');assert.notEqual(await hash(2),original);assert.equal(await hash(0),untouched);
  await button('Flip Horizontal');assert.equal(await pressed('Flip Horizontal'),'false');assert.equal(await hash(2),original);
  await button('Flip Vertical');assert.equal(await pressed('Flip Vertical'),'true');assert.notEqual(await hash(2),original);assert.equal(await hash(0),untouched);
  await button('Flip Vertical');assert.equal(await hash(2),original);

  await drag(2,'Length',[.4,.45],[.6,.45]);await until(async()=>/mm/.test(await svg(2)));
  const measurement=await svg(2);
  await drag(2,'Window/Level',[.45,.7],[.55,.75]);
  const voi=await evaluate('window.v4VOI[2]');assert.ok(voi);
  await button('Invert');await button('Rotate Right');await button('Flip Horizontal');await button('Flip Vertical');
  await drag(2,'Zoom',[.5,.4],[.5,.6]);await drag(2,'Pan',[.5,.5],[.65,.55]);
  await button('Fit');
  assert.deepEqual(await evaluate('window.v4VOI[2]'),voi,'Fit mantém VOI');
  assert.equal(await pressed('Invert'),'true');assert.equal(await pressed('Flip Horizontal'),'true');assert.equal(await pressed('Flip Vertical'),'true');
  assert.equal(Math.round((await camera(2)).rotation)%360,90);
  assert.equal(await svg(2),measurement,'Fit mantém annotation');
  const bounds=await evaluate(`(()=>{const c=document.querySelector('[data-viewport="image-2"] canvas'),d=c.getContext('2d').getImageData(0,0,c.width,c.height).data;let minX=c.width,minY=c.height,maxX=0,maxY=0;for(let y=0;y<c.height;y++)for(let x=0;x<c.width;x++){if(d[4*(y*c.width+x)]>8){minX=Math.min(minX,x);maxX=Math.max(maxX,x);minY=Math.min(minY,y);maxY=Math.max(maxY,y);}}return {minX,minY,maxX,maxY,width:c.width,height:c.height}})()`);
  assert.ok(bounds.minX>0 && bounds.minY>0 && bounds.maxX<bounds.width-1 && bounds.maxY<bounds.height-1,'Fit contém imagem rotacionada');
  assert.ok(bounds.maxY-bounds.minY>bounds.height*.85,'Fit utiliza espaço disponível');
  assert.equal(await hash(0),untouched,'apresentação do outro viewport é preservada');
  await drag(2,'Zoom',[.15,.25],[.15,.4]);await drag(2,'Pan',[.15,.6],[.22,.68]);
  const beforeMax=await hash(2), requestCount=calls.length;
  await maximize(2);
  assert.equal(await evaluate("document.querySelectorAll('[data-viewport]').length"),4,'maximização não desmonta slots');
  assert.equal(await evaluate("Array.from(document.querySelectorAll('[data-viewport]')).filter(e=>getComputedStyle(e).visibility==='visible').length"),1);
  await wait(0,'Imagem 2 / 3');await wait(2,'Imagem 1 / 1');
  assert.equal(await svg(2),measurement);
  assert.equal(await evaluate('window.v4Canvases.every((c,i)=>c===document.querySelectorAll("main canvas")[i])'),true);
  assert.equal(await evaluate("(()=>{const c=document.querySelector('[data-viewport=\"image-2\"] canvas');return Math.abs(c.width/window.devicePixelRatio-c.clientWidth)<2 && Math.abs(c.height/window.devicePixelRatio-c.clientHeight)<2})()"),true,'canvas redimensionado');
  for(const tag of ['input','textarea','select','div']) {
    await evaluate(`{const e=document.createElement('${tag}');e.id='v4-editable';if('${tag}'==='div')e.contentEditable='true';document.querySelector('[role=toolbar]').append(e);e.focus();}`);
    const before=await hash(2);await key('Escape');await key('r');await key('f');
    assert.equal(await evaluate("document.querySelector('[data-viewport=\"image-2\"]').dataset.maximized"),'true');assert.equal(await hash(2),before);
    await evaluate("document.querySelector('#v4-editable').remove()");
  }
  await key('Escape');await pause(300);
  assert.equal(await evaluate("document.querySelector('[data-viewport=\"image-2\"]').dataset.maximized"),'false');
  assert.equal(await hash(2),beforeMax,'restaura exatamente pixels/apresentação');assert.equal(await hash(0),untouched);
  assert.equal(await svg(2),measurement);assert.equal(calls.length,requestCount,'maximizar/restaurar não baixa novamente');
  await maximize(2);await button('Restaurar viewport');assert.equal(await hash(2),beforeMax);
  await key('r');await pause(100);assert.equal(Math.round((await camera(2)).rotation)%360,180);
  await key('f');await pause(100);assert.deepEqual(await evaluate('window.v4VOI[2]'),voi);
  await button('Reset');assert.equal(await hash(2),original,'Reset restaura toda a apresentação');
  assert.equal(await pressed('Invert'),'false');assert.equal(await pressed('Flip Horizontal'),'false');assert.equal(await pressed('Flip Vertical'),'false');
  assert.equal(Math.round((await camera(2)).rotation)%360,0);assert.equal(await svg(2),measurement);assert.equal(await hash(0),untouched);
  for (const index of [0,1,2]) assert.notEqual(await hash(index),0,`viewport ${index} continua renderizado`);
  const screenshot=await command('Page.captureScreenshot',{format:'png'});
  await writeFile(join(tmpdir(),'pacs-viewer-v4-synthetic.png'),Buffer.from(screenshot.data,'base64'));

  await activate(0);await button('Rotate Right');await button('Invert');
  await evaluate("document.querySelector('[aria-label=FPS]').focus(); document.querySelector('[aria-label=FPS]').select()");
  await command('Input.insertText',{text:'1'});
  await button('Play');await button('Reset');
  assert.equal(await evaluate("Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()==='Pause')"),true);
  assert.equal(await pressed('Invert'),'false');
  await maximize(0);await key('Escape');await pause(150);
  assert.equal(await evaluate("Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()==='Pause')"),true);
  await button('Pause');await button('1x1');
  console.log('PASS: V4 auto-layout/restauração manual, Rotate/Flip, Fit retangular, Reset, maximização/resize/preservação, Escape/R/F e Cine.');
}
