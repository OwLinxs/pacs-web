import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { ApiError, api, type SessionUser } from '../api/client';

/** Situação da sessão no cliente. */
export type SessionStatus = 'loading' | 'authenticated' | 'anonymous';

type SessionContextValue = {
  status: SessionStatus;
  user: SessionUser | null;
  /** Autentica; lança ApiError para o chamador exibir a mensagem adequada. */
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  /** Reconsulta /api/auth/me. */
  refresh: () => Promise<void>;
  /** Preenchido quando a própria consulta de sessão falhou (backend fora do ar). */
  erroDeRede: boolean;
};

const SessionContext = createContext<SessionContextValue | null>(null);

/**
 * Mantém a sessão do usuário. A fonte da verdade é o backend: na carga da
 * aplicação e em cada refresh, GET /api/auth/me decide quem está autenticado.
 */
export function SessionProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<SessionStatus>('loading');
  const [user, setUser] = useState<SessionUser | null>(null);
  const [erroDeRede, setErroDeRede] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const atual = await api.me();
      setUser(atual);
      setStatus(atual ? 'authenticated' : 'anonymous');
      setErroDeRede(false);
    } catch {
      // Falha de rede ou backend indisponível: sem sessão, mas o motivo é outro.
      setUser(null);
      setStatus('anonymous');
      setErroDeRede(true);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(async (username: string, password: string) => {
    const autenticado = await api.login(username, password);
    setUser(autenticado);
    setStatus('authenticated');
    setErroDeRede(false);
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } catch (erro) {
      // Sessão já inválida no servidor não impede sair da aplicação.
      if (!(erro instanceof ApiError) || erro.status !== 401) {
        throw erro;
      }
    } finally {
      setUser(null);
      setStatus('anonymous');
    }
  }, []);

  const valor = useMemo<SessionContextValue>(
    () => ({ status, user, login, logout, refresh, erroDeRede }),
    [status, user, login, logout, refresh, erroDeRede],
  );

  return <SessionContext.Provider value={valor}>{children}</SessionContext.Provider>;
}

/** Acessa a sessão atual. Precisa estar dentro de SessionProvider. */
export function useSession(): SessionContextValue {
  const contexto = useContext(SessionContext);
  if (!contexto) {
    throw new Error('useSession precisa estar dentro de <SessionProvider>.');
  }
  return contexto;
}

/** Iniciais do nome, para o avatar do header. */
export function iniciaisDe(nome: string): string {
  const partes = nome.trim().split(/\s+/).filter(Boolean);
  if (partes.length === 0) return '–';
  const primeira = partes[0]?.[0] ?? '';
  const ultima = partes.length > 1 ? (partes[partes.length - 1]?.[0] ?? '') : '';
  return (primeira + ultima).toUpperCase();
}

/** Rótulo do perfil como aparece na interface. */
export function rotuloDePerfil(role: SessionUser['role']): string {
  switch (role) {
    case 'ADMIN':
      return 'Administrador';
    case 'GESTOR':
      return 'Gestor';
    case 'MEDICO':
      return 'Médico';
  }
}
