import { useState } from 'react';
import { BlueprintCorners } from '../design-system/Blueprint';
import { Icon } from '../design-system/Icon';
import { USUARIOS, UNIDADES, UNIDADE_KEYS } from '../mocks/dados';
import { Dropdown } from './ExamesScreen';

const COLUNAS = 'minmax(0,1.6fr) minmax(0,1.1fr) minmax(0,1.2fr) 130px 110px 140px 100px';

type UsuariosScreenProps = {
  /** Abre a tela já com o modal "Novo médico" (estado 2b do design). */
  modalInicial?: boolean;
  onToast: (mensagem: string) => void;
};

/** Tela 2a do design — "Usuários", com o modal 2b "Novo médico". */
export function UsuariosScreen({ modalInicial = false, onToast }: UsuariosScreenProps) {
  const [q, setQ] = useState('');
  const [unidade, setUnidade] = useState<'all' | keyof typeof UNIDADES>('all');
  const [status, setStatus] = useState<'all' | 'on' | 'off'>('all');
  const [menu, setMenu] = useState<'unit' | 'status' | null>(null);
  const [modalAberto, setModalAberto] = useState(modalInicial);

  const termo = q.trim().toLowerCase();
  const linhas = USUARIOS.filter(
    (usuario) =>
      (unidade === 'all' || usuario.unitKey === unidade) &&
      (status === 'all' || (status === 'on') === usuario.active) &&
      (!termo || `${usuario.name} ${usuario.user}`.toLowerCase().includes(termo)),
  );

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
        <h1 style={{ margin: 0, fontSize: 40 }}>Usuários</h1>
        <span style={{ fontSize: 14, color: 'var(--color-neutral-700)' }}>
          {linhas.length} de {USUARIOS.length} usuários
        </span>
        <button
          type="button"
          className="btn btn-primary blueprint"
          onClick={() => setModalAberto(true)}
          style={{
            marginLeft: 'auto',
            alignSelf: 'center',
            height: 42,
            padding: '0 20px',
            fontSize: 15,
            gap: 8,
            overflow: 'visible',
          }}
        >
          <BlueprintCorners />
          <Icon name="plus" />
          Novo médico
        </button>
      </div>

      <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: 520, display: 'flex', alignItems: 'center' }}>
          <span style={{ position: 'absolute', left: 14, color: 'var(--color-neutral-600)' }}>
            <Icon name="search" />
          </span>
          <input
            className="pacs-search-sm"
            value={q}
            onChange={(event) => setQ(event.target.value)}
            placeholder="Pesquisar usuário..."
            aria-label="Pesquisar usuário"
            style={{
              width: '100%',
              height: 42,
              padding: '0 14px 0 40px',
              font: 'inherit',
              fontSize: 15,
              color: 'var(--color-text)',
              background: 'var(--color-bg)',
              border: '1px solid color-mix(in srgb, var(--color-text) 28%, transparent)',
              outline: 'none',
            }}
          />
        </div>

        <Dropdown
          label={unidade === 'all' ? 'Unidade' : UNIDADES[unidade]}
          aberto={menu === 'unit'}
          onToggle={() => setMenu((atual) => (atual === 'unit' ? null : 'unit'))}
          largura={260}
          altura={42}
          opcoes={[
            { key: 'all', label: 'Todas as unidades' },
            ...UNIDADE_KEYS.map((key) => ({ key, label: UNIDADES[key] })),
          ]}
          selecionada={unidade}
          onEscolher={(key) => {
            setUnidade(key as 'all' | keyof typeof UNIDADES);
            setMenu(null);
          }}
        />

        <Dropdown
          label={{ all: 'Status', on: 'Ativo', off: 'Inativo' }[status]}
          aberto={menu === 'status'}
          onToggle={() => setMenu((atual) => (atual === 'status' ? null : 'status'))}
          largura={180}
          altura={42}
          opcoes={[
            { key: 'all', label: 'Todos' },
            { key: 'on', label: 'Ativo' },
            { key: 'off', label: 'Inativo' },
          ]}
          selecionada={status}
          onEscolher={(key) => {
            setStatus(key as 'all' | 'on' | 'off');
            setMenu(null);
          }}
        />
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
          <span>Nome</span>
          <span>Usuário</span>
          <span>Unidade</span>
          <span>Perfil</span>
          <span>Validade</span>
          <span>Último acesso</span>
          <span>Status</span>
        </div>

        {linhas.map((usuario) => {
          const opacidade = usuario.active ? 1 : 0.62;
          return (
            <div
              key={usuario.user}
              className="pacs-hover-accent-7"
              style={{
                display: 'grid',
                gridTemplateColumns: COLUNAS,
                gap: 16,
                alignItems: 'center',
                minHeight: 58,
                padding: '0 20px',
                borderBottom: '1px solid color-mix(in srgb, var(--color-text) 8%, transparent)',
                cursor: 'pointer',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 12, opacity: opacidade }}>
                <div
                  style={{
                    width: 30,
                    height: 30,
                    flex: 'none',
                    display: 'grid',
                    placeItems: 'center',
                    border: '1px solid var(--color-divider)',
                    fontFamily: 'var(--font-heading)',
                    fontWeight: 600,
                    fontSize: 12,
                    color: 'var(--color-neutral-800)',
                  }}
                >
                  {usuario.initials}
                </div>
                <span style={{ fontSize: 15, fontWeight: 600 }}>{usuario.name}</span>
              </div>
              <span
                style={{
                  fontFamily: 'ui-monospace,Menlo,monospace',
                  fontSize: 13,
                  color: 'var(--color-neutral-800)',
                  opacity: opacidade,
                }}
              >
                {usuario.user}
              </span>
              <span style={{ fontSize: 14, opacity: opacidade }}>{usuario.unit}</span>
              <span style={{ fontSize: 14, color: 'var(--color-neutral-800)', opacity: opacidade }}>
                {usuario.role}
              </span>
              <span style={{ fontSize: 14, fontVariantNumeric: 'tabular-nums', opacity: opacidade }}>
                {usuario.valid}
              </span>
              <span style={{ fontSize: 13.5, color: 'var(--color-neutral-700)' }}>{usuario.last}</span>
              <span>
                <span
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    fontSize: 12,
                    fontWeight: 600,
                    padding: '3px 9px',
                    background: usuario.active ? 'var(--color-accent-100)' : 'var(--color-neutral-200)',
                    color: usuario.active ? 'var(--color-accent-800)' : 'var(--color-neutral-700)',
                  }}
                >
                  <span
                    style={{
                      width: 6,
                      height: 6,
                      background: usuario.active ? 'var(--color-accent)' : 'var(--color-neutral-500)',
                    }}
                  />
                  {usuario.active ? 'Ativo' : 'Inativo'}
                </span>
              </span>
            </div>
          );
        })}
      </div>

      {modalAberto && (
        <NovoMedicoModal
          onFechar={() => setModalAberto(false)}
          onCriar={() => {
            setModalAberto(false);
            onToast('Médico criado. Credenciais provisórias enviadas por e-mail.');
          }}
        />
      )}
    </div>
  );
}

