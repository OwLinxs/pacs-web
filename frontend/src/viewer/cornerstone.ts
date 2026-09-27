import { cache, Enums, imageLoader, init, RenderingEngine, utilities, type StackViewport } from '@cornerstonejs/core';
import { init as initLoader, wadouri } from '@cornerstonejs/dicom-image-loader';
import { utilities as metadata } from '@cornerstonejs/metadata';

export const VIEWER_SESSION_EXPIRED = 'pacs-viewer-session-expired';
let initialized = false;
const requests = new Map<XMLHttpRequest, string>();
// Uma única viewport. Serializa o encerramento de uma carga antes da próxima
// para não deixar pixels/metadados de uma sessão anterior no cache da biblioteca.
let previous: Promise<void> = Promise.resolve();

function initialize() {
  if (initialized) return;
  // Os loggers da biblioteca podem incluir metadados/IDs; não os habilitamos.
  utilities.logger.log.setLevel('silent');
  utilities.logger.cs3dLog.setLevel('silent');
  init();
  cache.setMaxCacheSize(256 * 1024 * 1024);
  initLoader({
    maxWebWorkers: 1,
    open(xhr, url) {
      xhr.open('GET', url, true);
      xhr.timeout = 130000;
      requests.set(xhr, url);
      xhr.addEventListener('loadend', () => requests.delete(xhr), { once: true });
    },
    errorInterceptor(error) {
      if (error.status === 401) window.dispatchEvent(new Event(VIEWER_SESSION_EXPIRED));
    },
  });
  initialized = true;
}

export type StackCallbacks = {
  loading: () => void;
  rendered: (index: number, total: number) => void;
  failed: () => void;
};
export type StackController = {
  setStack: (paths: string[], signal: AbortSignal) => Promise<void>;
  step: (delta: number) => void;
};

/** Um engine por tela. Operações serializadas; nunca prefetch de pixels. */
export async function createStackViewer(element: HTMLDivElement, lifetime: AbortSignal, callbacks: StackCallbacks): Promise<StackController> {
  const preceding = previous;
  let release!: () => void;
  previous = new Promise<void>((resolve) => { release = resolve; });
  await preceding;
  if (lifetime.aborted) { release(); lifetime.throwIfAborted(); }
  let engine: RenderingEngine;
  try {
    initialize();
    engine = new RenderingEngine(`pacs-${crypto.randomUUID()}`);
    try {
      engine.enableElement({ viewportId: 'image', element, type: Enums.ViewportType.STACK, defaultOptions: { background: [0, 0, 0] } });
    } catch (error) { engine.destroy(); throw error; }
  } catch (error) { release(); throw error; }
  const viewport = engine.getViewport<StackViewport>('image');
  let queue: Promise<void> = Promise.resolve();
  let currentSignal: AbortSignal | null = null;
  let imageIds: string[] = [];
  let targetIndex = -1;
  let busy = false;
  let revision = 0;
  const abortRequests = () => { for (const xhr of requests.keys()) xhr.abort(); };
  const observer = new ResizeObserver(() => { if (!engine.hasBeenDestroyed) engine.resize(true, true); });
  observer.observe(element);

  const render = async (index: number, signal: AbortSignal, load: () => Promise<unknown>) => {
    signal.throwIfAborted(); lifetime.throwIfAborted();
    // Aguarda somente a imagem solicitada antes de entregá-la ao viewport.
    // Em 5.11, o caminho GPU pode resolver a troca mesmo após erro do loader.
    // As APIs padrão abaixo permitem capturar a falha antes de mudar os pixels.
    const imageId = imageIds[index];
    if (!imageId) throw new Error('Imagem indisponível.');
    if (!cache.getImage(imageId)) {
      const image = await imageLoader.loadImage(imageId, { requestType: Enums.RequestType.Interaction, priority: 0 });
      signal.throwIfAborted(); lifetime.throwIfAborted();
      cache.putImageSync(imageId, image);
    }
    await load();
    signal.throwIfAborted(); lifetime.throwIfAborted();
    await new Promise<void>((resolve, reject) => {
      const cleanup = () => {
        window.clearTimeout(timer);
        element.removeEventListener(Enums.Events.IMAGE_RENDERED, rendered);
        signal.removeEventListener('abort', canceled);
        lifetime.removeEventListener('abort', canceled);
      };
      const rendered = () => {
        if (viewport.getCurrentImageId() !== imageIds[index] || viewport.getCornerstoneImage()?.imageId !== imageIds[index] || !viewport.getImageData()?.dimensions) return;
        cleanup(); resolve();
      };
      const canceled = () => { cleanup(); reject(new DOMException('Cancelado', 'AbortError')); };
      const timer = window.setTimeout(() => { cleanup(); reject(new Error('Renderização indisponível.')); }, 10000);
      element.addEventListener(Enums.Events.IMAGE_RENDERED, rendered);
      signal.addEventListener('abort', canceled, { once: true });
      lifetime.addEventListener('abort', canceled, { once: true });
      viewport.render();
    });
    signal.throwIfAborted(); lifetime.throwIfAborted();
    targetIndex = index;
    callbacks.rendered(index, imageIds.length);
  };

  lifetime.addEventListener('abort', () => {
    revision++;
    abortRequests();
    observer.disconnect();
    // Espera o decoder em execução antes de limpar os caches e liberar a próxima tela.
    void queue.then(() => {
      try {
        engine.destroy(); cache.purgeCache(); wadouri.dataSetCacheManager.purge(); metadata.clearCacheData();
      } finally { release(); }
    }).catch(() => { /* descarte sem detalhes clínicos em logs */ });
  }, { once: true });

  return {
    setStack(paths, signal) {
      const token = ++revision;
      currentSignal?.removeEventListener('abort', abortRequests);
      abortRequests();
      currentSignal = signal;
      signal.addEventListener('abort', abortRequests, { once: true });
      busy = true;
      const run = queue.then(async () => {
        signal.throwIfAborted(); lifetime.throwIfAborted();
        if (token !== revision) return;
        const id = '[a-f0-9]{8}(?:-[a-f0-9]{8}){4}';
        if (!paths.every((path) => new RegExp(`^/api/studies/${id}/series/${id}/instances/${id}/dicom$`).test(path))) throw new Error('Caminho inválido.');
        cache.purgeCache(); wadouri.dataSetCacheManager.purge(); metadata.clearCacheData();
        imageIds = paths.map((path) => `wadouri:${path}`);
        targetIndex = 0;
        if (imageIds.length) await render(0, signal, () => viewport.setStack(imageIds, 0));
      });
      queue = run.catch(() => {}).finally(() => { if (token === revision) busy = false; });
      return run;
    },
    step(delta) {
      const signal = currentSignal;
      if (!signal || signal.aborted || lifetime.aborted || busy || !imageIds.length) return;
      const index = Math.min(imageIds.length - 1, Math.max(0, targetIndex + delta));
      if (index === targetIndex) return;
      targetIndex = index;
      busy = true;
      callbacks.loading();
      const token = revision;
      queue = queue.then(() => render(index, signal, () => viewport.setImageIdIndex(index)))
        .catch(() => { if (!signal.aborted && !lifetime.aborted && token === revision) callbacks.failed(); })
        .finally(() => { if (token === revision) busy = false; });
    },
  };
}
