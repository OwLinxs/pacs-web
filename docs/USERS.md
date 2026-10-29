# Administração V1 — Etapa 2: Usuários

Este documento registra a entrega original de Usuários e, mais abaixo, a etapa
posterior de pré-visualização da importação. Para estado operacional atual e
hardening posterior, prevalece [PROJECT_CONTEXT.md](PROJECT_CONTEXT.md).

Implementação incremental sobre autenticação, CSRF, Argon2id e auditoria existentes.
Unidades (Etapa 1) foi implantada e validada pelo responsável no ambiente real.
Esta entrega não faz deploy, não aplica migrations em banco existente e não
acessa Orthanc. Worklist V2 e Viewer V4 permanecem inalterados; App apenas impede
montá-los enquanto há troca obrigatória e o shell apresenta os múltiplos vínculos.

## Modelo e migration 0003

`0003_users_management.sql` preserva 0001/0002 e seus checksums. Em transação:

- bloqueia escritas em users durante a alteração de schema;
- verifica usernames legados incompatíveis e falha com mensagem fixa, sem expor
  valores ou renomeá-los automaticamente;
- troca a constraint para `^[a-z0-9][a-z0-9_-]{2,63}$`: 3–64 caracteres,
  primeiro alfanumérico, demais letras minúsculas/números/underscore/hífen;
- acrescenta `must_change_password boolean NOT NULL DEFAULT false`, preservando
  o acesso dos usuários existentes, inclusive ADMIN;
- cria `user_units(user_id, unit_id)` com PK composta, FK users ON DELETE CASCADE,
  FK units ON DELETE RESTRICT e índice pelo unit_id;
- copia todo vínculo legado não-ADMIN com unit_id não nulo, inclusive unidade
  inativa; não modifica senha, validade, usuário ou unidade existente.

`users.unit_id` e sua FK permanecem como legado, sem sincronização automática de
novos vínculos. `user_units` é a fonte da nova administração e do array `units`
na sessão. O campo singular `unit` continua no DTO de sessão por compatibilidade,
mas não é usado pelo novo shell nem para autorização. ADMIN não requer unidade.
Não existe exclusão física pela API.

Novos MEDICO **e GESTOR** exigem 1–100 unidades distintas e ativas. Uma unidade
inativa já vinculada pode ser mantida na edição; novas associações a inativas
são recusadas. Edição exige ao menos um vínculo; permite preservar um conjunto
histórico inteiramente inativo. Desativar unidade nunca remove vínculos nem
bloqueia acesso clínico por unidade. O store bloqueia as linhas de unidades
contra desativação concorrente durante a associação.

Na entrega original, o servidor executava migrations pendentes ao iniciar. **O
hardening posterior mudou o comportamento em produção:** `serve` verifica
versões/checksums; migrations usam o comando `migrate` e uma role separada.
Ver [PROJECT_CONTEXT.md](PROJECT_CONTEXT.md). Não editar SQL já aplicado.

## Autorização efetiva

| Operação | ADMIN | GESTOR | MEDICO |
| --- | --- | --- | --- |
| Listar usuários | Todos; ADMIN aparece protegido | Somente MEDICO | Não |
| Criar MEDICO | Sim | Sim | Não |
| Criar GESTOR | Sim | Não | Não |
| Editar/ativar/desativar/renovar/resetar MEDICO | Sim | Sim | Não |
| Editar/ativar/desativar/renovar/resetar GESTOR | Sim | Não | Não |
| Administrar ADMIN por estas rotas | Não | Não | Não |
| Trocar a própria senha autenticada | Sim | Sim | Sim |

GESTOR não é limitado às próprias unidades nesta versão. Não há criação de ADMIN
via API/UI nem mudança de papel/username. O bootstrap CLI seguro continua criando
o primeiro ADMIN. Os helpers genéricos antigos em `user/role.go` não autorizam
as novas operações: `useradmin.CanManage` aplica a matriz mais restrita desta tela.

