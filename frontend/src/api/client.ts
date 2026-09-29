/**
 * Cliente HTTP do backend Go.
 *
 * Mesma origem: em desenvolvimento pelo proxy do Vite, em produção pelo próprio
 * backend (ou pelo proxy da infraestrutura). Nenhum token é guardado em
 * localStorage — a sessão vive num cookie HttpOnly que o navegador envia sozinho.
 */

/** Perfis de acesso, iguais aos do backend. */
export type Role = 'ADMIN' | 'GESTOR' | 'MEDICO';

/** Usuário autenticado como a API o devolve. Sem hash, sem token. */
export type SessionUser = {
  id: string;
  name: string;
  username: string;
  email?: string;
  role: Role;
  unit: { id: string; name: string } | null;
  active: boolean;
  accessValidUntil: string | null;
  lastLoginAt: string | null;
};

/** Erro devolvido pela API, já com o código que o frontend usa para decidir. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

const CSRF_COOKIE = 'pacs_csrf';
const CSRF_HEADER = 'X-CSRF-Token';

function lerCookie(nome: string): string | null {
  const alvo = `${nome}=`;
  for (const parte of document.cookie.split(';')) {
    const limpo = parte.trim();
    if (limpo.startsWith(alvo)) return decodeURIComponent(limpo.slice(alvo.length));
  }
  return null;
}

type Metodo = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

type OpcoesRequisicao = {
  body?: unknown;
  signal?: AbortSignal;
  /** Uso interno: evita laço infinito ao renovar o token de CSRF. */
  jaRepetiu?: boolean;
};

const METODOS_SEGUROS: Metodo[] = ['GET'];

async function requisitar<T>(metodo: Metodo, caminho: string, opcoes: OpcoesRequisicao = {}): Promise<T> {
  const cabecalhos: Record<string, string> = {};
  if (opcoes.body !== undefined) cabecalhos['Content-Type'] = 'application/json';

  if (!METODOS_SEGUROS.includes(metodo)) {
    // O backend emite o cookie de CSRF em qualquer resposta da API; se ainda não
    // temos um, buscamos antes de alterar estado.
    let token = lerCookie(CSRF_COOKIE);
    if (!token) {
      await fetch('/api/auth/me', { credentials: 'same-origin' });
      token = lerCookie(CSRF_COOKIE);
    }
    if (token) cabecalhos[CSRF_HEADER] = token;
  }

  const init: RequestInit = {
    method: metodo,
    credentials: 'same-origin',
    headers: cabecalhos,
  };
  if (opcoes.body !== undefined) init.body = JSON.stringify(opcoes.body);
  if (opcoes.signal) init.signal = opcoes.signal;

  const resposta = await fetch(caminho, init);

  if (resposta.status === 204) return undefined as T;

  const texto = await resposta.text();
  const corpo: unknown = texto ? JSON.parse(texto) : null;

  if (resposta.ok) return corpo as T;

  const erro = extrairErro(resposta.status, corpo);

  // Token de CSRF vencido (por exemplo após reiniciar o backend): renova e
  // repete uma única vez.
  if (erro.code === 'CSRF_INVALID' && !opcoes.jaRepetiu) {
    await fetch('/api/auth/me', { credentials: 'same-origin' });
    return requisitar<T>(metodo, caminho, { ...opcoes, jaRepetiu: true });
  }

  throw erro;
}

function extrairErro(status: number, corpo: unknown): ApiError {
  if (
    corpo &&
    typeof corpo === 'object' &&
    'error' in corpo &&
    corpo.error &&
    typeof corpo.error === 'object'
  ) {
    const detalhe = corpo.error as { code?: unknown; message?: unknown };
    const code = typeof detalhe.code === 'string' ? detalhe.code : 'INTERNAL_ERROR';
    const message =
      typeof detalhe.message === 'string' ? detalhe.message : 'Não foi possível concluir a operação.';
    return new ApiError(status, code, message);
  }
  return new ApiError(status, 'INTERNAL_ERROR', 'Não foi possível concluir a operação.');
}

/** Configuração da conexão com o PACS, como a API a devolve. */
export type OrthancSettings = {
  /** Falso enquanto nenhuma configuração foi salva. */
  configured: boolean;
  name: string;
  baseUrl: string;
  username: string;
  dicomWebPath: string;
  timeoutSeconds: number;
  verifyTls: boolean;
  /** Existe credencial guardada. O valor em si nunca vem para o navegador. */
  credentialConfigured: boolean;
  /** Falso quando o servidor não tem chave mestra para cifrar credenciais. */
  credentialSupported: boolean;
  /** "unverified" | "connected" | "failed" */
  status: string;
  /** Disponível quando existe configuração salva e cliente habilitado. */
  connectionTestAvailable: boolean;
  lastCheckedAt: string | null;
};

/**
 * Dados enviados ao salvar.
 *
 * `credential` ausente preserva a credencial guardada; string vazia a remove.
 */
export type OrthancSettingsInput = {
  name: string;
  baseUrl: string;
  username: string;
  dicomWebPath: string;
  timeoutSeconds: number;
  verifyTls: boolean;
  credential?: string;
};

