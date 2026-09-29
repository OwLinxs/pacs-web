import { useState, type ReactNode } from 'react';
import { iniciaisDe, rotuloDePerfil, useSession } from '../auth/SessionProvider';
import { Icon, type IconName } from '../design-system/Icon';

/** Telas que vivem dentro do shell (sidebar + header). */
export type AppScreen = 'exames' | 'usuarios' | 'auditoria' | 'configuracoes' | 'unidades';

/** `apenasAdmin` esconde o item de quem não é ADMIN. */
type NavItem = { key: AppScreen; icon: IconName; label: string; apenasAdmin?: boolean };

const NAV_PRINCIPAL: NavItem[] = [{ key: 'exames', icon: 'file', label: 'Exames' }];
const NAV_ADMIN: NavItem[] = [
  { key: 'unidades', icon: 'layers', label: 'Unidades', apenasAdmin: true },
  { key: 'usuarios', icon: 'users', label: 'Usuários' },
  { key: 'auditoria', icon: 'shield', label: 'Auditoria' },
  { key: 'configuracoes', icon: 'settings', label: 'Configurações', apenasAdmin: true },
];

type AppShellProps = {
  screen: AppScreen;
  onNavigate: (screen: AppScreen) => void;
  onLogout: () => void;
  onExpirar: () => void;
  onIndisponivel: () => void;
  children: ReactNode;
};

/**
 * Moldura das telas administrativas e da worklist: barra lateral fixa à
 * esquerda e header com a identidade do médico sempre visível — o design
 * assume computadores compartilhados.
 */
export function AppShell({
  screen,
  onNavigate,
  onLogout,
  onExpirar,
  onIndisponivel,
  children,
}: AppShellProps) {
  const [menuAberto, setMenuAberto] = useState(false);
  const { user } = useSession();

  // Sem sessão o shell não é montado; este fallback só evita quebra visual.
  const nome = user?.name ?? '—';
  const perfil = user ? rotuloDePerfil(user.role) : '—';
  const unidade = user?.unit?.name ?? 'Sem unidade';
  const desde = horaDoUltimoAcesso(user?.lastLoginAt ?? null);

  // A barra lateral esconde a administração de quem não administra. Isto é
  // conveniência visual: a autorização real é do backend, que recusa as rotas.
  const podeAdministrar = user?.role === 'ADMIN' || user?.role === 'GESTOR';
  const ehAdmin = user?.role === 'ADMIN';
  const itensAdmin = NAV_ADMIN.filter((item) => !item.apenasAdmin || ehAdmin);

  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        display: 'grid',
        gridTemplateColumns: '232px minmax(0,1fr)',
        gridTemplateRows: '64px minmax(0,1fr)',
      }}
    >
      <aside
        style={{
          gridRow: '1 / 3',
          borderRight: '1px solid var(--color-divider)',
          display: 'flex',
          flexDirection: 'column',
          padding: '0 12px 16px',
        }}
      >
        <div
          style={{
            height: 64,
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            borderBottom: '1px solid var(--color-divider)',
            margin: '0 -12px 16px',
            paddingLeft: 20,
            paddingRight: 8,
          }}
        >
          <div
            style={{
              width: 26,
              height: 26,
              border: '1px solid var(--color-accent)',
              display: 'grid',
              placeItems: 'center',
            }}
          >
            <div style={{ width: 10, height: 10, background: 'var(--color-accent)' }} />
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.15 }}>
            <span
              style={{
                fontFamily: 'var(--font-heading)',
                fontWeight: 600,
                fontSize: 20,
                letterSpacing: '.02em',
              }}
            >
              PACS
            </span>
            <span style={{ fontSize: 11, color: 'var(--color-neutral-700)' }}>
              Secretaria Municipal de Saúde
            </span>
          </div>
        </div>

        <nav style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {NAV_PRINCIPAL.map((item) => (
            <NavButton key={item.key} item={item} ativo={screen === item.key} onPick={onNavigate} />
          ))}
          {podeAdministrar && (
            <div
              style={{
                fontSize: 10.5,
                letterSpacing: '.1em',
                textTransform: 'uppercase',
                color: 'var(--color-neutral-600)',
                padding: '20px 12px 8px',
              }}
            >
              Administração
            </div>
          )}
          {podeAdministrar &&
            itensAdmin.map((item) => (
              <NavButton key={item.key} item={item} ativo={screen === item.key} onPick={onNavigate} />
            ))}
        </nav>

        <div
          style={{
            marginTop: 'auto',
            padding: 12,
            fontSize: 11.5,
            lineHeight: 1.5,
            color: 'var(--color-neutral-600)',
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
          }}
        >
          <span>Prefeitura de Francisco Beltrão</span>
          <span>PACS Web Municipal · v1.0</span>
        </div>
      </aside>

      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 20,
          padding: '0 24px 0 40px',
          borderBottom: '1px solid var(--color-divider)',
          position: 'relative',
          zIndex: 5,
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            fontSize: 12.5,
            color: 'var(--color-neutral-700)',
            whiteSpace: 'nowrap',
          }}
        >
          <Icon name="lock" />
          <span>Sessão protegida · expira após 30 min sem atividade</span>
        </div>

        <div style={{ marginLeft: 'auto', position: 'relative' }}>
          <button
            type="button"
            className="pacs-hover-text"
            onClick={() => setMenuAberto((aberto) => !aberto)}
            aria-expanded={menuAberto}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 12,
              height: 48,
              padding: '0 10px 0 6px',
              border: '1px solid transparent',
              background: 'transparent',
              font: 'inherit',
              color: 'inherit',
              cursor: 'pointer',
              textAlign: 'left',
            }}
          >
            <div
              style={{
                width: 36,
                height: 36,
                display: 'grid',
                placeItems: 'center',
                background: 'var(--color-accent-800)',
                color: 'var(--color-accent-100)',
                fontFamily: 'var(--font-heading)',
                fontWeight: 600,
                fontSize: 15,
                letterSpacing: '.04em',
              }}
            >
              {iniciaisDe(nome)}
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.25, whiteSpace: 'nowrap' }}>
              <span style={{ fontSize: 14.5, fontWeight: 600 }}>{nome}</span>
              <span style={{ fontSize: 12, color: 'var(--color-neutral-700)' }}>
                {perfil} · {unidade}
              </span>
            </div>
            <span style={{ color: 'var(--color-neutral-600)', marginLeft: 4 }}>
              <Icon name="down" />
            </span>
          </button>

          {menuAberto && (
            <div
              style={{
                position: 'absolute',
                right: 0,
                top: 54,
                width: 280,
                background: 'var(--color-bg)',
                border: '1px solid var(--color-divider)',
                boxShadow: 'var(--shadow-md)',
                padding: 6,
                display: 'flex',
                flexDirection: 'column',
                zIndex: 20,
              }}
            >
              <div
                style={{
                  padding: '10px 12px 12px',
                  borderBottom: '1px solid var(--color-divider)',
                  marginBottom: 6,
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 2,
                }}
              >
                <span style={{ fontSize: 12, color: 'var(--color-neutral-700)' }}>Autenticado como</span>
                <span style={{ fontWeight: 600 }}>{user?.username ?? '—'}</span>
                <span style={{ fontSize: 12, color: 'var(--color-neutral-700)' }}>
                  {perfil}
                  {desde ? ` · desde ${desde}` : ''}
                </span>
              </div>
              <MenuItem icon="key" label="Alterar senha" />
              <MenuItem
                icon="clock"
                label="Simular sessão expirada"
                onClick={() => {
                  setMenuAberto(false);
                  onExpirar();
                }}
              />
              <MenuItem
                icon="server"
                label="Simular sistema indisponível"
                onClick={() => {
                  setMenuAberto(false);
                  onIndisponivel();
                }}
              />
              <MenuItem
                icon="logout"
                label="Sair"
                separado
                onClick={() => {
                  setMenuAberto(false);
                  onLogout();
                }}
              />
            </div>
          )}
        </div>

        <button
          type="button"
          className="btn btn-secondary"
          onClick={onLogout}
          style={{ height: 40, padding: '0 16px', fontSize: 15, gap: 8 }}
        >
          <Icon name="logout" />
          Sair
        </button>
      </header>

      <main style={{ position: 'relative', overflow: 'auto', minWidth: 0 }}>{children}</main>
    </div>
  );
}