A API verifica sessão/perfil; o serviço carrega o papel real do alvo antes de
operar, e o store verifica novamente ator e alvo na transação. JSON com role,
active no cadastro, mustChangePassword ou data calculada pelo cliente é rejeitado.
ADMIN existente não pode ser rebaixado/desativado/resetado por estas rotas.

## API

Toda escrita exige sessão válida, Origin/Referer e CSRF existentes. Respostas
mantêm no-store. UUIDs de rota são canônicos; corpo limitado a 16 KiB; campos
repetidos/desconhecidos, null, tipos errados e JSON extra são rejeitados.

| Endpoint | Corpo / resultado |
| --- | --- |
| `GET /api/admin/users` | Página de usuários autorizados |
| `POST /api/admin/users/medicos` | name, username, email opcional, unitIds, validity, initialPassword; papel fixo MEDICO; 201 |
| `POST /api/admin/users/gestores` | Mesmo contrato; papel fixo GESTOR; 201 |
| `PATCH /api/admin/users/{id}` | Formulário de perfil: name e unitIds obrigatórios, email opcional (omitido/vazio remove); 200 |
| `POST /api/admin/users/{id}/active` | active boolean; 200 |
| `POST /api/admin/users/{id}/renew` | validity; 200 |
| `POST /api/admin/users/{id}/reset-password` | initialPassword; 200 |
| `POST /api/auth/change-password` | currentPassword e newPassword; 204 e limpeza do cookie |

Não existe DELETE nem endpoint genérico para criar papel arbitrário.

Listagem: search (substring literal de nome **ou** username, até 120 caracteres),
role, unitId, status (`active`, `inactive`, `expired`), limit 1–100 (default 25),
offset 0–10000. Campos combinados por AND, ordem global por nome/ID no PostgreSQL.
Query limitada a 1024 bytes, sem parâmetros repetidos ou desconhecidos. GESTOR
não pode pedir filtro ADMIN/GESTOR. Paginação busca limit+1 como sentinela;
responde items, limit, offset, hasMore, nextOffset, sem count global.

DTO de usuário: id, name, username, email, role, units[{id,name,active}], active,
status, accessValidUntil (data ou null), lastLoginAt (timestamp ou null),
mustChangePassword. Não há hash, senha, token ou configuração Orthanc.
Status: inativo prevalece; senão expirado quando vencido; senão ativo.

Erros: 400 validação/senha/unidades, 401 sessão inválida, 403 autorização ou troca
obrigatória, 404 alvo ausente, 409 username duplicado, 503 indisponibilidade.
Erros SQL não são retornados nem registrados pelos novos handlers/store.
Timeout: cinco segundos para lista, dez para escritas incluindo hashing/SQL.

## Validade e renovação

Opções da API: `1m`, `3m`, `6m`, `1y`, `unlimited`. UI não recebe data manual.
O backend usa o calendário **America/Sao_Paulo**, com tzdata embutida no binário.
`access_valid_until` continua DATE e representa o último dia permitido inclusive.
A expiração ocorre na virada para o dia seguinte nesse fuso, no login e em cada
resolução de sessão; não depende do fuso do processo/container/navegador.

Cadastro conta a partir de hoje. Renovação conta a partir do vencimento quando
esse dia ainda não passou; se vencido ou sem prazo anterior, parte de hoje.
Soma meses de calendário, limitando o dia ao último dia do mês de destino:
31 de janeiro + 1 mês termina em 28/29 de fevereiro; 29 de fevereiro + 1 ano
termina em 28 de fevereiro. Renovações repetidas partem da data já limitada.
`unlimited` grava NULL. Renovar não ativa conta inativa; reativar não renova conta
expirada, não troca senha e não recupera sessões revogadas.

## Senhas, primeiro acesso e sessões

