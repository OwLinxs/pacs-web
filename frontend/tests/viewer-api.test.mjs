import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';

const compile = (source) => `data:text/javascript;base64,${Buffer.from(ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText).toString('base64')}`;
const clientURL = compile(await readFile(new URL('../src/api/client.ts', import.meta.url), 'utf8'));
const { api, ApiError, viewerDICOMPath, viewerRoute, studyIDFromRoute } = await import(clientURL);
const selection = (await readFile(new URL('../src/viewer/selection.ts', import.meta.url), 'utf8')).replace("'../api/client'", JSON.stringify(clientURL));
const { initialSeries, loadSeriesStack, navigationDelta } = await import(compile(selection));
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

test('seleção inicial e montagem do stack preservam ordem do backend sem baixar pixels', async (t) => {
  const signal = new AbortController().signal;
  const series = [1, 2, 3].map((n) => ({ orthancSeriesId: id(n + 10), instanceCount: n === 1 ? 0 : 2 }));
  assert.equal(initialSeries(series), series[1]);
  assert.equal(initialSeries([]), null);
  assert.equal(initialSeries([series[0]]), series[0]);
  const requests = [];
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    requests.push(path);
    assert.equal(options.method, 'GET');
    assert.equal(options.credentials, 'same-origin');
    assert.equal(options.signal, signal);
    assert.equal(options.headers.Authorization, undefined);
    return Response.json({ items: [{ orthancInstanceId: id(32), number: 2 }, { orthancInstanceId: id(30), number: 10 }] });
  });
  assert.deepEqual(await loadSeriesStack(id(1), id(12), signal), [viewerDICOMPath(id(1), id(12), id(32)), viewerDICOMPath(id(1), id(12), id(30))]);
  await loadSeriesStack(id(1), id(13), signal);
  assert.deepEqual(requests, [`/api/studies/${id(1)}/series/${id(12)}/instances`, `/api/studies/${id(1)}/series/${id(13)}/instances`]);
});

test('série vazia não requisita DICOM', async (t) => {
  const fetch = t.mock.method(globalThis, 'fetch', async () => Response.json({ items: [] }));
  assert.deepEqual(await loadSeriesStack(id(1), id(2), new AbortController().signal), []);
  assert.equal(fetch.mock.callCount(), 1);
});

test('setas respeitam editáveis e modificadores', (t) => {
  class Element { constructor(editable = false, input = false) { this.isContentEditable = editable; this.input = input; } closest() { return this.input; } }
  const previous = globalThis.HTMLElement;
  globalThis.HTMLElement = Element;
  t.after(() => { if (previous) globalThis.HTMLElement = previous; else delete globalThis.HTMLElement; });
  for (const key of ['ArrowRight', 'ArrowDown']) assert.equal(navigationDelta({ key, target: new Element() }), 1);
  for (const key of ['ArrowLeft', 'ArrowUp']) assert.equal(navigationDelta({ key, target: new Element() }), -1);
  for (const event of [{ target: new Element(true) }, { target: new Element(false, true) }, { ctrlKey: true }, { metaKey: true }, { altKey: true }, { shiftKey: true }, { defaultPrevented: true }]) {
    assert.equal(navigationDelta({ key: 'ArrowDown', ...event }), 0);
  }
});

test('sessão expirada e falha sanitizada permanecem identificáveis', async (t) => {
  for (const status of [401, 502, 504]) {
    const mock = t.mock.method(globalThis, 'fetch', async () => Response.json({ error: { code: 'FIXTURE', message: 'Mensagem sanitizada.' } }, { status }));
    await assert.rejects(loadSeriesStack(id(1), id(2), new AbortController().signal), (error) => error instanceof ApiError && error.status === status && error.message === 'Mensagem sanitizada.');
    mock.mock.restore();
  }
});
