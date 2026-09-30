# Hardening pré-produção — estado local (sem deploy)

Este documento descreve a configuração exigida pelo código após o feature freeze. Não contém credenciais nem substitui uma janela de mudança aprovada. Nenhum comando abaixo foi executado contra produção.

## Build e PostgreSQL

`.dockerignore` exclui `.env`, `.env.*`, `**/.env` e `**/.env.*`. O contexto de build não deve receber arquivos reais de segredo. A imagem final contém somente o binário Go e `dist` do frontend; não há `COPY .` na etapa final.

`serve` em produção **não executa migrations**. Verifica todos os checksums de `schema_migrations` e recusa migrations pendentes/desconhecidas. Também recusa role runtime com superuser/CREATEDB/CREATEROLE/REPLICATION/BYPASSRLS ou CREATE no schema `public`. Desenvolvimento conserva auto-migration. `migrate` em produção exige `MIGRATION_DATABASE_URL` separado, fornecido somente ao processo pontual de migration. O Compose não injeta essa credencial no serviço `app`.

Antes de trocar o Compose em um ambiente real, provisionar uma role de login runtime distinta da role de migration e fazer grants de `CONNECT`, `USAGE` no schema, `SELECT/INSERT/UPDATE/DELETE` somente nas tabelas de aplicação (`units`, `users`, `sessions`, `audit_events`, `app_settings`, `user_units`) e `USAGE/SELECT` na sequência de auditoria. `schema_migrations` recebe **somente SELECT**. Não conceder DDL nem atribuir a role de migration à role runtime. Reaplicar grants de objetos necessários após migrations futuras. Testar em banco descartável e revisar o plano antes da janela de mudança. `APP_DATABASE_URL` aponta apenas para o banco da aplicação e precisa usar senha escapada para URL; `POSTGRES_PASSWORD` permanece exclusivo do container PostgreSQL/migration. Evitar `docker compose config` em logs, pois expande variáveis secretas.

## Proxy HTTPS, login e sessão

Produção exige `PUBLIC_ORIGIN=https://<domínio-publicado>` e `TRUSTED_PROXY_CIDRS` com o(s) CIDR(s) dos **peers reais** que alcançam o backend (não a rede inteira por conveniência). Determinar o peer efetivo em ambiente controlado. No Nginx Proxy Manager, preservar `Host`, sobrescrever `X-Forwarded-For` na fronteira confiável, publicar apenas HTTPS, redirecionar HTTP para HTTPS, aplicar HSTS só após validar domínio/TLS e não expor a porta 8080 fora do loopback nem 8042 do Orthanc. Não aceitar `X-Forwarded-For` vindo diretamente da Internet. O backend usa a cadeia da direita para a esquerda somente quando o peer imediato é confiável; header malformado cai no IP do peer. `ALLOWED_ORIGINS` fica vazio em produção; frontend e API compartilham origem.

O Compose publica `127.0.0.1:8080` no **host**. Se o NPM estiver em outro container, `127.0.0.1` desse container não alcança a publicação do host; confirmar a topologia e escolher uma ligação interna controlada antes da abertura. A configuração de rede real não foi modificada nem testada nesta entrega.

Métodos mutáveis em produção exigem Origin ou Referer que corresponda exatamente à origem HTTPS configurada, além do par cookie/header CSRF. Cookie de sessão permanece HttpOnly, Secure e SameSite=Lax, com token armazenado apenas como hash no banco. Logout revoga a sessão no servidor; o frontend só limpa o estado local após sucesso ou 401. O cookie CSRF continua válido após logout para permitir novo login sem recarregar a página. Alterar proxy/domínio requer verificar login, logout, CSRF e renovação de sessão em HTTPS antes de abrir acesso.

O rate limiter continua em memória e por processo. Agora falha fechado quando a tabela de IPs atinge o teto; produção limita configuração a no máximo 20 tentativas por janela de no mínimo um minuto. No máximo dois logins podem executar Argon2 simultaneamente por processo. Hashes PHC rejeitam parâmetros excessivos/malformados antes do cálculo. Se houver múltiplas réplicas, adicionar limitação também no proxy ou um armazenamento compartilhado; não considerar o contador local como limite global. Um ataque distribuído ainda pode produzir indisponibilidade parcial, exigindo monitoramento e proteção no NPM.

