import { cache, Enums, init, RenderingEngine, utilities, type StackViewport } from '@cornerstonejs/core';
import { init as initLoader } from '@cornerstonejs/dicom-image-loader';
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

export function renderInitialImage(element: HTMLDivElement, path: string, signal: AbortSignal): Promise<void> {
  const run = async () => {
    signal.throwIfAborted();
    const id = '[a-f0-9]{8}(?:-[a-f0-9]{8}){4}';
    if (!new RegExp(`^/api/studies/${id}/series/${id}/instances/${id}/dicom$`).test(path)) throw new Error('Caminho de imagem inválido.');
    initialize();
    const imageId = `wadouri:${path}`;
    const engine = new RenderingEngine(`pacs-${crypto.randomUUID()}`);
    let observer: ResizeObserver | undefined;
    const abort = () => {
      for (const [xhr, url] of requests) if (url === path) xhr.abort();
      if (!engine.hasBeenDestroyed) engine.destroy();
    };
    signal.addEventListener('abort', abort, { once: true });
    try {
      engine.enableElement({ viewportId: 'image', element, type: Enums.ViewportType.STACK, defaultOptions: { background: [0, 0, 0] } });
      const viewport = engine.getViewport<StackViewport>('image');
      await viewport.setStack([imageId], 0);
      signal.throwIfAborted();
      await new Promise<void>((resolve, reject) => {
        const cleanup = () => {
          window.clearTimeout(timer);
          element.removeEventListener(Enums.Events.IMAGE_RENDERED, rendered);
          signal.removeEventListener('abort', canceled);
        };
        const rendered = () => {
          if (!viewport.getImageData()?.dimensions) return;
          cleanup(); resolve();
        };
        const canceled = () => { cleanup(); reject(new DOMException('Cancelado', 'AbortError')); };
        const timer = window.setTimeout(() => { cleanup(); reject(new Error('Renderização indisponível.')); }, 10000);
        element.addEventListener(Enums.Events.IMAGE_RENDERED, rendered);
        signal.addEventListener('abort', canceled, { once: true });
        viewport.render();
      });
      signal.throwIfAborted();
      observer = new ResizeObserver(() => { if (!engine.hasBeenDestroyed) engine.resize(true, true); });
      observer.observe(element);
      // O chamador recebe a confirmação de carga via evento local e mantém a
      // operação viva até desmontar, sair ou trocar de estudo.
      element.dispatchEvent(new Event('pacs-image-ready'));
      await new Promise<void>((resolve) => {
        if (signal.aborted) resolve();
        else signal.addEventListener('abort', () => resolve(), { once: true });
      });
    } finally {
      observer?.disconnect();
      signal.removeEventListener('abort', abort);
      abort();
      cache.purgeCache();
      metadata.clearCacheData();
    }
  };
  const result = previous.then(run);
  previous = result.catch(() => { /* erros são apresentados pela tela, sem log bruto */ });
  return result;
}