export type OrthancConnectionTest = {
  success: boolean;
  status: 'connected' | 'failed';
  code: string;
  message: string;
  checkedAt: string;
};

type RespostaSessao = { user: SessionUser };

export type Study = {
  orthancStudyId: string;
  studyInstanceUid: string;
  studyDate: string;
  studyTime: string;
  patientName: string;
  patientId: string;
  accessionNumber: string;
  studyDescription: string;
  institutionName: string;
  modalities: string[];
  seriesCount: number;
};

export type StudyQuery = {
  modality?: string;
  sort?: 'dateDesc' | 'dateAsc' | 'native';
  limit?: number;
  offset?: number;
  dateFrom?: string;
  dateTo?: string;
  patientName?: string;
  patientId?: string;
  accessionNumber?: string;
  studyDescription?: string;
  institutionName?: string;
};

export type StudyPage = {
  items: Study[];
  limit: number;
  offset: number;
  hasMore: boolean;
  nextOffset: number | null;
};

export type ViewerSeries = {
  orthancSeriesId: string;
  description: string;
  number: string;
  modality: string;
  instanceCount: number;
};

export type ViewerInstance = { orthancInstanceId: string; number: number | null };

export function viewerResourceID(id: string): string {
  if (!/^[a-f0-9]{8}(-[a-f0-9]{8}){4}$/.test(id)) throw new Error('Identificador inválido.');
  return id;
}

/** Sempre mesma origem; não aceita URL nem caminho arbitrário. */
export function viewerDICOMPath(studyId: string, seriesId: string, instanceId: string): string {
  return `/api/studies/${viewerResourceID(studyId)}/series/${viewerResourceID(seriesId)}/instances/${viewerResourceID(instanceId)}/dicom`;
}

export function viewerRoute(studyId: string): string { return `/viewer/${viewerResourceID(studyId)}`; }

export function studyIDFromRoute(path: string): string | null {
  const match = /^\/viewer\/([a-f0-9]{8}(?:-[a-f0-9]{8}){4})$/.exec(path);
  return match?.[1] ?? null;
}

export type Unit = { id: string; name: string; active: boolean; createdAt: string; updatedAt: string };
export type UnitsPage = { items: Unit[]; limit: number; offset: number; hasMore: boolean; nextOffset: number | null };

export const api = {
  async getUnits(includeInactive: boolean, offset: number, signal: AbortSignal): Promise<UnitsPage> {
    return requisitar('GET', `/api/units?includeInactive=${includeInactive}&limit=50&offset=${offset}`, { signal });
  },
  async createUnit(name: string, signal: AbortSignal): Promise<Unit> {
    return requisitar('POST', '/api/admin/units', { body: { name }, signal });
  },
  async updateUnit(id: string, patch: { name?: string; active?: boolean }, signal: AbortSignal): Promise<Unit> {
    if (!/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(id)) throw new Error('Unidade inválida.');
    return requisitar('PATCH', `/api/admin/units/${id}`, { body: patch, signal });
  },
  async getViewerSeries(studyId: string, signal: AbortSignal): Promise<{ items: ViewerSeries[] }> {
    return requisitar('GET', `/api/studies/${viewerResourceID(studyId)}/series`, { signal });
  },

  async getViewerInstances(studyId: string, seriesId: string, signal: AbortSignal): Promise<{ items: ViewerInstance[] }> {
    return requisitar('GET', `/api/studies/${viewerResourceID(studyId)}/series/${viewerResourceID(seriesId)}/instances`, { signal });
  },

  async getStudies(query: StudyQuery, signal: AbortSignal): Promise<StudyPage> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== '') params.set(key, String(value));
    }
    return requisitar<StudyPage>('GET', `/api/studies?${params}`, { signal });
  },

  /** Teste explícito somente da configuração já salva. */
  async testOrthancConnection(): Promise<OrthancConnectionTest> {
    return requisitar<OrthancConnectionTest>('POST', '/api/admin/settings/orthanc/test');
  },

  /** POST /api/auth/login */
  async login(username: string, password: string): Promise<SessionUser> {
    const resposta = await requisitar<RespostaSessao>('POST', '/api/auth/login', {
      body: { username, password },
    });
    return resposta.user;
  },

  /** GET /api/auth/me — devolve null quando não há sessão válida. */
  async me(): Promise<SessionUser | null> {
    try {
      const resposta = await requisitar<RespostaSessao>('GET', '/api/auth/me');
      return resposta.user;
    } catch (erro) {
      if (erro instanceof ApiError && erro.status === 401) return null;
      throw erro;
    }
  },

  /** POST /api/auth/logout */
  async logout(): Promise<void> {
    await requisitar<void>('POST', '/api/auth/logout');
  },

  /** GET /api/admin/settings/orthanc — só ADMIN. */
  async getOrthancSettings(): Promise<OrthancSettings> {
    return requisitar<OrthancSettings>('GET', '/api/admin/settings/orthanc');
  },

  /** PUT /api/admin/settings/orthanc — só ADMIN. */
  async putOrthancSettings(entrada: OrthancSettingsInput): Promise<OrthancSettings> {
    return requisitar<OrthancSettings>('PUT', '/api/admin/settings/orthanc', { body: entrada });
  },
};
