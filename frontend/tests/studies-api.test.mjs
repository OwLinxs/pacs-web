import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';

// Usa o cliente real, transpilado em memória, e fetch fictício. Sem servidor,
// dependências adicionais, cookies reais ou acesso à configuração do ambiente.
const source = await readFile(new URL('../src/api/client.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
});
const { api, ApiError } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

test('Worklist envia filtros codificados, paginação, sessão e cancelamento', async (t) => {
  const controller = new AbortController();
  const page = { items: [], limit: 25, offset: 25, hasMore: false, nextOffset: null };
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    const url = new URL(path, 'https://app.invalid');
    assert.equal(url.pathname, '/api/studies');
    assert.equal(url.searchParams.get('patientName'), 'FICTICIO^A & B*');
    assert.equal(url.searchParams.get('patientId'), 'ID-FICTICIO');
    assert.equal(url.searchParams.get('accessionNumber'), 'ACC-FICTICIO');
    assert.equal(url.searchParams.get('institutionName'), 'Instituição fictícia');
    assert.equal(url.searchParams.get('dateFrom'), '2026-09-01');
    assert.equal(url.searchParams.get('dateTo'), '2026-09-26');
    assert.equal(url.searchParams.get('offset'), '25');
    assert.equal(url.searchParams.get('limit'), '25');
    assert.equal(options.method, 'GET');
    assert.equal(options.credentials, 'same-origin');
    assert.equal(options.signal, controller.signal);
    assert.equal(options.body, undefined);
    assert.equal(options.headers.Authorization, undefined);
    return Response.json(page);
  });
  assert.deepEqual(await api.getStudies({ limit: 25, offset: 25, dateFrom: '2026-09-01', dateTo: '2026-09-26',
    patientName: 'FICTICIO^A & B*', patientId: 'ID-FICTICIO', accessionNumber: 'ACC-FICTICIO', institutionName: 'Instituição fictícia',
  }, controller.signal), page);
});

test('401 é identificável como sessão expirada pela tela', async (t) => {
  t.mock.method(globalThis, 'fetch', async () => Response.json({ error: { code: 'UNAUTHENTICATED', message: 'Sessão inválida ou expirada.' } }, { status: 401 }));
  await assert.rejects(api.getStudies({}, new AbortController().signal), (error) => error instanceof ApiError && error.status === 401 && error.code === 'UNAUTHENTICATED');
});

test('erro PACS mantém mensagem sanitizada e não provoca nova consulta', async (t) => {
  const fetch = t.mock.method(globalThis, 'fetch', async () => Response.json({ error: { code: 'PACS_TIMEOUT', message: 'Refine os filtros.' } }, { status: 504 }));
  await assert.rejects(api.getStudies({}, new AbortController().signal), (error) => error instanceof ApiError && error.status === 504 && error.message === 'Refine os filtros.');
  assert.equal(fetch.mock.callCount(), 1);
});

test('cancelamento interrompe a consulta substituída ou desmontada', async (t) => {
  const controller = new AbortController();
  controller.abort();
  t.mock.method(globalThis, 'fetch', async (_path, options) => { options.signal.throwIfAborted(); });
  await assert.rejects(api.getStudies({}, controller.signal), { name: 'AbortError' });
});