Logs HTTP registram apenas padrão de rota, método, status, duração e IP; query/body, identificadores de rota, valores de panic e erros internos não são emitidos. Erros de inicialização também são genéricos; diagnóstico detalhado deve ocorrer em ambiente isolado, sem copiar DSNs para logs. Os eventos de auditoria continuam com vocabulário fechado e sem PHI.

## Rotação preparada, ainda não executada

**DATABASE_URL/password:** preparar nova role runtime (ou nova senha com sobreposição controlada), grants equivalentes e DSN nova fora do repositório. Em janela de mudança: validar acesso em banco descartável/cópia autorizada, trocar `APP_DATABASE_URL`, reiniciar somente a aplicação, verificar readiness/login/operações administrativas, então revogar a role/senha antiga. Guardar plano de rollback e não registrar DSNs completos. Se mudar a senha da mesma role, fazer troca coordenada porque conexões novas falharão até atualizar o app; a opção de duas roles reduz o intervalo de risco.

**PACS_MASTER_KEY:** o valor cifra a credencial Orthanc em `app_settings` via AES-GCM; substituição direta torna a credencial existente ilegível. A rotação depende de uma cópia válida da credencial Orthanc no cofre institucional. Planejar janela sem tráfego clínico, backup/rollback do banco e da chave antiga, atualizar a chave do backend, regravar a credencial Orthanc pela tela administrativa antes de reabrir tráfego, testar a integração read-only e só então aposentar a chave anterior. Não tentar regravar quando a credencial original não estiver disponível; nesse caso a rotação permanece bloqueada até existir procedimento de recriptografia offline revisado. Secrets previamente expostos devem ser rotacionados antes de produção, sem reproduzir valores aqui.

## Inventário npm (relatório local de 30/09/2026)

`npm audit --json` apontou inicialmente **12** pacotes (6 high, 6 moderate). A única alteração aplicada foi override de `fflate` 0.7.3 → 0.7.5, patch dentro da linha 0.7, usado por vtk.js/Cornerstone. Depois: **10** (7 high, 3 moderate). A classificação herdada de `@cornerstonejs/tools` mudou de moderate para high no novo relatório; não indica uma nova dependência. Não foi executado `npm audit fix`.

| Pacote original | Aviso/cadeia | Decisão nesta etapa |
| --- | --- | --- |
| `fflate` | ZIP64 malformado pode causar loop em `unzipSync` | Corrigido com override 0.7.5; smoke do Viewer necessário |
| `@kitware/vtk.js` | Herdava `fflate` | Saiu do relatório após patch |
| `adm-zip` | Oito avisos de ZIP hostil: alocação, bomba, symlink, permissões, nomes duplicados e caminhos async | Transitivo de `dcmjs`; não há import de `adm-zip` no bundle ES de `dcmjs` inspecionado. Não forçar 0.6.1 sem prova de compatibilidade; revisar no upstream antes da produção |
| `dcmjs` | Herda `adm-zip` | Pendente upstream; parsing DICOM do Viewer não usa fluxo ZIP da aplicação |
| `uuid` | v3/v5/v6 com buffer fornecido sem checagem de tamanho | Transitivo do loader; sem API de buffer no código da aplicação. Correção exige major, pendente upstream |
| `esbuild` | Dev server antigo aceita requisições cross-origin | Transitivo do plugin CommonJS de build, não do servidor de produção; nunca publicar Vite/dev server |
| `@originjs/vite-plugin-commonjs` | Herda `esbuild` antigo | Somente build; upgrade isolado requer regressão Cornerstone, pendente |
| `@cornerstonejs/core` | Herda `metadata`/`utils` | Pendente upstream, sem upgrade major durante feature freeze |
| `@cornerstonejs/dicom-image-loader` | Herda `core`/`metadata`/`uuid` | Pendente upstream; Viewer deve ser retestado com DICOM sintético e real em janela separada |
| `@cornerstonejs/metadata` | Herda `utils`/`dcmjs` | Pendente upstream |
| `@cornerstonejs/utils` | Herda `dcmjs` | Pendente upstream |
| `@cornerstonejs/tools` | Herda `core`; audit sugere downgrade incompatível 0.27.2 | Não aplicar downgrade/major automático; pendente upstream |

O relatório npm é sinal de revisão, não prova de explorabilidade. O pacote de produção contém o build estático, sem Node/npm/Vite. Antes da abertura, revisar advisories atualizados e confirmar por teste que o patch de `fflate` não afeta renderização. A política de retenção de secrets e a proteção distribuída do login seguem pendentes operacionais.
