import { useState } from 'react';
import { ApiError } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';
import { Icon } from '../design-system/Icon';

type LoginScreenProps = {
  /** Autentica no backend. Deve lançar ApiError quando a entrada é recusada. */
  onSubmit: (username: string, password: string) => Promise<void>;
};

/**
 * Tela 1a do design — "Login".
 *
 * A validação é toda do backend: esta tela só envia usuário e senha e mostra a
 * mensagem devolvida. Nenhuma credencial é guardada no navegador.
 */
export function LoginScreen({ onSubmit }: LoginScreenProps) {
  const [user, setUser] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [erro, setErro] = useState<string | null>(null);

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    if (submitting) return;

    setErro(null);
    setSubmitting(true);
    try {
      await onSubmit(user, password);
    } catch (falha) {
      setErro(
        falha instanceof ApiError
          ? falha.message
          : 'Não foi possível entrar. Verifique sua conexão e tente novamente.',
      );
      setPassword('');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      className="pacs-grid-ground"
      style={{ position: 'absolute', inset: 0, display: 'grid', gridTemplateRows: 'auto 1fr auto' }}
    >
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '24px 32px',
          fontSize: 12,
          color: 'var(--color-neutral-700)',
          letterSpacing: '.06em',
          textTransform: 'uppercase',
        }}
      >
        <span>Prefeitura de Francisco Beltrão · PR</span>
        <span>Secretaria Municipal de Saúde</span>
      </div>

      <div style={{ display: 'grid', placeItems: 'center' }}>
        <form
          className="blueprint"
          onSubmit={handleSubmit}
          style={{
            width: 400,
            background: 'var(--color-bg)',
            padding: '40px 40px 32px',
            display: 'flex',
            flexDirection: 'column',
            gap: 24,
          }}
        >
          <BlueprintCorners />

          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div
              style={{
                width: 34,
                height: 34,
                border: '1px solid var(--color-accent)',
                display: 'grid',
                placeItems: 'center',
              }}
            >
              <div style={{ width: 14, height: 14, background: 'var(--color-accent)' }} />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.1 }}>
              <span
                style={{
                  fontFamily: 'var(--font-heading)',
                  fontWeight: 600,
                  fontSize: 26,
                  letterSpacing: '.02em',
                }}
              >
                PACS
              </span>
              <span style={{ fontSize: 12, color: 'var(--color-neutral-700)' }}>
                Secretaria Municipal de Saúde
              </span>
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <h2 style={{ margin: 0, fontSize: 30 }}>Acesso ao sistema</h2>
            <span style={{ fontSize: 14, color: 'var(--color-neutral-700)' }}>
              Entre com seu usuário institucional.
            </span>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div className="field">
              <label htmlFor="login-user">Usuário</label>
              <input
                id="login-user"
                className="input"
                style={{ minHeight: 44, fontSize: 15 }}
                autoComplete="username"
                disabled={submitting}
                value={user}
                onChange={(event) => setUser(event.target.value)}
              />
            </div>

            <div className="field">
              <label htmlFor="login-password">Senha</label>
              <div style={{ position: 'relative', display: 'flex' }}>
                <input
                  id="login-password"
                  className="input"
                  type={showPassword ? 'text' : 'password'}
                  style={{ minHeight: 44, fontSize: 15, paddingRight: 44 }}
                  autoComplete="current-password"
                  disabled={submitting}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((value) => !value)}
                  title={showPassword ? 'Ocultar senha' : 'Mostrar senha'}
                  aria-label={showPassword ? 'Ocultar senha' : 'Mostrar senha'}
                  style={{
                    position: 'absolute',
                    right: 0,
                    top: 0,
                    width: 44,
                    height: 44,
                    border: 0,
                    background: 'transparent',
                    color: 'var(--color-neutral-700)',
                    cursor: 'pointer',
                    display: 'grid',
                    placeItems: 'center',
                  }}
                >
                  <Icon name={showPassword ? 'eyeOff' : 'eye'} size={20} />
                </button>
              </div>
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: -6 }}>
              <a href="#" style={{ fontSize: 13 }}>
                Esqueci minha senha / recuperar acesso
              </a>
            </div>
          </div>

          {erro && (
            <div
              role="alert"
              style={{
                display: 'flex',
                alignItems: 'flex-start',
                gap: 10,
                marginTop: -8,
                padding: '10px 12px',
                background: 'oklch(0.94 0.035 25)',
                color: 'oklch(0.42 0.13 25)',
                fontSize: 13,
                lineHeight: 1.45,
              }}
            >
              <span style={{ flex: 'none', marginTop: 1 }}>
                <Icon name="alert" />
              </span>
              <span>{erro}</span>
            </div>
          )}

          <button
            type="submit"
            className="btn btn-primary blueprint"
            disabled={submitting}
            style={{ minHeight: 46, fontSize: 16, width: '100%', overflow: 'visible' }}
          >
            <BlueprintCorners />
            {submitting ? 'Entrando…' : 'Entrar'}
          </button>

          <div
            style={{
              display: 'flex',
              gap: 10,
              alignItems: 'flex-start',
              fontSize: 12,
              lineHeight: 1.45,
              color: 'var(--color-neutral-700)',
              paddingTop: 16,
              borderTop: '1px solid var(--color-divider)',
            }}
          >
            <span style={{ flex: 'none', marginTop: 1 }}>
              <Icon name="lock" />
            </span>
            <span>
              Acesso restrito a profissionais autorizados. Todos os acessos e visualizações são
              registrados.
            </span>
          </div>
        </form>
      </div>

      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          padding: '24px 32px',
          fontSize: 12,
          color: 'var(--color-neutral-600)',
        }}
      >
        <span>PACS Web Municipal · v1.0</span>
        <span>Suporte: Departamento de TI · Saúde</span>
      </div>
    </div>
  );
}