function NavButton({
  item,
  ativo,
  onPick,
}: {
  item: NavItem;
  ativo: boolean;
  onPick: (screen: AppScreen) => void;
}) {
  return (
    <button
      type="button"
      className="pacs-hover-text"
      onClick={() => onPick(item.key)}
      aria-current={ativo ? 'page' : undefined}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        height: 42,
        padding: '0 12px',
        border: 0,
        background: ativo ? 'color-mix(in srgb, var(--color-accent) 12%, transparent)' : 'transparent',
        color: ativo ? 'var(--color-accent-800)' : 'var(--color-neutral-800)',
        font: 'inherit',
        fontSize: 15,
        fontWeight: ativo ? 600 : 500,
        cursor: 'pointer',
        textAlign: 'left',
      }}
    >
      <Icon name={item.icon} size={20} />
      <span>{item.label}</span>
    </button>
  );
}

function MenuItem({
  icon,
  label,
  onClick,
  separado = false,
}: {
  icon: IconName;
  label: string;
  onClick?: () => void;
  separado?: boolean;
}) {
  return (
    <button
      type="button"
      className="pacs-hover-accent"
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        height: 38,
        padding: '0 12px',
        border: 0,
        borderTop: separado ? '1px solid var(--color-divider)' : undefined,
        marginTop: separado ? 6 : undefined,
        background: 'transparent',
        font: 'inherit',
        fontSize: 14,
        color: 'inherit',
        cursor: 'pointer',
        textAlign: 'left',
      }}
    >
      <Icon name={icon} />
      {label}
    </button>
  );
}

/** Hora do último acesso, no formato HH:MM, para o menu do usuário. */
function horaDoUltimoAcesso(iso: string | null): string {
  if (!iso) return '';
  const data = new Date(iso);
  if (Number.isNaN(data.getTime())) return '';
  return data.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
}
