export type ImportStatus = 'valid' | 'duplicate' | 'invalid';
export type ImportRow = { line: number; name: string; status: ImportStatus; reason: string };
export type ImportPreview = { sheet: string; rows: ImportRow[]; valid: number; duplicate: number; invalid: number };
export type Sheet = { sheet: string; data: unknown[][] };

export const MAX_XLSX_BYTES = 2 * 1024 * 1024;
const MAX_UNCOMPRESSED_BYTES = 16 * 1024 * 1024;
const MAX_ROWS = 5000;
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

export function previewSheets(sheets: Sheet[]): ImportPreview {
  for (const sheet of sheets) {
    const header = sheet.data.slice(0, 20).findIndex(row => row.some(cell => typeof cell === 'string' && clean(cell).toLocaleLowerCase('pt-BR') === 'nome'));
    if (header < 0) continue;
    const column = sheet.data[header]!.findIndex(cell => typeof cell === 'string' && clean(cell).toLocaleLowerCase('pt-BR') === 'nome');
    const data = sheet.data.slice(header + 1);
    if (data.length > MAX_ROWS) throw new Error('A planilha excede 5.000 linhas. Divida o arquivo para pré-visualizar.');
    const seen = new Set<string>();
    const rows: ImportRow[] = [];
    for (let i = 0; i < data.length; i++) {
      const cells = data[i]!;
      if (!cells.some(filled)) continue;
      const raw = cells[column];
      const name = typeof raw === 'string' ? clean(raw) : '';
      let status: ImportStatus = 'valid', reason = 'Válido para prévia';
      if (!name || name.length > 120 || name.length < 2 || /[\p{Cc}\p{Cf}]/u.test(name)) {
        status = 'invalid'; reason = 'Nome ausente ou inválido';
      } else {
        const key = name.toLocaleLowerCase('pt-BR');
        if (seen.has(key)) { status = 'duplicate'; reason = 'Duplicado na planilha'; }
        else seen.add(key);
      }
      rows.push({ line: header + i + 2, name: name || '—', status, reason });
    }
    return { sheet: sheet.sheet, rows, valid: rows.filter(row => row.status === 'valid').length,
      duplicate: rows.filter(row => row.status === 'duplicate').length, invalid: rows.filter(row => row.status === 'invalid').length };
  }
  throw new Error('Coluna “Nome” não encontrada nas primeiras 20 linhas de nenhuma aba.');
}
