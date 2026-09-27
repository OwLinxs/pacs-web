import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';

const compile = (source) => `data:text/javascript;base64,${Buffer.from(ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText).toString('base64')}`;
const clientURL = compile(await readFile(new URL('../src/api/client.ts', import.meta.url), 'utf8'));
const { api, ApiError, viewerDICOMPath, viewerRoute, studyIDFromRoute } = await import(clientURL);
const selection = (await readFile(new URL('../src/viewer/selection.ts', import.meta.url), 'utf8')).replace("'../api/client'", JSON.stringify(clientURL));
const { selectInitialImage } = await import(compile(selection));
const id = (n) => `${String(n).padStart(8, '0')}-00000000-00000000-00000000-00000000`;

test('Viewer monta apenas rotas locais com IDs válidos', async () => {
  assert.equal(studyIDFromRoute(viewerRoute(id(1))), id(1));
  assert.equal(viewerDICOMPath(id(1), id(2), id(3)), `/api/studies/${id(1)}/series/${id(2)}/instances/${id(3)}/dicom`);
  for (const bad of ['https://other.invalid', '../system', 'paciente', id(1) + '?url=x']) {
    assert.throws(() => viewerRoute(bad));
    assert.throws(() => viewerDICOMPath(id(1), bad, id(3)));
    assert.equal(studyIDFromRoute(`/viewer/${bad}`), null);
    await assert.rejects(api.getViewerSeries(bad, new AbortController().signal));
  }
});

test('primeira série com instâncias usa sessão, cancelamento e nenhum prefetch', async (t) => {
  const signal = new AbortController().signal;
  const series = [1, 2, 3, 4].map((n) => ({ orthancSeriesId: id(n + 10), instanceCount: n === 1 ? 0 : 1 }));
  const requests = [];
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    requests.push(path);
    assert.equal(options.method, 'GET');
    assert.equal(options.credentials, 'same-origin');
    assert.equal(options.signal, signal);
    assert.equal(options.headers.Authorization, undefined);
    assert.equal(options.body, undefined);
    if (requests.length === 1) return Response.json({ items: series });
    if (requests.length === 2) return Response.json({ items: [] });
    return Response.json({ items: [{ orthancInstanceId: id(30), number: 1 }] });
  });
  const result = await selectInitialImage(id(1), signal);
  assert.equal(result.selected.orthancSeriesId, id(13));
  assert.equal(result.path, viewerDICOMPath(id(1), id(13), id(30)));
  assert.deepEqual(requests, [`/api/studies/${id(1)}/series`, `/api/studies/${id(1)}/series/${id(12)}/instances`, `/api/studies/${id(1)}/series/${id(13)}/instances`]);
});

test('estudo vazio não requisita DICOM', async (t) => {
  const fetch = t.mock.method(globalThis, 'fetch', async () => Response.json({ items: [] }));
  assert.equal((await selectInitialImage(id(1), new AbortController().signal)).path, null);
  assert.equal(fetch.mock.callCount(), 1);
});

test('sessão expirada e falha sanitizada permanecem identificáveis', async (t) => {
  for (const status of [401, 502, 504]) {
    const mock = t.mock.method(globalThis, 'fetch', async () => Response.json({ error: { code: 'FIXTURE', message: 'Mensagem sanitizada.' } }, { status }));
    await assert.rejects(selectInitialImage(id(1), new AbortController().signal), (error) => error instanceof ApiError && error.status === status && error.message === 'Mensagem sanitizada.');
    mock.mock.restore();
  }
});
