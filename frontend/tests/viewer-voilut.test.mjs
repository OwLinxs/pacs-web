import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';
const source=await readFile(new URL('../src/viewer/voiLutFunction.ts',import.meta.url),'utf8');
const code=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText;
const {normalizeVoiLutFunction}=await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`);

test('VOI LUT Function válida é preservada (string ou array)',()=>{
  assert.equal(normalizeVoiLutFunction('LINEAR'),'LINEAR');
  assert.equal(normalizeVoiLutFunction('LINEAR_EXACT'),'LINEAR_EXACT');
  assert.equal(normalizeVoiLutFunction(['LINEAR_EXACT','LINEAR']),'LINEAR_EXACT');
});

test('padding, caixa e NUL do DICOM são tolerados',()=>{
  assert.equal(normalizeVoiLutFunction('linear '),'LINEAR');
  assert.equal(normalizeVoiLutFunction('LINEAR_EXACT\0'),'LINEAR_EXACT');
});

test('SIGMOID cai para o padrão (resetProperties do CS3D 5.11 quebra com sigmoide)',()=>{
  assert.equal(normalizeVoiLutFunction('SIGMOID'),undefined);
  assert.equal(normalizeVoiLutFunction(['SIGMOID']),undefined);
});

test('valor truncado pelo bug do loader 5.11, vazio ou desconhecido vira padrão',()=>{
  for (const raw of ['L','S','', '   ','LIN','GAMMA',undefined,null,42,[],[7],{}]) assert.equal(normalizeVoiLutFunction(raw),undefined,String(raw));
});
