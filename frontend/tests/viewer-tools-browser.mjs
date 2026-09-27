import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';

// Exercita o build real por eventos de mouse/teclado do Chrome. Nenhum hook
// de teste é adicionado à aplicação, nem métodos internos do Cornerstone usados.
export async function testViewerTools({ evaluate, command, key, count, choose, pause, until }) {
  const tool = async (name) => {
    await evaluate(`document.querySelector('[role="toolbar"] button[aria-label="${name}"]').click()`);
    if (name !== 'Reset' && name !== 'Invert') assert.equal(await evaluate(`document.querySelector('[aria-label="${name}"]').getAttribute('aria-pressed')`), 'true');
    await pause(80);
  };
  const point = async (x, y) => evaluate(`(() => { const r=document.querySelector('main canvas').getBoundingClientRect(); return {x:r.x+r.width*${x}, y:r.y+r.height*${y}}; })()`);
  const mouse = (type, p, pressed = false) => command('Input.dispatchMouseEvent', { type, ...p, button: pressed || type === 'mouseReleased' ? 'left' : 'none', buttons: pressed ? 1 : 0, clickCount: type === 'mouseMoved' ? 0 : 1 });
  const drag = async (a, b) => {
    const start = await point(...a), end = await point(...b);
    await mouse('mouseMoved', start);
    await mouse('mousePressed', start, true); await pause(210);
    for (let i=1;i<=6;i++) { await mouse('mouseMoved', {x:start.x+(end.x-start.x)*i/6,y:start.y+(end.y-start.y)*i/6}, true); await pause(20); }
    await mouse('mouseReleased', end); await pause(450);
  };
  const click = async (x, y) => {
    const p = await point(x,y); await mouse('mouseMoved', p); await mouse('mousePressed', p, true); await mouse('mouseReleased', p); await pause(450);
  };
  const text = () => evaluate("document.querySelector('main .svg-layer')?.textContent || ''");
  const waitText = async (pattern) => {
    try { await until(async () => pattern.test(await text()), 10000); }
    catch (error) { console.error('Texto de annotations sintéticas:', await text()); throw error; }
  };
  const imageHash = () => evaluate(`(() => { const c=document.querySelector('main canvas'); const pixels=c.getContext('2d').getImageData(0,0,c.width,c.height).data; let hash=0; for(let i=0;i<pixels.length;i+=256) hash=(hash*31+pixels[i])|0; return hash; })()`);
  await evaluate(`(() => { const e=document.querySelector('[data-viewport-uid]'); window.fixturePresentation={};
    e.addEventListener('CORNERSTONE_VOI_MODIFIED',ev=>window.fixturePresentation.voi=ev.detail.range);
    e.addEventListener('CORNERSTONE_CAMERA_MODIFIED',ev=>window.fixturePresentation.camera=ev.detail.camera);
  })()`);
  await tool('Reset');
  const original = await imageHash();
  await tool('Window/Level'); await drag([.4,.4],[.55,.55]);
  assert.notEqual(await imageHash(), original, 'WL deve alterar pixels apresentados');
  await tool('Reset'); assert.equal(await imageHash(), original, 'Reset restaura VOI inicial');
  await tool('Zoom'); await drag([.5,.4],[.5,.6]);
  assert.notEqual(await imageHash(), original, 'Zoom deve alterar apresentação');
  await tool('Reset'); assert.equal(await imageHash(), original);
  await tool('Pan'); await drag([.5,.4],[.6,.5]);
  assert.notEqual(await imageHash(), original, 'Pan deve deslocar imagem');
  await tool('Reset'); assert.equal(await imageHash(), original);
  await tool('Invert'); assert.equal(await evaluate("document.querySelector('[aria-label=Invert]').getAttribute('aria-pressed')"), 'true');
  assert.notEqual(await imageHash(), original);
  await tool('Reset'); assert.equal(await imageHash(), original);
  assert.equal(await evaluate("document.querySelector('[aria-label=Invert]').getAttribute('aria-pressed')"), 'false');

  await tool('Length'); await drag([.35,.25],[.47,.25]); await waitText(/mm/);
  const lengthText = await text();
  await tool('Reset'); assert.equal(await text(), lengthText, 'Reset não remove medição');
  await tool('Angle'); await drag([.6,.25],[.7,.35]); await click(.6,.45); await waitText(/°/);
  await tool('Probe'); await click(.65,.65); await waitText(/\(\d+,\s*\d+,\s*\d+\)/);
  await tool('Rectangle ROI'); await drag([.35,.55],[.47,.7]); await waitText(/Mean/);
  const annotationsA = await text();
  assert.ok(annotationsA.includes('mm') && annotationsA.includes('°'));
  await tool('Reset'); assert.equal(await text(), annotationsA);
  const capture = await command('Page.captureScreenshot', { format: 'png' });
  await writeFile(join(tmpdir(), 'pacs-viewer-tools-synthetic.png'), Buffer.from(capture.data, 'base64'));
  await tool('Invert');
  await choose(1); await count('Imagem 1 / 2');
  assert.equal(await evaluate("document.querySelector('[aria-label=Invert]').getAttribute('aria-pressed')"), 'false', 'nova série começa na apresentação DICOM');
  assert.equal(await text(), '', 'medição da série A não aparece em B');
  await tool('Length'); await drag([.35,.25],[.47,.25]); await waitText(/px/);
  assert.equal((await text()).includes('mm'), false, 'sem Pixel Spacing não apresenta mm');
  await choose(0); await count('Imagem 1 / 3'); await waitText(/Mean/);
  assert.equal(await text(), annotationsA, 'annotations de A continuam ao retornar de B');
  await tool('Window/Level');

  // Reabrir remove annotations do Viewer anterior e não acumula keydown.
  const windowObject = await command('Runtime.evaluate', { expression:'window' });
  const listeners = async () => (await command('DOMDebugger.getEventListeners', {objectId:windowObject.result.objectId})).listeners.filter((l)=>l.type==='keydown').length;
  const mountedListeners = await listeners();
  await evaluate("document.querySelector('header button').click()");
  await until(() => evaluate("!!document.querySelector('[role=button]')"));
  assert.equal(await listeners(), mountedListeners - 1);
  await key('ArrowRight'); // sem Viewer não deve requisitar uma imagem
  await evaluate("document.querySelector('[role=button]').click()");
  await count('Imagem 1 / 3');
  assert.equal(await listeners(), mountedListeners);
  assert.equal(await text(), '', 'annotations devem terminar junto com a tela');
  assert.equal(await evaluate("document.querySelectorAll('main .svg-layer').length"), 1);
  await evaluate("window.fixtureEngine = document.querySelector('main canvas')");
}
