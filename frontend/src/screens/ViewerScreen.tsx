import { useCallback, useEffect, useRef, useState } from 'react';
import { api, ApiError, type Study, type ViewerSeries } from '../api/client';
import { iniciaisDe, useSession } from '../auth/SessionProvider';
import { Icon } from '../design-system/Icon';
import { initialSeries, loadSeriesStack, navigationDelta } from '../viewer/selection';
import type { ViewerSession } from '../viewer/cornerstone';
import { ViewportPane, type PaneControls, type PaneState } from '../viewer/ViewportPane';
import { SeriesThumbnail } from '../viewer/SeriesThumbnail';
import { fillLayout, type LayoutCount, type SeriesSlot } from '../viewer/layout';
import { ExportDialog } from '../viewer/ExportDialog';
import { ViewerToolbar } from '../viewer/ViewerToolbar';

const DARK = 'color-mix(in srgb, var(--color-neutral-900) 40%, black)';
const HEADER = 'color-mix(in srgb, var(--color-neutral-900) 80%, black)';
const PANEL = 'color-mix(in srgb, var(--color-neutral-900) 60%, black)';
const ACTION = { background: '#20252b', color: '#cbd5e1', border: '1px solid #424a55', padding: '5px 9px', font: 'inherit', cursor: 'pointer' };
const LINE = 'color-mix(in srgb, white 8%, transparent)';

type Props = {
  studyId: string;
  study: Study | null;
  onVoltar: () => void;
  onLogout: () => void;
  onSessionExpired: () => void;
};

/** Mantém header, faixa de ferramentas, painel e viewport do design original.
 * Ferramentas oficiais; sem persistência de medições.
 */