Senha inicial é informada pelo operador e processada pelo Hasher Argon2id
existente (política mínima de 12 caracteres). Nunca volta na resposta. Novo
MEDICO/GESTOR sempre tem must_change_password=true. Não há envio por e-mail.

Login com senha inicial cria sessão restrita. O middleware bloqueia APIs normais
com `403 PASSWORD_CHANGE_REQUIRED`; permite apenas GET auth/me, POST
change-password e POST logout. Health/estáticos continuam públicos. O frontend
mostra uma tela específica antes de montar Worklist/Viewer/administração.

Troca exige senha atual e nova diferente, com política existente. O store
verifica hash esperado, conta ativa/válida e sessão ainda viva sob bloqueio;
atualiza o hash, limpa flag e revoga **todas** as sessões na mesma transação.
Cookie de sessão é limpo, CSRF renovado; o usuário precisa entrar novamente.
Não se mantém o identificador antigo nem se cria sessão automaticamente.

Reset administrativo grava outro hash, marca flag e revoga todas as sessões
atomicamente. Desativação também revoga sessões na mesma transação. As mutações
verificam novamente o ator para rejeitar reset/desativação concorrente do operador.

Login usa `CreateVerified`: bloqueia o usuário e confirma que a senha verificada
continua atual antes de inserir sessão, serializando com reset/desativação.
Rehash usa compare-and-swap, evitando sobrescrever um reset concorrente. Requisições
que já concluíram autorização antes de uma revogação não são canceladas globalmente;
requisições subsequentes são rejeitadas pela resolução existente.

## Auditoria

Eventos: USER_CREATED, USER_UPDATED, USER_ACTIVATED, USER_DEACTIVATED,
USER_ACCESS_RENEWED, USER_PASSWORD_RESET, USER_PASSWORD_CHANGED, USER_UNITS_CHANGED.
Usam audit.RecordTx na mesma transação, ator/username administrativo, origem e Detail fixo com
`target_user_id`. Timestamp vem do banco. Nenhuma senha/hash/e-mail/nome do alvo,
lista de vínculos ou dado clínico é registrada. Nome/e-mail/vínculos/ativo sem
mudança não geram evento correspondente; renovar/resetar são ações explícitas.

Atualização Auditoria V1: alterações administrativas, troca de senha, revogação
de sessões e eventos correspondentes são atômicos. Falha de auditoria obrigatória
faz rollback. Login/logout continuam com política separada. A política anterior
best-effort administrativa foi substituída; detalhes em [AUDIT.md](AUDIT.md).

## Frontend e privacidade

Tela Usuários real substitui mock: tabela clínica, busca nome/username, filtros,
paginação de 25, criação de médico/gestor, edição, renovação e reset; confirmação
para desativar/resetar. ADMIN é visível sem ações. GESTOR vê somente médicos e
não recebe controle Novo gestor; MEDICO não recebe navegação administrativa.
Unidades carregam páginas de 50, com ação para mais opções; picker mostra vínculos
inativos existentes do alvo. O filtro visual lista unidades ativas carregadas;
a API também aceita ID de unidade inativa para consulta histórica.

Dados de formulário/filtros ficam somente em estado React. Senhas são campos
password, limpas ao fechar/salvar/errar; sem armazenamento persistente ou e-mail.
Busca tem debounce/cancelamento e proteção contra resposta obsoleta; gravações
bloqueiam submissão duplicada e são canceladas no unmount. Falha de rede é
sanitizada; 401 segue a tela de expiração. Cancelar fetch não garante rollback de
uma operação já concluída pelo servidor; reconsultar em caso de resultado incerto.
Query de busca administrativa pode aparecer em DevTools/proxies como já ocorre
com o GET existente; o logger Go não registra query strings/corpos.

## Testes e resultados desta entrega

Fixtures sintéticas, sem PACS real ou credenciais reais:

- Go: matriz ADMIN/GESTOR/MEDICO, IDOR/JSON manipulado, CSRF, nomes de usuário,
  múltiplos vínculos/duplicação, Argon2id, calendário/fim de mês/ano, fuso,
  gate obrigatório, troca/reset/revogação, auditoria sem senha e erros sanitizados.