/**
 * Tela 2b do design — modal "Novo médico".
 * O perfil é fixo em "Médico": gestor não cria administradores nem outros
 * gestores por este fluxo. A regra real é do backend (Fase 7); aqui é só o campo.
 */
function NovoMedicoModal({ onFechar, onCriar }: { onFechar: () => void; onCriar: () => void }) {
  const [exigirTroca, setExigirTroca] = useState(true);

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Novo médico"
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 30,
        display: 'grid',
        placeItems: 'center',
        background: 'color-mix(in srgb, var(--color-neutral-900) 50%, transparent)',
      }}
    >
      <div
        className="blueprint"
        style={{
          width: 540,
          background: 'var(--color-bg)',
          boxShadow: 'var(--shadow-lg)',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <BlueprintCorners />
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '22px 28px 0',
          }}
        >
          <h3 style={{ margin: 0, fontSize: 26 }}>Novo médico</h3>
          <button
            type="button"
            className="pacs-hover-text"
            onClick={onFechar}
            title="Fechar"
            aria-label="Fechar"
            style={{
              width: 36,
              height: 36,
              display: 'grid',
              placeItems: 'center',
              border: 0,
              background: 'transparent',
              color: 'var(--color-neutral-700)',
              cursor: 'pointer',
            }}
          >
            <Icon name="x" size={20} />
          </button>
        </div>

        <div style={{ padding: '20px 28px', display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
          <div className="field" style={{ gridColumn: '1 / 3' }}>
            <label htmlFor="novo-nome">Nome completo</label>
            <input
              id="novo-nome"
              className="input"
              style={{ minHeight: 42, background: 'var(--color-bg)' }}
              placeholder="Ex.: Ana Paula Rossetto"
            />
          </div>
          <div className="field">
            <label htmlFor="novo-usuario">Usuário</label>
            <input
              id="novo-usuario"
              className="input"
              style={{ minHeight: 42, background: 'var(--color-bg)' }}
              placeholder="nome.sobrenome"
            />
          </div>
          <div className="field">
            <label htmlFor="novo-email">E-mail</label>
            <input
              id="novo-email"
              className="input"
              style={{ minHeight: 42, background: 'var(--color-bg)' }}
              placeholder="nome@saude.fb.pr.gov.br"
            />
          </div>
          <div className="field">
            <label>Unidade</label>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                height: 42,
                padding: '0 12px',
                border: '1px solid var(--color-divider)',
                fontSize: 14,
                color: 'var(--color-neutral-700)',
              }}
            >
              Selecione a unidade
              <span>
                <Icon name="down" />
              </span>
            </div>
          </div>
          <div className="field">
            <label>Validade do acesso</label>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                height: 42,
                padding: '0 12px',
                border: '1px solid var(--color-divider)',
                fontSize: 14,
              }}
            >
              <Icon name="calendar" />
              31/12/2026
            </div>
          </div>
          <div className="field" style={{ gridColumn: '1 / 3' }}>
            <label>Perfil</label>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                height: 42,
                padding: '0 12px',
                background: 'var(--color-neutral-200)',
                fontSize: 14,
                fontWeight: 600,
                color: 'var(--color-neutral-800)',
              }}
            >
              <Icon name="lock" />
              Médico
              <span
                style={{
                  marginLeft: 'auto',
                  fontWeight: 400,
                  fontSize: 12.5,
                  color: 'var(--color-neutral-700)',
                }}
              >
                Somente visualização de exames
              </span>
            </div>
          </div>
          <button
            type="button"
            onClick={() => setExigirTroca((valor) => !valor)}
            aria-pressed={exigirTroca}
            style={{
              gridColumn: '1 / 3',
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              padding: '4px 0',
              border: 0,
              background: 'transparent',
              font: 'inherit',
              fontSize: 14,
              color: 'inherit',
              cursor: 'pointer',
              textAlign: 'left',
            }}
          >
            <span
              style={{
                width: 18,
                height: 18,
                flex: 'none',
                display: 'grid',
                placeItems: 'center',
                border: '1px solid var(--color-accent)',
                background: exigirTroca ? 'var(--color-accent)' : 'transparent',
                color: 'var(--color-bg)',
              }}
            >
              {exigirTroca && <Icon name="check" />}
            </span>
            Exigir alteração da senha no primeiro acesso
          </button>
        </div>

        <div
          style={{
            display: 'flex',
            justifyContent: 'flex-end',
            gap: 10,
            padding: '16px 28px 22px',
            borderTop: '1px solid var(--color-divider)',
          }}
        >
          <button
            type="button"
            className="btn btn-secondary"
            onClick={onFechar}
            style={{ height: 42, padding: '0 18px', fontSize: 15 }}
          >
            Cancelar
          </button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={onCriar}
            style={{ height: 42, padding: '0 20px', fontSize: 15 }}
          >
            Criar médico
          </button>
        </div>
      </div>
    </div>
  );
}
