# Viewer DICOM — V2: ferramentas básicas

## Histórico

- **Viewer V0:** pipeline Orthanc → backend → Cornerstone validado com DICOM
  real no ambiente PACS pelo responsável pelo sistema.
- **Viewer V1:** séries, stack e navegação por scroll validados com DICOM real.
  As setas apresentaram falha no teste real; a correção integra a V2.
- **Viewer V2:** ferramentas básicas e correção do teclado, verificadas com
  fixtures sintéticas e Chrome. Nenhum acesso ao Orthanc real nesta entrega.

## Fluxo e escopo

`Worklist → /viewer/{orthancStudyId} → API Go autenticada → internal/orthanc → Orthanc REST`.
O navegador usa exclusivamente caminhos `/api/...` da mesma origem para conteúdo
clínico. A URL interna e a credencial do Orthanc não participam das respostas.
A composição reutiliza o cliente Orthanc e `settings.OrthancConnection`, incluindo
a decifragem AES-GCM existente. Não há migration, cópia de arquivos ou mudança
na configuração do PACS, autenticação, rede Docker ou exposição de portas.

O clique/Enter/Espaço na Worklist abre a tela escura com uma viewport. O painel
lateral mostra todas as séries, com número, descrição, modalidade e quantidade
de instâncias. Descrição vazia usa “Série sem descrição”; IDs internos não são
exibidos. O card selecionado possui destaque e `aria-pressed`.

A primeira série com instâncias é selecionada inicialmente; se todas estiverem
vazias, seleciona a primeira e mostra o estado vazio. Clicar em outro card cancela
a seleção anterior, consulta suas instâncias e começa pela primeira imagem.
Respostas de seleções obsoletas não podem substituir a seleção atual. Erros em
uma série mantêm o painel disponível para escolher outra.

## Stack, navegação e carregamento