- Teste PostgreSQL descartável preparado: migration com legado incompatível,
  rollback, preservação de vínculos/senha/validade/ADMIN, constraints, criação
  concorrente, queries de filtros, unidade inativa, reset/desativação/troca
  transacionais e proteção contra login/rehash com hash obsoleto.
- Node: contratos, papéis fixos, sessão/CSRF, cancelamento, erros e campos imutáveis.
- Chrome: cadastro dos dois papéis, edição, múltiplas unidades, renovação, reset,
  desativação/reativação/confirmação, busca/filtro, retry, perfis, sessão expirada,
  troca obrigatória antes de abrir Viewer e retorno ao login.

Resultados: gofmt sem diferenças; go vet/test/build aprovados; race em auth,
user, useradmin, httpapi e units aprovado. Typecheck/build frontend e **28 testes
Node aprovados**. Chrome Usuários, Unidades, Worklist e smoke Viewer V2/V3/V4,
incluindo Invert OFF, aprovados. Cleanup do novo teste Chrome usa tentativas
limitadas para remover perfil temporário após término dos processos.

**Validação SQL concluída:** a primeira execução retornou SKIP por ausência do
servidor local. Depois, um bottle PostgreSQL 18.6 foi baixado e extraído somente
em diretório temporário (sem instalar/iniciar serviço Homebrew). Os testes de
Unidades e Usuários passaram em clusters próprios descartáveis, somente socket
Unix, incluindo migration 0003 e transações. Nenhum DATABASE_URL/.env ou banco
existente foi usado. O teste continua retornando SKIP explícito em máquinas sem
postgres/initdb no PATH; não confundir esse SKIP com validação de persistência.

Comandos: ver [PROJECT_CONTEXT.md](PROJECT_CONTEXT.md). Novos browser tests:
`node tests/users-browser.mjs`; teste SQL: `go test -run TestPostgresMigrationAndUsers -v ./internal/useradmin`.
Warnings preexistentes de chunks grandes e módulos Node externalizados no Vite
permanecem. Nenhuma dependência foi alterada; sem npm audit fix; vulnerabilidades
npm não foram reavaliadas nesta entrega.

## Importar médicos — etapa 1: leitura e pré-visualização `.xlsx`

**Estado:** implementada e testada localmente; sem deploy nem validação com planilha
operacional. Esta seção descreve uma entrega posterior à Administração V1 acima.
Os resultados de 28 testes Node e a afirmação de que nenhuma dependência mudou
referem-se à entrega original de gerenciamento de usuários, não a esta etapa.

Em Administração → Usuários, o botão **Importar médicos** aparece para ADMIN e
GESTOR, que já podem administrar MEDICO. MEDICO não recebe a navegação
administrativa. A funcionalidade é inteiramente de pré-visualização: não há
endpoint novo, POST de importação, criação de usuário, username, senha, vínculo,
evento de auditoria ou gravação no PostgreSQL. A planilha não é enviada ao
backend ou a serviço externo. As únicas requisições da tela continuam sendo as
consultas administrativas existentes, incluindo `GET /api/units` para obter
unidades ativas. Esta etapa não acrescenta uma nova regra de autorização no
backend porque não acrescenta operação de servidor; uma futura importação que
grave usuários precisará de autorização e validação próprias no backend.

### Fluxo e regras implementadas

1. O operador abre a janela, escolhe uma unidade ativa já cadastrada e seleciona
   um `.xlsx`. A unidade **não é criada automaticamente**; se UPA 24h existir e
   estiver ativa, pode ser escolhida. A lista usa a paginação já disponível,
   com **Carregar mais unidades**. É possível ler a planilha sem selecionar
   unidade; nesse caso, a prévia mostra `—` até uma unidade ser escolhida.
