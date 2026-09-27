import { cache, Enums, imageLoader, init, RenderingEngine, utilities, type StackViewport } from '@cornerstonejs/core';
import { init as initLoader, wadouri } from '@cornerstonejs/dicom-image-loader';
import { utilities as metadata } from '@cornerstonejs/metadata';
import { createAnnotationSession, createViewerTools, initializeViewerTools, type ViewerTool } from './tools';

export const VIEWER_SESSION_EXPIRED = 'pacs-viewer-session-expired';
let initialized = false;
const requests = new Map<XMLHttpRequest, string>();
// Sessões sucessivas aguardam o descarte dos decoders da tela anterior.
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
  initializeViewerTools();
  initialized = true;
}

export type StackCallbacks = {
  loading: () => void;
  rendered: (index: number, total: number) => void;
  failed: () => void;
  presentation: (inverted: boolean) => void;
};
export type StackController = {
  setStack: (paths: string[], signal: AbortSignal) => Promise<void>;
  step: (delta: number, loop?: boolean) => Promise<void>;
  selectTool: (tool: ViewerTool) => boolean;
  activate: (active: boolean) => void;
  invert: () => void;
  reset: () => void;
  suspend: () => void;
  clearAnnotations: () => void;
  deleteSelected: () => void;
};

export async function createViewerSession(lifetime: AbortSignal) {
  const preceding = previous;
  let release!: () => void;
  previous = new Promise<void>((resolve) => { release = resolve; });
  await preceding;
  if (lifetime.aborted) { release(); lifetime.throwIfAborted(); }
  let engine: RenderingEngine;
  try { initialize(); engine = new RenderingEngine(`pacs-${crypto.randomUUID()}`); }
  catch (error) { release(); throw error; }
  const annotations = createAnnotationSession(engine.id, () => { if (!engine.hasBeenDestroyed) engine.render(); });
  const pending = new Set<Promise<unknown>>();
  const retiring = new Map<string, Promise<unknown>>();
  // Uma decodificação/download por vez, inclusive miniaturas. Cache é o oficial.
  let pixels: Promise<unknown> = Promise.resolve();
  const load = (imageId: string, signal: AbortSignal) => {
    const run = pixels.then(async () => {
      signal.throwIfAborted(); lifetime.throwIfAborted();
      const cached = cache.getImage(imageId);
      if (cached) return cached;
      const image = await imageLoader.loadImage(imageId, { requestType: Enums.RequestType.Interaction, priority: 0 });
      lifetime.throwIfAborted();
      if (!cache.getImage(imageId)) await cache.putImageLoadObject(imageId, {
        promise: Promise.resolve(image),
        decache: () => wadouri.dataSetCacheManager.unload(imageId.slice('wadouri:'.length)),
      });
      return image;
    });
    pixels = run.catch(() => {});
    return run;
  };
  lifetime.addEventListener('abort', () => {
    for (const xhr of requests.keys()) xhr.abort();
    void Promise.resolve().then(() => Promise.allSettled([pixels, ...pending])).then(() => {
      try { annotations.dispose(); engine.destroy(); cache.purgeCache(); wadouri.dataSetCacheManager.purge(); metadata.clearCacheData(); }
      finally { release(); }
    });
  }, { once: true });
  return {
    engine, annotations, load,
    waitForViewport(id: string) { return retiring.get(id) ?? Promise.resolve(); },
    retire(id: string, promise: Promise<unknown>) { retiring.set(id, promise); },
    track(promise: Promise<unknown>) { pending.add(promise); void promise.finally(() => pending.delete(promise)).catch(() => {}); },
    async thumbnail(path: string, canvas: HTMLCanvasElement, signal: AbortSignal) {
      const image = await load(`wadouri:${path}`, signal);
      signal.throwIfAborted(); lifetime.throwIfAborted();
      await utilities.renderToCanvasCPU(canvas, image);
    },
  };
}
export type ViewerSession = Awaited<ReturnType<typeof createViewerSession>>;

