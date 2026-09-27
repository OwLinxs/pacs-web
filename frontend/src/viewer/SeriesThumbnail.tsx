import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../api/client';
import type { ViewerSession } from './cornerstone';

/** Apenas cards visíveis; uma instância por card e fila de pixels compartilhada. */
export function SeriesThumbnail({ seriesId, count, session, load, signal, expired }: {
  seriesId: string; count: number; session: Promise<ViewerSession>;
  load: (id: string) => Promise<string[]>; signal: AbortSignal; expired: () => void;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [state, setState] = useState<'placeholder' | 'loading' | 'ready' | 'error'>('placeholder');
  useEffect(() => {
    const target = canvas.current!;
    let current: AbortController | null = null;
    let complete = false;
    const observer = new IntersectionObserver((entries) => {
      if (!entries[0]?.isIntersecting) { current?.abort(); current = null; return; }
      if (complete || current || !count || signal.aborted) return;
      const request = new AbortController(); current = request;
      setState('loading');
      void (async () => {
        try {
          const paths = await load(seriesId);
          if (request.signal.aborted || signal.aborted) return;
          if (!paths[0]) throw new Error('Miniatura indisponível.');
          const shared = await session;
          if (request.signal.aborted || signal.aborted) return;
          await shared.thumbnail(paths[0], target, request.signal);
          if (!request.signal.aborted && !signal.aborted) { complete = true; setState('ready'); }
        } catch (error) {
          if (request.signal.aborted || signal.aborted) return;
          if (error instanceof ApiError && error.status === 401) expired();
          else { complete = true; setState('error'); }
        }
      })();
    }, { root: target.closest('aside'), threshold: 0.1 });
    observer.observe(target);
    return () => { current?.abort(); observer.disconnect(); };
  }, [seriesId, count, session, load, signal, expired]);
  return <div data-thumbnail={state} style={{ position: 'relative', height: 80, background: '#080808', marginBottom: 8 }}>
    <canvas ref={canvas} width={112} height={80} aria-label="Miniatura da série" style={{ width: '100%', height: '100%', objectFit: 'contain', opacity: state === 'ready' ? 1 : 0 }} />
    {state !== 'ready' && <span style={{ position: 'absolute', inset: 0, display: 'grid', placeContent: 'center', fontSize: 11, color: '#777' }}>{state === 'loading' ? 'Carregando…' : 'Sem miniatura'}</span>}
  </div>;
}
