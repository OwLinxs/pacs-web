import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';
const source = await readFile(new URL('../src/viewer/cine.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
const { createCine } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`);

test('Cine aguarda imagem, limita FPS e não acumula ticks; Pause cancela', async (t) => {
  const timers = new Map(); let sequence=0, delay;
  t.mock.method(globalThis, 'setTimeout', (fn, ms) => { delay=ms; timers.set(++sequence,fn); return sequence; });
  t.mock.method(globalThis, 'clearTimeout', (id) => timers.delete(id));
  let calls=0, finish; const states=[];
  const cine=createCine(()=>{calls++;return new Promise(resolve=>{finish=resolve;});},value=>states.push(value));
  cine.play(999); assert.equal(delay,1000/30); assert.equal(timers.size,1);
  const tick=timers.values().next().value; timers.clear(); tick();
  assert.equal(calls,1); assert.equal(timers.size,0,'não programa tick enquanto imagem carrega');
  finish(); await Promise.resolve(); assert.equal(timers.size,1);
  cine.stop(); assert.equal(timers.size,0); assert.equal(states.at(-1),false);
  cine.play(-1); assert.equal(delay,1000); cine.stop();
});
test('Cine cancelado durante carregamento não ressuscita timer antigo', async (t) => {
  const timers=new Map(); let seq=0,finish;
  t.mock.method(globalThis,'setTimeout',fn=>{timers.set(++seq,fn);return seq;});
  t.mock.method(globalThis,'clearTimeout',id=>timers.delete(id));
  const cine=createCine(()=>new Promise(resolve=>{finish=resolve;}),()=>{});
  cine.play(10); const tick=timers.values().next().value; timers.clear(); tick();
  cine.stop(); finish(); await Promise.resolve(); assert.equal(timers.size,0);
});
test('Falha durante Cine encerra reprodução sem novo request', async (t) => {
  const timers=new Map();let seq=0; const states=[];
  t.mock.method(globalThis,'setTimeout',fn=>{timers.set(++seq,fn);return seq;});
  t.mock.method(globalThis,'clearTimeout',id=>timers.delete(id));
  const cine=createCine(async()=>{throw new Error('Falha sintética');},value=>states.push(value));
  cine.play(10);const tick=timers.values().next().value;timers.clear();tick();
  await Promise.resolve();await Promise.resolve();assert.equal(timers.size,0);assert.equal(states.at(-1),false);
});
