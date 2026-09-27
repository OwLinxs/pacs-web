# Teste READ-ONLY de conexão Orthanc

Este documento descreve o botão administrativo. A Worklist usa o mesmo
transporte seguro e está documentada separadamente em [WORKLIST.md](WORKLIST.md).

O botão existente em Administração → Configurações → PACS / Orthanc chama
`POST /api/admin/settings/orthanc/test`, sem parâmetros nem corpo. O endpoint
exige sessão autenticada ADMIN e passa pela proteção CSRF existente.
MEDICO e GESTOR recebem 403; sem sessão válida, 401 após a validação CSRF.
Uma requisição POST sem CSRF válido recebe 403 mesmo sem sessão.

O backend lê configuração, revisão e credencial da mesma linha de `app_settings`,
decifra a credencial com o mecanismo AES-GCM existente e chama
`internal/orthanc.Client.TestSystem(context.Context, Config)`.
Não há polling, conexão na inicialização, conexão ao salvar ou URL fornecida
pela requisição de teste.

## Operação e resultado

A única operação externa deste teste é `GET <baseURL>/system`, preservando um eventual
prefixo de reverse proxy. Usuário e credencial usam HTTP Basic quando
configurados; ambos vazios omitem Authorization. Não há acesso a DICOM,
pacientes, estudos, DICOMweb ou ao banco do Orthanc.

Uma resposta 200 deve conter JSON com Version e DicomAet não vazios, DicomPort
e HttpPort válidos. A identificação é estrutural: outro serviço pode imitar
esse JSON. Referência: [REST API oficial do Orthanc](https://orthanc.uclouvain.be/api/index.html).
O corpo tem limite de 64 KiB e os cabeçalhos, 16 KiB. O conteúdo não é devolvido
ao navegador nem registrado em logs ou auditoria.

Um teste concluído responde HTTP 200 com `success`, `status`, `code`, `message`
e `checkedAt`, inclusive quando a conectividade falha. Erros de pré-condição,
autorização ou persistência usam o envelope de erro existente e HTTP 4xx/5xx.
Timeout, DNS, conexão recusada, certificado TLS inválido, 401, 403, outros
status HTTP, redirect e resposta inválida são classificados localmente.
Somente categorias e mensagens fixas sanitizadas chegam à API, log e auditoria.
O evento é `ORTHANC_CONNECTION_TESTED` e inclui o autor e a origem existentes.

Status e data UTC ficam no JSONB atual, sem migration. Uma alteração de
configuração durante o teste impede gravar seu resultado sobre a nova
configuração. Salvar novamente limpa a verificação anterior. A indicação
"Conectado" representa o último teste explícito, não monitoramento contínuo.
A tela exige salvar alterações pendentes antes de testar, mostra carregamento
e apresenta o resultado sem redesenhar o formulário.

## Timeout e TLS

O timeout salvo (1–120 segundos) limita a operação HTTP completa, incluindo DNS,
conexão, TLS e leitura do corpo, junto ao cancelamento do contexto da requisição.
Somente o endpoint de teste ajusta seu prazo de escrita para acomodar esse
intervalo e a persistência do resultado; os demais endpoints não mudam.

Cada teste usa seu próprio `http.Client` e `http.Transport`. Para HTTPS,
`verifyTls=true` valida cadeia e hostname; `false` desativa explicitamente essas
verificações somente neste cliente. TLS mínimo 1.2. Nenhum estado TLS global
é alterado. HTTP continua permitido pela configuração existente; nesse caso,
Basic Auth não oferece sigilo de transporte.

## Proteção SSRF e limites operacionais

- Destino exclusivamente da configuração salva pelo ADMIN; sem proxy genérico.
- Apenas HTTP/HTTPS; sem userinfo, query, fragmento, porta inválida, zone IPv6
  ou caminhos ambíguos/escapados. Prefixos simples, como `/pacs`, são permitidos.
- Não segue nenhum redirect e não usa HTTP_PROXY/HTTPS_PROXY do ambiente.
- Resolve DNS uma vez, valida todos os IPs retornados e conecta diretamente
  ao IP validado, preservando hostname para Host e TLS. Respostas DNS mistas
  com um IP proibido também são rejeitadas.
- Permite redes privadas RFC1918 e IPv6 ULA. Bloqueia loopback, link-local,
  multicast, endereços não especificados e faixas especiais definidas no cliente,
  incluindo destinos conhecidos de metadados e mecanismos de tradução IPv6.

Essa política não é uma allowlist de servidores: um ADMIN ainda pode configurar
serviços em redes privadas ou públicas permitidas. É uma fronteira de confiança
administrativa e não elimina todo SSRF em serviços internos. O endereço do
Orthanc deve ser um endereço interno roteável acessível pelo backend; localhost,
loopback e link-local não são aceitos. Proxies HTTP de ambiente e redes especiais
bloqueadas exigiriam uma decisão explícita futura, não uma exceção automática.

## Validação sem ambiente real

Os testes de `internal/orthanc` usam `httptest.Server` e `httptest` TLS, com
resolução/discagem injetadas internamente apenas nos testes para direcionar o
hostname fictício aos listeners locais. O cliente de produção não possui essa
exceção. Cobrem sucesso, 401/403/500, timeout, JSON inválido, formato inesperado,
redirects, credencial fictícia e ausência de credencial; também TLS, cancelamento,
destinos bloqueados, DNS e limites da resposta.

Os testes HTTP da aplicação cobrem sessão, perfis, CSRF, sanitização, auditoria,
status/data, configuração ausente e alteração concorrente. Usam stores em
memória; não validam as queries contra um PostgreSQL real. O frontend possui
typecheck e build, sem suíte automatizada de interação visual.

Nenhuma dependência adicional ou migration foi criada. Nenhum teste desta
entrega deve utilizar Orthanc real, credenciais reais ou dados de pacientes.
