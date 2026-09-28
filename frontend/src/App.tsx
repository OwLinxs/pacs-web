import { useCallback, useEffect, useState } from 'react';
import { AppShell, type AppScreen } from './app/AppShell';
import { useSession } from './auth/SessionProvider';
import { Toast } from './components/Toast';
import { studyIDFromRoute, viewerRoute, type Study } from './api/client';
import { AuditoriaScreen } from './screens/AuditoriaScreen';
import { ConfiguracoesScreen } from './screens/ConfiguracoesScreen';
import { SessaoExpiradaScreen, IndisponivelScreen } from './screens/EstadoSistemaScreens';
import { initialWorklist } from './worklist/model';
import { ExamesScreen } from './screens/ExamesScreen';
import { LoginScreen } from './screens/LoginScreen';
import { UsuariosScreen } from './screens/UsuariosScreen';
import { ViewerScreen } from './screens/ViewerScreen';
import { PainelDeTelas } from './dev/PainelDeTelas';

export type Screen = AppScreen | 'login' | 'viewer' | 'expirada' | 'indisponivel';

/** Telas que fazem sentido sem sessão. */
const TELAS_SEM_SESSAO: Screen[] = ['login', 'expirada', 'indisponivel'];

/**
 * Estado de navegação da aplicação.
 *
 * Worklist e Viewer obtêm os dados da API. A rota do viewer contém apenas o ID Orthanc.
 */
export type DemoState = {
  screen: Screen;
  studyId: string | null;
  modalNovoMedico: boolean;
};

const ESTADO_INICIAL: DemoState = {
  screen: studyIDFromRoute(window.location.pathname) ? 'viewer' : 'exames',
  studyId: studyIDFromRoute(window.location.pathname),
  modalNovoMedico: false,
};

export function App() {
  const sessao = useSession();
  const [demo, setDemo] = useState<DemoState>(ESTADO_INICIAL);
  const [selectedStudy, setSelectedStudy] = useState<Study | null>(null);
  const [worklist, setWorklist] = useState(initialWorklist);
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

  useEffect(() => {
    const onPopState = () => {
      const studyId = studyIDFromRoute(window.location.pathname);
      setSelectedStudy(null);
      setDemo((atual) => ({ ...atual, screen: studyId ? 'viewer' : 'exames', studyId }));
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  function irPara(screen: Screen) {
    if (TELAS_SEM_SESSAO.includes(screen)) setWorklist(initialWorklist());
    if (screen !== 'viewer' && window.location.pathname.startsWith('/viewer/')) window.history.pushState(null, '', '/');
    setSelectedStudy(null);
    setDemo((atual) => ({ ...atual, screen, studyId: null, modalNovoMedico: false }));
  }

  function abrirExame(study: Study) {
    window.history.pushState(null, '', viewerRoute(study.orthancStudyId));
    setSelectedStudy(study);
    setDemo((atual) => ({ ...atual, screen: 'viewer', studyId: study.orthancStudyId }));
  }

  const expirarSessao = useCallback(() => {
    setWorklist(initialWorklist());
    setSelectedStudy(null);
    setDemo((atual) => ({ ...atual, screen: 'expirada' }));
  }, []);

  async function sair() {
    setWorklist(initialWorklist());
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

    if (demo.screen === 'viewer' && demo.studyId) {
      return (
        <ViewerScreen
          key={demo.studyId}
          studyId={demo.studyId}
          study={selectedStudy?.orthancStudyId === demo.studyId ? selectedStudy : null}
          onSessionExpired={expirarSessao}
          onVoltar={() => irPara('exames')}
          onLogout={() => void sair()}
        />
      );
    }

    const telaDoShell: AppScreen = demo.screen === 'login' || demo.screen === 'viewer' ? 'exames' : demo.screen;

    return (
      <AppShell
        screen={telaDoShell}
        onNavigate={irPara}
        onLogout={() => void sair()}
        onExpirar={() => irPara('expirada')}
        onIndisponivel={() => irPara('indisponivel')}
      >
        {telaDoShell === 'exames' && <ExamesScreen state={worklist} onStateChange={setWorklist} onSessionExpired={expirarSessao} onAbrirExame={abrirExame} />}
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
