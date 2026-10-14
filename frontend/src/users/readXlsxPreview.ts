import { MAX_XLSX_BYTES, previewSheets, validateXlsxArchive, type ImportPreview, type Sheet } from './importPreview';

export async function readXlsxPreview(file: File): Promise<ImportPreview> {
  if (!/\.xlsx$/i.test(file.name) || !file.size || file.size > MAX_XLSX_BYTES) {
    throw new Error('Selecione um arquivo .xlsx de até 2 MiB.');
  }
  const bytes = new Uint8Array(await file.arrayBuffer());
  validateXlsxArchive(bytes);
  try {
    // Carregado sob demanda. Os bytes permanecem locais ao navegador.
    const { default: readExcelFile } = await import('read-excel-file/browser');
    const sheets = await readExcelFile(new Blob([bytes])) as Sheet[];
    return previewSheets(sheets);
  } catch (error) {
    if (error instanceof Error && (error.message.startsWith('Coluna') || error.message.startsWith('A planilha excede'))) throw error;
    throw new Error('Não foi possível ler este arquivo .xlsx. Verifique se ele não está corrompido.');
  }
}
