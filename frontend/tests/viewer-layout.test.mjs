import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';
const source=await readFile(new URL('../src/viewer/layout.ts',import.meta.url),'utf8');
const code=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText;
const {fillLayout}=await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`);
const series=Array.from({length:5},(_,index)=>({orthancSeriesId:`synthetic-series-${index}`,instanceCount:1}));
const ids=slots=>slots.map(slot=>slot.series?.orthancSeriesId ?? null);
const slot=(i,manual=false)=>({series:series[i],manual});

test('auto-layout 1→2→4 preserva principal e preenche ordem sem duplicar',()=>{
  const two=fillLayout([slot(0)],series,2,1);
  assert.deepEqual(ids(two),[series[0].orthancSeriesId,series[1].orthancSeriesId,null,null]);
  const four=fillLayout(two,series,4,2);
  assert.deepEqual(ids(four),series.slice(0,4).map(s=>s.orthancSeriesId));
});
test('menos séries deixa vazios; mais séries usa apenas slots necessários',()=>{
  assert.deepEqual(ids(fillLayout([],[],4,1)),[null,null,null,null]);
  assert.deepEqual(ids(fillLayout([],series.slice(0,2),4,1)),[series[0].orthancSeriesId,series[1].orthancSeriesId,null,null]);
  assert.deepEqual(ids(fillLayout([slot(4,true)],series,2,1)),[series[4].orthancSeriesId,series[0].orthancSeriesId,null,null]);
});
test('reduzir e expandir restaura seleção manual sem reorganizar',()=>{
  const before=[slot(0),slot(1),slot(4,true),slot(3)];
  const reduced=fillLayout(before,series,2,4);
  assert.deepEqual(ids(reduced),ids(before));
  assert.deepEqual(ids(fillLayout(reduced,series,4,2)),ids(before));
  assert.deepEqual(ids(before),[0,1,4,3].map(i=>series[i].orthancSeriesId),'não muta entrada');
});
test('histórico automático não duplica nova seleção manual; duplicação manual é mantida',()=>{
  const before=[slot(1,true),slot(1),slot(4,true),slot(3)];
  assert.deepEqual(ids(fillLayout(before,series,4,1)),[1,0,4,3].map(i=>series[i].orthancSeriesId));
  before[1]=slot(1,true);
  assert.deepEqual(ids(fillLayout(before,series,4,1)),[1,1,4,3].map(i=>series[i].orthancSeriesId));
});
test('atribuição inválida é descartada; série vazia não ganha semântica inventada',()=>{
  const available=[{...series[0],instanceCount:0},series[1]];
  const result=fillLayout([slot(4,true)],available,2,1);
  assert.deepEqual(ids(result),[series[0].orthancSeriesId,series[1].orthancSeriesId,null,null]);
  assert.equal(result[0].manual,false);
});
