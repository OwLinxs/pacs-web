# Worklist V2 — consulta clínica READ-ONLY

## Arquitetura e escopo

`ExamesScreen → GET /api/studies → httpapi → internal/orthanc → Orthanc`.
`internal/studies` define DTO e validação; `internal/settings` fornece configuração
salva e credencial decifrada pelo mecanismo existente. Mantidos autenticação,
RBAC ADMIN/GESTOR/MEDICO, TLS configurável, proteções SSRF, bloqueio de redirects,
limites de resposta e contexto cancelável. Sem endpoint genérico ou acesso direto
Browser → Orthanc. Não há cópia de estudos no PostgreSQL da aplicação.

Nenhuma migration, dependência, infraestrutura ou configuração Orthanc foi
alterada. Desenvolvimento e testes usam exclusivamente fixtures sintéticas.
A validação real prévia da Worklist V1 foi informada pelo responsável pelo PACS;
a V2 requer revisão e validação manual posterior no ambiente autorizado.

## Contrato GET /api/studies

Sessão válida e perfil ADMIN, GESTOR ou MEDICO obrigatórios. Outros perfis são
negados no backend. Não se introduz autorização por InstitutionName/unidade.
Cada parâmetro pode ocorrer uma única vez; desconhecidos são rejeitados.
Query string limitada a 4096 bytes.

| Parâmetro | Regra |
| --- | --- |
| `limit` | 1–50, padrão 25; UI fixa em 25 |
| `offset` | 0–10000, padrão 0 |
| `dateFrom`, `dateTo` | Datas reais AAAA-MM-DD; inclusivas; início ≤ fim; API admite limites abertos |
| `patientName`, `patientId`, `accessionNumber`, `studyDescription`, `institutionName` | Até 128 caracteres UTF-8; sem controles ou listas com barra invertida |
| `modality` | Um código DICOM CS de até 16 caracteres: letras maiúsculas, dígitos, espaços ou underscore; vazio omite filtro; sem curingas/listas |
| `sort` | `dateDesc` (padrão), `dateAsc`, `native` |

Todos os filtros são combinados por **AND**. Texto mantém o padrão recebido:
`*` e `?` têm semântica DICOM/Orthanc, sem adicionar curingas nem fazer busca OR
entre campos. `CaseSensitive:false` configura PN; não se promete normalização
de acentos nem equivalência entre espaços e separadores de nome DICOM.

Resposta mantém o contrato existente:

```ts
{
  items: Array<{
    orthancStudyId: string;
    studyInstanceUid: string;
    studyDate: string;
    studyTime: string;
    patientName: string;
    patientId: string;
    accessionNumber: string;
    studyDescription: string;
    institutionName: string;
    modalities: string[];
    seriesCount: number;
  }>;
  limit: number;
  offset: number;
  hasMore: boolean;
  nextOffset: number | null;
}
```

Tags ausentes permanecem vazias no DTO; arrays vazios são `[]`. Sem resposta bruta,
configuração, credencial ou IDs de séries. Erros sanitizados no envelope existente:
400 entrada inválida; 401 sessão ausente/expirada; 403 perfil negado;
422 `PACS_QUERY_UNSUPPORTED`; 503 configuração indisponível;
502 falha/resposta incompatível; 504 timeout. Um 401 do Orthanc vira 502, não uma
expiração da sessão do usuário.

## Ordenação global e capacidades

`POST /tools/find` é uma operação de consulta, com `Level:Study`, `Expand:true`,
`Since:offset`, `Limit:limit+1` e filtros explícitos. Não usa GET global de estudos,
consulta de todo o acervo ou ordenação local da página.

Para `dateDesc`/`dateAsc`, envia `OrderBy`:

1. StudyDate DESC/ASC;
2. StudyTime DESC/ASC;
3. StudyInstanceUID ASC para desempate entre estudos com mesmo horário.

A ordem global é executada pelo Orthanc **antes** da paginação. O cliente preserva
a ordem recebida. Tags de data/hora ausentes mantêm a semântica de ordenação do
índice Orthanc, sem preencher data clínica fictícia. UIDs válidos identificam os
estudos no Orthanc; não se infere ordem clínica a partir deles.

Antes de consulta ordenada ou com modalidade, um GET `/system` no mesmo transporte
e timeout verifica `Capabilities.HasExtendedFind`. Esse recurso existe a partir
do Orthanc 1.12.5 e depende do backend do índice (SQLite compatível ou plugin
PostgreSQL com ExtendedFind). Não basta apenas a versão do servidor.

