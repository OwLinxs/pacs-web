/**
 * DADOS DE DEMONSTRAÇÃO — TODOS FICTÍCIOS.
 *
 * Nenhum dado real de paciente, profissional, prontuário, IP ou estação entra
 * neste arquivo. Os registros abaixo são os mesmos do protótipo de design
 * (PacsPrototipo.dc.html), criados para conferência visual da interface.
 *
 * Fase 1: serão substituídos pela API própria a partir da Fase 4.
 */

export const UNIDADES = {
  upa: 'UPA Francisco Beltrão',
  h18: 'Unidade 18 Horas',
  cango: 'UBS Cango',
  ulrico: 'UBS Padre Ulrico',
  alvorada: 'UBS Alvorada',
  hm: 'Hospital Municipal',
} as const;

export type UnidadeKey = keyof typeof UNIDADES;

export const UNIDADE_KEYS = Object.keys(UNIDADES) as UnidadeKey[];

/** Rótulo curto do dia, indexado por `day` (0 = hoje). */
const DIAS = ['Hoje', 'Ontem', '23/09', '22/09', '21/09', '20/09'] as const;
const DATAS = ['25/09/2026', '24/09/2026', '23/09/2026', '22/09/2026', '21/09/2026', '20/09/2026'] as const;

export type Serie = {
  label: string;
  count: number;
  n: number;
};

export type Exame = {
  id: number;
  day: number;
  time: string;
  name: string;
  sex: 'F' | 'M';
  age: number;
  pront: string;
  exam: string;
  unitKey: UnidadeKey;
  unit: string;
  series: Serie[];
  imgs: number;
  dayLabel: string;
  date: string;
  sexAge: string;
  /** "SOBRENOME, Nome" — formato das sobreposições do viewer. */
  short: string;
};

type ExameSeed = [
  day: number,
  time: string,
  name: string,
  sex: 'F' | 'M',
  age: number,
  pront: string,
  exam: string,
  unitKey: UnidadeKey,
  series: [label: string, count: number][],
];

const EXAMES_SEED: ExameSeed[] = [
  [0, '09:58', 'João Pedro Andrade', 'M', 32, '018.233', 'Joelho D — AP e Perfil', 'h18', [['AP', 1], ['PERFIL', 1]]],
  [0, '09:42', 'Maria Aparecida Silva', 'F', 54, '004.812', 'Tórax — PA e Perfil', 'upa', [['PA', 3], ['PERFIL', 1]]],
  [0, '09:15', 'Lucas Henrique Moreira', 'M', 8, '027.541', 'Punho E — AP e Oblíqua', 'upa', [['AP', 1], ['OBL', 1]]],
  [0, '08:51', 'Rosângela Ferreira Lima', 'F', 67, '003.907', 'Coluna lombar — AP e Perfil', 'cango', [['AP', 1], ['PERFIL', 1]]],
  [0, '08:20', 'Antônio Carlos Bortolini', 'M', 71, '011.264', 'Tórax — PA', 'hm', [['PA', 1]]],
  [0, '07:48', 'Camila Rodrigues Souza', 'F', 29, '022.019', 'Tornozelo D — AP, Perfil, Mortise', 'h18', [['AP', 1], ['PERFIL', 1], ['MORT', 1]]],
  [1, '22:37', 'Valdir José Kunz', 'M', 45, '009.388', 'Ombro E — AP e Axial', 'upa', [['AP', 1], ['AXIAL', 1]]],
  [1, '19:05', 'Eduarda Pires Costa', 'F', 17, '031.552', 'Seios da face — Waters e Caldwell', 'ulrico', [['WATERS', 1], ['CALDW', 1]]],
  [1, '16:40', 'Sebastião Mendes', 'M', 83, '001.476', 'Bacia — AP', 'hm', [['AP', 1]]],
  [1, '14:12', 'Ana Júlia Dal Pra', 'F', 5, '033.810', 'Tórax — AP', 'upa', [['AP', 1]]],
  [3, '10:30', 'Gilmar Antunes', 'M', 58, '007.721', 'Mão D — PA e Oblíqua', 'alvorada', [['PA', 1], ['OBL', 1]]],
  [5, '15:55', 'Terezinha Wolff', 'F', 76, '002.150', 'Quadril E — AP e Perfil', 'cango', [['AP', 1], ['PERFIL', 1]]],
];