O frontend conserva a ordem entregue pelo backend e monta um imageId por
instância: `wadouri:/api/studies/{study}/series/{series}/instances/{instance}/dicom`.
`StackViewport.setStack(imageIds, 0)` registra a série completa; somente a imagem
inicial é solicitada. Antes de cada troca, o adaptador aguarda o loader oficial
`imageLoader.loadImage` da imagem escolhida e usa `cache.putImageSync` do cache
padrão. Isso permite capturar erros antes de trocar pixels: na versão 5.11, a
promessa do caminho GPU pode concluir mesmo quando o loader falha. Não é um
cache customizado; não existe armazenamento paralelo nem prefetch. A confirmação
verifica também se a imagem efetivamente presente no viewport corresponde ao
imageId solicitado. `setImageIdIndex` navega sob demanda, sem prefetch ou download
antecipado da série. Engine e viewport são mantidos ao trocar imagem e série.
A API utilizada é descrita na [referência StackViewport](https://www.cornerstonejs.org/docs/api/core/namespaces/types/classes/istackviewport/).

Scroll vertical sobre o viewport, ArrowDown/Right avançam; ArrowUp/Left recuam.
As teclas ficam ativas enquanto a tela Viewer estiver montada, sem capturar
inputs/editáveis ou combinações com modificadores. Clicar numa série coloca foco na área da imagem.
Não há navegação circular: os extremos permanecem na primeira/última imagem.
O contador “Imagem X / Y” só confirma a imagem depois de `IMAGE_RENDERED`.

As cargas são serializadas: durante uma carga, novos eventos de navegação são
ignorados, evitando fila ilimitada e rajadas de requests. A troca de série cancela
a requisição anterior; um decoder já iniciado termina antes da próxima carga.
Falha de imagem apresenta mensagem fixa, permite avançar/recuar ou selecionar
outra série; “Tentar novamente” recarrega a série desde a primeira instância.
Há estados separados para lista de séries, carga de série, carga de imagem,
estudo vazio, série vazia, erros e sessão expirada, inclusive 401 do arquivo.

Usa somente o cache padrão do Cornerstone, com orçamento de imagens de 256 MiB.
Os caches de imagem, dataset Part 10 e metadados são limpos entre séries e ao
encerrar a tela. Esse orçamento não é um limite absoluto de memória do processo:
datasets, WebGL e buffers de decodificação também consomem memória. Não há cache
customizado. Um único worker faz a decodificação; sair cancela HTTP e libera a
viewport após encerrar a operação corrente.

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

## Dependências e validação

- `@cornerstonejs/core`, `@cornerstonejs/dicom-image-loader` e
  `@cornerstonejs/metadata`: 5.11.0, versões fixadas;
- `events`: 3.3.0, compatibilidade EventEmitter de dependência transitiva;
- `@originjs/vite-plugin-commonjs`: 1.0.3, desenvolvimento/build Vite.

Vite usa worker ES e configuração CommonJS recomendada para o loader. A carga do
engine é dinâmica ao entrar no Viewer. `@cornerstonejs/tools` 5.11.0 foi adicionado na V2; seu peer dependency exige
exatamente `@cornerstonejs/core` 5.11.0, já instalado. As versões dos outros
pacotes Cornerstone foram preservadas.
O build emite alertas de tamanho do chunk e módulos Node externos nas bibliotecas
de codecs/XML. A instalação/auditoria npm da V2 reportou 12 vulnerabilidades (6 moderadas e
6 altas) no grafo instalado; não foi aplicado audit fix ou atualização ampla
nesta entrega. Estes alertas precisam de revisão antes de produção.

Referências: [REST Orthanc](https://orthanc.uclouvain.be/api/index.html),
[Stack Cornerstone](https://www.cornerstonejs.org/docs/tutorials/basic-stack/),
[integração Vite](https://www.cornerstonejs.org/docs/getting-started/vue-angular-react-etc/).

Testes Go usam httptest e stores em memória: autorização em todos os endpoints,
IDs inválidos, relações entre recursos, vazio, erro sanitizado, timeout, JSON/MIME/
Part 10 inválidos, tamanho excedido, redirects/SSRF, streaming antes de terminar
a resposta upstream, cancelamento e interrupção sem JSON anexado. Há verificações
de ausência de credenciais e identificadores nos erros/logs.

Testes frontend verificam caminhos fixos, sessão/cancelamento, seleção inicial,
stack completo na ordem do backend, vazio, erros e proteção de inputs/editáveis. O smoke test abre o build real no Chrome com perfil
temporário e servidor fictício, navega desde a Worklist e exige IMAGE_RENDERED.
A fixture gera em memória um DICOM grayscale 32×32 Explicit VR Little Endian,
com dados exclusivamente fictícios. Não usa .env ou backend/Orthanc real.

```sh
cd backend
gofmt -l .
go vet ./...
go test ./...
go test -race ./internal/orthanc ./internal/httpapi
go build ./...
cd ../frontend
npm run typecheck
node --test tests/*.test.mjs
npm run build
node tests/viewer-smoke.mjs
```

O smoke test requer Node com WebSocket nativo e Chrome; no macOS usa o caminho
padrão do aplicativo, ou `CHROME_BIN`. A captura fica no diretório temporário do
sistema. Não depende de PostgreSQL ou dados reais.

## Limitações e próxima validação

Uma viewport WebGL2, navegação entre instâncias e apenas primeiro frame de cada
instância multiframe. Não há navegação interna de frames ou thumbnails,
download UI, impressão, MPR, cine ou alterações no Orthanc. Objetos sem pixels (por exemplo SR), formatos
não suportados e falhas de decodificação apresentam erro controlado. Não se
promete suporte validado a todas as modalidades/transfer syntaxes; o teste visual
cobre apenas a fixture não comprimida. A próxima revisão deve confirmar o fluxo
no ambiente controlado e decidir quais formatos/modalidades validar antes de
expandir funcionalidades. Nenhuma conexão real foi feita nesta implementação.

## Resultado da validação desta entrega

`gofmt -l .` sem saída; `go vet ./...`, `go test ./...`,
`go test -race ./internal/orthanc ./internal/httpapi` e `go build ./...` passaram.
Frontend: typecheck, nove testes Node e build passaram. O smoke test Chrome
passou com navegação entre imagens/séries e a captura foi inspecionada: gradiente
sintético efetivamente renderizado.
A validação de browser cobre o build frontend com API simulada; a integração Go
com REST/streaming é coberta separadamente por httptest. Não equivale a uma
validação de ponta a ponta contra o Orthanc real.

## Arquivos da entrega V0 (histórico)

Criados:

- `backend/internal/viewer/viewer.go`
- `backend/internal/orthanc/viewer.go`
- `backend/internal/orthanc/viewer_test.go`
- `backend/internal/httpapi/viewer_handlers.go`
- `backend/internal/httpapi/viewer_test.go`
- `frontend/src/viewer/selection.ts`
- `frontend/src/viewer/cornerstone.ts`
- `frontend/tests/viewer-api.test.mjs`
- `frontend/tests/dicom-fixture.mjs`
- `frontend/tests/viewer-smoke.mjs`
- `docs/VIEWER.md`

Modificados:

- `backend/cmd/server/serve.go`
- `backend/internal/orthanc/client.go`
- `backend/internal/httpapi/api.go`
- `backend/internal/httpapi/api_test.go`
- `backend/internal/httpapi/middleware.go`
- `backend/internal/httpapi/errors.go`
- `frontend/src/api/client.ts`
- `frontend/src/App.tsx`
- `frontend/src/screens/ExamesScreen.tsx`
- `frontend/src/screens/ViewerScreen.tsx`
- `frontend/src/dev/PainelDeTelas.tsx`
- `frontend/vite.config.ts`
- `frontend/package.json`
- `frontend/package-lock.json`
- `README.md`
- `docs/BACKEND.md`
- `docs/WORKLIST.md`

Artefatos de build/dependências são gerados pelas verificações; não há alteração
de Dockerfile, Compose, migrations, banco, credenciais ou configuração Orthanc.


## Alterações V1

Nenhum endpoint, dependência ou arquivo de código novo. Formatos DTO preservados;
apenas a origem/semântica de `number` da instância passa a ser InstanceNumber.
Nenhuma alteração em autenticação, Worklist, banco, configuração Orthanc ou Docker.

Arquivos modificados:

- `backend/internal/orthanc/viewer.go`
- `backend/internal/orthanc/viewer_test.go`
- `backend/internal/viewer/viewer.go`
- `frontend/src/screens/ViewerScreen.tsx`
- `frontend/src/viewer/cornerstone.ts`
- `frontend/src/viewer/selection.ts`
- `frontend/tests/viewer-api.test.mjs`
- `frontend/tests/viewer-smoke.mjs`
- `docs/VIEWER.md`

Testes V1 acrescentam InstanceNumber explícito (inclusive divergente do
IndexInSeries), ordenação numérica, empate, tag vazia/inválida, fallback e
associação de instância inválida. Os testes anteriores continuam verificando
séries/metadados, série inválida/vazia, associação estudo/série, sessão/perfis,
streaming e ausência de configuração/credenciais em respostas.

No Chrome, verifica stack sob demanda, reutilização da viewport, contador,
quatro setas, scroll, limites, troca de série, série vazia, erro de série,
seleção obsoleta, falha de imagem com recuperação e expiração de sessão no
arquivo DICOM. O teste bloqueia fontes externas e usa exclusivamente fixtures.
As vulnerabilidades npm permanecem para análise separada; nenhum audit fix.


## Viewer V2 — ToolGroup e ferramentas

`frontend/src/viewer/tools.ts` registra as sete classes oficiais uma única vez,
após inicializar o core e antes de habilitar a viewport. Cria um ToolGroup com
ID derivado do RenderingEngine, associa explicitamente engine/viewport e o
reutiliza durante a navegação e troca de séries. A toolbar é implementada em
`ViewerToolbar.tsx`, com os ícones SVG locais, sem biblioteca adicional de ícones.
Referência: [ToolGroups e bindings oficiais](https://www.cornerstonejs.org/docs/tutorials/basic-manipulation-tool/).

Somente a ferramenta selecionada fica Active com `MouseBindings.Primary`.
As outras ficam Passive, permitindo a interação padrão com annotations já
criadas. Ao carregar metadados/pixels, ficam Enabled (visíveis, sem interação).
Nenhum binding de botão direito/meio, touch especial ou roda foi adicionado.
Scroll e setas continuam usando a navegação serializada V1, sem StackScrollTool
concorrente. Durante um gesto de ferramenta, a navegação é ignorada; troca de
série cancela a manipulação ativa pela API oficial.

| Controle | Comportamento |
| --- | --- |
| Window/Level | WindowLevelTool; esquerdo + arrastar, sem presets |
| Zoom | ZoomTool; esquerdo + arrastar, zoom central e limites oficiais configurados de 0,1 a 20 |
| Pan | PanTool; esquerdo + arrastar |
| Length | LengthTool, cálculos/texto oficiais; arrastar ou completar os pontos conforme padrão da biblioteca |
| Angle | AngleTool, três pontos, ângulo calculado pela biblioteca |
| Probe | ProbeTool, posição/valor/unidade disponíveis na imagem |
| Rectangle ROI | RectangleROITool, área e estatísticas fornecidas pela biblioteca |
| Invert | Alterna `viewport.setProperties({invert})`, sem alterar pixels do arquivo |
| Reset | Restaura a apresentação DICOM da imagem corrente, preservando annotations |

A ferramenta ativa e Invert têm indicação visual e `aria-pressed`. A toolbar
fica indisponível durante carga, vazio ou erro. A ferramenta selecionada permanece
ao trocar série; a apresentação volta ao padrão DICOM. Invert reflete a propriedade
efetiva do viewport, inclusive a polaridade inicial da imagem. Não são criadas
interpretações clínicas, unidades, presets ou valores derivados pela aplicação.

### Calibração e unidades

Length/ROI usam o tratamento de calibração oficial. Sem Pixel Spacing, a fixture
foi medida em px, não mm; área usa a unidade correspondente da biblioteca.
Não se atribui espaçamento artificial. A presença de espaçamento DICOM também
não garante por si só calibração anatômica de uma radiografia: a validade depende
da aquisição/metadados. Probe usa somente o valor/unidade que a biblioteca suporta.
Referência: [unidades calibradas do Cornerstone](https://www.cornerstonejs.org/docs/api/tools/namespaces/utilities/functions/getcalibratedlengthunitsandscale/).

### Reset e annotations

Reset usa `setCamera` para remover flips, `setViewPresentation` para rotação zero,
`resetCamera` para enquadramento/zoom/pan e `resetProperties` para VOI e inversão
iniciais da imagem corrente, seguidos de render. Assim, WC/WW DICOM são respeitados
quando presentes; caso contrário, vale o padrão calculado pelo Cornerstone.
Reset não remove medições nem muda a ferramenta selecionada.

Annotations ficam somente na memória da tela atual, associadas à imagem pelos
mecanismos oficiais. Permanecem ao navegar A → B → A e não são mostradas em
outra instância/série. Os UIDs de annotations criadas por este engine/viewport
são acompanhados em memória para remoção no cleanup. Não há API, banco, arquivo,
localStorage, DICOM SR ou exportação de annotations.

Ao desmontar: aborta requisições, interrompe manipulações, aguarda o decoder,
remove annotations próprias e listener de criação, limpa o histórico interno de
gestos do único Viewer, remove/destroi o ToolGroup, destrói o RenderingEngine e
limpa caches. Registros globais das classes Tools permanecem inicializados para
reabertura, sem novo registro das mesmas classes. Reset não executa esse cleanup.

### Correção das setas

Na V1, o listener estava no `<main>` e dependia de seu foco (ou de um descendente).
Foco no painel, botão ou body não propagava o evento por esse elemento. O teste
anterior disparava `KeyboardEvent` diretamente no main, escondendo a fragilidade.
Esta é a causa identificada no código; não houve inspeção do ambiente real nesta
entrega. O listener agora fica no window em capture durante a montagem da tela,
com remoção simétrica no cleanup. Ignora input, textarea, select, contenteditable,
role textbox, teclas com modificadores e eventos já tratados. Não depende de
refocar artificialmente o canvas para navegar.

O smoke test usa `Input.dispatchKeyEvent` do Chrome, incluindo foco no painel,
quatro setas, extremos do stack e editáveis. Também confere que reabrir não acumula
listeners e que fechar reduz em um o listener keydown da janela.

### Testes V2

As interações gráficas são testadas no Chrome real/headless, não em jsdom:

- seleção/estado ativo das sete ferramentas;
- WL, Zoom e Pan por arrastos reais e mudança dos pixels apresentados;
- Invert e Reset por estado visual e comparação do canvas;
- Length em mm com spacing e em px sem spacing;
- Angle com ângulo exibido, Probe com coordenadas/valor, ROI com estatísticas;
- Reset conserva annotations; A → B → A mantém associação correta;
- reabertura remove annotations antigas e não duplica viewport/listener;
- contador, carga sob demanda, falhas e expiração de sessão mantêm os testes V1.

`tests/viewer-tools-browser.mjs` é chamado pelo smoke existente. Captura a tela
com medições fictícias em `pacs-viewer-tools-synthetic.png` no diretório temporário.
Não injeta APIs de inspeção na aplicação de produção. As fixtures e respostas
são inteiramente sintéticas. O teste visual não comprova exatidão diagnóstica para
todas as modalidades, calibrações, codecs ou equipamentos; requer validação
controlada antes de utilização clínica. Não há teste real do PACS nesta entrega.

### Dependências e avisos V2

Adicionado `@cornerstonejs/tools` **5.11.0** (versão exata), mais suas dependências
resolvidas no lockfile. Nenhuma atualização deliberada de pacote não relacionado.
`npm audit` reportou **12 vulnerabilidades: 6 moderadas e 6 altas**, inclusive
avisos propagados por dependências transitivas do Cornerstone. A sugestão de
correção do tools envolve versão incompatível; não foi aplicada. `lodash.get`
transitivo também emitiu aviso de depreciação. Permanecem os avisos de módulos
Node externos e tamanho dos chunks; Tools adiciona seu worker oficial ao build.
Nenhum `npm audit fix`, downgrade ou correção ampla foi executado.

### Arquivos V2

Criados:

- `frontend/src/viewer/tools.ts`
- `frontend/src/viewer/ViewerToolbar.tsx`
- `frontend/tests/viewer-tools-browser.mjs`

Modificados:

- `frontend/src/viewer/cornerstone.ts`
- `frontend/src/screens/ViewerScreen.tsx`
- `frontend/src/design-system/Icon.tsx`
- `frontend/package.json`
- `frontend/package-lock.json`
- `frontend/tests/dicom-fixture.mjs`
- `frontend/tests/viewer-smoke.mjs`
- `docs/VIEWER.md`

Backend, autenticação, Worklist, endpoints, banco e configuração Orthanc permanecem
inalterados. Nenhuma porta/rede/produção foi alterada. A próxima etapa depende da
revisão desta entrega e validação manual do V2 no ambiente controlado; MPR, cine,
multiview e persistência continuam fora do escopo.

### Resultado final V2

Passaram: gofmt (verificação sem diferenças), go vet, go test, go build e
`go test -race ./internal/orthanc ./internal/httpapi`. O backend não foi modificado.
No frontend passaram typecheck, os nove testes Node existentes, build e smoke
completo do Chrome com ferramentas, teclado real, roda real, recuperação de falhas
e cleanup. A captura com Length, Angle, Probe e Rectangle ROI foi inspecionada.
`npm audit` terminou com código 1 por reportar as vulnerabilidades descritas acima;
isso não foi tratado como resultado aprovado de segurança nem corrigido nesta etapa.
