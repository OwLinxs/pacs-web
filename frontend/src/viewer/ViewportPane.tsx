import { useEffect, useRef, useState } from 'react';
import { ApiError, type ViewerSeries } from '../api/client';
import type { StackController, ViewerSession } from './cornerstone';
import { createCine } from './cine';
import type { ViewerTool } from './tools';

export type PaneState = { ready: boolean; tool: ViewerTool; inverted: boolean; playing: boolean; total: number };
export type PaneControls = {
  selectTool: (tool: ViewerTool) => void; invert: () => void; reset: () => void;
  step: (delta: number) => void; play: (fps: number) => void; pause: () => void;
  clear: () => void; deleteSelected: () => void;
};
type Props = {
  id: string; active: boolean; series: ViewerSeries | null;
  session: Promise<ViewerSession>; signal: AbortSignal;
  load: (seriesId: string) => Promise<string[]>;
  activate: () => void; expired: () => void;
  register: (controls: PaneControls | null) => void;
  report: (state: PaneState) => void;
};

export function ViewportPane({ id, active, series, session, signal, load, activate, expired, register, report }: Props) {
  const element = useRef<HTMLDivElement>(null);
  const driver = useRef<StackController | null>(null);
  const factory = useRef<Promise<StackController> | null>(null);
  const activeRef = useRef(active); activeRef.current = active;
  const [state, setState] = useState<'empty' | 'loading' | 'ready' | 'error'>('empty');
  const [position, setPosition] = useState({ index: 0, total: 0 });
  const [tool, setTool] = useState<ViewerTool>('WindowLevel');
  const [inverted, setInverted] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [cineFPS, setCineFPS] = useState(10);
  const [attempt, setAttempt] = useState(0);
  const cine = useRef<ReturnType<typeof createCine> | null>(null);

  useEffect(() => {
    const lifetime = new AbortController();
    const abort = () => { lifetime.abort(); cine.current?.stop(); };
    signal.addEventListener('abort', abort, { once: true });
    if (signal.aborted) abort();
    const playback = createCine(async () => { await driver.current?.step(1, true); }, (value) => { if (!lifetime.signal.aborted) setPlaying(value); });
    cine.current = playback;
    factory.current = session.then(async (shared) => {
      lifetime.signal.throwIfAborted();
      const { createStackViewer } = await import('./cornerstone');
      const controller = await createStackViewer(element.current!, lifetime.signal, {
        loading: () => { if (!lifetime.signal.aborted) setState('loading'); },
        rendered: (index, total) => { if (!lifetime.signal.aborted) { setPosition({ index, total }); setState('ready'); } },
        presentation: (value) => { if (!lifetime.signal.aborted) setInverted(value); },
        failed: () => { playback.stop(); if (!lifetime.signal.aborted) setState('error'); },
      }, shared, id);
      driver.current = controller; controller.activate(activeRef.current);
      return controller;
    });
    void factory.current.catch(() => { if (!lifetime.signal.aborted) setState('error'); });
    register({
      selectTool(value) { if (driver.current?.selectTool(value)) setTool(value); },
      invert: () => driver.current?.invert(), reset: () => driver.current?.reset(),
      step(delta) { playback.stop(); void driver.current?.step(delta); },
      play: (fps) => { setCineFPS(fps); playback.play(fps); }, pause: playback.stop,
      clear: () => { playback.stop(); driver.current?.clearAnnotations(); }, deleteSelected: () => { playback.stop(); driver.current?.deleteSelected(); },
    });
    return () => {
      abort(); signal.removeEventListener('abort', abort); register(null);
      driver.current = null; factory.current = null;
    };
  }, [id, session, signal, register]);

  useEffect(() => {
    driver.current?.activate(active);
    if (!active) cine.current?.stop();
  }, [active]);

  useEffect(() => {
    const selection = new AbortController();
    cine.current?.stop(); driver.current?.suspend();
    setPosition({ index: 0, total: 0 }); setState(series ? 'loading' : 'empty');
    if (series) void (async () => {
      try {
        const paths = await load(series.orthancSeriesId);
        if (selection.signal.aborted || signal.aborted) return;
        const controller = await factory.current;
        if (!controller || selection.signal.aborted || signal.aborted) return;
        await controller.setStack(paths, selection.signal);
        if (!paths.length && !selection.signal.aborted && !signal.aborted) setState('empty');
      } catch (error) {
        if (selection.signal.aborted || signal.aborted) return;
        cine.current?.stop();
        if (error instanceof ApiError && error.status === 401) expired();
        else setState('error');
      }
    })();
    return () => selection.abort();
  }, [series, load, signal, expired, attempt]);

  useEffect(() => { report({ ready: state === 'ready', tool, inverted, playing, total: position.total }); }, [state, tool, inverted, playing, position.total, report]);
  useEffect(() => {
    const area = element.current?.parentElement;
    if (!area) return;
    const wheel = (event: WheelEvent) => {
      if (!activeRef.current || event.ctrlKey || event.altKey || event.metaKey || event.shiftKey || !event.deltaY) return;
      event.preventDefault(); cine.current?.stop(); void driver.current?.step(Math.sign(event.deltaY));
    };
    area.addEventListener('wheel', wheel, { passive: false });
    return () => area.removeEventListener('wheel', wheel);
  }, []);
  return <section data-viewport={id} data-active={active} tabIndex={0} aria-label={`Viewport ${Number(id.slice(-1)) + 1}`} onPointerDownCapture={(event) => { if (!active) event.preventDefault(); activate(); event.currentTarget.focus({ preventScroll: true }); }}
    style={{ position: 'relative', minWidth: 0, minHeight: 0, background: 'black', outline: 'none', border: `1px solid ${active ? 'var(--color-accent-400)' : '#252525'}` }}>
    <div ref={element} style={{ position: 'absolute', inset: 0 }} onContextMenu={(event) => event.preventDefault()} />
    {state !== 'ready' && <div role={state === 'error' ? 'alert' : 'status'} style={{ position: 'absolute', inset: 0, background: 'black', display: 'grid', placeContent: 'center', textAlign: 'center', padding: 20, gap: 12 }}>
      <span>{state === 'loading' ? 'Carregando imagem…' : state === 'error' ? 'Não foi possível carregar esta série ou imagem.' : series ? 'Série sem imagens' : 'Selecione uma série'}</span>
      {state === 'error' && <button type="button" onClick={() => setAttempt((value) => value + 1)}>Tentar novamente</button>}
    </div>}
    {state === 'ready' && <>
      <div style={{ position: 'absolute', left: 12, top: 10, fontSize: 12, pointerEvents: 'none', maxWidth: '75%', textShadow: '0 1px 2px black' }}>
        <div>{series?.modality || '—'} · Série {series?.number || '—'}</div><div>{series?.description || 'Série sem descrição'}</div>
      </div>
      <span role="status" style={{ position: 'absolute', right: 12, bottom: 10, fontSize: 12, pointerEvents: 'none', textShadow: '0 1px 2px black' }}>Imagem {position.index + 1} / {position.total}{playing ? ` · Cine ${cineFPS} FPS` : ''}</span>
    </>}
  </section>;
}
