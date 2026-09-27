import { useEffect, useState } from 'react';
import { api, ApiError, type StudyPage } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';
import { Icon, type IconName } from '../design-system/Icon';

/** Estados da consulta real, usando a apresentação existente. */
type ListState = 'ok' | 'loading' | 'empty' | 'error';

type PeriodoKey = 'hoje' | 'ontem' | '7d' | 'per';

const PERIODOS: { key: PeriodoKey; label: string; icon?: IconName }[] = [
  { key: 'hoje', label: 'Hoje' },
  { key: 'ontem', label: 'Ontem' },
  { key: '7d', label: '7 dias' },
  { key: 'per', label: 'Período', icon: 'calendar' },
];

const PAGE_SIZE = 25;
const CAMPOS = [
  { key: 'patientName', label: 'Nome do paciente' },
  { key: 'patientId', label: 'Identificação do paciente' },
  { key: 'accessionNumber', label: 'Accession Number' },
  { key: 'studyDescription', label: 'Descrição do estudo' },
] as const;
type Campo = typeof CAMPOS[number]['key'];

function dataLocal(diasAtras: number): string {
  const date = new Date();
  date.setDate(date.getDate() - diasAtras);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

function exibirData(value: string): string {
  return /^\d{8}$/.test(value) ? `${value.slice(6, 8)}/${value.slice(4, 6)}/${value.slice(0, 4)}` : value || '—';
}

function exibirHora(value: string): string {
  return /^\d{4}/.test(value) ? `${value.slice(0, 2)}:${value.slice(2, 4)}` : value || '—';
}

const COLUNAS = '104px minmax(0,1.4fr) 92px 90px minmax(0,1.6fr) minmax(0,1.3fr) 64px 60px';

const ESQUELETOS = [1, 2, 3, 4, 5, 6, 7, 8].map((i) => ({
  w1: `${50 + ((i * 7) % 40)}%`,
  w2: `${40 + ((i * 13) % 45)}%`,
}));

type ExamesScreenProps = {
  onSessionExpired: () => void;
};

/** Worklist real: filtros e paginação são processados no backend/Orthanc. */
export function ExamesScreen({ onSessionExpired }: ExamesScreenProps) {
  const [q, setQ] = useState('');
  const [periodo, setPeriodo] = useState<PeriodoKey>('7d');
  const [instituicao, setInstituicao] = useState('');
  const [campo, setCampo] = useState<Campo>('patientName');
  const [menu, setMenu] = useState(false);
  const [dataInicial, setDataInicial] = useState(() => dataLocal(6));
  const [dataFinal, setDataFinal] = useState(() => dataLocal(0));
  const [offset, setOffset] = useState(0);
  const [page, setPage] = useState<StudyPage | null>(null);
  const [listState, setListState] = useState<ListState>('loading');
  const [erro, setErro] = useState('');
  const [atualizadoEm, setAtualizadoEm] = useState<Date | null>(null);
  const [tentativa, setTentativa] = useState(0);

  const inicio = periodo === 'per' ? dataInicial : dataLocal(periodo === '7d' ? 6 : periodo === 'ontem' ? 1 : 0);
  const fim = periodo === 'per' ? dataFinal : dataLocal(periodo === 'ontem' ? 1 : 0);

  useEffect(() => {
    const controller = new AbortController();
    setListState('loading');
    setPage(null);
    setAtualizadoEm(null);
    setErro('');
    const timer = window.setTimeout(() => {
      void api.getStudies({ limit: PAGE_SIZE, offset, dateFrom: inicio, dateTo: fim,
        [campo]: q.trim(), institutionName: instituicao.trim(),
      }, controller.signal).then((result) => {
        if (controller.signal.aborted) return;
        setPage(result);
        setListState(result.items.length ? 'ok' : 'empty');
        setAtualizadoEm(new Date());
      }).catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (error instanceof ApiError && error.status === 401) {
          onSessionExpired();
          return;
        }
        setErro(error instanceof ApiError ? error.message : 'Não foi possível consultar os exames. Verifique sua conexão.');
        setListState('error');
      });
    }, 350);
    return () => { window.clearTimeout(timer); controller.abort(); };
  }, [q, campo, instituicao, inicio, fim, offset, tentativa, onSessionExpired]);

  const linhas = page?.items ?? [];
  const listaOk = listState === 'ok';
  const listaVazia = listState === 'empty';

  function limparFiltros() {
    setQ('');
    setInstituicao('');
    setCampo('patientName');
    setPeriodo('7d');
    setOffset(0);
  }

  return (
    <div
      style={{
        padding: '32px 40px 48px',
        display: 'flex',
        flexDirection: 'column',
        gap: 20,
        maxWidth: 1440,
        minWidth: 1080,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 16 }}>
        <h1 style={{ margin: 0, fontSize: 40 }}>Exames</h1>
        <span style={{ fontSize: 14, color: 'var(--color-neutral-700)' }}>
          {listState === 'loading' ? 'Carregando…' : `${linhas.length} exame(s) nesta página`}
          {atualizadoEm && ` · atualizado às ${atualizadoEm.toLocaleTimeString('pt-BR')}`}
        </span>
      </div>

      <div style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
        <span style={{ position: 'absolute', left: 18, color: 'var(--color-neutral-600)' }}>
          <Icon name="search" size={20} />
        </span>
        <input
          className="pacs-search"
          value={q}
          onChange={(event) => { setQ(event.target.value); setOffset(0); }}
          maxLength={128}
          placeholder={`${CAMPOS.find((item) => item.key === campo)?.label} — use * para busca parcial`}
          aria-label="Pesquisar exames"
          style={{
            width: '100%',
            height: 56,
            padding: '0 120px 0 52px',
            font: 'inherit',
            fontSize: 17,
            color: 'var(--color-text)',
            background: 'var(--color-bg)',
            border: '1px solid color-mix(in srgb, var(--color-text) 28%, transparent)',
            borderRadius: 0,
            outline: 'none',
            caretColor: 'var(--color-accent)',
          }}
        />
        {q && (
          <button
            type="button"
            onClick={() => { setQ(''); setOffset(0); }}
            title="Limpar"
            aria-label="Limpar pesquisa"
            style={{
              position: 'absolute',
              right: 62,
              width: 32,
              height: 32,
              border: 0,
              background: 'transparent',
              color: 'var(--color-neutral-700)',
              cursor: 'pointer',
              display: 'grid',
              placeItems: 'center',
            }}
          >
            <Icon name="x" />
          </button>
        )}
        <span
          style={{
            position: 'absolute',
            right: 16,
            fontFamily: 'ui-monospace,Menlo,monospace',
            fontSize: 11,
            color: 'var(--color-neutral-600)',
            border: '1px solid var(--color-divider)',
            padding: '2px 7px',
          }}
        >
          /
        </span>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', border: '1px solid var(--color-divider)' }}>
          {PERIODOS.map((item) => {
            const ativo = periodo === item.key;
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => { setPeriodo(item.key); setOffset(0); }}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  height: 38,
                  padding: '0 16px',
                  border: 0,
                  borderRight: '1px solid var(--color-divider)',
                  background: ativo ? 'var(--color-accent)' : 'transparent',
                  color: ativo ? 'var(--color-bg)' : 'var(--color-text)',
                  font: 'inherit',
                  fontSize: 14,
                  fontWeight: 500,
                  cursor: 'pointer',
                }}
              >
                {item.icon && <Icon name={item.icon} />}
                {item.label}
              </button>
            );
          })}
        </div>

        {periodo === 'per' && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 14 }}>
            <input type="date" className="input" value={dataInicial} onChange={(event) => { setDataInicial(event.target.value); setOffset(0); }} style={{ width: 150, minHeight: 38 }} aria-label="Data inicial" />
            <span style={{ color: 'var(--color-neutral-600)' }}>até</span>
            <input type="date" className="input" value={dataFinal} onChange={(event) => { setDataFinal(event.target.value); setOffset(0); }} style={{ width: 150, minHeight: 38 }} aria-label="Data final" />
          </div>
        )}

        <div style={{ width: 1, height: 24, background: 'var(--color-divider)', margin: '0 4px' }} />

        <Dropdown
          label={CAMPOS.find((item) => item.key === campo)?.label ?? 'Campo de pesquisa'}
          aberto={menu}
          onToggle={() => setMenu((atual) => !atual)}
          largura={260}
          opcoes={[...CAMPOS]}
          selecionada={campo}
          onEscolher={(key) => { setCampo(key as Campo); setOffset(0); setMenu(false); }}
        />
        <input
          className="input"
          aria-label="Instituição do estudo"
          placeholder="Instituição (use * para busca parcial)"
          value={instituicao}
          maxLength={128}
          onChange={(event) => { setInstituicao(event.target.value); setOffset(0); }}
          style={{ width: 280, minHeight: 38 }}
        />

        <button
          type="button"
          className="btn btn-ghost"
          onClick={limparFiltros}
          style={{ marginLeft: 'auto', fontSize: 14 }}
        >
          Limpar filtros
        </button>
      </div>

      <div className="blueprint" style={{ display: 'flex', flexDirection: 'column' }}>
        <BlueprintCorners />
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: COLUNAS,
            alignItems: 'center',
            height: 42,
            padding: '0 20px',
            gap: 16,
            fontSize: 11,
            letterSpacing: '.08em',
            textTransform: 'uppercase',
            color: 'var(--color-neutral-700)',
            borderBottom: '1px solid var(--color-divider)',
          }}
        >
          <span>Data / Hora</span>
          <span>Paciente</span>
          <span>Identificação</span>
          <span>Mod.</span>
          <span>Exame</span>
          <span>Instituição</span>
          <span style={{ textAlign: 'right' }}>Séries</span>
          <span />
        </div>

        {listaOk &&
          linhas.map((exame) => (
            <div
              key={exame.orthancStudyId}
              style={{
                display: 'grid',
                gridTemplateColumns: COLUNAS,
                alignItems: 'center',
                minHeight: 62,
                padding: '0 20px',
                gap: 16,
                borderBottom: '1px solid color-mix(in srgb, var(--color-text) 8%, transparent)',
                background: 'transparent',
              }}
            >
              <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.3 }}>
                <span style={{ fontSize: 14, fontWeight: 600 }}>{exibirData(exame.studyDate)}</span>
                <span
                  style={{
                    fontSize: 13,
                    color: 'var(--color-neutral-700)',
                    fontVariantNumeric: 'tabular-nums',
                  }}
                >
                  {exibirHora(exame.studyTime)}
                </span>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.3, minWidth: 0 }}>
                <span
                  style={{
                    fontSize: 15.5,
                    fontWeight: 600,
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                  }}
                >
                  {exame.patientName.replaceAll('^', ' ') || '—'}
                </span>
                <span style={{ fontSize: 13, color: 'var(--color-neutral-700)' }}>{exame.accessionNumber ? `Accession: ${exame.accessionNumber}` : '—'}</span>
              </div>
              <span
                style={{
                  fontFamily: 'ui-monospace,Menlo,monospace',
                  fontSize: 13,
                  color: 'var(--color-neutral-800)',
                }}
              >
                {exame.patientId || '—'}
              </span>
              <span>
                <span
                  className="tag tag-accent"
                  style={{ fontWeight: 600, letterSpacing: '.06em', padding: '2px 8px', whiteSpace: 'normal', overflowWrap: 'anywhere' }}
                >
                  {exame.modalities.join(' / ') || '—'}
                </span>
              </span>
              <span
                title={exame.studyDescription || '—'}
                style={{ fontSize: 14.5, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}
              >
                {exame.studyDescription || '—'}
              </span>
              <span
                style={{
                  fontSize: 14,
                  color: 'var(--color-neutral-800)',
                  whiteSpace: 'nowrap',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                }}
              >
                {exame.institutionName || '—'}
              </span>
              <span
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'flex-end',
                  gap: 6,
                  fontSize: 14,
                  fontVariantNumeric: 'tabular-nums',
                  color: 'var(--color-neutral-800)',
                }}
              >
                {exame.seriesCount}
                <span style={{ color: 'var(--color-neutral-500)' }}>
                  <Icon name="image" />
                </span>
              </span>
              <span />
            </div>
          ))}

        {listState === 'loading' &&
          ESQUELETOS.map((esqueleto, i) => (
            <div
              key={i}
              style={{
                display: 'grid',
                gridTemplateColumns: COLUNAS,
                alignItems: 'center',
                height: 62,
                padding: '0 20px',
                gap: 16,
                borderBottom: '1px solid color-mix(in srgb, var(--color-text) 8%, transparent)',
                animation: 'pacsShimmer 1.4s ease-in-out infinite',
              }}
            >
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <div style={{ height: 10, width: 50, background: 'var(--color-neutral-300)' }} />
                <div style={{ height: 8, width: 36, background: 'var(--color-neutral-200)' }} />
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <div style={{ height: 10, width: esqueleto.w1, background: 'var(--color-neutral-300)' }} />
                <div style={{ height: 8, width: 90, background: 'var(--color-neutral-200)' }} />
              </div>
              <div style={{ height: 9, width: 64, background: 'var(--color-neutral-200)' }} />
              <div style={{ height: 18, width: 30, background: 'var(--color-neutral-200)' }} />
              <div style={{ height: 10, width: esqueleto.w2, background: 'var(--color-neutral-300)' }} />
              <div style={{ height: 9, width: '70%', background: 'var(--color-neutral-200)' }} />
              <div style={{ height: 9, width: 24, marginLeft: 'auto', background: 'var(--color-neutral-200)' }} />
              <span />
            </div>
          ))}

        {listaVazia && (
          <div
            style={{
              padding: '72px 24px',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              gap: 12,
              textAlign: 'center',
            }}
          >
            <span style={{ color: 'var(--color-accent)' }}>
              <Icon name="searchX" size={32} />
            </span>
            <h3 style={{ margin: '4px 0 0', fontSize: 24 }}>Nenhum exame encontrado</h3>
            <p
              style={{
                margin: 0,
                maxWidth: 420,
                fontSize: 14.5,
                color: 'var(--color-neutral-700)',
                textWrap: 'pretty',
              }}
            >
              Nenhum resultado para os filtros atuais. Verifique a grafia do nome ou amplie o período
              da pesquisa.
            </p>
            <button
              type="button"
              className="btn btn-secondary"
              onClick={limparFiltros}
              style={{ marginTop: 8, height: 40, padding: '0 18px', fontSize: 15 }}
            >
              Limpar filtros
            </button>
          </div>
        )}

        {listState === 'error' && (
          <div
            style={{
              padding: '72px 24px',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              gap: 12,
              textAlign: 'center',
            }}
          >
            <span style={{ color: 'oklch(0.5 0.14 25)' }}>
              <Icon name="alert" size={32} />
            </span>
            <h3 style={{ margin: '4px 0 0', fontSize: 24 }}>Não foi possível carregar os exames</h3>
            <p
              style={{
                margin: 0,
                maxWidth: 440,
                fontSize: 14.5,
                color: 'var(--color-neutral-700)',
                textWrap: 'pretty',
              }}
            >
              {erro}
            </p>
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => setTentativa((value) => value + 1)}
              style={{ marginTop: 8, height: 40, padding: '0 18px', fontSize: 15, gap: 8 }}
            >
              <Icon name="rotate" />
              Tentar novamente
            </button>
          </div>
        )}
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, fontSize: 14 }}>
        <button className="btn btn-secondary" type="button" disabled={listState === 'loading' || offset === 0}
          onClick={() => setOffset((value) => Math.max(0, value - PAGE_SIZE))}>Anterior</button>
        <span>Página {Math.floor(offset / PAGE_SIZE) + 1} · até {PAGE_SIZE} estudos</span>
        <button className="btn btn-secondary" type="button" disabled={listState === 'loading' || page?.nextOffset == null}
          onClick={() => { if (page?.nextOffset != null) setOffset(page.nextOffset); }}>Próxima</button>
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
