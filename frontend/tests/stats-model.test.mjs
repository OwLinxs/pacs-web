import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const source = await readFile(new URL('../src/stats/model.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
const m = await import(`data:text/javascript,${encodeURIComponent(js)}`);

test('navegação de períodos nunca ultrapassa hoje e ajusta fim de mês', () => {
  assert.equal(m.shiftDate('day', '2026-10-29', 1, '2026-10-30'), '2026-10-30');
  assert.equal(m.shiftDate('day', '2026-10-30', 1, '2026-10-30'), '2026-10-30');
  assert.equal(m.shiftDate('week', '2026-10-29', -1, '2026-10-30'), '2026-10-22');
  assert.equal(m.shiftDate('week', '2026-10-25', 1, '2026-10-30'), '2026-10-30');
  assert.equal(m.shiftDate('month', '2026-03-31', -1, '2026-10-30'), '2026-02-28');
  assert.equal(m.shiftDate('month', '2026-01-15', -1, '2026-10-30'), '2025-12-15');
  assert.equal(m.todaySaoPaulo(new Date('2026-10-30T02:00:00Z')), '2026-10-29');
});

test('rótulos de período, barras e variação sem inventar percentuais', () => {
  assert.equal(m.periodLabel('day', '2026-10-29', '2026-10-29'), '29/10/2026');
  assert.equal(m.periodLabel('week', '2026-10-26', '2026-11-01'), '26/10 a 01/11/2026');
  assert.equal(m.periodLabel('month', '2026-03-01', '2026-03-31'), 'março de 2026');
  assert.equal(m.bucketLabel('week', '2026-10-26'), 'seg 26');
  assert.equal(m.bucketLabel('month', '2026-10-05'), '5');
  assert.equal(m.bucketLabel('day', '08'), '08h');
  assert.equal(m.bucketTitle('day', '08'), '08:00–08:59');
  assert.equal(m.delta(12, 10, 'week'), '▲ 20% vs semana anterior');
  assert.equal(m.delta(5, 10, 'month'), '▼ 50% vs mês anterior');
  assert.equal(m.delta(3, 0, 'day'), 'sem registros no dia anterior');
  assert.equal(m.delta(0, 0, 'day'), 'igual ao dia anterior');
  assert.equal(m.delta(4, 4, 'week'), 'igual à semana anterior');
  assert.equal(m.delta(2, 0, 'week'), 'sem registros na semana anterior');
});

test('escala com topo limpo e rótulos compactos', () => {
  assert.deepEqual(m.niceScale(0), { top: 4, step: 1 });
  assert.deepEqual(m.niceScale(3), { top: 3, step: 1 });
  assert.deepEqual(m.niceScale(7), { top: 8, step: 2 });
  assert.deepEqual(m.niceScale(137), { top: 150, step: 50 });
  assert.equal(m.diskLabel(512), '512 MB');
  assert.equal(m.diskLabel(1536), '1,5 GB');
});