Sem suporte, retorna 422 explícito: não ignora sort nem simula ordem global.
A UI oferece **Ordem nativa do PACS** (`sort=native`), uma alternativa explícita,
sem promessa cronológica. Sem modalidade, esse modo não exige `/system` e mantém
o caminho compatível anterior. A aplicação não altera nem atualiza o Orthanc.

## Modalidade e quantidade de chamadas

Filtro usa `Query.ModalitiesInStudy` no nível Study: estudos que contenham o código
nas séries. Não filtra a página em memória nem faz busca global de séries.
Exige ExtendedFind e versão numérica estável **≥ 1.12.6**, que corrigiu a combinação
de ModalitiesInStudy com paginação. Versões desconhecidas/pré-release não são
assumidas compatíveis para modalidade. Sem suporte, a consulta falha claramente;
não remove o filtro silenciosamente.

O filtro de modalidade continua disponível na API, mas foi retirado da UI por
preferência de apresentação. A coluna de modalidades continua visível.
Estudos com múltiplas modalidades continuam exibindo todas, sem duplicatas e em
ordem determinística; filtrar CT não oculta SR do mesmo estudo.

A hidratação existente foi preservada: GET `/studies/{id}/series?expand=true`
apenas para estudos da página com séries, validando relações e obtendo Modality.
A sentinela adicional não é hidratada. `RequestedTags:ModalitiesInStudy` foi
avaliado, mas não adotado para substituir a hidratação validada nesta entrega.

- Ordenação/modalidade: no máximo **2 + limit** chamadas (27 para 25, 52 para 50).
- Ordem nativa sem modalidade: no máximo **1 + limit** (26 ou 51).
- Apenas quatro consultas de séries simultâneas por requisição de Worklist.
- Sem cache de configuração/credencial/capacidades; `/system` é pequeno e consultado
  somente quando uma consulta de Worklist o exige, nunca por polling.
- Timeout configurado limita a operação inteira, incluindo capacidades e séries.
- Limite existente: 4 MiB por resposta de estudos/séries, 64 KiB de `/system`,
  até 2000 séries por estudo. Cancelamento interrompe requisições e workers.

Isso limita a concorrência HTTP, mas filtros amplos/computados e offsets altos
podem exigir trabalho significativo no índice Orthanc. Recomenda-se refinar o
período, sem prometer custo constante da busca.

