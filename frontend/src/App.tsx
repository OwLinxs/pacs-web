import { useCallback, useEffect, useState } from 'react';
import { AppShell, type AppScreen } from './app/AppShell';
import { useSession } from './auth/SessionProvider';
import { Toast } from './components/Toast';
import { exameById } from './mocks/dados';
import { AuditoriaScreen } from './screens/AuditoriaScreen';
import { ConfiguracoesScreen } from './screens/ConfiguracoesScreen';
import { SessaoExpiradaScreen, IndisponivelScreen } from './screens/EstadoSistemaScreens';
import { ExamesScreen } from './screens/ExamesScreen';
import { LoginScreen } from './screens/LoginScreen';
import { UsuariosScreen } from './screens/UsuariosScreen';
import { ViewerScreen, type ViewerState } from './screens/ViewerScreen';
import { PainelDeTelas } from './dev/PainelDeTelas';

export type Screen = AppScreen | 'login' | 'viewer' | 'expirada' | 'indisponivel';

/** Telas que fazem sentido sem sessão. */
const TELAS_SEM_SESSAO: Screen[] = ['login', 'expirada', 'indisponivel'];

/**
 * Estado de navegação da aplicação.
 *
 * O painel de desenvolvimento mantém os estados do protótipo do viewer.
 * A Worklist obtém seus estados e dados exclusivamente da API.
 */
export type DemoState = {
  screen: Screen;
  exameId: number;
  viewerState: ViewerState;
  modalNovoMedico: boolean;
};

const ESTADO_INICIAL: DemoState = {
  screen: 'exames',
  exameId: 1,
  viewerState: 'ok',
  modalNovoMedico: false,
};

export function App() {
  const sessao = useSession();
  const [demo, setDemo] = useState<DemoState>(ESTADO_INICIAL);
  const [toast, setToast] = useState<string | null>(null);

  useEffect(() => {
    if (!toast) return;
    const id = window.setTimeout(() => setToast(null), 3200);
    return () => window.clearTimeout(id);
  }, [toast]);

  // Ao entrar, a aplicação começa sempre na worklist.
  useEffect(() => {
    if (sessao.status === 'authenticated') {
      setDemo((atual) => (TELAS_SEM_SESSAO.includes(atual.screen) ? { ...atual, screen: 'exames' } : atual));
    }
  }, [sessao.status]);

  function irPara(screen: Screen) {
    setDemo((atual) => ({ ...atual, screen, modalNovoMedico: false }));
  }

  const expirarSessao = useCallback(() => {
    setDemo((atual) => ({ ...atual, screen: 'expirada' }));
  }, []);

  async function sair() {
    await sessao.logout();
    irPara('login');
  }

  return (
    <>
      {renderTela()}
      {toast && <Toast mensagem={toast} />}
      {import.meta.env.DEV && <PainelDeTelas demo={demo} onMudar={setDemo} />}
    </>
  );

  function renderTela() {
    // Enquanto GET /api/auth/me não responde, nada de piscar tela de login.
    if (sessao.status === 'loading') {
      return <div className="pacs-grid-ground" style={{ position: 'absolute', inset: 0 }} />;
    }

    // Telas de estado do sistema valem com ou sem sessão.
    if (demo.screen === 'expirada') {
      return <SessaoExpiradaScreen onEntrarNovamente={() => void sair()} />;
    }
    if (demo.screen === 'indisponivel') {
      return <IndisponivelScreen onTentarNovamente={() => void sessao.refresh()} />;
    }

    if (sessao.status !== 'authenticated') {
      return <LoginScreen onSubmit={sessao.login} />;
    }

    if (demo.screen === 'viewer') {
      return (
        <ViewerScreen
          exame={exameById(demo.exameId)}
          viewerState={demo.viewerState}
          onVoltar={() => irPara('exames')}
          onLogout={() => void sair()}
        />
      );
    }

    const telaDoShell: AppScreen = demo.screen === 'login' ? 'exames' : demo.screen;

    return (
      <AppShell
        screen={telaDoShell}
        onNavigate={irPara}
        onLogout={() => void sair()}
        onExpirar={() => irPara('expirada')}
        onIndisponivel={() => irPara('indisponivel')}
      >
        {telaDoShell === 'exames' && <ExamesScreen onSessionExpired={expirarSessao} />}
        {telaDoShell === 'usuarios' && (
          <UsuariosScreen
            key={demo.modalNovoMedico ? 'com-modal' : 'sem-modal'}
            modalInicial={demo.modalNovoMedico}
            onToast={setToast}
          />
        )}
        {telaDoShell === 'auditoria' && <AuditoriaScreen />}
        {telaDoShell === 'configuracoes' && <ConfiguracoesScreen onToast={setToast} />}
      </AppShell>
    );
  }
}
