import type { StatsPeriod } from '../api/client';

const MONTHS = ['janeiro','fevereiro','março','abril','maio','junho','julho','agosto','setembro','outubro','novembro','dezembro'];
const WEEKDAYS = ['dom','seg','ter','qua','qui','sex','sáb'];

/** Data civil (YYYY-MM-DD) sem fuso: aritmética em UTC. */
function parse(date: string): Date { const [y, m, d] = date.split('-').map(Number); return new Date(Date.UTC(y!, m! - 1, d!)); }
function format(date: Date): string { return date.toISOString().slice(0, 10); }
const br = (date: string) => date.split('-').reverse().join('/');

/** Data de hoje em São Paulo, no formato YYYY-MM-DD. */
export function todaySaoPaulo(now = new Date()): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'America/Sao_Paulo', year: 'numeric', month: '2-digit', day: '2-digit' }).format(now);
}

/** Mesma data de referência deslocada de um período; nunca passa de hoje. */
export function shiftDate(period: StatsPeriod, date: string, step: -1 | 1, today: string): string {
  const d = parse(date);
  if (period === 'day') d.setUTCDate(d.getUTCDate() + step);
  else if (period === 'week') d.setUTCDate(d.getUTCDate() + 7 * step);
  else { const day = d.getUTCDate(); d.setUTCDate(1); d.setUTCMonth(d.getUTCMonth() + step); const last = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 0)).getUTCDate(); d.setUTCDate(Math.min(day, last)); }
  const result = format(d);
  return result > today ? today : result;
}

export function periodLabel(period: StatsPeriod, from: string, to: string): string {
  if (period === 'day') return br(from);
  if (period === 'week') return `${br(from).slice(0, 5)} a ${br(to)}`;
  const d = parse(from);
  return `${MONTHS[d.getUTCMonth()]} de ${d.getUTCFullYear()}`;
}

export const previousLabel: Record<StatsPeriod, string> = { day: 'dia anterior', week: 'semana anterior', month: 'mês anterior' };
const equalTo: Record<StatsPeriod, string> = { day: 'igual ao dia anterior', week: 'igual à semana anterior', month: 'igual ao mês anterior' };
const noneIn: Record<StatsPeriod, string> = { day: 'sem registros no dia anterior', week: 'sem registros na semana anterior', month: 'sem registros no mês anterior' };

/** Rótulo curto de barra: hora no dia, "seg 26" na semana, dia do mês no mês. */
export function bucketLabel(period: StatsPeriod, label: string): string {
  if (period === 'day') return `${label}h`;
  const d = parse(label);
  return period === 'week' ? `${WEEKDAYS[d.getUTCDay()]} ${String(d.getUTCDate()).padStart(2, '0')}` : String(d.getUTCDate());
}

/** Rótulo completo para tooltip e tabela. */
export function bucketTitle(period: StatsPeriod, label: string): string {
  if (period === 'day') return `${label}:00–${label}:59`;
  const d = parse(label);
  return `${WEEKDAYS[d.getUTCDay()]}, ${br(label)}`;
}

/** Variação contra o período anterior; sem base, não inventa percentual. */
export function delta(current: number, previous: number, period: StatsPeriod): string {
  if (previous === 0) return current === 0 ? equalTo[period] : noneIn[period];
  const pct = Math.round(((current - previous) / previous) * 100);
  if (pct === 0) return equalTo[period];
  return `${pct > 0 ? '▲' : '▼'} ${Math.abs(pct)}% vs ${previousLabel[period]}`;
}

export const integer = (n: number) => n.toLocaleString('pt-BR');
export const compact = (n: number) => new Intl.NumberFormat('pt-BR', { notation: n >= 1000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(n);

/** Escala com topo "limpo" (1/2/5 × 10^k) e ~4 divisões. */
export function niceScale(max: number): { top: number; step: number } {
  if (max <= 0) return { top: 4, step: 1 };
  const raw = max / 4, power = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 5, 10].map(m => m * power).find(s => s >= raw)!;
  const whole = Math.max(1, step);
  return { top: Math.ceil(max / whole) * whole, step: whole };
}

export function diskLabel(mb: number): string {
  return mb >= 1024 ? `${(mb / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} GB` : `${integer(mb)} MB`;
}
