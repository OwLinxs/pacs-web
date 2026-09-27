import { useCallback, useEffect, useState } from 'react';
import { ApiError, api, type OrthancSettings } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';
import { Icon } from '../design-system/Icon';

/**
 * Tela Administração → Configurações → PACS / Orthanc.
 *
 * Construída no sistema Industry, seguindo o padrão das outras telas
 * administrativas: título de 40 px, cartão blueprint com marcas de registro,
 * campos `.field`/`.input`, ações no rodapé do cartão.
 *
 * Salvar a configuração NÃO estabelece conexão com o Orthanc: o backend apenas
 * guarda os dados, com a credencial cifrada. Testar conexão consulta somente
 * /system pelo backend, usando a configuração já salva.
 */

type ConfiguracoesScreenProps = {
  onToast: (mensagem: string) => void;
};

type Formulario = {
  name: string;
  baseUrl: string;
  username: string;
  dicomWebPath: string;
  timeoutSeconds: string;
  verifyTls: boolean;
};

const FORMULARIO_VAZIO: Formulario = {
  name: '',
  baseUrl: '',
  username: '',
  dicomWebPath: '',
  timeoutSeconds: '10',
  verifyTls: true,
};

function paraFormulario(config: OrthancSettings): Formulario {
  return {
    name: config.name,
    baseUrl: config.baseUrl,
    username: config.username,
    dicomWebPath: config.dicomWebPath,
    timeoutSeconds: String(config.timeoutSeconds || 10),
    verifyTls: config.verifyTls,
  };
}

