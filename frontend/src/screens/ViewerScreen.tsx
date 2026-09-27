import { useEffect, useRef, useState } from 'react';
import { ApiError, type Study, type ViewerSeries } from '../api/client';
import { iniciaisDe, useSession } from '../auth/SessionProvider';
import { Icon } from '../design-system/Icon';
import { selectInitialImage } from '../viewer/selection';

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
 * Sem simulações de pixels, marcadores de lateralidade ou ferramentas clínicas.
 */
export function ViewerScreen({ studyId, study, onVoltar, onLogout, onSessionExpired }: Props) {
  const viewport = useRef<HTMLDivElement>(null);
  const { user } = useSession();
  const [state, setState] = useState<'loading' | 'ready' | 'empty' | 'error'>('loading');
  const [message, setMessage] = useState('');
  const [series, setSeries] = useState<ViewerSeries[]>([]);
  const [selected, setSelected] = useState<ViewerSeries | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const element = viewport.current;
    if (!element) return;
    const controller = new AbortController();
    setState('loading'); setMessage(''); setSeries([]); setSelected(null);
    const ready = () => { if (!controller.signal.aborted) setState('ready'); };
    const expired = () => { controller.abort(); onSessionExpired(); };
    element.addEventListener('pacs-image-ready', ready);
    window.addEventListener('pacs-viewer-session-expired', expired);
    void (async () => {
      try {
        const initial = await selectInitialImage(studyId, controller.signal);
        if (controller.signal.aborted) return;
        setSeries(initial.series); setSelected(initial.selected);
        if (!initial.path) { setState('empty'); return; }
        const { renderInitialImage } = await import('../viewer/cornerstone');
        if (controller.signal.aborted) return;
        await renderInitialImage(element, initial.path, controller.signal);
      } catch (error: unknown) {
        if (controller.signal.aborted) return;
        if (error instanceof ApiError && error.status === 401) { expired(); return; }
        // Nunca exibe o erro do decoder, que pode conter metadados do DICOM.
        setMessage(error instanceof ApiError ? error.message : 'Não foi possível renderizar esta imagem. Tente novamente; se persistir, contate o suporte de TI.');
        setState('error');
      }
    })();
    return () => {
      controller.abort();
      element.removeEventListener('pacs-image-ready', ready);
      window.removeEventListener('pacs-viewer-session-expired', expired);
    };
  }, [studyId, attempt, onSessionExpired]);

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
      <div style={{ height: 50, flex: 'none', display: 'flex', alignItems: 'center', gap: 12, padding: '0 16px', borderBottom: `1px solid ${LINE}`, background: 'color-mix(in srgb, var(--color-neutral-900) 70%, black)' }}>
        <Icon name="panel" size={20} />
        <span style={{ fontSize: 13 }}>Visualizador</span>
        <span style={{ marginLeft: 'auto', fontSize: 12, color: 'var(--color-neutral-500)' }}>Imagem inicial · 1 viewport</span>
      </div>
      <div style={{ flex: 1, minHeight: 0, display: 'flex' }}>
        <aside style={{ width: 160, flex: 'none', overflowY: 'auto', padding: 16, borderRight: `1px solid ${LINE}`, background: PANEL }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, letterSpacing: '.1em', textTransform: 'uppercase', marginBottom: 20 }}><span>Séries</span><span>{series.length}</span></div>
          {series.map((item) => (
            <div key={item.orthancSeriesId} style={{ padding: '12px 0', borderBottom: `1px solid ${LINE}`, color: item.orthancSeriesId === selected?.orthancSeriesId ? 'var(--color-accent-300)' : 'var(--color-neutral-500)', fontSize: 12, overflowWrap: 'anywhere' }}>
              <div>{item.description || 'Série sem descrição'}</div>
              <div style={{ marginTop: 6 }}>S{item.number || '—'} · {item.modality || '—'} · {item.instanceCount} instância(s)</div>
            </div>
          ))}
        </aside>
        <main style={{ flex: 1, minWidth: 0, position: 'relative', background: 'black' }} aria-label="Imagem DICOM">
          <div ref={viewport} style={{ position: 'absolute', inset: 0 }} onContextMenu={(event) => event.preventDefault()} />
          {state !== 'ready' && (
            <div role={state === 'error' ? 'alert' : 'status'} style={{ position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 14, background: 'black', padding: 32, textAlign: 'center' }}>
              <Icon name={state === 'error' ? 'alert' : 'image'} size={32} />
              <span style={{ fontFamily: 'var(--font-heading)', fontWeight: 600, fontSize: 22, color: 'var(--color-neutral-100)' }}>
                {state === 'loading' ? 'Carregando imagem…' : state === 'empty' ? 'Nenhuma imagem disponível' : 'Não foi possível carregar a imagem'}
              </span>
              <span style={{ fontSize: 13.5, color: 'var(--color-neutral-400)', maxWidth: 480 }}>
                {state === 'error' ? message : state === 'empty' ? 'O estudo não contém séries com instâncias disponíveis.' : 'Consultando séries e preparando a imagem.'}
              </span>
              {state === 'error' && <button type="button" className="pacs-hover-accent-dark" onClick={() => setAttempt((value) => value + 1)}
                style={{ marginTop: 8, padding: '8px 16px', border: '1px solid var(--color-accent-500)', background: 'transparent', color: 'var(--color-accent-300)', font: 'inherit', cursor: 'pointer' }}>Tentar novamente</button>}
            </div>
          )}
          {state === 'ready' && <span role="status" style={{ position: 'absolute', right: 14, bottom: 12, fontSize: 12, color: 'var(--color-neutral-300)', pointerEvents: 'none' }}>Imagem inicial renderizada · {selected?.modality || '—'}</span>}
        </main>
      </div>
    </div>
  );
}
