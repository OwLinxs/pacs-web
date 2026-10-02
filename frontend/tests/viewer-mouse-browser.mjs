import assert from 'node:assert/strict';

// Gestos reais no Chrome com as instâncias DICOM exclusivamente sintéticas do smoke.
export async function testViewerMouse({ evaluate, command, pause, until }) {
  const pane = (index) => `[data-viewport="image-${index}"]`;
  const point = (index, x, y) => evaluate(`(() => { const r=document.querySelector('${pane(index)} canvas').getBoundingClientRect(); return {x:r.x+r.width*${x},y:r.y+r.height*${y}}; })()`);
  const hash = (index) => evaluate(`(() => { const c=document.querySelector('${pane(index)} canvas'); const d=c.getContext('2d').getImageData(0,0,c.width,c.height).data; let h=0; for(let i=0;i<d.length;i+=64) h=(h*31+d[i])|0; return h; })()`);
  const label = (index) => evaluate(`document.querySelector('${pane(index)} [role="status"]')?.textContent`);
  const select = async (name) => { await evaluate(`document.querySelector('[role="toolbar"] button[aria-label="${name}"]').click()`); await pause(100); };
  const pressed = (name) => evaluate(`document.querySelector('[role="toolbar"] button[aria-label="${name}"]').getAttribute('aria-pressed')`);
  const drag = async (index, button, from = [.4,.4], to = [.58,.57]) => {
    const start = await point(index, ...from), end = await point(index, ...to);
    const buttons = button === 'right' ? 2 : 1;
    await command('Input.dispatchMouseEvent', { type:'mouseMoved', ...start, button:'none', buttons:0 });
    await command('Input.dispatchMouseEvent', { type:'mousePressed', ...start, button, buttons, clickCount:1 });
    await pause(180);
    for(let i=1;i<=6;i++) {
      await command('Input.dispatchMouseEvent', { type:'mouseMoved', x:start.x+(end.x-start.x)*i/6, y:start.y+(end.y-start.y)*i/6, button, buttons });
      await pause(20);
    }
    await command('Input.dispatchMouseEvent', { type:'mouseReleased', ...end, button, buttons:0, clickCount:1 });
    await pause(250);
  };
  const wheel = async (index, deltaY) => { const p=await point(index,.5,.5); await command('Input.dispatchMouseEvent', { type:'mouseWheel', ...p, deltaX:0, deltaY }); await pause(200); };
  const reset = async () => { await select('Reset'); await pause(150); };

  assert.equal(await pressed('Pan'), 'true', 'Pan é a ferramenta inicial do botão esquerdo');
  await reset(); const original = await hash(0);
  await drag(0,'left'); assert.notEqual(await hash(0),original,'esquerdo move a imagem com Pan');
  await reset(); assert.equal(await hash(0),original);
  await drag(0,'right'); assert.notEqual(await hash(0),original,'direito ajusta Window/Level');
  await reset(); assert.equal(await hash(0),original);
  const firstImage = await label(0);
  await wheel(0,100); assert.notEqual(await hash(0),original,'roda aplica Zoom');
  assert.equal(await label(0),firstImage,'roda não navega stack');
  await wheel(0,-100); assert.equal(await label(0),firstImage);
  await reset();
  assert.equal(await evaluate(`(() => { const e=document.querySelector('${pane(0)} [data-viewport-uid]'); return !e.dispatchEvent(new MouseEvent('contextmenu',{bubbles:true,cancelable:true})); })()`),true,'menu de contexto é bloqueado somente no viewport');

  await select('Length'); assert.equal(await pressed('Length'),'true');
  await drag(0,'left',[.35,.25],[.55,.25]);
  await until(() => evaluate(`!!document.querySelector('${pane(0)} .svg-layer')?.textContent`));
  const withMeasurement = await hash(0);
  await drag(0,'right',[.3,.4],[.5,.6]); assert.notEqual(await hash(0),withMeasurement,'direito permanece WL durante medição');
  const afterWL = await hash(0);
  await wheel(0,100); assert.notEqual(await hash(0),afterWL,'roda permanece Zoom durante medição');
  assert.equal(await label(0),firstImage);
  await select('Length'); assert.equal(await pressed('Pan'),'true','clicar novamente na medição retorna Pan');
  await reset(); const afterReset = await hash(0);
  await drag(0,'left'); assert.notEqual(await hash(0),afterReset,'esquerdo volta a Pan');
  await reset();
  for(let i=0;i<3;i++) for(const name of ['Length','Angle','Probe','Rectangle ROI','Pan']) await select(name);
  assert.equal(await pressed('Pan'),'true');
  const stable = await hash(0);
  await drag(0,'right'); assert.notEqual(await hash(0),stable,'trocas repetidas não acumulam conflito no direito');
  await reset(); await wheel(0,100); assert.notEqual(await hash(0),stable,'trocas repetidas não acumulam conflito na roda');
  await reset();

  await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()==='1x2').click()`);
  await until(() => evaluate(`document.querySelector('${pane(1)} [role="status"]')?.textContent?.includes('Imagem 1 /')`));
  const before0=await hash(0), before1=await hash(1);
  await drag(0,'left'); assert.notEqual(await hash(0),before0); assert.equal(await hash(1),before1,'Pan isolado no ToolGroup ativo');
  await reset(); await wheel(0,100); assert.equal(await hash(1),before1,'Zoom isolado por viewport');
  await reset();
  const p=await point(1,.05,.08);
  await command('Input.dispatchMouseEvent',{type:'mousePressed',...p,button:'left',buttons:1,clickCount:1});
  await command('Input.dispatchMouseEvent',{type:'mouseReleased',...p,button:'left',buttons:0,clickCount:1});
  await until(() => evaluate(`document.querySelector('${pane(1)}').dataset.active==='true'`));
  await drag(1,'left'); assert.notEqual(await hash(1),before1); assert.equal(await hash(0),before0,'segundo ToolGroup não afeta o primeiro');
  await evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent.trim()==='1x1').click()`);
  await until(() => evaluate("document.querySelectorAll('[data-viewport]').length===1"));
  console.log('PASS: mouse Pan/WL/Zoom, medição temporária, limpeza de bindings e ToolGroups independentes.');
}
