# Viewer DICOM — V4

## Histórico e validação

- **Viewer V0:** pipeline Orthanc → backend Go → Cornerstone validado com DICOM real pelo responsável pelo PACS.
- **Viewer V1:** séries, stack e scroll validados; setas apresentaram falha no teste real, corrigida na V2.
- **Viewer V2: VALIDADO COM DICOM REAL**, conforme validação informada pelo responsável: séries, stack, scroll, quatro Arrow keys, Window/Level, Zoom, Pan, Length, Angle, Probe, Rectangle ROI, Invert, Reset e troca de ferramentas.
- **Viewer V3:** thumbnails, layouts, múltiplos viewports, viewport ativo, Cine e gerenciamento de annotations. Teste manual no ambiente PACS apresentou funcionamento geral satisfatório, conforme informado pelo responsável.
- **Viewer V4:** auto-layout, maximização interna, Rotate/Flip, Fit, Reset validado com as novas apresentações, toolbar e atalhos. Implementação e testes exclusivamente sintéticos; nenhuma conexão ao Orthanc real, deploy ou mudança de produção nesta entrega.

## Arquitetura atual

`Browser → /api/... com sessão PACS → backend Go → internal/orthanc → Orthanc REST`.
A URL interna e as credenciais nunca entram nos imageIds. Nenhum novo endpoint,
DTO, migration, dependência ou mudança de configuração foi necessário nas V3/V4.
O cabeçalho mantém somente os dados que a Worklist já disponibilizava; acesso
direto à rota não consulta novos dados identificáveis.

Um **RenderingEngine por sessão do Viewer**, com até quatro StackViewports de IDs
estáveis. Cada viewport possui um **ToolGroup próprio**, seleção de ferramenta,
série, stack, índice, rotação, flips, inversão, estado de carga e fila de apresentação. Classes
Tools são registradas uma vez por aplicação. Apenas o grupo ativo recebe bindings
de interação; grupos inativos mantêm desenho de annotations com ferramentas Enabled.
Nenhuma sincronização de câmera, VOI ou scrolling foi adicionada.

