import { cache, Enums, imageLoader, init, RenderingEngine, utilities, type StackViewport } from '@cornerstonejs/core';
import { init as initLoader, wadouri } from '@cornerstonejs/dicom-image-loader';
import { utilities as metadata } from '@cornerstonejs/metadata';
import { createAnnotationSession, createViewerTools, initializeViewerTools, type ViewerTool } from './tools';

import type { ExportSnapshot, ExportMetadata } from './exportModel';
import { fitToWindow, readPresentation, type PresentationState } from './presentation';

type NaturalizedImage = Partial<Record<'PatientName'|'PatientID'|'StudyDate'|'Modality'|'StudyDescription'|'InstitutionName'|'SeriesNumber'|'InstanceNumber'|'BurnedInAnnotation', unknown>>;
function naturalValue(value:unknown):string {
 if(Array.isArray(value)) return naturalValue(value[0]);
 if(value && typeof value==='object') {
  const person=value as {Alphabetic?:unknown;Ideographic?:unknown;Phonetic?:unknown;Value?:unknown};
  return naturalValue(person.Alphabetic ?? person.Ideographic ?? person.Phonetic ?? person.Value);
 }
 return (typeof value==='string'||typeof value==='number'?String(value):'').replace(/[\u0000-\u001f\u007f]/g,' ').trim().slice(0,512);
}
function exportTags(natural: NaturalizedImage | undefined): Omit<ExportMetadata,'index'|'total'> {
 const value=(tag:keyof NaturalizedImage)=>naturalValue(natural?.[tag]);
 return {patientName:value('PatientName'),patientId:value('PatientID'),studyDate:value('StudyDate'),modality:value('Modality'),studyDescription:value('StudyDescription'),institution:value('InstitutionName'),seriesNumber:value('SeriesNumber'),instanceNumber:value('InstanceNumber'),burnedIn:value('BurnedInAnnotation')};
}

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
  presentation: (value: PresentationState) => void;
};
export type StackController = {
  capture: () => ExportSnapshot;
  setStack: (paths: string[], signal: AbortSignal) => Promise<void>;
  step: (delta: number, loop?: boolean) => Promise<void>;
  selectTool: (tool: ViewerTool) => ViewerTool | null;
  activate: (active: boolean) => void;
  invert: () => void;
  reset: () => void;
  rotate: (delta: number) => void;
  flip: (axis: 'horizontal' | 'vertical') => void;
  fit: () => void;
  suspend: () => void;
  prepareResize: () => void;
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
  let resizeFrame = 0;
  const resizePresentations = new Map<StackViewport, { imageId: string; presentation: ReturnType<StackViewport['getViewPresentation']> }>();
  const prepareResize = (viewport: StackViewport) => {
    if (resizePresentations.has(viewport) || lifetime.aborted) return;
    const imageId = viewport.getCurrentImageId();
    if (imageId && viewport.getCornerstoneImage()?.imageId === imageId) {
      resizePresentations.set(viewport, { imageId, presentation: viewport.getViewPresentation() });
    }
  };
  const resize = () => {
    if (resizeFrame || lifetime.aborted) return;
    resizeFrame = requestAnimationFrame(() => {
      resizeFrame = 0;
      if (lifetime.aborted || engine.hasBeenDestroyed) return;
      const viewports = engine.getViewports();
      engine.resize(false, true);
      // Captura feita ANTES de React mudar as dimensões: getPan depende delas.
      // Não aplica estado antigo se o slot foi removido ou a imagem já mudou.
      for (const [viewport, saved] of resizePresentations) {
        if (!viewports.includes(viewport) || viewport.getCurrentImageId() !== saved.imageId) continue;
        viewport.resetCamera();
        viewport.setViewPresentation(saved.presentation);
      }
      resizePresentations.clear();
      engine.render();
    });
  };
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
    cancelAnimationFrame(resizeFrame);
    resizePresentations.clear();
    for (const xhr of requests.keys()) xhr.abort();
    void Promise.resolve().then(() => Promise.allSettled([pixels, ...pending])).then(() => {
      try { annotations.dispose(); engine.destroy(); cache.purgeCache(); wadouri.dataSetCacheManager.purge(); metadata.clearCacheData(); }
      finally { release(); }
    });
  }, { once: true });
  return {
    engine, annotations, load, resize, prepareResize,
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
  let presented = false;
  let revision = 0;
  const observer = new ResizeObserver(session.resize);
  observer.observe(element);

  const resetPresentation = () => {
    viewport.setCamera({ flipHorizontal: false, flipVertical: false });
    viewport.setViewPresentation({ rotation: 0 });
    viewport.resetCamera();
    viewport.resetProperties();
    // Preferência PACS: apresentação inicial/Reset sem inversão, inclusive MONOCHROME1.
    viewport.setProperties({ invert: false });
    viewport.render();
    callbacks.presentation(readPresentation(viewport));
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
    presented = true;
    tools.suspend(!active);
    callbacks.presentation(readPresentation(viewport));
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
    capture() {
      if (!active || !presented || busy || lifetime.aborted || currentSignal?.aborted || tools.interacting()) throw new Error('Aguarde o carregamento da imagem.');
      const imageId=viewport.getCurrentImageId();
      if(!imageId || viewport.getCornerstoneImage()?.imageId!==imageId)throw new Error('Imagem indisponível.');
      const source=viewport.getCanvas();
      if(!source?.width || !source.height || source.width*source.height>32*1024*1024)throw new Error('Canvas indisponível.');
      const canvas=document.createElement('canvas');canvas.width=source.width;canvas.height=source.height;
      const ctx=canvas.getContext('2d');if(!ctx)throw new Error('Canvas indisponível.');
      ctx.drawImage(source,0,0);
      return {canvas,metadata:{...exportTags((cache.getImage(imageId) as {data?: NaturalizedImage} | undefined)?.data ?? (viewport.getCornerstoneImage() as {data?: NaturalizedImage} | undefined)?.data),index:viewport.getCurrentImageIdIndex(),total:imageIds.length},dispose(){canvas.width=0;canvas.height=0;}};
    },
    setStack(paths, signal) {
      presented = false;
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
    prepareResize() { session.prepareResize(viewport); },
    suspend() { presented = false; tools.suspend(true); },
    activate(value) { active = value; tools.suspend(!active || busy || !imageIds.length || !!currentSignal?.aborted); },
    clearAnnotations() { if (active && !busy) { tools.cancel(); session.annotations.clear(imageIds); } },
    deleteSelected() { if (active && !busy && !tools.interacting()) session.annotations.deleteSelected(viewport.getCurrentImageId()); },
    selectTool(tool) {
      if (busy || lifetime.aborted || currentSignal?.aborted) return null;
      return tools.select(tool);
    },
    invert() {
      if (busy || lifetime.aborted || currentSignal?.aborted || !imageIds.length || tools.interacting()) return;
      viewport.setProperties({ invert: !viewport.getProperties().invert });
      viewport.render(); callbacks.presentation(readPresentation(viewport));
    },
    reset() {
      if (busy || lifetime.aborted || currentSignal?.aborted || !imageIds.length || tools.interacting()) return;
      resetPresentation();
    },
    rotate(delta) {
      if (!active || !presented || busy || lifetime.aborted || currentSignal?.aborted || !imageIds.length || tools.interacting()) return;
      viewport.setViewPresentation({ rotation: (Math.round(viewport.getRotation()) + delta + 360) % 360 });
      viewport.render(); callbacks.presentation(readPresentation(viewport));
    },
    flip(axis) {
      if (!active || !presented || busy || lifetime.aborted || currentSignal?.aborted || !imageIds.length || tools.interacting()) return;
      const camera = viewport.getCamera();
      viewport.setCamera(axis === 'horizontal' ? { flipHorizontal: !camera.flipHorizontal } : { flipVertical: !camera.flipVertical });
      viewport.render(); callbacks.presentation(readPresentation(viewport));
    },
    fit() {
      if (!active || !presented || busy || lifetime.aborted || currentSignal?.aborted || !imageIds.length || tools.interacting()) return;
      fitToWindow(viewport); viewport.render(); callbacks.presentation(readPresentation(viewport));
    },
    async step(delta, loop = false) {
      const signal = currentSignal;
      if (!signal || signal.aborted || lifetime.aborted || busy || !imageIds.length || tools.interacting()) return;
      const index = loop ? (targetIndex + delta + imageIds.length) % imageIds.length : Math.min(imageIds.length - 1, Math.max(0, targetIndex + delta));
      if (index === targetIndex) return;
      targetIndex = index;
      presented = false;
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
