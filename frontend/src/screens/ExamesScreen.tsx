import { useEffect, useState } from 'react';
import { api, ApiError, type Study, type StudyPage } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';
import { Icon } from '../design-system/Icon';
import { SEARCH_FIELDS, initialWorklist, periodDates, studyQuery, validateWorklist, dicomDate, dicomTime, modalities, type WorklistState, type SearchField, type Period } from '../worklist/model';

type Props = {
  state: WorklistState;
  onStateChange: (state: WorklistState) => void;
  onSessionExpired: () => void;
  onAbrirExame: (study: Study) => void;
};
const PERIODS: { key: Period; label: string }[] = [
  { key: 'today', label: 'Hoje' }, { key: 'yesterday', label: 'Ontem' },
  { key: '7days', label: 'Últimos 7 dias' }, { key: '30days', label: 'Últimos 30 dias' }, { key: 'custom', label: 'Personalizado' },
];
const COLUMNS = '110px minmax(140px,1.4fr) 105px 85px minmax(130px,1.4fr) 110px minmax(120px,1fr) 48px';

/** Estado de filtros vive somente na memória de App durante a sessão. */
export function ExamesScreen({ state, onStateChange, onSessionExpired, onAbrirExame }: Props) {
  const [page, setPage] = useState<StudyPage | null>(null);
  const [status, setStatus] = useState<'loading' | 'refreshing' | 'ok' | 'error'>('loading');
  const [error, setError] = useState('');
  const [unsupportedSort, setUnsupportedSort] = useState(false);
  const [updated, setUpdated] = useState<Date | null>(null);
  const [revision, setRevision] = useState(0);
  const [refresh, setRefresh] = useState(false);
  const validation = validateWorklist(state);
  // String usada apenas como dependência em memória; nunca URL de navegação/log/storage.
  const queryKey = JSON.stringify(studyQuery(state));
  useEffect(() => {
    const controller = new AbortController();
    if (validation) { setStatus('error'); setError(validation); setPage(null); return () => controller.abort(); }
    setStatus(refresh ? 'refreshing' : 'loading');
    if (!refresh) setPage(null);
    setError('');
    setUnsupportedSort(false);
    const timer = window.setTimeout(() => {
      void api.getStudies(JSON.parse(queryKey), controller.signal).then(result => {
        if (controller.signal.aborted) return;
        setPage(result); setStatus('ok'); setUpdated(new Date());
      }).catch((cause: unknown) => {
        if (controller.signal.aborted) return;
        if (cause instanceof ApiError && cause.status === 401) { onSessionExpired(); return; }
        setError(cause instanceof ApiError ? cause.message : 'Não foi possível consultar os exames. Verifique sua conexão.');
        setUnsupportedSort(cause instanceof ApiError && cause.code === 'PACS_QUERY_UNSUPPORTED');
        setStatus('error'); setPage(null);
      });
    }, 350);
    return () => { window.clearTimeout(timer); controller.abort(); };
  }, [queryKey, revision, validation, refresh, onSessionExpired]);

  const change = (patch: Partial<WorklistState>) => { setRefresh(false); onStateChange({ ...state, ...patch, offset: 0 }); };
  const filter = (key: SearchField | 'institutionName', value: string) => change({ filters: { ...state.filters, [key]: value } });
  const reload = () => { setRefresh(true); onStateChange({ ...state, offset: 0 }); setRevision(value => value + 1); };
  const busy = status === 'loading' || status === 'refreshing';
  const text = (value: string) => value.trim() || '—';
  return (
    <div style={{ padding: '32px 40px 48px', display: 'flex', flexDirection: 'column', gap: 18, maxWidth: 1520, minWidth: 1080 }}>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 16 }}>
        <h1 style={{ margin: 0, fontSize: 40 }}>Exames</h1>
        <span role="status" style={{ fontSize: 14, color: 'var(--color-neutral-700)' }}>
          {status === 'loading' ? 'Carregando…' : status === 'refreshing' ? 'Atualizando…' : status === 'ok' ? `${page?.items.length ?? 0} estudo(s) nesta página` : ''}
          {updated && status === 'ok' && ` · atualizado às ${updated.toLocaleTimeString('pt-BR')}`}
        </span>
        <button className="btn btn-secondary" style={{ marginLeft: 'auto' }} disabled={busy || !!validation} onClick={reload}><Icon name="rotate" /> Atualizar</button>
      </div>
      <div style={{ display: 'flex', gap: 10 }}>
        <select className="input" aria-label="Tipo de busca" value={state.field} style={{ width: 220 }}
          onChange={event => onStateChange({ ...state, field: event.target.value as SearchField })}>
          {SEARCH_FIELDS.map(field => <option key={field.key} value={field.key}>{field.label}</option>)}
        </select>
        <input className="input pacs-search" style={{ flex: 1, height: 48 }} aria-label="Pesquisar exames" maxLength={128}
          placeholder="Valor exato; use * para busca parcial" value={state.filters[state.field]} onChange={event => filter(state.field, event.target.value)} />
        <button className="btn btn-secondary" aria-expanded={state.advanced} onClick={() => onStateChange({ ...state, advanced: !state.advanced })}>Filtros avançados</button>
      </div>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
        {PERIODS.map(period => <button key={period.key} className={`btn ${state.period === period.key ? 'btn-primary' : 'btn-secondary'}`} aria-pressed={state.period === period.key}
          onClick={() => change({ period: period.key, ...(period.key === 'custom' ? {} : periodDates(period.key)) })}>{period.label}</button>)}
        <span style={{ fontSize: 13, color: 'var(--color-neutral-700)' }}>{dicomDate(state.dateFrom.replaceAll('-', ''))} — {dicomDate(state.dateTo.replaceAll('-', ''))}</span>
        <button className="btn btn-ghost" style={{ marginLeft: 'auto' }} onClick={() => { setRefresh(false); onStateChange(initialWorklist()); }}>Limpar filtros</button>
      </div>
      {state.period === 'custom' && <div style={{ display: 'flex', gap: 16 }}>
        <label>Data inicial <input type="date" className="input" aria-label="Data inicial" value={state.dateFrom} onChange={event => change({ dateFrom: event.target.value })} /></label>
        <label>Data final <input type="date" className="input" aria-label="Data final" value={state.dateTo} onChange={event => change({ dateTo: event.target.value })} /></label>
      </div>}
      {state.advanced && <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 12 }}>
        {[...SEARCH_FIELDS, { key: 'institutionName' as const, label: 'Instituição' }].map(field => <label key={field.key} style={{ fontSize: 13 }}>{field.label}
          <input className="input" style={{ width: '100%' }} aria-label={`Filtro ${field.label}`} maxLength={128} value={state.filters[field.key]} onChange={event => filter(field.key, event.target.value)} />
        </label>)}
        <p style={{ margin: 0, alignSelf: 'end', fontSize: 13 }}>Filtros combinados por E. Use * ou ? para padrões DICOM; nenhum curinga é acrescentado.</p>
      </div>}
      {!state.advanced && Object.entries(state.filters).some(([key, value]) => key !== state.field && value.trim()) &&
        <span style={{ fontSize: 13 }}>Há filtros adicionais ativos. Abra “Filtros avançados” para revisá-los.</span>}
      <div className="blueprint" aria-busy={busy} style={{ position: 'relative' }}>
        <BlueprintCorners />
        <div style={{ display: 'grid', gridTemplateColumns: COLUMNS, gap: 12, padding: '14px 16px', fontSize: 11, letterSpacing: '.06em', textTransform: 'uppercase', borderBottom: '1px solid var(--color-divider)' }}>
          <button type="button" className="pacs-hover-text"
            aria-label={`Data / Hora: ${state.sort === 'dateAsc' ? 'mais antigos primeiro' : state.sort === 'native' ? 'ordem nativa' : 'mais recentes primeiro'}. Clique para alternar ordenação.`}
            title={state.sort === 'dateAsc' ? 'Ordenar pelos mais recentes' : 'Ordenar pelos mais antigos'}
            onClick={() => change({ sort: state.sort === 'dateAsc' ? 'dateDesc' : 'dateAsc' })}
            style={{ padding: 0, border: 0, background: 'transparent', color: 'inherit', font: 'inherit', letterSpacing: 'inherit', textTransform: 'inherit', textAlign: 'left', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6 }}>
            Data / Hora <span aria-hidden="true">{state.sort === 'dateAsc' ? '↑' : state.sort === 'native' ? '↕' : '↓'}</span>
          </button>
          {['Paciente', 'ID', 'Modalidade', 'Descrição', 'Accession', 'Instituição', 'Séries'].map(label => <span key={label}>{label}</span>)}
        </div>
        {page?.items.map(study => <div key={study.orthancStudyId} role="button" tabIndex={busy ? -1 : 0} aria-disabled={busy} className="pacs-hover-accent" aria-label="Abrir estudo"
          onClick={() => { if (!busy) onAbrirExame(study); }}
          onKeyDown={event => { if (event.target === event.currentTarget && !busy && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); onAbrirExame(study); } }}
          style={{ display: 'grid', gridTemplateColumns: COLUMNS, gap: 12, alignItems: 'center', padding: '14px 16px', minHeight: 62, fontSize: 14, cursor: busy ? 'wait' : 'pointer', borderBottom: '1px solid var(--color-divider)', opacity: busy ? .65 : 1 }}>
          <span style={{ fontVariantNumeric: 'tabular-nums' }}>{dicomDate(study.studyDate)}<br /><small>{dicomTime(study.studyTime)}</small></span>
          <strong style={{ overflowWrap: 'anywhere' }}>{text(study.patientName.replaceAll('^', ' '))}</strong>
          <span style={{ overflowWrap: 'anywhere' }}>{text(study.patientId)}</span>
          <span className="tag tag-accent">{modalities(study.modalities).join(' / ') || '—'}</span>
          <span style={{ overflowWrap: 'anywhere' }}>{text(study.studyDescription)}</span>
          <span style={{ overflowWrap: 'anywhere' }}>{text(study.accessionNumber)}</span>
          <span style={{ overflowWrap: 'anywhere' }}>{text(study.institutionName)}</span><span>{study.seriesCount}</span>
        </div>)}
        {status === 'loading' && <div style={{ padding: 48, textAlign: 'center' }}>Consultando exames…</div>}
        {status === 'ok' && page?.items.length === 0 && <div style={{ padding: 48, textAlign: 'center' }}>Nenhum estudo encontrado para os filtros selecionados.</div>}
        {status === 'error' && <div role="alert" style={{ padding: 40, textAlign: 'center' }}><p>{error}</p>
          {!validation && <button className="btn btn-secondary" onClick={reload}>Tentar novamente</button>}
          {unsupportedSort && <button className="btn btn-secondary" onClick={() => change({ sort: 'native', modality: '' })}>Usar ordem nativa do PACS</button>}</div>}
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, fontSize: 14 }}>
        <button className="btn btn-secondary" disabled={busy || !!validation || state.offset === 0} onClick={() => { setRefresh(false); onStateChange({ ...state, offset: Math.max(0, state.offset - state.limit) }); }}>Anterior</button>
        <span>Página {Math.floor(state.offset / state.limit) + 1} · até {state.limit} estudos</span>
        <button className="btn btn-secondary" disabled={busy || status === 'error' || page?.nextOffset == null} onClick={() => { if (page?.nextOffset != null) { setRefresh(false); onStateChange({ ...state, offset: page.nextOffset }); } }}>Próxima</button>
        {status === 'ok' && page && !page.hasMore && <span>Fim dos resultados.</span>}
        {page?.hasMore && page.nextOffset === null && <span>Refine o período ou os filtros para continuar.</span>}
      </div>
    </div>
  );
}

