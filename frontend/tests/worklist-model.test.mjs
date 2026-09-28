import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';
const source = await readFile(new URL('../src/worklist/model.ts', import.meta.url), 'utf8');
const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
const { initialWorklist, periodDates, validateWorklist, studyQuery, dicomDate, dicomTime, modalities } = await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`);

test('períodos usam calendário local, inclusive mudança de mês/ano', () => {
 const now = new Date(2026, 0, 2, 0, 15);
 assert.deepEqual(periodDates('today',now),{dateFrom:'2026-01-02',dateTo:'2026-01-02'});
 assert.deepEqual(periodDates('yesterday',now),{dateFrom:'2026-01-01',dateTo:'2026-01-01'});
 assert.deepEqual(periodDates('7days',now),{dateFrom:'2025-12-27',dateTo:'2026-01-02'});
 assert.deepEqual(periodDates('30days',now),{dateFrom:'2025-12-04',dateTo:'2026-01-02'});
});
test('estado inicial e filtros AND explícitos sem inventar curingas', () => {
 const state = initialWorklist(new Date(2026,8,28));
 assert.equal(state.sort,'dateDesc'); assert.equal(state.limit,25); assert.equal(state.offset,0);
 state.filters.patientName=' FICTICIO* ';state.filters.patientId='SYNTHETIC';state.modality='NEW';
 const query=studyQuery(state);assert.equal(query.patientName,'FICTICIO*');assert.equal(query.patientId,'SYNTHETIC');assert.equal(query.modality,'NEW');assert.equal(query.dateFrom,'2026-09-22');
});
test('datas inválidas e período invertido são bloqueados antes da consulta', () => {
 const state=initialWorklist();assert.equal(validateWorklist(state),'');
 for(const date of ['','2026-02-30','2026-13-01','2026-2-01'])assert.ok(validateWorklist({...state,dateFrom:date}));
 assert.ok(validateWorklist({...state,dateFrom:'2026-09-29',dateTo:'2026-09-28'}));
 assert.ok(validateWorklist({...state,modality:'CT*'}));
});
test('StudyDate é data DICOM sem timezone e valida calendário', () => {
 assert.equal(dicomDate('20260928'),'28/09/2026');assert.equal(dicomDate('20240229'),'29/02/2024');
 for(const value of ['','20260229','20260931','20261301','invalid','2026-09-28']) assert.equal(dicomDate(value),'—');
});
test('StudyTime parcial, fração, ausente e inválida não viram UTC', () => {
 for(const [value,want] of [['143259.123','14:32'],['1432','14:32'],['14','14h'],['235960','23:59'],['','—'],['240000','—'],['126000','—'],['123','—'],['123456.1234567','—'],['143299','—']])assert.equal(dicomTime(value),want);
});
test('modalidades múltiplas, desconhecidas e ausentes são determinísticas', () => {
 assert.deepEqual(modalities(['SR','CT','CT',' ','NEW',' CT ']),['CT','NEW','SR']);assert.deepEqual(modalities([]),[]);
});