2. O navegador verifica extensão `.xlsx`, tamanho não vazio até 2 MiB e a
   estrutura básica do diretório ZIP. O pré-check rejeita ZIP criptografado,
   métodos de compressão não suportados, ZIP64, mais de 256 entradas e soma dos
   tamanhos descomprimidos **declarados** acima de 16 MiB. Depois o leitor
   `read-excel-file` processa o arquivo localmente. Arquivo inválido/corrompido
   mostra mensagem controlada, sem conteúdo da planilha em logs.
3. É utilizada a **primeira aba** que contenha cabeçalho `Nome` nas primeiras
   20 linhas; comparação do cabeçalho ignora espaços e caixa. A coluna `QTD`
   não fornece nome nem quantidade para a prévia. Linhas inteiramente vazias
   são omitidas. Uma linha com `QTD` preenchido e `Nome` ausente é inválida,
   não vazia.
4. Nomes textuais passam por normalização Unicode NFC, espaços consecutivos
   viram um espaço e espaços nas extremidades são removidos. Nome ausente,
   não textual, menor que 2 ou maior que 120 caracteres, ou com caracteres de
   controle/formatação, é inválido. Duplicidade é detectada **somente dentro
   da planilha**, após normalização e comparação sem diferenciar caixa em
   `pt-BR`: a primeira ocorrência é válida para a prévia e as seguintes são
   marcadas duplicadas. Nenhuma consulta ao banco verifica nomes já cadastrados.
5. A tabela apresenta linha de origem, nome, perfil fixo **Médico**, unidade
   escolhida e situação. Os contadores mostram válidos, duplicados e inválidos.
   Há limite de 5.000 linhas de dados na aba selecionada; a tabela exibe as
   primeiras 200 linhas não vazias, enquanto os contadores abrangem todas.
   “Válido para prévia” **não significa apto a cadastrar usuário**: faltam
   username, validade, senha e validações de backend da etapa futura.

A prévia e a unidade selecionada ficam apenas no estado React da janela; ao
fechar, são descartadas. Leituras antigas não atualizam uma janela desmontada.
Não se usa localStorage, sessionStorage, IndexedDB nem URLs com nomes. A
interface renderiza o conteúdo como texto React; não executa fórmulas da
planilha. Não carregar planilhas reais em testes, screenshots ou documentação.

Código: `frontend/src/screens/UsuariosScreen.tsx` abre a janela e reutiliza a
lista de unidades; `frontend/src/users/ImportarMedicosDialog.tsx` contém a UI e
o estado local; `readXlsxPreview.ts` confere arquivo e carrega o leitor sob
demanda; `importPreview.ts` valida o ZIP e classifica as linhas.

### Dependências, testes e validação

Esta etapa adicionou `read-excel-file` **9.3.12** fixo. O override `fflate`
passou de 0.7.5 para **0.8.3** para satisfazer o leitor e a árvore Cornerstone;
nenhum pacote Cornerstone foi atualizado. O bundle de leitura é carregado apenas
quando uma planilha é aberta. Não foi executado `npm audit fix`.

Resultados obtidos na implementação local (diretório `frontend/`):

| Comando | Resultado |
| --- | --- |
| `npm run typecheck` | aprovado |
| `node --test tests/users-import.test.mjs` | 4 testes aprovados: coluna/limpeza/vazios, duplicidade/inválidos, abas/cabeçalho e limites/ZIP inválido |
| `node --test tests/*.test.mjs` | 37 testes aprovados |
| `npm run build` | aprovado; avisos já conhecidos de módulos Node externalizados pelos codecs Cornerstone e chunks grandes |
| `node tests/users-browser.mjs` | aprovado em Chrome com `.xlsx` sintético: prévia, contadores, unidade, arquivos inválidos, ADMIN/GESTOR e nenhuma escrita na API, além da regressão de Usuários |
| `node tests/viewer-smoke.mjs` | aprovado: regressão do Viewer, incluindo mouse, exportação, layouts e Invert OFF após a mudança de `fflate` |