`ViewerScreen` coordena layout, seleção ativa, metadados compartilhados e sessão.
`ViewportPane` mantém estados e Cine de seu viewport. `cornerstone.ts` administra
engine, cache oficial, fila de pixels e controladores. `tools.ts` administra grupos
e annotations pertencentes à sessão. `layout.ts` contém o algoritmo puro de atribuição e `presentation.ts` concentra leitura de apresentação e Fit. Não usa internals do Cornerstone.
Referência: [ToolGroups oficiais](https://www.cornerstonejs.org/docs/concepts/cornerstone-tools/toolGroups/).

## Layout e navegação

- **1x1:** um viewport; **1x2:** duas colunas; **2x2:** quatro viewports.
- O primeiro viewport abre a primeira série com instâncias. Novos slots são preenchidos automaticamente segundo o algoritmo V4 abaixo.
- Clique no viewport (borda destacada) e depois no card da série. O primeiro clique num viewport inativo só o ativa, sem começar um gesto da ferramenta. A série começa na primeira instância.
- Slots preservados mantêm câmera, série e índice quando o layout muda. Slots removidos param Cine e são destruídos; a atribuição da série é lembrada. Ao reabrir, a série restaurada começa na primeira imagem com apresentação inicial. Isso difere da maximização, que não remove nenhum slot.
- Scroll sobre o viewport ativo e ArrowRight/Down avançam; ArrowLeft/Up recuam. Scroll sobre viewport inativo não navega nenhum stack.
- Navegação manual respeita extremos. O contador confirma a imagem após `IMAGE_RENDERED`.
- Teclado ignora inputs, textarea, select, contenteditable, role textbox, modificadores e eventos já tratados. Há um único listener capture no window durante a tela.
- A correção V2 das setas permanece: não depende mais do foco exato no antigo main, que falhava quando o painel/toolbar/body tinha foco.

Falhas são locais: outro viewport permanece utilizável. Série vazia, carregamento,
erro e retry não removem o painel. Respostas obsoletas não substituem a nova seleção.
Uma transferência compartilhada já iniciada pode terminar e alimentar o cache;
seu resultado não muda uma seleção cancelada. Expiração 401 encerra toda a sessão
e segue o fluxo existente, incluindo cargas de thumbnails e Cine.

## Decisões V4: auto-layout e maximização

Auto-layout é determinístico, sem hanging protocol ou inferência clínica:

1. Mantém atribuições válidas dos slots que continuam visíveis, inclusive a principal.
2. Restaura primeiro escolhas **manuais** dos slots que voltam ao layout.
3. Restaura atribuições automáticas anteriores quando não duplicam uma série já usada.
4. Preenche somente slots vazios com a primeira série ainda não usada, na ordem da lista do backend.
5. Sem séries restantes, mantém o slot vazio; não carrega séries além dos slots necessários.

Atribuições manuais deliberadamente duplicadas são preservadas. Uma atribuição
automática antiga que passou a duplicar uma nova escolha manual é substituída
por outra disponível. Não reordena escolhas existentes. Séries sem instâncias
também pertencem à lista e exibem estado vazio; erro numa série não dispara
reorganização ou tentativas automáticas sem fim. Metadados continuam deduplicados
e imagens entram na fila compartilhada V3.

**Maximização interna:** botão discreto no canto superior direito de cada viewport;
“Restaurar viewport” ou **Escape** retorna ao layout. Não usa Fullscreen Browser API.
Optou-se pelo botão, sem duplo clique, para evitar conflito com gestos de medições.
O layout original permanece no estado React. O viewport escolhido ocupa toda a
grade; os demais continuam montados com `visibility: hidden`, dimensões válidas,
sem foco/interação. Preserva séries, índices, apresentação e annotations de todos.
Não há novos engines, grupos, caches ou timers. Selecionar outro layout enquanto
maximizado encerra a maximização e aplica explicitamente o novo layout.

Resize usa os ResizeObservers existentes e uma única tarefa requestAnimationFrame
por engine para agrupar notificações. Antes de React mudar layout/maximização, captura a apresentação oficial de cada stack (zoom e pan dependem das dimensões vigentes),
chama `RenderingEngine.resize(false, true)`, recalcula a câmera-base com `resetCamera`, reaplica `setViewPresentation` e renderiza.
Assim conserva zoom relativo e pan após atualizar a câmera-base ao novo tamanho,
evita corte por simples mudança de proporção e preserva rotação/flips. Descarta snapshots se o viewport foi removido ou a imagem mudou durante uma carga. Resize externo da janela usa a preservação de câmera padrão do engine. Não descarta
zoom/pan manual; Fit reenquadra quando desejado. Os testes verificam canvas atualizado,
imagem inicialmente ajustada inteira em transições repetidas para 1x2, pixels presentes em todos os viewports carregados e igualdade dos pixels após maximizar/restaurar com zoom/pan manuais.

## Decisões V4: apresentação, toolbar e atalhos

As APIs foram conferidas no código/distribuição instalada **5.11.0** e na
[referência pública StackViewport](https://www.cornerstonejs.org/docs/api/core/namespaces/types/classes/istackviewport/).
Não houve atualização para APIs de outra versão.

- **Rotate Left/Right:** `setViewPresentation({ rotation })`, em passos de −90/+90, normalizados entre 0 e 359 graus. Apenas apresentação do ativo.
- **Flip Horizontal/Vertical:** alterna o campo correspondente via `setCamera`. Botões refletem ligado/desligado; não alteram pixel data ou arquivo DICOM.
- **Fit:** centraliza e reajusta zoom/pan. Como `resetCamera` em 5.11 também desfaz orientação, restaura rotação/flips explicitamente. Projeta os quatro cantos da imagem usando `transformIndexToWorld`/`worldToCanvas` e ajusta o zoom para ocupar até 95% do espaço disponível, incluindo imagem retangular rotacionada. Não altera VOI, invert, annotations nem ferramenta ativa.
- **Reset:** remove flips e rotação, restaura enquadramento/zoom/pan e `resetProperties` recupera VOI/invert iniciais da imagem. Preserva annotations, stack/índice e ferramenta selecionada.
- **Cine:** Reset e maximização do ativo não iniciam, pausam ou reiniciam Cine. Ações de apresentação ficam indisponíveis durante carga/manipulação; não são enfileiradas para execução tardia. Maximizar outro viewport ativa-o e pausa o anterior, como qualquer troca de ativo V3.

A toolbar mantém ferramentas persistentes destacadas. Rotate/Fit/Reset são ações
momentâneas sem `aria-pressed`; Flip/Invert mostram estado de apresentação.
Novos ícones são SVGs locais do design system, com title e aria-label. Layout,
Cine e limpeza continuam na faixa separada. Não foi adicionada biblioteca de ícones.

Atalhos: setas e Delete/Backspace permanecem; **Escape** restaura o layout;
**R** gira +90° e **F** aplica Fit no ativo. R/F são alternativas diretas aos
botões frequentes, sem modificadores e sem repetição ao segurar a tecla.
Todos ignoram inputs/textarea/select/contenteditable/role textbox, eventos já
tratados e combinações modificadas. Reutilizam o único listener de teclado,
removido ao sair. Nenhum atalho altera o arquivo original.

## Stack e consumo

O backend ordena instâncias. O frontend registra o stack completo com
`StackViewport.setStack(imageIds, 0)`, usando apenas
`wadouri:/api/studies/{study}/series/{series}/instances/{instance}/dicom`.
Somente a imagem solicitada é baixada/decodificada; não se baixa o stack completo
antes da primeira renderização. Engine e ToolGroup não são recriados por imagem
ou por troca de série. [API StackViewport](https://www.cornerstonejs.org/docs/api/core/namespaces/types/classes/istackviewport/).

O adaptador aguarda `imageLoader.loadImage` antes de trocar pixels porque, em
5.11, o caminho GPU pode resolver a troca mesmo após falha do loader. Após sucesso,
registra o objeto resolvido com `cache.putImageLoadObject`, incluindo callback
`decache` para liberar o dataset Part 10 via API pública do loader. A confirmação
verifica imageId solicitado, imagem efetiva e dimensões. Erros brutos não são exibidos.

Há uma fila compartilhada de download/decodificação de pixels e um worker. Assim,
thumbnails e múltiplos viewports não disparam downloads DICOM concorrentes sem
limite, e pedidos do mesmo objeto reutilizam o cache oficial. A fila ignora pedidos
cancelados antes de iniciá-los. Durante uma imagem em carga, navegação adicional
naquele viewport é ignorada; Cine só agenda o próximo avanço após concluir o atual.
Metadados das séries são deduplicados na sessão, com remoção de promessas falhas
para permitir retry. Não há cache customizado de pixels.

O orçamento oficial de imagens é **256 MiB**. Não representa limite total de RAM:
HTTP Part 10, datasets, WebGL e decoder também consomem memória. Cache não é
purgado a cada série, pois é compartilhado pelos viewports. Sair da tela limpa
imagens, datasets e metadados depois de encerrar operações pendentes.

## Thumbnails

`IntersectionObserver`, com root no painel lateral, agenda somente cards visíveis.
Uma miniatura utiliza **a primeira instância na ordem retornada pelo backend**,
unca uma série inteira. Usa o mesmo endpoint DICOM autenticado, cache oficial e
fila única de pixels, depois `utilities.renderToCanvasCPU` em canvas pequeno.
Não cria engine/viewport extra, blob persistente, endpoint preview ou conexão
direta ao Orthanc. Ao sair da área visível, uma carga ainda não iniciada é cancelada.
Miniaturas prontas são mantidas enquanto a tela existir.

Falhas e séries vazias mostram placeholder e não desabilitam o card. Cada miniatura
pode transferir um DICOM completo (até o limite do gateway); uma imagem já
carregada é reutilizada pelo viewport. Não há thumbnail server-side ou garantia
de baixo custo para objetos individuais muito grandes. A fila é conservadora:
uma thumbnail já em transferência pode atrasar uma imagem solicitada pelo usuário.
Não representa qualidade diagnóstica ou frame clínico escolhido.

## Ferramentas, overlays e Cine

A toolbar atua exclusivamente no viewport ativo. Mouse esquerdo controla a
ferramenta selecionada; scroll e setas mantêm a navegação do stack.

| Ação | Comportamento |
| --- | --- |
| Window/Level | WindowLevelTool; VOI inicial fornecida pelo DICOM/Cornerstone |
| Zoom | ZoomTool; limites 0,1–20 da API oficial |
| Pan | PanTool |
| Length | LengthTool; unidade/calibração fornecida pelo Cornerstone |
| Angle | AngleTool |
| Probe | ProbeTool; sem interpretação clínica adicional |
| Rectangle ROI | RectangleROITool; estatísticas oficiais |
| Invert | Alterna propriedade invert apenas no viewport ativo |
| Rotate Left/Right | Rotação de −90/+90 graus no ativo |
| Flip Horizontal/Vertical | Alterna apresentação por eixo no ativo |
| Fit | Enquadra, preservando VOI, invert, rotação, flips e annotations |
| Reset | Restaura câmera, pan/zoom, flips, rotação, VOI e invert; preserva annotations e ferramenta |

Sem Pixel Spacing não inventa mm: fixture sem calibração é apresentada em px.
Trocar série restaura apresentação DICOM da nova série, incluindo invert desligado
conforme propriedades iniciais. Overlays mostram modalidade, descrição, número
da série, contador e Cine/FPS; não buscam PHI adicional.

**Cine:** Play/Pause no ativo, loop contínuo, FPS ajustável **1–30**, inicial **10**.
Esse valor é preferência de reprodução, não frame rate clínico extraído do DICOM.
Usa setTimeout sequencial após aguardar o avanço/render, sem intervalos concorrentes
ou catch-up. FPS efetivo pode ser inferior ao solicitado por transferência/decoder.
Pausa ao navegar manualmente, mudar FPS, série ou viewport ativo; erro, expiração,
remoção do slot e saída encerram timers. Não há cine multiframe interno nesta versão.

## Annotations e remoção

Annotations ficam temporariamente na memória da sessão; A → B → A preserva as
de A. Remover um slot não apaga medições daquela série. Não envia ao Orthanc,
não persiste no banco/localStorage, não exporta e não cria DICOM SR.

O gerenciador oficial relaciona annotations à imagem/Frame of Reference. A mesma
imagem aberta em dois viewports compartilha a mesma annotation; não foi criada
sincronização própria. Series distintas não recebem medições umas das outras.
UIDs de annotations pertencentes a este engine são acompanhados para limitar
remoção/cleanup à sessão.

- **Delete/Backspace:** remove uma annotation selecionada da imagem atualmente exibida no viewport ativo. Se houver seleção múltipla aplicável, não remove nada: escolha apenas uma. Ignora editáveis/modificadores. Não apaga todas as annotations nem navega para outra página.
- **Limpar medições:** pausa Cine, cancela manipulação em andamento e pede confirmação com quantidade; limpa somente annotations da série ativa (todas as suas imagens). A confirmação explicita que outros viewports da **mesma série** também serão afetados. Outras séries são preservadas. Cancelar não remove nada; sem annotations é no-op.
- **Reset:** apenas apresentação; nunca remove medições.

Usa `annotation.selection.getAnnotationsSelected`, `annotation.state.getAnnotation`
e `removeAnnotation`; filtra referências de imagem pelo stack ativo e pela
propriedade da sessão. Referência: [seleção oficial de annotations](https://www.cornerstonejs.org/docs/concepts/cornerstone-tools/annotation/selection/).

## Cleanup

Cada slot interrompe Cine, cancela sua seleção, remove wheel listener/ResizeObserver,
interrompe manipulações e aguarda sua fila antes de remover viewport e ToolGroup.
Recriar rapidamente o mesmo slot espera o descarte anterior. Não remove annotations
no descarte individual. A tela remove listener de teclado e expiração, aborta
HTTP, aguarda filas/decoder, remove annotations próprias/histórico de gestos e
finalmente destrói engine e caches. Uma nova sessão aguarda o cleanup anterior.
O requestAnimationFrame de resize é cancelado ao encerrar a sessão. Não há novos timers persistentes nem requests disparados após expiração.

## Endpoints da aplicação

Todos exigem sessão válida e ADMIN, GESTOR ou MEDICO no backend. Sessão ausente
ou expirada retorna 401; outro perfil retorna 403. As operações são GET e mantêm
a política CSRF existente para leitura. Não há restrição adicional por unidade,
assim como na Worklist atual.

| Endpoint GET | Resposta |
| --- | --- |
| `/api/studies/{studyID}/series` | `{items: [{orthancSeriesId, description, number, modality, instanceCount}]}` |
| `/api/studies/{studyID}/series/{seriesID}/instances` | `{items: [{orthancInstanceId, number}]}` |
| `/api/studies/{studyID}/series/{seriesID}/instances/{instanceID}/dicom` | Stream `application/dicom`, `Content-Disposition: inline` |

`number` da série é string DICOM (vazia se ausente); da instância é inteiro ou
null (`MainDicomTags.InstanceNumber`, tag 0020,0013). Listas vazias são `[]`. Tags opcionais ausentes não
causam falha.

### Ordenação de instâncias

`GET /series/{id}/instances?expand=true` já fornece MainDicomTags. Não são
adicionadas chamadas por instância para buscar tags. `number` do DTO agora
representa explicitamente InstanceNumber, substituindo o IndexInSeries usado
no V0. O formato JSON permanece igual. A tag está entre as
[Main DICOM Tags do Orthanc](https://orthanc.uclouvain.be/book/faq/main-dicom-tags.html).

A ordenação usa InstanceNumber numérico crescente (inteiro de 32 bits, removendo
espaços nas extremidades). Tags ausentes, vazias, inválidas ou fora desse intervalo
produzem null e ficam depois das numeradas. Valores repetidos e itens sem número
usam ID Orthanc em ordem lexicográfica como desempate determinístico. O
IndexInSeries não é usado como substituto implícito. Não se infere anatomia,
sequência clínica ou posição espacial. Séries continuam ordenadas por SeriesNumber
numérico, com ID como desempate. O frontend não reordena a lista de instâncias.

IDs devem ter o formato Orthanc de cinco grupos de oito hexadecimais minúsculos.
O servidor verifica ParentStudy e associação da instância à série antes de
entregar conteúdo. URLs, query strings e Range não são aceitos nestes endpoints;
não há proxy genérico, transcode ou parâmetros upstream controlados pelo browser.

Erros JSON seguem o envelope existente: 400 para entrada inválida, 404 para
recurso/associação ausente, 502 para PACS indisponível/conteúdo incompatível/limite,
503 para configuração indisponível e 504 para timeout antes de iniciar o stream.
Falhas de autenticação do Orthanc não viram 401 do usuário. Após iniciar os bytes,
uma falha interrompe a conexão, sem anexar JSON a um DICOM parcial.

## Orthanc REST e streaming

Operações upstream permitidas nesta integração:

- `GET /studies/{id}/series?expand=true`;
- `GET /series/{id}` para conferir estudo e instâncias;
- `GET /series/{id}/instances?expand=true`;
- `GET /instances/{id}/file` para um arquivo DICOM Part 10.

A escolha usa REST já disponível, sem exigir plugin DICOMweb. O loader oficial
Cornerstone lê o arquivo com imageId `wadouri:/api/.../dicom`. Este prefixo é o
esquema do loader para arquivos; o backend não implementa WADO-RS nem um serviço
DICOMweb nesta entrega. Não há POST/PUT/DELETE upstream no fluxo do Viewer.

O backend valida MIME e prefixo Part 10 (`DICM`) lendo somente 132 bytes antes de
iniciar a resposta, e transmite com buffer de 32 KiB. Não faz ReadAll do DICOM
nem grava arquivos. Limite de 128 MiB por arquivo, inclusive quando o tamanho
upstream é desconhecido. Listas JSON têm limite de 4 MiB; até 2000 séries por
estudo e 10000 instâncias por série. Estes limites não são paginação e podem
rejeitar estudos excepcionalmente grandes. Headers upstream não são copiados.

O timeout configurado limita cada operação completa, incluindo verificação de
parentesco e streaming; o prazo de escrita HTTP recebe cinco segundos adicionais.
Contexto cancelado/cliente desconectado encerra a operação e fecha o corpo upstream.
O browser tem timeout de transferência de 130 segundos e recebe o arquivo em
memória para decodificação, apesar de o backend fazer streaming.

## Segurança e privacidade

O transporte mantém validação SSRF na resolução/discagem, endereços internos
permitidos, bloqueio de destinos especiais, redirects desabilitados e ausência
de proxy de ambiente. TLS obedece verifyTLS salvo pelo ADMIN e permanece local
ao transporte Orthanc. Credenciais só são aplicadas no servidor.

As respostas da API preservam `Cache-Control: no-store`. Os logs de rotas do
Viewer substituem IDs por placeholders; não registram metadados, bytes DICOM,
query, headers de autenticação ou resposta bruta. Falhas de transferência geram
somente mensagem operacional fixa. Nenhum evento de auditoria clínico novo foi
criado. Loggers configuráveis do Cornerstone ficam desabilitados, e erros brutos
do decoder não são exibidos. O histórico do navegador contém somente o ID
Orthanc; dados do cabeçalho ficam em memória e vêm da Worklist. No acesso direto
à rota, o cabeçalho usa “Estudo DICOM”, sem consulta adicional de paciente.

Access logs de proxies externos devem igualmente evitar identificadores nestas
rotas; infraestrutura externa não foi alterada. As fontes externas preexistentes
do design foram preservadas e são bloqueadas no teste sintético de navegador.

## Dependências, testes e limitações V4

Nenhuma dependência adicionada ou atualizada. Cornerstone core, tools, metadata
e dicom-image-loader continuam em **5.11.0**. Sem npm audit fix.

Todos os checks abaixo passaram na validação local: `gofmt -l` sem diferenças; `go vet ./...`, `go test ./...`,
`go build ./...` e race em `internal/httpapi` e `internal/orthanc` para regressão.
Frontend: typecheck, 17 testes Node (incluindo auto-layout e temporização/cancelamento/falha de
Cine), build e smoke Chrome com servidor sintético. Comandos:

```sh
cd backend
gofmt -l .
go vet ./...
go test ./...
go build ./...
go test -race ./internal/httpapi ./internal/orthanc
cd ../frontend
npm run typecheck
node --test tests/*.test.mjs
npm run build
node tests/viewer-smoke.mjs
```

Chrome usa perfil temporário, fixtures grayscale 32×32 e retangular 48×24 e eventos reais CDP de
mouse/teclado. Não usa .env, banco, backend vivo ou PACS real. `CHROME_BIN` pode
substituir o caminho macOS. Testa canvas e SVG reais; jsdom não demonstra a
renderização GPU/Tools. Backend continua coberto separadamente por httptest.

Cobertura browser: ferramentas V2, layouts e mudanças rápidas, ativo, stacks e
contadores independentes, Invert/Reset isolados, falha localizada, Cine play/pause,
avanço/loop/paradas, thumbnails/placeholder, seleção obsoleta, campos editáveis,
Delete/Backspace, confirmação/cancelamento, preservação por série e compartilhamento
nativo, expiração durante Cine e thumbnails, interrupção de requests após 401 e reabertura sem listeners/annotations acumulados. Na V4, inclui auto-preenchimento, restauração manual, Rotate/Flip, Fit de imagem retangular, preservação de annotations/VOI/orientação, Reset isolado, maximização/restauração e Escape/R/F com proteção de editáveis. Compara pixels e referências de canvas e confirma ausência de novos downloads ao maximizar.

Limitações: WebGL2; até quatro viewports; somente primeiro frame de cada instância
multiframe; sem sincronização, MPR, SR, persistência, download ou impressão.
A miniatura CPU pode falhar em formatos que funcionem no viewport GPU; placeholder
não impede seleção. Não há validação de todos os codecs/modalidades/equipamentos
ou de uso diagnóstico. V4 precisa de validação manual controlada pelo responsável. Sem reconstrução 3D, hanging protocol ou recursos avançados. Nenhum deploy foi feito.

Permanecem warnings Vite de chunks grandes e módulos Node externalizados em
codecs/XML. A auditoria V2 registrou **12 vulnerabilidades npm (6 moderadas e 6
altas)** e depreciação transitiva de lodash.get. Não se afirma auditoria atualizada
nem segurança aprovada; nenhuma dessas pendências foi corrigida nesta etapa.

## Arquivos V3

Criados:
- `frontend/src/viewer/ViewportPane.tsx`
- `frontend/src/viewer/SeriesThumbnail.tsx`
- `frontend/src/viewer/cine.ts`
- `frontend/tests/viewer-cine.test.mjs`
- `frontend/tests/viewer-v3-browser.mjs`
- `frontend/tests/viewer-annotations-browser.mjs`

Modificados:
- `frontend/src/screens/ViewerScreen.tsx`
- `frontend/src/viewer/cornerstone.ts`
- `frontend/src/viewer/tools.ts`
- `frontend/tests/viewer-smoke.mjs`
- `frontend/tests/viewer-tools-browser.mjs`
- `docs/VIEWER.md`

Backend, endpoints, DTOs, autenticação, banco, Worklist, configuração Orthanc,
Docker e dependências permanecem inalterados.

## Arquivos V4

Criados:

- `frontend/src/viewer/layout.ts`
- `frontend/src/viewer/presentation.ts`
- `frontend/tests/viewer-layout.test.mjs`
- `frontend/tests/viewer-v4-browser.mjs`

Modificados:

- `frontend/src/screens/ViewerScreen.tsx`
- `frontend/src/viewer/ViewportPane.tsx`
- `frontend/src/viewer/ViewerToolbar.tsx`
- `frontend/src/viewer/cornerstone.ts`
- `frontend/src/design-system/Icon.tsx`
- `frontend/tests/dicom-fixture.mjs`
- `frontend/tests/viewer-smoke.mjs`
- `docs/VIEWER.md`

Nenhuma dependência, endpoint, backend, banco, autenticação, Worklist ou
infraestrutura foi alterada na V4. Não houve migração nem deploy.
