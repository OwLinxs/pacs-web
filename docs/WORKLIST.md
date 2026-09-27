# Worklist de estudos — primeira versão READ-ONLY

## Arquitetura

`ExamesScreen → GET /api/studies → httpapi → internal/orthanc → Orthanc`.
`internal/studies` define o DTO e as regras de consulta. `internal/settings`
fornece a configuração salva e a credencial decifrada pelo AES-GCM existente.
Nenhuma migration ou dependência foi adicionada. Não há cópia dos estudos no
PostgreSQL da aplicação, que permanece separado do banco do Orthanc.

O cliente Orthanc é compartilhado na composição do servidor. Cada operação usa
seu próprio transporte seguro e contexto. O código de transporte foi extraído
do teste `/system` para reutilização; não há método público de proxy genérico.

## Contrato GET /api/studies

Requer sessão válida e perfil ADMIN, GESTOR ou MEDICO, verificados pelo backend.
A lista explícita preserva o acesso à tela Exames dos três perfis existentes;
outros perfis são negados. Não há restrição por unidade nesta entrega:
`InstitutionName` não é mapeado para `units` e não é uma regra de autorização.

Parâmetros opcionais (cada um pode aparecer uma única vez):

| Parâmetro | Regra |
| --- | --- |
| `limit` | Inteiro 1–50; padrão 25 |
| `offset` | Inteiro 0–10000; padrão 0 |
| `dateFrom`, `dateTo` | Datas reais AAAA-MM-DD; limites inclusivos de StudyDate; podem ser abertos |
| `patientName` | Filtro de PatientName |
| `patientId` | Filtro de PatientID |
| `accessionNumber` | Filtro de AccessionNumber |
| `studyDescription` | Filtro de StudyDescription |
| `institutionName` | Filtro de InstitutionName, sem inferir unidade solicitante |

Filtros de texto aceitam até 128 caracteres UTF-8, sem controles ou listas
separadas por barra invertida. Valores vazios são omitidos. Todos os filtros
informados são combinados por AND no Orthanc. O backend preserva o padrão
informado, inclusive `*` e `?`; não adiciona curingas, não implementa OR entre
campos e não faz busca local por substring. `CaseSensitive:false` configura a
busca PN; não se promete normalização de acentos nem igualdade entre espaços e
separadores de nomes DICOM. As demais regras de comparação são as do Orthanc.

Resposta 200:

```json
{
  "items": [{
    "orthancStudyId": "00000001-00000000-00000000-00000000-00000000",
    "studyInstanceUid": "2.25.123456789",
    "studyDate": "20260920",
    "studyTime": "101112",
    "patientName": "FICTICIO^UM",
    "patientId": "SYNTH-1",
    "accessionNumber": "ACC-SYNTH",
    "studyDescription": "Exame ficticio",
    "institutionName": "Instituicao Ficticia",
    "modalities": ["CT", "SR"],
    "seriesCount": 3
  }],
  "limit": 25,
  "offset": 0,
  "hasMore": false,
  "nextOffset": null
}
```

Todos os valores do exemplo são fictícios. Tags ausentes ou vazias produzem
strings vazias; modalidades desconhecidas produzem `[]`, nunca um valor
inventado. `items` vazio é `[]`. Datas/horas mantêm o formato DICOM original,
sem converter fuso horário. A resposta contém apenas o DTO, não tags brutas,
IDs de séries, configuração ou credenciais.

Erros seguem o envelope existente `{ "error": { "code": "...", "message": "..." } }`:
400 para consulta inválida, 401 para sessão ausente/expirada, 403 para perfil
não autorizado, 503 para configuração/serviço indisponível, 502 para falha ou
resposta incompatível do PACS, 504 para timeout. Um 401 do Orthanc vira 502 da
aplicação, evitando confundi-lo com expiração da sessão do usuário.

## Paginação e modalidades

O cliente envia `POST /tools/find` com `Level: Study`, `Expand: true`,
`Since: offset`, `Limit: limit + 1` e Query com os filtros permitidos. Esse POST
é exclusivamente uma consulta. O registro adicional identifica `hasMore` e
não é enriquecido nem entregue na página. Não se usa `GET /studies` global,
contagem global nem carregamento de todo o acervo.

As modalidades são lidas em `GET /studies/{id}/series?expand=true` apenas para
os estudos retornados que têm séries. O cliente confere os IDs de séries e
ParentStudy contra o resultado da busca. Extrai MainDicomTags.Modality, remove
duplicatas e ordena a lista. A quantidade de séries vem da lista Series do
estudo, não da quantidade de modalidades ou imagens. Modalidade ausente não
causa erro. Divergência de séries durante a consulta gera erro sanitizado para
repetir a busca; não se apresenta uma lista parcial como completa.

Há no máximo 1 + limit chamadas por página, com até quatro consultas de séries
simultâneas. Respostas têm limite de 4 MiB por chamada e até 2000 séries por
estudo. O timeout configurado limita a página inteira, incluindo todas as
chamadas, e não é reiniciado por série. Cancelar a requisição interrompe o fluxo.

