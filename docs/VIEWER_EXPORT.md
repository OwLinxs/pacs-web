# Viewer V5 — exportação clínica

**Implementado localmente; ainda não validado com DICOM real.** Exporta somente a imagem atual do viewport ativo. Não altera DICOM, Orthanc, stack, apresentação, ferramentas, medições ou Cine. O navegador continua acessando apenas o backend autenticado para obter DICOM. A geração do arquivo derivado ocorre em memória no browser.

## Captura e formatos

O controller Cornerstone do viewport ativo obtém `StackViewport.getCanvas()` após confirmar que há imagem apresentada, que o índice corresponde ao imageId atual e que não há carregamento/interação em andamento. Copia os pixels renderizados para um canvas destacado. Assim, enquadramento, VOI, zoom/pan, rotação, flip e Invert refletem a imagem exibida, sem resetar ou alterar o viewport. Overlays HTML e annotations SVG não são incorporados. A cópia tem limite de 32 Mi pixels e lado máximo de 16384 pixels.

PNG sem identificação codifica diretamente a cópia em PNG sem perda. JPEG usa qualidade 0,95 e é derivado para documentação, não substitui o DICOM. Nos formatos identificados, um canvas separado do mesmo tamanho do viewport recebe os textos **sobrepostos nos quatro cantos**, no mesmo modelo do PACS atual (ver “Sobreposições” abaixo), sem faixa extra e sem redimensionar os pixels. Campos longos podem ser abreviados visualmente para caber. PDF usa página A4 retrato/paisagem a aproximadamente 144 dpi, proporção preservada, imagem centralizada na maior área disponível; no modo identificado as sobreposições são desenhadas na resolução da página (texto nítido) e há rodapé de derivação. O PDF é gerado com `pdf-lib` 1.17.1, carregado sob demanda; textos Unicode são rasterizados na composição PNG antes da incorporação ao PDF. Sua imagem pode ser reduzida para caber na página. Não é um laudo.

O nome é neutro: `pacs-imagem-AAAAMMDD-HHMMSS.{png,jpg,pdf}` no horário local. Nenhum nome/ID de paciente ou UID compõe o filename. Canvas temporário é liberado após a codificação e Object URL é revogada após o clique de download. Não há armazenamento persistente novo.

## Sobreposições (modo identificado) — implementado localmente em 29/10/2026

Layout de referência fornecido pelo responsável (imagem do PACS atual; **não versionada**, contém PHI). Texto branco com contorno escuro; somente tags presentes; rótulos com valor ausente mostram `—`; linhas sem rótulo são omitidas.

| Canto | Conteúdo |
| --- | --- |
| Superior esquerdo (negrito) | PatientID; PatientName; `PatientAge/PatientSex/PatientBirthDate` (ex.: `026Y/O/01-jan-2000`) |
| Superior direito | StudyDate (`29-set-2026`); `Acq:` AcquisitionTime (ou ContentTime); `Se:` SeriesNumber; `Im:` posição/total do stack; `Lat/Pos:` ImageLaterality (ou Laterality)/ViewPosition |
| Inferior esquerdo | `KVP:` KVP kV; `Exp:` Exposure mAs (ou ExposureInuAs/1000); `Filter:` FilterType; `Zoom:` pixels de tela por pixel da imagem; `W: L:` VOI atual |
| Inferior direito | StationName; InstitutionName; `DFOV:` campo exibido em cm; `Calibration:` PixelSpacingCalibrationType, ou `Detector` quando só há ImagerPixelSpacing |

Réguas em centímetros (traço a cada 1 cm, maior a cada 5 cm; 5 cm quando denso) na borda direita e inferior, somente quando a instância tem PixelSpacing ou ImagerPixelSpacing; sem calibração não há régua nem DFOV. A escala usa coordenadas de mundo do Cornerstone, respeitando zoom/rotação. AccessionNumber, UIDs, Modality e StudyDescription continuam fora do arquivo. Marcadores gravados nos pixels (ex.: letra de lateralidade) vêm da própria imagem. O modo sem identificação permanece apenas com pixels, sem sobreposições.

## Identificação e origem