export function ConfiguracoesScreen({ onToast }: ConfiguracoesScreenProps) {
  const [config, setConfig] = useState<OrthancSettings | null>(null);
  const [form, setForm] = useState<Formulario>(FORMULARIO_VAZIO);
  const [carregando, setCarregando] = useState(true);
  const [salvando, setSalvando] = useState(false);
  const [testando, setTestando] = useState(false);
  const [erro, setErro] = useState<string | null>(null);

  // Credencial: enquanto existe uma guardada, o campo só aparece quando o
  // administrador escolhe alterá-la. Salvar o resto não a substitui.
  const [alterandoCredencial, setAlterandoCredencial] = useState(false);
  const [credencial, setCredencial] = useState('');
  const [removerCredencial, setRemoverCredencial] = useState(false);

  const carregar = useCallback(async () => {
    setCarregando(true);
    setErro(null);
    try {
      const atual = await api.getOrthancSettings();
      setConfig(atual);
      setForm(paraFormulario(atual));
      setAlterandoCredencial(!atual.credentialConfigured);
      setCredencial('');
      setRemoverCredencial(false);
    } catch (falha) {
      setErro(
        falha instanceof ApiError
          ? falha.message
          : 'Não foi possível carregar a configuração. Verifique sua conexão.',
      );
    } finally {
      setCarregando(false);
    }
  }, []);

  useEffect(() => {
    void carregar();
  }, [carregar]);

  function alterar<C extends keyof Formulario>(campo: C, valor: Formulario[C]) {
    setForm((atual) => ({ ...atual, [campo]: valor }));
  }

  const temAlteracoes = config !== null && (
    JSON.stringify(form) !== JSON.stringify(paraFormulario(config)) ||
    removerCredencial || (alterandoCredencial && credencial !== '')
  );
  const ocupado = salvando || testando;

  async function testarConexao() {
    if (ocupado || !config?.connectionTestAvailable || temAlteracoes) return;
    setTestando(true);
    setErro(null);
    try {
      const resultado = await api.testOrthancConnection();
      setConfig((atual) => atual ? {
        ...atual, status: resultado.status, lastCheckedAt: resultado.checkedAt,
      } : atual);
      if (resultado.success) onToast(resultado.message);
      else setErro(resultado.message);
    } catch (falha) {
      setErro(falha instanceof ApiError ? falha.message : 'Não foi possível concluir o teste. Verifique sua conexão.');
    } finally {
      setTestando(false);
    }
  }

  async function salvar(event: React.FormEvent) {
    event.preventDefault();
    if (ocupado) return;

    setErro(null);
    setSalvando(true);
    try {
      const entrada = {
        name: form.name,
        baseUrl: form.baseUrl,
        username: form.username,
        dicomWebPath: form.dicomWebPath,
        timeoutSeconds: Number(form.timeoutSeconds) || 0,
        verifyTls: form.verifyTls,
        // Campo ausente preserva a credencial; string vazia a remove.
        ...(removerCredencial
          ? { credential: '' }
          : alterandoCredencial && credencial
            ? { credential: credencial }
            : {}),
      };

      const salva = await api.putOrthancSettings(entrada);
      setConfig(salva);
      setForm(paraFormulario(salva));
      setAlterandoCredencial(!salva.credentialConfigured);
      setCredencial('');
      setRemoverCredencial(false);
      onToast('Configuração do PACS salva.');
    } catch (falha) {
      setErro(
        falha instanceof ApiError
          ? falha.message
          : 'Não foi possível salvar a configuração. Verifique sua conexão.',
      );
    } finally {
      setSalvando(false);
    }
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
        <h1 style={{ margin: 0, fontSize: 40 }}>Configurações</h1>
        <span style={{ fontSize: 14, color: 'var(--color-neutral-700)' }}>
          Integração com o servidor de imagens
        </span>
      </div>

      <form
        className="blueprint"
        onSubmit={salvar}
        style={{ display: 'flex', flexDirection: 'column', background: 'var(--color-bg)' }}
      >
        <BlueprintCorners />

        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 14,
            padding: '20px 28px',
            borderBottom: '1px solid var(--color-divider)',
          }}
        >
          <div
            style={{
              width: 36,
              height: 36,
              flex: 'none',
              display: 'grid',
              placeItems: 'center',
              border: '1px solid var(--color-accent)',
              color: 'var(--color-accent-700)',
            }}
          >
            <Icon name="server" size={20} />
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.25 }}>
            <h3 style={{ margin: 0, fontSize: 24 }}>PACS / Orthanc</h3>
            <span style={{ fontSize: 13.5, color: 'var(--color-neutral-700)' }}>
              Endereço e credencial que o backend usa para falar com o servidor de imagens.
            </span>
          </div>
          <div style={{ marginLeft: 'auto' }}>
            <BadgeStatus config={config} carregando={carregando} />
            {config?.lastCheckedAt && (
              <div style={{ fontSize: 12, color: 'var(--color-neutral-700)', marginTop: 4 }}>
                Última verificação: {new Date(config.lastCheckedAt).toLocaleString('pt-BR')}
              </div>
            )}
          </div>
        </div>

        {carregando ? (
          <Esqueleto />
        ) : (
          <>
            <div
              style={{
                padding: '24px 28px',
                display: 'grid',
                gridTemplateColumns: '1fr 1fr',
                gap: 20,
              }}
            >
              <div className="field" style={{ gridColumn: '1 / 3' }}>
                <label htmlFor="cfg-nome">Nome da conexão</label>
                <input
                  id="cfg-nome"
                  className="input"
                  style={{ minHeight: 42, background: 'var(--color-bg)' }}
                  placeholder="Ex.: Orthanc Principal"
                  value={form.name}
                  disabled={ocupado}
                  onChange={(event) => alterar('name', event.target.value)}
                />
              </div>

              <div className="field" style={{ gridColumn: '1 / 3' }}>
                <label htmlFor="cfg-url">URL base do Orthanc</label>
                <input
                  id="cfg-url"
                  className="input"
                  style={{
                    minHeight: 42,
                    background: 'var(--color-bg)',
                    fontFamily: 'ui-monospace,Menlo,monospace',
                    fontSize: 13.5,
                  }}
                  placeholder="http://servidor-interno:8042"
                  value={form.baseUrl}
                  disabled={ocupado}
                  onChange={(event) => alterar('baseUrl', event.target.value)}
                />
                <span
                  style={{
                    display: 'block',
                    marginTop: 5,
                    fontSize: 12,
                    color: 'var(--color-neutral-700)',
                  }}
                >
                  Endereço acessível pelo backend. Não coloque usuário ou senha na URL.
                </span>
              </div>

              <div className="field">
                <label htmlFor="cfg-usuario">Usuário</label>
                <input
                  id="cfg-usuario"
                  className="input"
                  style={{ minHeight: 42, background: 'var(--color-bg)' }}
                  placeholder="deixe vazio se não houver autenticação"
                  value={form.username}
                  disabled={ocupado}
                  onChange={(event) => alterar('username', event.target.value)}
                />
              </div>

              <CampoCredencial
                config={config}
                alterando={alterandoCredencial}
                removendo={removerCredencial}
                valor={credencial}
                desabilitado={ocupado}
                onAlterar={setCredencial}
                onIniciarAlteracao={() => {
                  setAlterandoCredencial(true);
                  setRemoverCredencial(false);
                }}
                onCancelarAlteracao={() => {
                  setAlterandoCredencial(false);
                  setCredencial('');
                }}
                onRemover={() => {
                  setRemoverCredencial(true);
                  setAlterandoCredencial(false);
                  setCredencial('');
                }}
                onCancelarRemocao={() => setRemoverCredencial(false)}
              />

              <div className="field">
                <label htmlFor="cfg-dicomweb">Endpoint DICOMweb</label>
                <input
                  id="cfg-dicomweb"
                  className="input"
                  style={{
                    minHeight: 42,
                    background: 'var(--color-bg)',
                    fontFamily: 'ui-monospace,Menlo,monospace',
                    fontSize: 13.5,
                  }}
                  placeholder="/dicom-web"
                  value={form.dicomWebPath}
                  disabled={ocupado}
                  onChange={(event) => alterar('dicomWebPath', event.target.value)}
                />
              </div>

              <div className="field">
                <label htmlFor="cfg-timeout">Timeout (segundos)</label>
                <input
                  id="cfg-timeout"
                  className="input"
                  type="number"
                  min={1}
                  max={120}
                  style={{ minHeight: 42, background: 'var(--color-bg)', width: 140 }}
                  value={form.timeoutSeconds}
                  disabled={ocupado}
                  onChange={(event) => alterar('timeoutSeconds', event.target.value)}
                />
              </div>

              <div className="field" style={{ gridColumn: '1 / 3' }}>
                <label>Verificação TLS</label>
                <button
                  type="button"
                  onClick={() => alterar('verifyTls', !form.verifyTls)}
                  aria-pressed={form.verifyTls}
                  disabled={ocupado}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 10,
                    height: 42,
                    padding: '0 2px',
                    border: 0,
                    background: 'transparent',
                    font: 'inherit',
                    fontSize: 14,
                    color: 'inherit',
                    cursor: ocupado ? 'not-allowed' : 'pointer',
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
                      background: form.verifyTls ? 'var(--color-accent)' : 'transparent',
                      color: 'var(--color-bg)',
                    }}
                  >
                    {form.verifyTls && <Icon name="check" />}
                  </span>
                  Verificar o certificado do servidor em conexões HTTPS
                </button>
              </div>
            </div>

            {erro && (
              <div
                role="alert"
                style={{
                  display: 'flex',
                  alignItems: 'flex-start',
                  gap: 10,
                  margin: '0 28px 20px',
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

            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 12,
                padding: '16px 28px 20px',
                borderTop: '1px solid var(--color-divider)',
              }}
            >
              <span
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  fontSize: 12.5,
                  color: 'var(--color-neutral-700)',
                }}
              >
                <Icon name="lock" />
                Salvar não conecta ao Orthanc. A credencial é guardada cifrada e nunca volta ao
                navegador. {temAlteracoes ? 'Salve as alterações antes de testar.' : 'O teste usa a configuração salva.'}
              </span>

              <div style={{ marginLeft: 'auto', display: 'flex', gap: 10 }}>
                <button
                  type="button"
                  className="btn btn-secondary"
                  disabled={ocupado || !config?.connectionTestAvailable || temAlteracoes}
                  onClick={() => void testarConexao()}
                  aria-busy={testando}
                  title={temAlteracoes ? 'Salve as alterações antes de testar.' : 'Testar a configuração salva do PACS.'}
                  style={{ height: 42, padding: '0 18px', fontSize: 15, gap: 8 }}
                >
                  <Icon name="plug" />
                  {testando ? 'Testando…' : 'Testar conexão'}
                </button>
                <button
                  type="submit"
                  className="btn btn-primary"
                  disabled={ocupado}
                  style={{ height: 42, padding: '0 20px', fontSize: 15 }}
                >
                  {salvando ? 'Salvando…' : 'Salvar configuração'}
                </button>
              </div>
            </div>
          </>
        )}
      </form>
    </div>
  );
}