Ao alcançar o offset máximo, `hasMore` pode ser true e `nextOffset` null:
o usuário deve refinar período/filtros. Não há total global ou promessa de ordem
cronológica. A ordem é a do Orthanc; não se ordena só a página em memória.
`OrderBy`/ExtendedFind não são exigidos nesta versão. Paginação por offset não
é um snapshot: ingestões ou remoções entre páginas podem deslocar resultados.
O desempenho de offsets altos e filtros amplos depende do índice/backend do
Orthanc; para acervos grandes, deve-se refinar o período.

Base técnica consultada: [API oficial do Orthanc](https://orthanc.uclouvain.be/api/index.html)
e [buscas REST no Orthanc Book](https://orthanc.uclouvain.be/book/users/rest.html#finding-resources-within-orthanc).
As decisões desta aplicação (limites, DTO e perfis) são locais. Os testes
simulam esses contratos; o comportamento de filtros, Since/Limit e desempenho
deve ser confirmado manualmente na versão/plugin do ambiente controlado.

## Segurança e privacidade

Reutiliza URL e credencial armazenadas, TLS configurável por ADMIN, validação
SSRF, bloqueio de redirects e ausência de proxies de ambiente. A credencial
não aparece no DTO, logs ou auditoria. Redes privadas continuam permitidas e
os bloqueios de destinos especiais permanecem os mesmos do teste de conexão.
Cada nova conexão valida os IPs resolvidos antes de discar; as conexões podem
ser reutilizadas dentro de uma página e são encerradas ao fim da operação.

Não são expostos caminhos REST arbitrários. IDs usados nos caminhos são
validados. Cabeçalhos upstream são limitados a 16 KiB. Erros de rede/JSON não
são registrados com conteúdo bruto. `Cache-Control: no-store` permanece ativo.
O GET não exige token CSRF por ser leitura, conforme middleware existente.

Não se cria evento de auditoria por paciente nem evento contendo a lista.
O requestLogger existente registra método, rota fixa, status, duração e origem,
sem query, corpo, PatientName, PatientID, UID ou outros campos clínicos.
O frontend mantém resultados/filtros apenas em memória e cancela consultas
substituídas. Como os filtros usam query string, a configuração dos access logs
de proxies externos deve omitir argumentos desta rota; esta entrega não altera
proxies, Orthanc ou infraestrutura de produção.

## Frontend

Preserva a estrutura visual da tela Exames, substituindo a lista mockada.
Mantém skeleton, vazio e erro; 401 abre a tela existente de sessão expirada.
Há navegação Anterior/Próxima e tentativa de recarga após falha. Filtros usam
debounce de 350 ms e resetam o offset. Consultas obsoletas são canceladas e não
podem sobrescrever resultados novos.

O período inicial é de sete dias conforme o calendário local do navegador.
O campo de pesquisa é explícito (nome, identificação, accession ou descrição);
Instituição usa o valor DICOM, sem lista fictícia de unidades. O filtro mockado
de modalidade foi retirado nesta versão; as modalidades reais são exibidas.
A coluna conta Séries, não imagens. As linhas agora abrem o Viewer mínimo
descrito em [VIEWER.md](VIEWER.md), usando somente o ID Orthanc na rota.
Os controles de estados mockados da Worklist também foram retirados do painel.

## Testes e limites desta entrega

`internal/orthanc/studies_test.go` usa httptest e dados fictícios: consulta
válida, DTO, paginação, tags ausentes, múltiplas modalidades, deduplicação,
relações de séries, vazio, falhas HTTP, timeout/cancelamento, JSON inválido,
limites, concorrência, redirects e SSRF. Os testes de `/system` continuam ativos.

`internal/httpapi/studies_test.go` cobre sessão ausente/revogada, três perfis
permitidos, perfil desconhecido negado, filtros inválidos, limites, consumo da
configuração segura, respostas sanitizadas, ausência de dados clínicos nos logs
e ausência de eventos de auditoria clínicos. Os stores são em memória.

`frontend/tests/studies-api.test.mjs` testa o cliente HTTP real com fetch
simulado: filtros/paginação, 401, erro sanitizado e cancelamento, sem novas
dependências. Não é um teste visual de navegador.

Comandos de validação:

```sh
cd backend
gofmt -w internal/orthanc internal/studies internal/httpapi cmd/server/serve.go
go vet ./...
go test ./...
go build ./...
cd ../frontend
npm run typecheck
node --test tests/studies-api.test.mjs
npm run build
```

Nenhum teste automatizado depende do PACS real, PostgreSQL real ou dados reais
de pacientes. O Viewer mínimo foi adicionado em entrega própria, descrita em
[VIEWER.md](VIEWER.md). Não há DICOMweb, upload, delete, modify, anonymize ou
outra operação de escrita no Orthanc.
