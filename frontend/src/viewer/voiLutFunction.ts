// Subconjunto de Enums.VOILUTFunctionType que o StackViewport 5.11 renderiza;
// duplicado aqui para manter o módulo puro e testável sem a biblioteca.
// SIGMOID fica de fora: em 5.11, viewport.resetProperties() quebra em
// setColormapGPU (null.name) com função sigmoide. Exibir como LINEAR mantém
// WW/WL do DICOM e só muda a curva de contraste — melhor que não abrir.
const VALID = new Set(['LINEAR', 'LINEAR_EXACT']);

/**
 * Normaliza VOI LUT Function (0028,1056) para um valor aceito pelo viewport.
 *
 * Contorna bug do @cornerstonejs/dicom-image-loader 5.11: createImage trata a
 * string do metadado como array e grava só o 1º caractere ("LINEAR"[0] === "L").
 * O StackViewport então lança "Invalid VOI LUT function" e a imagem principal
 * não abre (a miniatura, em CPU, não passa por esse caminho). Equipamentos que
 * gravam a tag — ex.: consoles DR E-COM — eram afetados.
 *
 * Valor ausente, vazio, desconhecido ou SIGMOID vira undefined (padrão LINEAR).
 */
export function normalizeVoiLutFunction(raw: unknown): string | undefined {
  const first = Array.isArray(raw) ? raw[0] : raw;
  if (typeof first !== 'string') return undefined;
  const value = first.replace(/\0/g, '').trim().toUpperCase();
  return VALID.has(value) ? value : undefined;
}
