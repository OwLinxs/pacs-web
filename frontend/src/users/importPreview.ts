export type ImportStatus = 'valid' | 'duplicate' | 'invalid';
export type ImportRow = { line: number; name: string; status: ImportStatus; reason: string; username: string };
export type ImportPreview = { sheet: string; headerless: boolean; rows: ImportRow[]; valid: number; duplicate: number; invalid: number };
export type Sheet = { sheet: string; data: unknown[][] };

export const MAX_XLSX_BYTES = 2 * 1024 * 1024;
const MAX_UNCOMPRESSED_BYTES = 16 * 1024 * 1024;
const MAX_ROWS = 5000;
/** Contas criadas por operação; acima disso a planilha deve ser dividida. */
export const MAX_IMPORT_ACCOUNTS = 200;
export const USERNAME_PATTERN = /^[a-z0-9][a-z0-9_-]{2,63}$/;
const invalidFile = () => new Error('Arquivo .xlsx inválido ou fora dos limites permitidos.');

/** Confere o diretório ZIP antes de entregar um arquivo não confiável ao leitor Excel. */
export function validateXlsxArchive(bytes: Uint8Array): void {
  if (bytes.byteLength < 22 || bytes.byteLength > MAX_XLSX_BYTES) throw invalidFile();
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  let end = -1;
  for (let i = bytes.length - 22; i >= Math.max(0, bytes.length - 65557); i--) {
    if (view.getUint32(i, true) === 0x06054b50 && i + 22 + view.getUint16(i + 20, true) === bytes.length) { end = i; break; }
  }
  if (end < 0 || view.getUint16(end + 4, true) !== 0 || view.getUint16(end + 6, true) !== 0) throw invalidFile();
  const count = view.getUint16(end + 10, true);
  const size = view.getUint32(end + 12, true);
  const start = view.getUint32(end + 16, true);
  if (!count || count > 256 || count === 0xffff || start + size > end) throw invalidFile();
  let cursor = start, expanded = 0;
  for (let i = 0; i < count; i++) {
    if (cursor + 46 > end || view.getUint32(cursor, true) !== 0x02014b50) throw invalidFile();
    const flags = view.getUint16(cursor + 8, true), method = view.getUint16(cursor + 10, true);
    const compressed = view.getUint32(cursor + 20, true), uncompressed = view.getUint32(cursor + 24, true);
    const next = cursor + 46 + view.getUint16(cursor + 28, true) + view.getUint16(cursor + 30, true) + view.getUint16(cursor + 32, true);
    if (next > end || flags & 1 || (method !== 0 && method !== 8) || compressed === 0xffffffff || uncompressed === 0xffffffff) throw invalidFile();
    expanded += uncompressed;
    if (expanded > MAX_UNCOMPRESSED_BYTES) throw invalidFile();
    cursor = next;
  }
  if (cursor !== start + size) throw invalidFile();
}

const clean = (value: string) => value.normalize('NFC').replace(/\s+/gu, ' ').trim();
const filled = (value: unknown) => value !== null && value !== undefined && clean(String(value)) !== '';

const isHeader = (cell: unknown) => typeof cell === 'string' && clean(cell).toLocaleLowerCase('pt-BR') === 'nome';
const PARTICLES = new Set(['da', 'das', 'de', 'del', 'di', 'do', 'dos', 'du', 'e']);

/** Primeiro + último nome, sem acentos, minúsculo, separados por "_". */
export function usernameBase(name: string): string {
  const words = name.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase().split(/[^a-z0-9]+/).filter(Boolean);
  const parts = words.filter(word => !PARTICLES.has(word));
  const use = parts.length ? parts : words;
  const first = use[0] ?? '', last = use.length > 1 ? use[use.length - 1]! : '';
  return (last ? `${first}_${last}` : first).slice(0, 64);
}

/** Sugere usernames únicos na lista; sufixo numérico quando a base se repete. */
export function suggestUsernames(names: string[]): string[] {
  const taken = new Set<string>();
  return names.map(name => {
    const base = usernameBase(name);
    let candidate = base;
    for (let n = 2; taken.has(candidate); n++) candidate = base.slice(0, 64 - String(n).length) + n;
    taken.add(candidate);
    return candidate;
  });
}

function classify(sheet: string, headerless: boolean, data: unknown[][], column: number, firstLine: number): ImportPreview {
  if (data.length > MAX_ROWS) throw new Error('A planilha excede 5.000 linhas. Divida o arquivo para pré-visualizar.');
  const seen = new Set<string>();
  const rows: ImportRow[] = [];
  for (let i = 0; i < data.length; i++) {
    const cells = data[i]!;
    if (!cells.some(filled)) continue;
    const raw = cells[column];
    const name = typeof raw === 'string' ? clean(raw) : '';
    let status: ImportStatus = 'valid', reason = 'Válido';
    if (!name || name.length > 120 || name.length < 2 || /[\p{Cc}\p{Cf}]/u.test(name)) {
      status = 'invalid'; reason = 'Nome ausente ou inválido';
    } else {
      const key = name.toLocaleLowerCase('pt-BR');
      if (seen.has(key)) { status = 'duplicate'; reason = 'Duplicado na planilha'; }
      else seen.add(key);
    }
    rows.push({ line: firstLine + i, name: name || '—', status, reason, username: '' });
  }
  const valid = rows.filter(row => row.status === 'valid');
  suggestUsernames(valid.map(row => row.name)).forEach((username, i) => { valid[i]!.username = username; });
  return { sheet, headerless, rows, valid: valid.length,
    duplicate: rows.filter(row => row.status === 'duplicate').length, invalid: rows.filter(row => row.status === 'invalid').length };
}

export function previewSheets(sheets: Sheet[]): ImportPreview {
  for (const sheet of sheets) {
    const header = sheet.data.slice(0, 20).findIndex(row => row.some(isHeader));
    if (header < 0) continue;
    return classify(sheet.sheet, false, sheet.data.slice(header + 1), sheet.data[header]!.findIndex(isHeader), header + 2);
  }
  // Sem cabeçalho: aceita a primeira aba cujas células preenchidas ocupam uma única coluna.
  for (const sheet of sheets) {
    const columns = new Set<number>();
    for (const row of sheet.data) row.forEach((cell, index) => { if (filled(cell)) columns.add(index); });
    if (columns.size === 1) return classify(sheet.sheet, true, sheet.data, [...columns][0]!, 1);
  }
  throw new Error('Coluna “Nome” não encontrada nas primeiras 20 linhas, e nenhuma aba contém uma única coluna de nomes.');
}
