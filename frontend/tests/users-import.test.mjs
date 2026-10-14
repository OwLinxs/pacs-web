import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';
import { strToU8, zipSync } from 'fflate';

const source = await readFile(new URL('../src/users/importPreview.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
const { previewSheets, validateXlsxArchive } = await import(`data:text/javascript,${encodeURIComponent(js)}`);

test('identifica Nome, ignora QTD, limpa espaços e omite linhas vazias', () => {
  const preview = previewSheets([{ sheet: 'Entrada', data: [
    ['QTD', ' Nome '], [1, '  EXEMPLO   A  '], [null, null], [2, 'EXEMPLO B'],
  ] }]);
  assert.deepEqual(preview.rows.map(row => [row.line, row.name, row.status]), [
    [2, 'EXEMPLO A', 'valid'], [4, 'EXEMPLO B', 'valid'],
  ]);
  assert.equal(preview.valid, 2);
});

test('detecta duplicados na aba e conta nomes inválidos sem confundir QTD com Nome', () => {
  const preview = previewSheets([{ sheet: 'Entrada', data: [
    ['Nome', 'QTD'], ['Exemplo A', 1], [' exemplo  a ', 2], ['', 3], [42, 4],
  ] }]);
  assert.deepEqual([preview.valid, preview.duplicate, preview.invalid], [1, 1, 2]);
  assert.equal(preview.rows[1].status, 'duplicate');
});

test('localiza cabeçalho em outra aba e rejeita cabeçalho ausente', () => {
  const sheets = [{ sheet: 'Capa', data: [['Legenda']] }, { sheet: 'Dados', data: [[''], ['Nome'], ['EXEMPLO A']] }];
  assert.equal(previewSheets(sheets).sheet, 'Dados');
  assert.throws(() => previewSheets([{ sheet: 'Capa', data: [['QTD']] }]), /Coluna “Nome”/);
});

test('rejeita planilha excessiva e arquivo ZIP inválido', () => {
  assert.throws(() => previewSheets([{ sheet: 'Dados', data: [['Nome'], ...Array.from({ length: 5001 }, () => ['EXEMPLO'])] }]), /5.000/);
  assert.throws(() => validateXlsxArchive(strToU8('não é XLSX')), /Arquivo .xlsx inválido/);
  const tinyZip = zipSync({ 'arquivo.txt': strToU8('fixture sintética') });
  assert.doesNotThrow(() => validateXlsxArchive(tinyZip));
});
