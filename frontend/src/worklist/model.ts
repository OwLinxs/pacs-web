import type { StudyQuery } from '../api/client';

export const SEARCH_FIELDS = [
  { key: 'patientName', label: 'Nome do paciente' },
  { key: 'patientId', label: 'ID do paciente' },
  { key: 'accessionNumber', label: 'Accession Number' },
  { key: 'studyDescription', label: 'Descrição do estudo' },
] as const;
export type SearchField = typeof SEARCH_FIELDS[number]['key'];
export type Period = 'today' | 'yesterday' | '7days' | '30days' | 'custom';
export type WorklistState = {
  field: SearchField;
  filters: Record<SearchField | 'institutionName', string>;
  period: Period;
  dateFrom: string;
  dateTo: string;
  modality: string;
  sort: NonNullable<StudyQuery['sort']>;
  limit: 25 | 50;
  offset: number;
  advanced: boolean;
};
export function localDate(now: Date, daysAgo = 0): string {
  const date = new Date(now);
  date.setDate(date.getDate() - daysAgo);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}
export function periodDates(period: Period, now = new Date()): { dateFrom: string; dateTo: string } {
  const ago = period === 'yesterday' ? 1 : period === '7days' ? 6 : period === '30days' ? 29 : 0;
  return { dateFrom: localDate(now, ago), dateTo: localDate(now, period === 'yesterday' ? 1 : 0) };
}
export function initialWorklist(now = new Date()): WorklistState {
  return { field: 'patientName', filters: { patientName: '', patientId: '', accessionNumber: '', studyDescription: '', institutionName: '' },
    period: '7days', ...periodDates('7days', now), modality: '', sort: 'dateDesc', limit: 25, offset: 0, advanced: false };
}
function validDate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const [year = 0, month = 0, day = 0] = value.split('-').map(Number);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  return year > 0 && month >= 1 && month <= 12 && day >= 1 && day <= ([31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1] ?? 0);
}
export function validateWorklist(state: WorklistState): string {
  if (!validDate(state.dateFrom) || !validDate(state.dateTo)) return 'Informe as datas inicial e final válidas.';
  if (state.dateFrom > state.dateTo) return 'A data inicial deve ser anterior ou igual à data final.';
  if (state.modality && !/^[A-Z0-9_ ]{1,16}$/.test(state.modality)) return 'Informe um código de modalidade DICOM válido.';
  return '';
}
export function studyQuery(state: WorklistState): StudyQuery {
  return { ...Object.fromEntries(Object.entries(state.filters).map(([key, value]) => [key, value.trim()])),
    dateFrom: state.dateFrom, dateTo: state.dateTo, modality: state.modality.trim(), sort: state.sort, limit: state.limit, offset: state.offset };
}
export function dicomDate(value: string): string {
  if (!/^\d{8}$/.test(value)) return '—';
  const iso = `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
  return validDate(iso) ? `${value.slice(6, 8)}/${value.slice(4, 6)}/${value.slice(0, 4)}` : '—';
}
export function dicomTime(value: string): string {
  // TM: HH, HHMM ou HHMMSS[.ffffff], sem fuso; segundos intercalares são válidos.
  if (!/^(\d{2})(\d{2})?(\d{2}(\.\d{1,6})?)?$/.test(value)) return '—';
  const hour = Number(value.slice(0, 2)), minute = Number(value.slice(2, 4) || 0), second = Number(value.slice(4) || 0);
  if (hour > 23 || minute > 59 || second >= 61) return '—';
  return value.length === 2 ? `${value}h` : `${value.slice(0, 2)}:${value.slice(2, 4)}`;
}
export function modalities(values: string[]): string[] {
  return [...new Set(values.map(value => value.trim()).filter(Boolean))].sort();
}