export function ViewerScreen({ studyId, study, onVoltar, onLogout, onSessionExpired }: Props) {
  const { user } = useSession();
  const [series, setSeries] = useState<ViewerSeries[]>([]);
  const [listState, setListState] = useState('loading');
  const [layout, setLayout] = useState<LayoutCount>(1);
  const [active, setActive] = useState(0);
  const activeRef = useRef(0); activeRef.current = active;
  const [assigned, setAssigned] = useState<SeriesSlot[]>([]);
  const [maximized, setMaximized] = useState<number | null>(null);
  const maximizedRef = useRef(maximized); maximizedRef.current = maximized;
  const [states, setStates] = useState<Record<number, PaneState>>({});
  const [exportOpen,setExportOpen]=useState(false);
  const [fps, setFps] = useState(10);
  const controls = useRef<(PaneControls | null)[]>([]);
  const [runtime, setRuntime] = useState<{ session: Promise<ViewerSession>; signal: AbortSignal; load: (id: string) => Promise<string[]>; expired: () => void } | null>(null);
  const [attempt, setAttempt] = useState(0);
  const selected = assigned[active]?.series;
  const state = states[active];
  const register = useRef([0, 1, 2, 3].map((index) => (value: PaneControls | null) => { controls.current[index] = value; }));
  const report = useRef([0, 1, 2, 3].map((index) => (value: PaneState) => setStates((before) => ({ ...before, [index]: value }))));
  const activate = useCallback((index: number) => {
    if (activeRef.current !== index) controls.current[activeRef.current]?.pause();
    activeRef.current = index; setActive(index);
  }, []);

  useEffect(() => {
    const lifetime = new AbortController();
    const expired = () => { if (!lifetime.signal.aborted) { lifetime.abort(); onSessionExpired(); } };
    const session = import('../viewer/cornerstone').then(({ createViewerSession }) => createViewerSession(lifetime.signal));
    void session.catch(() => {});
    // Compartilha apenas metadados/futuros; pixels ficam no cache oficial.
    const metadata = new Map<string, Promise<string[]>>();
    const load = (id: string) => {
      if (!metadata.has(id)) {
        const request = loadSeriesStack(studyId, id, lifetime.signal);
        metadata.set(id, request);
        void request.catch(() => metadata.delete(id));
      }
      return metadata.get(id)!;
    };
    setRuntime({ session, signal: lifetime.signal, load, expired });
    setListState('loading');
    void api.getViewerSeries(studyId, lifetime.signal).then(({ items }) => {
      if (lifetime.signal.aborted) return;
      setSeries(items); setAssigned(fillLayout([{ series: initialSeries(items), manual: false }], items, 1, 1)); setListState(items.length ? 'ready' : 'empty');
    }).catch((error) => {
      if (lifetime.signal.aborted) return;
      if (error instanceof ApiError && error.status === 401) expired(); else setListState('error');
    });
    const key = (event: KeyboardEvent) => {
      if (lifetime.signal.aborted || document.querySelector('dialog[open]')) return;
      const delta = navigationDelta(event);
      if (delta) { event.preventDefault(); controls.current[activeRef.current]?.step(delta); return; }
      const target = event.target;
      if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey ||
        (target instanceof HTMLElement && (target.isContentEditable || target.closest('input, textarea, select, [role="textbox"]')))) return;
      if (event.key === 'Escape' && maximizedRef.current !== null) { event.preventDefault(); controls.current.forEach((pane) => pane?.prepareResize()); setMaximized(null); }
      if (!event.repeat && event.key.toLowerCase() === 'r') { event.preventDefault(); controls.current[activeRef.current]?.rotate(90); }
      if (!event.repeat && event.key.toLowerCase() === 'f') { event.preventDefault(); controls.current[activeRef.current]?.fit(); }
      if (event.key === 'Delete' || event.key === 'Backspace') { event.preventDefault(); controls.current[activeRef.current]?.deleteSelected(); }
    };
    window.addEventListener('keydown', key, true);
    window.addEventListener('pacs-viewer-session-expired', expired);
    return () => {
      lifetime.abort(); metadata.clear();
      window.removeEventListener('keydown', key, true);
      window.removeEventListener('pacs-viewer-session-expired', expired);
    };
  }, [studyId, onSessionExpired, attempt]);

  return (
    <div style={{ position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column', background: DARK, color: 'var(--color-neutral-300)' }}>
      <header style={{ height: 46, flex: 'none', display: 'flex', alignItems: 'center', gap: 16, padding: '0 12px 0 8px', borderBottom: `1px solid ${LINE}`, background: HEADER }}>
        <button type="button" className="pacs-hover-white" onClick={onVoltar}
          style={{ display: 'flex', alignItems: 'center', gap: 6, height: 32, padding: '0 12px 0 8px', border: 0, background: 'transparent', color: 'var(--color-neutral-200)', font: 'inherit', fontSize: 14, fontWeight: 600, cursor: 'pointer' }}>
          <Icon name="arrowLeft" /> Exames
        </button>
        <div style={{ width: 1, height: 20, background: LINE }} />
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 14, fontSize: 13.5, minWidth: 0, whiteSpace: 'nowrap', overflow: 'hidden' }}>
          <span style={{ fontWeight: 600, color: 'var(--color-neutral-100)' }}>{study?.patientName.replaceAll('^', ' ') || 'Estudo DICOM'}</span>
          <span>{study?.patientId}</span>
          <span>{study?.studyDescription}</span>
          <span style={{ color: 'var(--color-neutral-500)' }}>{study?.institutionName}</span>
        </div>
        <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 10, flex: 'none' }}>
          <span style={{ background: 'var(--color-accent-800)', color: 'var(--color-accent-100)', padding: 5, fontSize: 12 }}>{iniciaisDe(user?.name ?? '')}</span>
          <span style={{ fontSize: 13.5 }}>{user?.name}</span>
          <button type="button" className="pacs-hover-white" onClick={onLogout}
            style={{ display: 'flex', alignItems: 'center', gap: 6, height: 30, padding: '0 12px', border: `1px solid ${LINE}`, background: 'transparent', color: 'var(--color-neutral-200)', font: 'inherit', fontSize: 13, cursor: 'pointer' }}>
            <Icon name="logout" /> Sair
          </button>
        </div>
      </header>
      <ViewerToolbar selected={state?.tool ?? 'Pan'} inverted={state?.inverted ?? false} flipHorizontal={state?.flipHorizontal ?? false} flipVertical={state?.flipVertical ?? false} disabled={!state?.ready}
        onSelect={(value) => controls.current[active]?.selectTool(value)}
        onRotate={(delta) => controls.current[active]?.rotate(delta)} onFlip={(axis) => controls.current[active]?.flip(axis)} onFit={() => controls.current[active]?.fit()}
        onInvert={() => controls.current[active]?.invert()} onReset={() => controls.current[active]?.reset()} />
      <div role="toolbar" aria-label="Layout e reprodução" style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '5px 12px', background: HEADER, fontSize: 12 }}>
        {([1, 2, 4] as const).map((count) => <button type="button" style={{ ...ACTION, borderColor: layout === count ? 'var(--color-accent-400)' : '#424a55' }} key={count} disabled={listState !== 'ready'} aria-pressed={layout === count} onClick={() => {
          controls.current.forEach((pane) => pane?.prepareResize());
          for (let i = count; i < 4; i++) controls.current[i]?.pause();
          if (active >= count) activate(0);
          setMaximized(null);
          setAssigned((before) => fillLayout(before, series, count, layout));
          setLayout(count);
        }}>{count === 1 ? '1x1' : count === 2 ? '1x2' : '2x2'}</button>)}
        <span>Viewport {active + 1}</span><button type="button" style={ACTION} disabled={!state?.ready} onClick={()=>setExportOpen(true)}>Exportar</button>
        <button type="button" style={ACTION} disabled={!state?.playing && (!state?.ready || state.total < 2)} onClick={() => state?.playing ? controls.current[active]?.pause() : controls.current[active]?.play(fps)}>{state?.playing ? 'Pause' : 'Play'}</button>
        <label>FPS <input aria-label="FPS" type="number" min={1} max={30} value={fps} onChange={(event) => { controls.current[active]?.pause(); setFps(Math.min(30, Math.max(1, Number(event.target.value) || 10))); }} style={{ ...ACTION, width: 42, padding: 4 }} /></label>
        <button type="button" style={ACTION} disabled={!state?.ready} onClick={() => controls.current[active]?.clear()}>Limpar medições</button>
      </div>
      {exportOpen && runtime && <ExportDialog viewport={active} signal={runtime.signal} expired={runtime.expired} close={()=>setExportOpen(false)} capture={()=>{const pane=controls.current[active];if(!pane)throw new Error('Imagem indisponível.');return pane.capture();}}/>}
      <div style={{ flex: 1, minHeight: 0, display: 'flex' }}>
        <aside style={{ width: 160, flex: 'none', overflowY: 'auto', padding: 16, borderRight: `1px solid ${LINE}`, background: PANEL }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, letterSpacing: '.1em', textTransform: 'uppercase', marginBottom: 20 }}><span>Séries</span><span>{series.length}</span></div>
          {series.map((item) => (
            <button type="button" aria-pressed={item.orthancSeriesId === selected?.orthancSeriesId} key={item.orthancSeriesId} onClick={() => { controls.current[active]?.pause(); setAssigned((before) => before.map((value, index) => index === active ? { series: item, manual: true } : value)); }} style={{ display: 'block', width: '100%', textAlign: 'left', font: 'inherit', cursor: 'pointer', background: item.orthancSeriesId === selected?.orthancSeriesId ? 'rgba(255,255,255,.06)' : 'transparent', border: 0, padding: '12px 0', borderBottom: `1px solid ${LINE}`, color: item.orthancSeriesId === selected?.orthancSeriesId ? 'var(--color-accent-300)' : 'var(--color-neutral-500)', fontSize: 12, overflowWrap: 'anywhere' }}>
              {runtime && <SeriesThumbnail seriesId={item.orthancSeriesId} count={item.instanceCount} {...runtime} />}
              <div>{item.description || 'Série sem descrição'}</div>
              <div style={{ marginTop: 6 }}>S{item.number || '—'} · {item.modality || '—'} · {item.instanceCount} imagem(ns)</div>
            </button>
          ))}
        </aside>
        <main tabIndex={0} style={{ flex: 1, minWidth: 0, minHeight: 0, display: 'grid', gridTemplateColumns: layout === 1 ? '1fr' : '1fr 1fr', gridTemplateRows: layout === 4 ? '1fr 1fr' : '1fr', background: 'black' }} aria-label="Imagens DICOM">
          {listState !== 'ready' ? <div role={listState === 'error' ? 'alert' : 'status'} style={{ padding: 32 }}>
            {listState === 'loading' ? 'Carregando séries…' : listState === 'empty' ? 'Estudo sem séries' : 'Não foi possível consultar as séries.'}
            {listState === 'error' && <button type="button" onClick={() => setAttempt((value) => value + 1)}>Tentar novamente</button>}
          </div> : runtime && Array.from({ length: layout }, (_, index) => <ViewportPane key={`${attempt}-${index}`} id={`image-${index}`} active={index === active} series={assigned[index]?.series ?? null}
            maximized={maximized === index} hidden={maximized !== null && maximized !== index}
            row={Math.floor(index / (layout === 1 ? 1 : 2)) + 1} column={index % (layout === 1 ? 1 : 2) + 1}
            maximize={() => { controls.current.forEach((pane) => pane?.prepareResize()); activate(index); setMaximized((before) => before === index ? null : index); }}
            {...runtime} activate={() => activate(index)} register={register.current[index]!} report={report.current[index]!} />)}
        </main>
      </div>
    </div>
  );
}