A identificação vem do metadata DICOM naturalizado da **instância exibida**, associado pelo loader Cornerstone à imagem já carregada no cache oficial, e não de texto do DOM, query string ou consulta nova ao Orthanc. Mapeamento: PatientName `(0010,0010)`, PatientID `(0010,0020)`, PatientBirthDate `(0010,0030)`, PatientSex `(0010,0040)`, PatientAge `(0010,1010)`, StudyDate `(0008,0020)`, AcquisitionTime `(0008,0032)`/ContentTime `(0008,0033)`, InstitutionName `(0008,0080)`, StationName `(0008,1010)`, SeriesNumber `(0020,0011)`, ImageLaterality `(0020,0062)`/Laterality `(0020,0060)`, ViewPosition `(0018,5101)`, KVP `(0018,0060)`, Exposure `(0018,1152)`/ExposureInuAs `(0018,1153)`, FilterType `(0018,1160)`, PixelSpacing `(0028,0030)`, ImagerPixelSpacing `(0018,1164)`, PixelSpacingCalibrationType `(0028,0A02)` e BurnedInAnnotation `(0028,0301)`. `Im:` usa posição atual/total do stack. `PatientName` usa a primeira representação disponível separada por `=`, depois junta componentes `^` com espaços; sem inferir nomes. A decodificação de caracteres usa o loader DICOM já instalado; representações ilegíveis/ausentes ficam vazias. Datas DICOM são formatadas como calendário (`dd-mmm-aaaa`, meses em português), sem conversão UTC; datas/horas inválidas são tratadas como ausentes.

O operador escolhe explicitamente o modo antes de gerar o arquivo. **Com identificação:** PNG/JPEG/PDF incluem as sobreposições nos cantos e réguas; PDF acrescenta nota de derivação. A interface avisa que o arquivo contém dados de paciente. **Sem identificação:** PNG/JPEG incluem apenas os pixels renderizados; PDF inclui a imagem, margem branca e nenhuma identificação textual. A exportação só é habilitada se a instância declarar `BurnedInAnnotation=NO` e o operador confirmar visualmente que os pixels não contêm identificação. Isso não é anonimização DICOM nem prova automática de ausência de PHI no conteúdo visual: a tag pode estar incorreta e a inspeção humana é necessária. Se ausente/desconhecida, a modalidade sem identificação fica bloqueada nesta V1. Não se capturam overlays do Viewer. Não são colocados dados de paciente em armazenamento do browser, URLs ou logs.

## Auditoria e autorização

`POST /api/viewer/exports`, sessão válida, CSRF e roles ADMIN/GESTOR/MEDICO. Corpo estrito: `{ "format": "PNG|JPEG|PDF", "identified": boolean }`. Responde 204 quando o evento é registrado; não recebe ator, nome do arquivo, IDs DICOM nem metadata arbitrária. O backend fixa `VIEWER_IMAGE_EXPORTED`, atribui o ator pela sessão e aceita apenas detalhe canônico `format=<formato>;identified=<bool>`. A Auditoria V1 apresenta categoria `viewer`/rótulo amigável. Em falha da auditoria, o frontend revalida a sessão antes de liberar o download e avisa que o registro não foi confirmado. Expiração de sessão bloqueia download. O evento indica preparação e solicitação de download; o servidor não consegue confirmar que o navegador salvou o arquivo. Não há transação distribuída entre auditoria e browser.

## Limites e validação

Exportação é da imagem atual do viewport ativo, inclusive em layouts múltiplos. A composição preserva o estado do Viewer e não exporta Cine, séries inteiras, DICOM ou medições SVG. O canvas representa a resolução renderizada disponível no viewport; não reprocessa os pixels originais em resolução nativa. O PDF incorpora a imagem de página em PNG; arquivos grandes podem exigir memória temporária. Campos identificados muito longos podem ser visualmente abreviados. A verificação BurnedInAnnotation e a confirmação visual não removem texto gravado nos pixels nem substituem revisão humana. Política de retenção da auditoria segue decisão futura.

Testes usam somente DICOM e metadata sintéticos (`tests/dicom-fixture.mjs` com tags técnicas fictícias). `tests/viewer-export.test.mjs` cobre o layout dos cantos, ausências e formatos de data/hora; o teste Chrome confere textos sobrepostos e dimensões iguais ao viewport. Incluem seis combinações de formato/modo, inspeção de bytes e PDF, identidade do canvas PNG, estado dos viewports e limpeza de Object URLs, além de autorização/validação da operação de auditoria no backend. Validação com imagens reais depende de execução controlada posterior pelo responsável, sem deploy nesta entrega.