Validação manual **pendente**: em ambiente controlado, entrar como ADMIN e
GESTOR; abrir Usuários → Importar médicos; escolher uma unidade ativa existente
(por exemplo UPA 24h, se cadastrada); usar planilha **inteiramente fictícia**
com colunas `Nome` e `QTD`, espaços, duplicata e linha vazia; conferir prévia
e contadores; testar arquivo inválido; fechar e atualizar Usuários para confirmar
que nenhuma conta foi criada. Confirmar que MEDICO não vê Administração. Não
usar nomes reais em registros, capturas ou relatos de teste.

Limites deliberados: não há conciliação com usuários existentes, seleção de
múltiplas unidades por linha, cadastro em lote, geração de credenciais,
persistência da prévia ou histórico de importação. A proteção ZIP usa tamanhos
**declarados** no arquivo; não é uma garantia absoluta contra arquivo malicioso.
Qualquer etapa posterior que escreva no banco exige desenho próprio de
validação, autorização, auditoria e tratamento seguro de concorrência.

## Importar médicos — etapa 2: lista só com nomes e criação das contas

**Estado:** implementada e testada localmente em 29/10/2026; sem deploy nem validação
com planilha operacional. Substitui o caráter “somente pré-visualização” da etapa 1.

- **Planilha sem cabeçalho:** se nenhuma aba tiver `Nome` nas 20 primeiras linhas,
  usa a primeira aba cujas células preenchidas ocupem uma única coluna; cada célula é
  um nome (linha 1 inclusive). Com cabeçalho `Nome`, o comportamento da etapa 1 vale.
- **Username sugerido e editável:** primeiro + último nome, sem acentos, minúsculo,
  separados por `_`, ignorando partículas (da, de, do, das, dos, e…); colisão dentro
  da lista recebe sufixo numérico (`joao_souza2`). O operador pode editar cada um e
  desmarcar linhas. Conflito com usuário já existente é informado na linha (409) e
  pode ser corrigido e reenviado; não há pré-consulta, porque GESTOR não vê ADMIN/GESTOR.
- **Senha inicial única do lote**, digitada e confirmada pelo operador (mínimo 12),
  com `must_change_password=true`. Decisão do responsável: mais simples de distribuir;
  risco aceito de quem conhecer senha e username entrar antes do médico. Campos de senha
  são limpos após cada execução.
- **Validade única** do lote (1/3/6 meses, 1 ano, sem expiração) e uma unidade ativa.
- Até 200 linhas preenchidas por planilha; acima disso a criação é bloqueada.
- **Sem endpoint novo:** cada conta usa `POST /api/admin/users/medicos`, uma chamada
  por linha, em sequência. Reaproveita validação, Argon2id, autorização ADMIN/GESTOR,
  CSRF e auditoria atômica (`USER_CREATED` + `USER_UNITS_CHANGED`). Não é tudo-ou-nada:
  409/400 marcam a linha e seguem; 401 encerra pela sessão expirada; demais falhas
  interrompem e marcam as restantes como “Não processado”.
- A planilha continua lida apenas no navegador; somente nome e username das linhas
  selecionadas são enviados. Não há verificação de nome já cadastrado (apenas username).

Testes: `tests/users-import.test.mjs` (6 casos, incluindo sem cabeçalho e sugestão de
username) e `tests/users-browser.mjs` (lista só com nomes, confirmação de senha, conflito
409, correção e reenvio, corpo das requisições).

## Fora de escopo

Escopo por unidades do GESTOR, criação de ADMIN via interface, mudança de roles,
remoção física, recuperação por e-mail, MFA, LDAP/AD/Keycloak, permissões por estudo,
PHI em logs, escrita Orthanc/DICOM, mudanças de infraestrutura e recursos adicionais
de Worklist/Viewer. O Invert continua OFF por padrão. Nenhum deploy foi executado.
