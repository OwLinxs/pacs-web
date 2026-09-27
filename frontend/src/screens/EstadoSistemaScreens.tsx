import { BlueprintCorners } from '../design-system/Blueprint';
import { Icon } from '../design-system/Icon';

/** Tela 3g do design — "Sessão expirada". Nenhum dado de paciente permanece. */
export function SessaoExpiradaScreen({ onEntrarNovamente }: { onEntrarNovamente: () => void }) {
  return (
    <div
      className="pacs-grid-ground"
      style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center' }}
    >
      <div
        className="blueprint"
        style={{
          width: 420,
          background: 'var(--color-bg)',
          padding: 40,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'flex-start',
          gap: 14,
        }}
      >
        <BlueprintCorners />
        <div
          style={{
            width: 48,
            height: 48,
            display: 'grid',
            placeItems: 'center',
            border: '1px solid var(--color-accent)',
            color: 'var(--color-accent-700)',
          }}
        >
          <Icon name="clock" size={20} />
        </div>
        <h2 style={{ margin: '8px 0 0', fontSize: 30 }}>Sua sessão expirou por segurança.</h2>
        <p
          style={{
            margin: 0,
            fontSize: 14.5,
            lineHeight: 1.55,
            color: 'var(--color-neutral-700)',
            textWrap: 'pretty',
          }}
        >
          Após 30 minutos sem atividade o acesso é encerrado e nenhuma informação de paciente
          permanece na tela. Entre novamente para continuar.
        </p>
        <button
          type="button"
          className="btn btn-primary blueprint"
          onClick={onEntrarNovamente}
          style={{ marginTop: 10, width: '100%', minHeight: 46, fontSize: 16, overflow: 'visible' }}
        >
          <BlueprintCorners />
          Entrar novamente
        </button>
      </div>
    </div>
  );
}

/** Tela 3f do design — "Sistema indisponível". */
export function IndisponivelScreen({ onTentarNovamente }: { onTentarNovamente: () => void }) {
  return (
    <div
      className="pacs-grid-ground"
      style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center' }}
    >
      <div
        className="blueprint"
        style={{
          width: 460,
          background: 'var(--color-bg)',
          padding: 40,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'flex-start',
          gap: 14,
        }}
      >
        <BlueprintCorners />
        <div
          style={{
            width: 48,
            height: 48,
            display: 'grid',
            placeItems: 'center',
            border: '1px solid oklch(0.6 0.13 25)',
            color: 'oklch(0.48 0.14 25)',
          }}
        >
          <Icon name="server" size={20} />
        </div>
        <h2 style={{ margin: '8px 0 0', fontSize: 30 }}>Sistema temporariamente indisponível</h2>
        <p
          style={{
            margin: 0,
            fontSize: 14.5,
            lineHeight: 1.55,
            color: 'var(--color-neutral-700)',
            textWrap: 'pretty',
          }}
        >
          Não conseguimos conectar ao servidor de imagens da Secretaria. Em caso de urgência, siga o
          protocolo de contingência da unidade.
        </p>
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 4,
            width: '100%',
            padding: '12px 14px',
            background: 'var(--color-neutral-200)',
            fontFamily: 'ui-monospace,Menlo,monospace',
            fontSize: 12,
            color: 'var(--color-neutral-800)',
          }}
        >
          <span>Status: sem resposta (PACS-503)</span>
          <span>Última verificação: 10:04:12 · nova tentativa em 30 s</span>
        </div>
        <div style={{ display: 'flex', gap: 10, width: '100%', marginTop: 8 }}>
          <button
            type="button"
            className="btn btn-primary"
            onClick={onTentarNovamente}
            style={{ flex: 1, minHeight: 44, fontSize: 15, gap: 8 }}
          >
            <Icon name="rotate" />
            Tentar novamente
          </button>
          <button type="button" className="btn btn-secondary" style={{ minHeight: 44, padding: '0 16px', fontSize: 15 }}>
            Suporte TI
          </button>
        </div>
      </div>
    </div>
  );
}