/** Um engine compartilhado, um stack e ToolGroup por viewport. */
export async function createStackViewer(element: HTMLDivElement, lifetime: AbortSignal, callbacks: StackCallbacks, session: ViewerSession, viewportId: string): Promise<StackController> {
  await session.waitForViewport(viewportId);
  lifetime.throwIfAborted();
  const { engine } = session;
  engine.enableElement({ viewportId, element, type: Enums.ViewportType.STACK, defaultOptions: { background: [0, 0, 0] } });
  const viewport = engine.getViewport<StackViewport>(viewportId);
  const tools = createViewerTools(element, viewport, engine.id);
  let active = false;
  let queue: Promise<void> = Promise.resolve();
  let currentSignal: AbortSignal | null = null;
  let imageIds: string[] = [];
  let targetIndex = -1;
  let busy = false;
  let revision = 0;
  const observer = new ResizeObserver(() => { if (!engine.hasBeenDestroyed) engine.resize(true, true); });
  observer.observe(element);

  const resetPresentation = () => {
    viewport.setCamera({ flipHorizontal: false, flipVertical: false });
    viewport.setViewPresentation({ rotation: 0 });
    viewport.resetCamera();
    viewport.resetProperties();
    viewport.render();
    callbacks.presentation(!!viewport.getProperties().invert);
  };

  const render = async (index: number, signal: AbortSignal, load: () => Promise<unknown>, reset = false) => {
    signal.throwIfAborted(); lifetime.throwIfAborted();
    // Aguarda somente a imagem solicitada antes de entregá-la ao viewport.
    // Em 5.11, o caminho GPU pode resolver a troca mesmo após erro do loader.
    // As APIs padrão abaixo permitem capturar a falha antes de mudar os pixels.
    const imageId = imageIds[index];
    if (!imageId) throw new Error('Imagem indisponível.');
    await session.load(imageId, signal);
    signal.throwIfAborted(); lifetime.throwIfAborted();
    await load();
    signal.throwIfAborted(); lifetime.throwIfAborted();
    if (reset) resetPresentation();
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
    tools.suspend(!active);
    callbacks.presentation(!!viewport.getProperties().invert);
    callbacks.rendered(index, imageIds.length);
  };

  lifetime.addEventListener('abort', () => {
    revision++;
    tools.suspend(true);
    observer.disconnect();
    const cleanup = queue.then(() => {
      tools.dispose();
      if (!engine.hasBeenDestroyed) engine.disableElement(viewportId);
    });
    session.retire(viewportId, cleanup);
    session.track(cleanup);
  }, { once: true });

  return {
    setStack(paths, signal) {
      tools.suspend(true);
      const token = ++revision;
      currentSignal = signal;
      busy = true;
      const run = queue.then(async () => {
        signal.throwIfAborted(); lifetime.throwIfAborted();
        if (token !== revision) return;
        const id = '[a-f0-9]{8}(?:-[a-f0-9]{8}){4}';
        if (!paths.every((path) => new RegExp(`^/api/studies/${id}/series/${id}/instances/${id}/dicom$`).test(path))) throw new Error('Caminho inválido.');
        imageIds = paths.map((path) => `wadouri:${path}`);
        targetIndex = 0;
        if (imageIds.length) await render(0, signal, () => viewport.setStack(imageIds, 0), true);
      });
      queue = run.catch(() => {}).finally(() => { if (token === revision) busy = false; });
      session.track(queue);
      return run;
    },
    suspend() { tools.suspend(true); },
    activate(value) { active = value; tools.suspend(!active || busy || !imageIds.length || !!currentSignal?.aborted); },
    clearAnnotations() { if (active && !busy) { tools.cancel(); session.annotations.clear(imageIds); } },
    deleteSelected() { if (active && !busy && !tools.interacting()) session.annotations.deleteSelected(viewport.getCurrentImageId()); },
    selectTool(tool) {
      if (busy || lifetime.aborted || currentSignal?.aborted) return false;
      return tools.select(tool);
    },
    invert() {
      if (busy || lifetime.aborted || currentSignal?.aborted || !imageIds.length || tools.interacting()) return;
      viewport.setProperties({ invert: !viewport.getProperties().invert });
      viewport.render(); callbacks.presentation(!!viewport.getProperties().invert);
    },
    reset() {
      if (busy || lifetime.aborted || currentSignal?.aborted || !imageIds.length || tools.interacting()) return;
      resetPresentation();
    },
    async step(delta, loop = false) {
      const signal = currentSignal;
      if (!signal || signal.aborted || lifetime.aborted || busy || !imageIds.length || tools.interacting()) return;
      const index = loop ? (targetIndex + delta + imageIds.length) % imageIds.length : Math.min(imageIds.length - 1, Math.max(0, targetIndex + delta));
      if (index === targetIndex) return;
      targetIndex = index;
      busy = true;
      tools.suspend(true);
      callbacks.loading();
      const token = revision;
      queue = queue.then(() => render(index, signal, () => viewport.setImageIdIndex(index)))
        .catch(() => { if (!signal.aborted && !lifetime.aborted && token === revision) callbacks.failed(); })
        .finally(() => { if (token === revision) busy = false; });
      session.track(queue);
      await queue;
    },
  };
}