export const EXAMES: Exame[] = EXAMES_SEED.map(
  ([day, time, name, sex, age, pront, exam, unitKey, series], id) => {
    const parts = name.split(' ');
    const sobrenome = parts[parts.length - 1] ?? name;
    return {
      id,
      day,
      time,
      name,
      sex,
      age,
      pront,
      exam,
      unitKey,
      unit: UNIDADES[unitKey],
      series: series.map(([label, count], j) => ({ label, count, n: j + 1 })),
      imgs: series.reduce((total, [, count]) => total + count, 0),
      dayLabel: DIAS[day] ?? '',
      date: DATAS[day] ?? '',
      sexAge: `${sex === 'F' ? 'Feminino' : 'Masculino'} · ${age} ${age === 1 ? 'ano' : 'anos'}`,
      short: `${sobrenome.toUpperCase()}, ${parts.slice(0, -1).join(' ')}`,
    };
  },
);

export function exameById(id: number): Exame {
  const exame = EXAMES[id];
  if (!exame) throw new Error(`Exame de demonstração inexistente: ${id}`);
  return exame;
}

export function serieAt(exame: Exame, index: number): Serie {
  const serie = exame.series[index];
  if (!serie) throw new Error(`Série inexistente: ${exame.id}/${index}`);
  return serie;
}

export type Usuario = {
  name: string;
  user: string;
  unit: string;
  unitKey: UnidadeKey;
  role: string;
  valid: string;
  last: string;
  active: boolean;
  initials: string;
};

const USUARIOS_SEED: [string, string, UnidadeKey, string, string, string, 'on' | 'off'][] = [
  ['Helena Marques', 'helena.marques', 'upa', 'Administrador', '31/12/2026', 'Agora', 'on'],
  ['Rafael Tonello', 'rafael.tonello', 'upa', 'Médico', '31/12/2026', 'Hoje, 08:12', 'on'],
  ['Juliana Boff', 'juliana.boff', 'h18', 'Médico', '30/06/2027', 'Hoje, 07:40', 'on'],
  ['Marcos Zanella', 'marcos.zanella', 'h18', 'Médico', '31/12/2026', 'Ontem, 23:05', 'on'],
  ['Patrícia Oliveira', 'patricia.oliveira', 'cango', 'Médico', '31/10/2026', '22/09, 14:31', 'on'],
  ['Diego Hartmann', 'diego.hartmann', 'hm', 'Médico', '31/12/2026', '21/09, 10:02', 'on'],
  ['Cláudia Nardi', 'claudia.nardi', 'ulrico', 'Médico', '15/08/2026', '12/08, 16:47', 'off'],
  ['Fernando Lazzari', 'fernando.lazzari', 'alvorada', 'Médico', '31/12/2026', 'Nunca acessou', 'on'],
  ['Silvia Kappes', 'silvia.kappes', 'upa', 'Médico', '30/06/2026', '28/06, 09:20', 'off'],
];

export const USUARIOS: Usuario[] = USUARIOS_SEED.map(([name, user, unitKey, role, valid, last, status]) => ({
  name,
  user,
  unitKey,
  unit: UNIDADES[unitKey],
  role,
  valid,
  last,
  active: status === 'on',
  initials: name
    .split(' ')
    .map((part) => part[0] ?? '')
    .join(''),
}));

export type EventoKind = 'login' | 'logout' | 'view' | 'fail' | 'create' | 'deact';

export type EventoAuditoria = {
  ts: string;
  user: string;
  unit: string;
  kind: EventoKind;
  event: string;
  detail: string;
  origin: string;
};