Referências oficiais:
[Orthanc Book: find e OrderBy](https://orthanc.uclouvain.be/book/users/rest.html#finding-resources-within-orthanc),
[release 1.12.6: correção de paginação com modalidade](https://orthanc.uclouvain.be/hg/orthanc/file/Orthanc-1.12.6/NEWS).

## Busca, períodos e apresentação

Busca principal tem seletor Nome/ID/Accession/Descrição, sem adivinhar o tipo.
Cada campo tem seu próprio filtro. Trocar o seletor mostra o valor daquele campo,
sem reinterpretar o anterior nem emitir consulta se os filtros não mudaram.
Filtros de outros campos continuam ativos em AND; um aviso aparece quando estão
ocultos, e o painel avançado permite revisar todos, incluindo Instituição.

Período inicial: últimos sete dias (hoje e seis anteriores). Atalhos Hoje, Ontem,
Últimos 7/30 dias e Personalizado. Datas dos atalhos são resolvidas no calendário
local ao selecionar e mantidas como datas explícitas durante a navegação; Atualizar
mantém esse intervalo. Para avançar o intervalo após a virada do dia, selecione o
atalho novamente. A UI exige ambos os limites e valida antes de consultar.

StudyDate é validada por calendário e formatada DD/MM/AAAA sem Date/UTC.
StudyTime aceita HH, HHMM e HHMMSS com fração; HH mostra `HHh` sem inventar minutos,
os demais mostram HH:MM. Ausência/valor inválido mostra `—`; nenhuma conversão de
fuso. Descrição, Instituição, Accession e ID ausentes mostram `—`, sem fallback
clínico ou substituição por descrição de série. Modalidades exibidas como CT / SR.

Lista desktop-first mantém direção visual existente, cabeçalho claro e colunas
Data/Hora, Paciente, ID, Modalidade, Descrição, Accession, Instituição e Séries.
Linha abre com clique, Enter ou Espaço, e só trata teclado originado nela.
Não há controles internos da linha nem dashboard adicionado.

## Paginação, atualização, concorrência e retorno

A UI usa 25 estudos por página, sem seletor de tamanho. Mantém Anterior/Próxima,
página atual e fim dos resultados. O título Data / Hora é um botão acessível por
clique e teclado, com seta indicando a ordem; alterna mais recentes/mais antigos
e retorna à primeira página. Outros títulos não simulam ordenação local.
Os controles avulsos de Ordenação, Modalidade e Por página foram removidos.
Ordem nativa é oferecida apenas no erro de capacidade incompatível.
`hasMore` usa uma sentinela, sem contagem total inventada. Offset máximo pode ter
hasMore=true e nextOffset=null; a UI pede filtros mais restritos.
Paginação por offset **não é snapshot**: ingestão/remoção entre páginas pode mover
registros. Ordenação é global, mas não elimina essa limitação.

Alterar filtro/ordem/tamanho retorna à primeira página. Atualizar mantém critérios
e tamanho, retorna à primeira página e conserva linhas visíveis desabilitadas
até concluir. Sem polling/WebSocket. Debounce 350 ms também evita consultas por
cada tecla. Estados: loading, refreshing, vazio, erro/retry e sessão expirada.

Cada effect tem AbortController e cancela timer/requisição ao substituir consulta
ou desmontar. Resposta/erro só modifica estado se o sinal ainda estiver ativo.
Resultados antigos não sobrescrevem novos; não há updates após unmount.

Filtros, datas, seletor, ordenação, tamanho, offset e expansão do painel vivem no
estado de App, **apenas em memória**. Voltar pelo Viewer ou histórico restaura
esse estado e refaz uma única consulta; resultados clínicos não são mantidos como
cache paralelo. Logout/expiração limpam o estado. Recarregar a página perde filtros.
Não usa localStorage, sessionStorage, history.state nem parâmetros de navegação
para guardar filtros. A rota do Viewer contém somente o ID Orthanc.

## Privacidade

GET existente foi preservado: filtros estão na query string da **requisição fetch**,
não na URL da página/histórico de navegação. Isso ainda pode expô-los em DevTools,
telemetria de rede ou logs de proxy. Não há promessa de confidencialidade da URL
HTTP. O logger Go existente registra somente rota fixa sem argumentos/corpo;
proxies externos devem omitir query strings dessa rota antes da produção.
Nenhuma infraestrutura foi alterada nesta entrega.

`Cache-Control:no-store`, sessão e política CSRF de leitura são mantidos.
Não registra paciente, identificadores, termos de busca, resposta Orthanc,
credenciais ou corpo clínico em logs/métricas/auditoria. Não se cria evento com
lista de pacientes nem amplia a política de auditoria.

## Testes

- Go: filtros, datas, limites, parâmetros desconhecidos/repetidos, modalidade,
  OrderBy global e preservação de ordem, checagem de capacidade/versão, tags
  ausentes, modalidades múltiplas, relações, timeout, erro, cancelamento,
  autorização, logs sem dados clínicos, concorrência máxima quatro e sanitização.
- Node: cliente HTTP, períodos/calendário, consulta AND, validação, StudyDate,
  StudyTime parcial/inválida e modalidades genéricas/deduplicadas.
- Chrome (`tests/worklist-browser.mjs`): tela inicial, busca/seletor, avançados,
  todos os períodos, intervalo inválido sem consulta, remoção dos controles avulsos,
  ordenação no cabeçalho, anterior/próxima com 25 registros, Atualizar/refreshing, vazio/erro/retry,
  tags ausentes, abertura por teclado, retorno e preservação, debounce, resposta
  obsoleta, abort/unmount e expiração. Servidor e perfil temporários sintéticos.
- Viewer: smoke V2/V3/V4 com DICOM sintético e cenário separado MONOCHROME1 para
  Invert OFF inicial, troca de série, auto-layout, outro viewport e Reset.

Comandos de validação:

```sh
cd backend
gofmt -l $(rg --files -g '*.go')
go vet ./...
go test ./...
go build ./...
go test -race ./internal/studies ./internal/orthanc ./internal/httpapi
cd ../frontend
npm run typecheck
node --test tests/*.test.mjs
npm run build
node tests/worklist-browser.mjs
node tests/viewer-smoke.mjs
```

Resultado desta entrega: gofmt sem diferenças; go vet, go test, go build e race
nos três pacotes aprovados. Typecheck/build frontend aprovados, 23 testes Node
aprovados, Chrome Worklist V2 e regressão Viewer V2/V3/V4 + Invert OFF aprovados.

Sem deploy, conexão Orthanc real, nova dependência ou npm audit fix.
Warnings existentes de chunks grandes e módulos Node externalizados em codecs
permanecem. Vulnerabilidades npm não foram reavaliadas/corrigidas nesta entrega.
