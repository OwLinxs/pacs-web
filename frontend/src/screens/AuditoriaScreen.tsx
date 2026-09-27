import { BlueprintCorners } from '../design-system/Blueprint';
import { Icon, type IconName } from '../design-system/Icon';
import { AUDITORIA, ESTILO_EVENTO } from '../mocks/dados';

const COLUNAS = '170px minmax(0,1fr) minmax(0,1fr) 150px minmax(0,1.6fr) minmax(0,1.2fr)';

/**
 * Tela 2c do design — "Auditoria".
 * Fase 1: os cinco filtros são estáticos, como no protótipo; a consulta real
 * chega junto do backend de auditoria (Fase 8).
 */
export function AuditoriaScreen() {
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
        <h1 style={{ margin: 0, fontSize: 40 }}>Auditoria</h1>
        <span style={{ fontSize: 14, color: 'var(--color-neutral-700)' }}>
          {AUDITORIA.length} eventos · últimas 48 horas
        </span>
        <button
          type="button"
          className="btn btn-secondary"
          style={{ marginLeft: 'auto', alignSelf: 'center', height: 40, padding: '0 16px', fontSize: 14 }}
        >
          Exportar CSV
        </button>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(5,minmax(0,1fr))',
          gap: 12,
          alignItems: 'end',
        }}
      >
        <CampoFiltro label="Período" valor="24/09/2026 – 25/09/2026" iconeEsquerda="calendar" />
        <CampoFiltro label="Usuário" valor="Todos" iconeDireita="down" esmaecido />
        <CampoFiltro label="Unidade" valor="Todas" iconeDireita="down" esmaecido />
        <CampoFiltro label="Evento" valor="Todos os eventos" iconeDireita="down" esmaecido />
        <CampoFiltro label="Origem" valor="IP ou estação" iconeDireita="search" esmaecido />
      </div>

      <div className="blueprint" style={{ display: 'flex', flexDirection: 'column' }}>
        <BlueprintCorners />
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: COLUNAS,
            gap: 16,
            alignItems: 'center',
            height: 42,
            padding: '0 20px',
            fontSize: 11,
            letterSpacing: '.08em',
            textTransform: 'uppercase',
            color: 'var(--color-neutral-700)',
            borderBottom: '1px solid var(--color-divider)',
          }}
        >
          <span>Data / Hora</span>
          <span>Usuário</span>
          <span>Unidade</span>
          <span>Evento</span>
          <span>Detalhe</span>
          <span>Origem</span>
        </div>

        {AUDITORIA.map((evento, i) => {
          const estilo = ESTILO_EVENTO[evento.kind];
          return (
            <div
              key={`${evento.ts}-${i}`}
              className="pacs-hover-text-4"
              style={{
                display: 'grid',
                gridTemplateColumns: COLUNAS,
                gap: 16,
                alignItems: 'center',
                minHeight: 50,
                padding: '0 20px',
                borderBottom: '1px solid color-mix(in srgb, var(--color-text) 8%, transparent)',
              }}
            >
              <span
                style={{
                  fontSize: 13,
                  fontVariantNumeric: 'tabular-nums',
                  color: 'var(--color-neutral-800)',
                }}
              >
                {evento.ts}
              </span>
              <span style={{ fontFamily: 'ui-monospace,Menlo,monospace', fontSize: 13 }}>{evento.user}</span>
              <span style={{ fontSize: 13.5, color: 'var(--color-neutral-800)' }}>{evento.unit}</span>
              <span>
                <span
                  style={{
                    display: 'inline-flex',
                    fontSize: 12,
                    fontWeight: 600,
                    padding: '3px 9px',
                    background: estilo.bg,
                    color: estilo.fg,
                    border: `1px solid ${estilo.bd ?? 'transparent'}`,
                  }}
                >
                  {evento.event}
                </span>
              </span>
              <span style={{ fontSize: 13.5 }}>{evento.detail}</span>
              <span
                style={{
                  fontFamily: 'ui-monospace,Menlo,monospace',
                  fontSize: 12,
                  color: 'var(--color-neutral-700)',
                }}
              >
                {evento.origin}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function CampoFiltro({
  label,
  valor,
  iconeEsquerda,
  iconeDireita,
  esmaecido = false,
}: {
  label: string;
  valor: string;
  iconeEsquerda?: IconName;
  iconeDireita?: IconName;
  esmaecido?: boolean;
}) {
  return (
    <div className="field">
      <label>{label}</label>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: iconeDireita ? 'space-between' : undefined,
          gap: 8,
          height: 40,
          padding: '0 12px',
          border: '1px solid var(--color-divider)',
          fontSize: 14,
          color: esmaecido ? 'var(--color-neutral-700)' : undefined,
        }}
      >
        {iconeEsquerda && <Icon name={iconeEsquerda} />}
        {valor}
        {iconeDireita && (
          <span>
            <Icon name={iconeDireita} />
          </span>
        )}
      </div>
    </div>
  );
}