/** Badge do estado da conexão, no padrão dos badges de status do design. */
function BadgeStatus({ config, carregando }: { config: OrthancSettings | null; carregando: boolean }) {
  if (carregando) return null;

  const naoConfigurado = !config?.configured;
  const rotulo = naoConfigurado
    ? 'Não configurado'
    : config.status === 'connected'
      ? 'Conectado'
      : config.status === 'failed'
        ? 'Falha na conexão'
        : 'Não verificada';

  const falha = config?.status === 'failed';
  const conectado = config?.status === 'connected';

  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        fontSize: 12,
        fontWeight: 600,
        padding: '3px 9px',
        background: falha
          ? 'oklch(0.94 0.035 25)'
          : conectado
            ? 'var(--color-accent-100)'
            : 'var(--color-neutral-200)',
        color: falha
          ? 'oklch(0.42 0.13 25)'
          : conectado
            ? 'var(--color-accent-800)'
            : 'var(--color-neutral-700)',
      }}
    >
      <span
        style={{
          width: 6,
          height: 6,
          background: falha
            ? 'oklch(0.55 0.14 25)'
            : conectado
              ? 'var(--color-accent)'
              : 'var(--color-neutral-500)',
        }}
      />
      {rotulo}
    </span>
  );
}

type CampoCredencialProps = {
  config: OrthancSettings | null;
  alterando: boolean;
  removendo: boolean;
  valor: string;
  desabilitado: boolean;
  onAlterar: (valor: string) => void;
  onIniciarAlteracao: () => void;
  onCancelarAlteracao: () => void;
  onRemover: () => void;
  onCancelarRemocao: () => void;
};