const AUDITORIA_SEED: [string, string, UnidadeKey | '—', EventoKind, string, string, string][] = [
  ['25/09/2026 10:02:14', 'helena.marques', 'upa', 'view', 'Visualização', 'Exame 004.812 · Tórax PA e Perfil', '10.12.4.21 · UPA-CONS-03'],
  ['25/09/2026 09:58:40', 'helena.marques', 'upa', 'login', 'Login', 'Sessão iniciada', '10.12.4.21 · UPA-CONS-03'],
  ['25/09/2026 09:31:07', 'juliana.boff', 'h18', 'view', 'Visualização', 'Exame 018.233 · Joelho D', '10.12.7.14 · 18H-CONS-01'],
  ['25/09/2026 09:12:55', 'desconhecido', '—', 'fail', 'Falha de login', 'Usuário inexistente: "medico1"', '10.12.7.30 · 18H-RECEP'],
  ['25/09/2026 09:12:40', 'marcos.zanella', 'h18', 'fail', 'Falha de login', 'Senha incorreta (2ª tentativa)', '10.12.7.30 · 18H-RECEP'],
  ['25/09/2026 08:44:18', 'helena.marques', 'upa', 'create', 'Usuário criado', 'fernando.lazzari · UBS Alvorada', '10.12.4.21 · UPA-CONS-03'],
  ['25/09/2026 08:12:09', 'rafael.tonello', 'upa', 'login', 'Login', 'Sessão iniciada', '10.12.4.18 · UPA-CONS-01'],
  ['25/09/2026 07:40:33', 'juliana.boff', 'h18', 'login', 'Login', 'Sessão iniciada', '10.12.7.14 · 18H-CONS-01'],
  ['24/09/2026 23:35:02', 'marcos.zanella', 'h18', 'logout', 'Logout', 'Sessão expirada por inatividade', '10.12.7.12 · 18H-CONS-02'],
  ['24/09/2026 23:05:47', 'marcos.zanella', 'h18', 'view', 'Visualização', 'Exame 009.388 · Ombro E', '10.12.7.12 · 18H-CONS-02'],
  ['24/09/2026 18:20:11', 'helena.marques', 'upa', 'deact', 'Usuário desativado', 'claudia.nardi · validade expirada', '10.12.4.21 · UPA-CONS-03'],
  ['24/09/2026 18:02:36', 'patricia.oliveira', 'cango', 'logout', 'Logout', 'Encerrada pelo usuário', '10.12.9.05 · CANGO-CONS'],
];

export const AUDITORIA: EventoAuditoria[] = AUDITORIA_SEED.map(([ts, user, unitKey, kind, event, detail, origin]) => ({
  ts,
  user,
  unit: unitKey === '—' ? '—' : UNIDADES[unitKey],
  kind,
  event,
  detail,
  origin,
}));

/** Cor do badge de evento na auditoria. `bd` só existe onde o design usa contorno. */
export const ESTILO_EVENTO: Record<EventoKind, { bg: string; fg: string; bd?: string }> = {
  login: { bg: 'var(--color-accent-100)', fg: 'var(--color-accent-800)' },
  logout: { bg: 'var(--color-neutral-200)', fg: 'var(--color-neutral-800)' },
  view: { bg: 'transparent', fg: 'var(--color-accent-700)', bd: 'var(--color-accent-400)' },
  fail: { bg: 'oklch(0.94 0.035 25)', fg: 'oklch(0.42 0.13 25)' },
  create: { bg: 'var(--color-accent-200)', fg: 'var(--color-accent-900)' },
  deact: { bg: 'var(--color-neutral-100)', fg: 'var(--color-neutral-700)', bd: 'var(--color-neutral-400)' },
};

/** Médico autenticado na demonstração. Identidade fictícia. */
export const MEDICO_DEMO = {
  initials: 'HM',
  name: 'Dra. Helena Marques',
  user: 'helena.marques',
  crm: 'CRM-PR 34.512',
  unit: UNIDADES.upa,
  role: 'Administrador',
  since: '09:58',
} as const;
