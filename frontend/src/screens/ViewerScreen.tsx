import { useEffect, useRef, useState } from 'react';
import { api, ApiError, type Study, type ViewerSeries } from '../api/client';
import { iniciaisDe, useSession } from '../auth/SessionProvider';
import { Icon } from '../design-system/Icon';
import { initialSeries, loadSeriesStack, navigationDelta } from '../viewer/selection';
import type { StackController } from '../viewer/cornerstone';
import type { ViewerTool } from '../viewer/tools';
import { ViewerToolbar } from '../viewer/ViewerToolbar';

const DARK = 'color-mix(in srgb, var(--color-neutral-900) 40%, black)';
const HEADER = 'color-mix(in srgb, var(--color-neutral-900) 80%, black)';
const PANEL = 'color-mix(in srgb, var(--color-neutral-900) 60%, black)';
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
  const viewport = useRef<HTMLDivElement>(null);
  const { user } = useSession();
  const [state, setState] = useState<'loading-series' | 'loading' | 'image-loading' | 'ready' | 'empty-study' | 'empty' | 'error'>('loading-series');
  const [message, setMessage] = useState('');
  const [series, setSeries] = useState<ViewerSeries[]>([]);
  const [selected, setSelected] = useState<ViewerSeries | null>(null);
  const [position, setPosition] = useState({ index: 0, total: 0 });
  const select = useRef<(item: ViewerSeries) => void>(() => {});
  const retry = useRef<() => void>(() => {});
  const controls = useRef<StackController | null>(null);
  const [tool, setTool] = useState<ViewerTool>('WindowLevel');
  const [inverted, setInverted] = useState(false);

  useEffect(() => {
    const element = viewport.current;
    const area = element?.parentElement;
    if (!element || !area) return;
    const lifetime = new AbortController();
    let selection: AbortController | null = null;
    let driver: StackController | null = null;
    let active: ViewerSeries | null = null;
    let viewer: Promise<StackController> | null = null;
    setState('loading-series'); setSeries([]); setSelected(null); setPosition({ index: 0, total: 0 });
    const expired = () => { lifetime.abort(); selection?.abort(); onSessionExpired(); };
    const fail = (error: unknown, fallback: string) => {
      if (error instanceof ApiError && error.status === 401) { expired(); return; }
      setMessage(error instanceof ApiError ? error.message : fallback);
      setState('error');
    };
    const load = async (item: ViewerSeries) => {
      selection?.abort();
      driver?.suspend();
      const current = new AbortController(); selection = current;
      active = item; setSelected(item); setPosition({ index: 0, total: 0 }); setState('loading');
      area.focus({ preventScroll: true });
      try {
        const paths = await loadSeriesStack(studyId, item.orthancSeriesId, current.signal);
        if (current.signal.aborted || lifetime.signal.aborted) return;
        if (!viewer) {
          viewer = import('../viewer/cornerstone').then(({ createStackViewer }) => createStackViewer(element, lifetime.signal, {
            loading: () => setState('image-loading'),
            presentation: (value) => { if (!lifetime.signal.aborted) setInverted(value); },
            rendered: (index, total) => { setPosition({ index, total }); setState('ready'); },
            failed: () => fail(null, 'Não foi possível carregar a imagem. Selecione outra imagem ou série, ou tente novamente.'),
          }));
          // Uma falha de inicialização permite nova tentativa sem recarregar a tela.
          void viewer.catch(() => { viewer = null; });
        }
        driver = await viewer;
        controls.current = driver;
        if (current.signal.aborted || lifetime.signal.aborted) return;
        await driver.setStack(paths, current.signal);
        if (!current.signal.aborted && !lifetime.signal.aborted && !paths.length) setState('empty');
      } catch (error: unknown) {
        if (!current.signal.aborted && !lifetime.signal.aborted) fail(error, 'Não foi possível carregar esta série ou imagem. Tente novamente ou selecione outra série.');
      }
    };
    const list = async () => {
      setState('loading-series');
      try {
        const { items } = await api.getViewerSeries(studyId, lifetime.signal);
        if (lifetime.signal.aborted) return;
        setSeries(items);
        const first = initialSeries(items);
        if (first) await load(first);
        else setState('empty-study');
      } catch (error: unknown) {
        if (!lifetime.signal.aborted) fail(error, 'Não foi possível consultar as séries. Tente novamente.');
      }
    };
    select.current = (item) => { if (active?.orthancSeriesId !== item.orthancSeriesId) void load(item); };
    retry.current = () => { if (active) void load(active); else void list(); };
    const wheel = (event: WheelEvent) => {
      if (event.ctrlKey || event.altKey || event.metaKey || event.shiftKey || event.deltaY === 0) return;
      event.preventDefault(); driver?.step(Math.sign(event.deltaY));
    };
    const key = (event: KeyboardEvent) => {
      const delta = navigationDelta(event);
      if (delta) { event.preventDefault(); driver?.step(delta); }
    };
    area.addEventListener('wheel', wheel, { passive: false });
    // Capture enquanto esta tela está montada: foco pode estar no canvas,
    // toolbar, painel ou body. Não depende do foco exato no <main>.
    window.addEventListener('keydown', key, true);
    window.addEventListener('pacs-viewer-session-expired', expired);
    void list();
    return () => {
      lifetime.abort(); selection?.abort();
      select.current = () => {}; retry.current = () => {};
      area.removeEventListener('wheel', wheel); window.removeEventListener('keydown', key, true);
      controls.current = null;
      window.removeEventListener('pacs-viewer-session-expired', expired);
    };
  }, [studyId, onSessionExpired]);

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
      <ViewerToolbar selected={tool} inverted={inverted} disabled={state !== 'ready'}
        onSelect={(value) => { if (controls.current?.selectTool(value)) setTool(value); }}
        onInvert={() => controls.current?.invert()} onReset={() => controls.current?.reset()} />
      <div style={{ flex: 1, minHeight: 0, display: 'flex' }}>
        <aside style={{ width: 160, flex: 'none', overflowY: 'auto', padding: 16, borderRight: `1px solid ${LINE}`, background: PANEL }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, letterSpacing: '.1em', textTransform: 'uppercase', marginBottom: 20 }}><span>Séries</span><span>{series.length}</span></div>
          {series.map((item) => (
            <button type="button" aria-pressed={item.orthancSeriesId === selected?.orthancSeriesId} key={item.orthancSeriesId} onClick={() => select.current(item)} style={{ display: 'block', width: '100%', textAlign: 'left', font: 'inherit', cursor: 'pointer', background: item.orthancSeriesId === selected?.orthancSeriesId ? 'rgba(255,255,255,.06)' : 'transparent', border: 0, padding: '12px 0', borderBottom: `1px solid ${LINE}`, color: item.orthancSeriesId === selected?.orthancSeriesId ? 'var(--color-accent-300)' : 'var(--color-neutral-500)', fontSize: 12, overflowWrap: 'anywhere' }}>
              <div>{item.description || 'Série sem descrição'}</div>
              <div style={{ marginTop: 6 }}>S{item.number || '—'} · {item.modality || '—'} · {item.instanceCount} imagem(ns)</div>
            </button>
          ))}
        </aside>
        <main tabIndex={0} onPointerDown={(event) => event.currentTarget.focus({ preventScroll: true })} style={{ flex: 1, minWidth: 0, position: 'relative', background: 'black' }} aria-label="Imagem DICOM">
          <div ref={viewport} style={{ position: 'absolute', inset: 0 }} onContextMenu={(event) => event.preventDefault()} />
          {state !== 'ready' && (
            <div role={state === 'error' ? 'alert' : 'status'} style={{ position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 14, background: 'black', padding: 32, textAlign: 'center' }}>
              <Icon name={state === 'error' ? 'alert' : 'image'} size={32} />
              <span style={{ fontFamily: 'var(--font-heading)', fontWeight: 600, fontSize: 22, color: 'var(--color-neutral-100)' }}>
                {state === 'loading-series' ? 'Carregando séries…' : state === 'loading' ? 'Carregando série…' : state === 'image-loading' ? 'Carregando imagem…' : state === 'empty-study' ? 'Estudo sem séries' : state === 'empty' ? 'Série sem imagens' : 'Não foi possível carregar'}
              </span>
              <span style={{ fontSize: 13.5, color: 'var(--color-neutral-400)', maxWidth: 480 }}>
                {state === 'error' ? message : state === 'empty' ? 'Selecione outra série para continuar.' : state === 'empty-study' ? 'O estudo não contém séries disponíveis.' : 'Consultando séries e preparando a imagem.'}
              </span>
              {state === 'error' && <button type="button" className="pacs-hover-accent-dark" onClick={() => retry.current()}
                style={{ marginTop: 8, padding: '8px 16px', border: '1px solid var(--color-accent-500)', background: 'transparent', color: 'var(--color-accent-300)', font: 'inherit', cursor: 'pointer' }}>Tentar novamente</button>}
            </div>
          )}
          {state === 'ready' && <div style={{ position: 'absolute', left: 14, top: 12, fontSize: 12, color: 'var(--color-neutral-300)', pointerEvents: 'none', maxWidth: '70%' }}>
            <div>{selected?.modality || '—'} · Série {selected?.number || '—'}</div>
            <div>{selected?.description || 'Série sem descrição'}</div>
          </div>}
          {state === 'ready' && <span role="status" style={{ position: 'absolute', right: 14, bottom: 12, fontSize: 12, color: 'var(--color-neutral-300)', pointerEvents: 'none' }}>Imagem {position.index + 1} / {position.total} · {selected?.modality || '—'}</span>}
        </main>
      </div>
    </div>
  );
}