/**
 * Campo de credencial.
 *
 * Com credencial guardada, mostra apenas que ela existe — o valor nunca chega ao
 * navegador. Alterar é uma ação explícita, e salvar as outras configurações não
 * mexe na credencial.
 */
function CampoCredencial({
  config,
  alterando,
  removendo,
  valor,
  desabilitado,
  onAlterar,
  onIniciarAlteracao,
  onCancelarAlteracao,
  onRemover,
  onCancelarRemocao,
}: CampoCredencialProps) {
  const guardada = config?.credentialConfigured ?? false;
  const suportada = config?.credentialSupported ?? true;

  if (!suportada) {
    return (
      <div className="field">
        <label>Credencial</label>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            minHeight: 42,
            padding: '0 12px',
            background: 'var(--color-neutral-200)',
            fontSize: 13.5,
            color: 'var(--color-neutral-700)',
          }}
        >
          <Icon name="alert" />
          Este servidor não está preparado para guardar credenciais.
        </div>
      </div>
    );
  }

  if (removendo) {
    return (
      <div className="field">
        <label>Credencial</label>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            minHeight: 42,
            padding: '0 12px',
            background: 'oklch(0.94 0.035 25)',
            color: 'oklch(0.42 0.13 25)',
            fontSize: 13.5,
          }}
        >
          Será removida ao salvar
          <button
            type="button"
            className="btn btn-ghost"
            onClick={onCancelarRemocao}
            disabled={desabilitado}
            style={{ marginLeft: 'auto', fontSize: 13 }}
          >
            Manter
          </button>
        </div>
      </div>
    );
  }

  if (guardada && !alterando) {
    return (
      <div className="field">
        <label>Credencial</label>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            minHeight: 42,
            padding: '0 12px',
            border: '1px solid var(--color-divider)',
            background: 'var(--color-bg)',
          }}
        >
          <span
            style={{
              fontFamily: 'ui-monospace,Menlo,monospace',
              fontSize: 14,
              letterSpacing: '.12em',
              color: 'var(--color-neutral-700)',
            }}
          >
            ••••••••••••
          </span>
          <button
            type="button"
            className="btn btn-ghost"
            onClick={onIniciarAlteracao}
            disabled={desabilitado}
            style={{ marginLeft: 'auto', fontSize: 13 }}
          >
            Alterar credencial
          </button>
          <button
            type="button"
            className="btn btn-ghost"
            onClick={onRemover}
            disabled={desabilitado}
            style={{ fontSize: 13, color: 'oklch(0.45 0.14 25)' }}
          >
            Remover
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="field">
      <label htmlFor="cfg-credencial">{guardada ? 'Nova credencial' : 'Credencial'}</label>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <input
          id="cfg-credencial"
          className="input"
          type="password"
          autoComplete="new-password"
          style={{ minHeight: 42, background: 'var(--color-bg)' }}
          placeholder={guardada ? 'digite a nova credencial' : 'deixe vazio se não houver'}
          value={valor}
          disabled={desabilitado}
          onChange={(event) => onAlterar(event.target.value)}
        />
        {guardada && (
          <button
            type="button"
            className="btn btn-ghost"
            onClick={onCancelarAlteracao}
            disabled={desabilitado}
            style={{ fontSize: 13, flex: 'none' }}
          >
            Cancelar
          </button>
        )}
      </div>
    </div>
  );
}

/** Esqueleto enquanto a configuração carrega, no padrão do design. */
function Esqueleto() {
  return (
    <div
      style={{
        padding: '24px 28px',
        display: 'flex',
        flexDirection: 'column',
        gap: 18,
        animation: 'pacsShimmer 1.4s ease-in-out infinite',
      }}
    >
      {[70, 100, 45, 45, 60].map((largura, i) => (
        <div key={i} style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
          <div style={{ height: 8, width: 90, background: 'var(--color-neutral-200)' }} />
          <div style={{ height: 14, width: `${largura}%`, background: 'var(--color-neutral-300)' }} />
        </div>
      ))}
    </div>
  );
}
