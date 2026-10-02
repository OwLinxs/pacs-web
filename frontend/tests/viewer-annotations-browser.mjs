import assert from 'node:assert/strict';

export async function testViewerAnnotations({ evaluate, command, key, choose, pause, until }) {
  const button = (name) => evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()===${JSON.stringify(name)}).click()`);
  const svg = (index) => evaluate(`document.querySelector('[data-viewport="image-${index}"] .svg-layer')?.textContent || ''`);
  const point = (index,x,y) => evaluate(`(()=>{const r=document.querySelector('[data-viewport="image-${index}"]').getBoundingClientRect();return {x:r.x+r.width*${x},y:r.y+r.height*${y}}})()`);
  const mouse = (type,p,pressed=false) => command('Input.dispatchMouseEvent',{type,...p,button:pressed||type==='mouseReleased'?'left':'none',buttons:pressed?1:0,clickCount:type==='mouseMoved'?0:1});
  const activate = async (index) => {
    const p=await point(index,.02,.02); await mouse('mousePressed',p,true); await mouse('mouseReleased',p); await pause(80);
  };
  const draw = async (index) => {
    if (await evaluate("document.querySelector('[aria-label=Length]').getAttribute('aria-pressed')") !== 'true') await button('Length');
    const a=await point(index,.35,.3), b=await point(index,.6,.3);
    await mouse('mouseMoved',a); await mouse('mousePressed',a,true); await pause(210);
    for(let i=1;i<=6;i++){await mouse('mouseMoved',{x:a.x+(b.x-a.x)*i/6,y:a.y},true);await pause(20);}
    await mouse('mouseReleased',b); await until(async()=> /mm|px/.test(await svg(index))); await pause(120);
  };
  const selected = async (index) => {
    const p=await point(index,.475,.3); await mouse('mouseMoved',p); await mouse('mousePressed',p,true); await mouse('mouseReleased',p); await pause(100);
  };
  await draw(0); await selected(0); await key('Delete'); await until(async()=>await svg(0)==='');
  await draw(0); await selected(0);
  for(const tag of ['input','textarea','select','div']) {
    await evaluate(`{const e=document.createElement('${tag}');e.id='v3-editable';if('${tag}'==='div')e.contentEditable='true';document.querySelector('main').append(e);e.focus();}`);
    await key('Delete'); await key('Backspace'); assert.notEqual(await svg(0),'');
    await evaluate("document.querySelector('#v3-editable').remove()");
  }
  await key('Backspace'); await until(async()=>await svg(0)==='');
  await draw(0);
  await button('1x2'); await activate(1); await choose(1);
  await until(()=>evaluate("document.querySelector('[data-viewport=\"image-1\"]').textContent.includes('Imagem 1 / 2')"));
  await draw(1); const other=await svg(1);
  await activate(0); await button('Reset'); await pause(120); assert.notEqual(await svg(0),'');
  // Confirmação nativa do Chrome: cancelar e depois confirmar, sem mock da API.
  const clear = async (accept) => {
    const click=button('Limpar medições'); await pause(150);
    await command('Page.handleJavaScriptDialog',{accept}); await click; await pause(200);
  };
  await clear(false); assert.notEqual(await svg(0),''); assert.equal(await svg(1),other);
  await clear(true); await until(async()=>await svg(0)===''); assert.equal(await svg(1),other);
  // Mesma imagem em dois viewports segue a associação nativa por imageId.
  await draw(0); await activate(1); await choose(0);
  await until(()=>evaluate("document.querySelector('[data-viewport=\"image-1\"]').textContent.includes('Imagem 1 / 3')"));
  await until(async()=>/mm/.test(await svg(1)));
  await clear(true); await until(async()=>await svg(0)==='' && await svg(1)==='');
  await choose(1); await until(async()=>/px/.test(await svg(1)));
  assert.equal(await svg(1),other,'limpar A preserva annotations B');
  await button('1x1'); await pause(150);
  console.log('PASS: Delete/Backspace, editáveis, confirmação/cancelamento, Reset preserva, escopo por série e compartilhamento nativo da mesma imagem.');
}