type DropdownProps = {
  label: string;
  destacado?: boolean;
  aberto: boolean;
  onToggle: () => void;
  largura: number;
  opcoes: { key: string; label: string }[];
  selecionada: string;
  onEscolher: (key: string) => void;
  altura?: number;
};

/** Botão de filtro + lista suspensa, no padrão do design (marca com "✓"). */
export function Dropdown({
  label,
  destacado = false,
  aberto,
  onToggle,
  largura,
  opcoes,
  selecionada,
  onEscolher,
  altura = 38,
}: DropdownProps) {
  return (
    <div style={{ position: 'relative' }}>
      <button
        type="button"
        className="pacs-hover-text"
        onClick={onToggle}
        aria-expanded={aberto}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          height: altura,
          padding: '0 12px 0 14px',
          border: `1px solid ${destacado ? 'var(--color-accent)' : 'var(--color-divider)'}`,
          background: 'transparent',
          font: 'inherit',
          fontSize: 14,
          color: 'inherit',
          cursor: 'pointer',
        }}
      >
        {label}
        <span style={{ color: 'var(--color-neutral-600)' }}>
          <Icon name="down" />
        </span>
      </button>
      {aberto && (
        <div
          style={{
            position: 'absolute',
            left: 0,
            top: altura + 6,
            width: largura,
            background: 'var(--color-bg)',
            border: '1px solid var(--color-divider)',
            boxShadow: 'var(--shadow-md)',
            padding: 6,
            zIndex: 10,
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          {opcoes.map((opcao) => (
            <button
              key={opcao.key}
              type="button"
              className="pacs-hover-accent"
              onClick={() => onEscolher(opcao.key)}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                height: 36,
                padding: '0 10px',
                border: 0,
                background: 'transparent',
                font: 'inherit',
                fontSize: 14,
                color: 'inherit',
                cursor: 'pointer',
                textAlign: 'left',
              }}
            >
              {opcao.label}
              <span style={{ color: 'var(--color-accent)' }}>
                {selecionada === opcao.key && <Icon name="check" />}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
