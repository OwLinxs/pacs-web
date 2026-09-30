export const eventLabels = {
 USER_CREATED:'Usuário criado', USER_UPDATED:'Usuário atualizado', USER_ACTIVATED:'Usuário ativado', USER_DEACTIVATED:'Usuário desativado',
 USER_ACCESS_RENEWED:'Acesso renovado', USER_PASSWORD_RESET:'Senha redefinida', USER_PASSWORD_CHANGED:'Senha alterada', USER_UNITS_CHANGED:'Unidades do usuário alteradas',
 UNIT_CREATED:'Unidade criada', UNIT_UPDATED:'Unidade atualizada', UNIT_ACTIVATED:'Unidade ativada', UNIT_DEACTIVATED:'Unidade desativada',
 LOGIN_SUCCESS:'Login realizado', LOGIN_FAILURE:'Login recusado', LOGOUT:'Logout', ORTHANC_SETTINGS_CHANGED:'Configuração do PACS alterada', ORTHANC_CONNECTION_TESTED:'Conexão com PACS testada',
} as const;
export function eventLabel(event:string) { return eventLabels[event as keyof typeof eventLabels] ?? 'Evento histórico não reconhecido'; }
export function auditDate(value:string) {
 const date=new Date(value);if(!Number.isFinite(date.getTime()))return '—';
 return new Intl.DateTimeFormat('pt-BR',{timeZone:'America/Sao_Paulo',dateStyle:'short',timeStyle:'medium'}).format(date);
}
export function resultLabel(value:string) { return ({success:'Sucesso',failure:'Falha',unknown:'Não informado'} as Record<string,string>)[value] ?? 'Não informado'; }
// Display only known operational details. No JSON dump or arbitrary metadata rendering.
export function detailLabel(value:string) {
 const labels:Record<string,string>={
  'unidade criada':'Unidade cadastrada.', 'nome alterado':'Nome da unidade atualizado.', 'estado ativo alterado':'Situação da unidade alterada.',
  'sessão iniciada':'Sessão iniciada.', 'encerrada pelo usuário':'Sessão encerrada pelo usuário.',
  'usuário inexistente':'Conta não identificada.', 'senha incorreta':'Credenciais recusadas.', 'conta desativada':'Conta inativa.', 'validade de acesso expirada':'Conta expirada.',
 };
 if(labels[value])return labels[value];
 if(value.startsWith('PACS/Orthanc: ')) {
  const outcome=value.slice(14);
  const results:Record<string,string>={connected:'Conexão estabelecida.',configuration_error:'Configuração indisponível.',timeout:'Tempo limite excedido.',canceled:'Teste interrompido.',dns:'Destino indisponível.',connection_refused:'Conexão recusada.',tls:'Falha TLS.',unauthorized:'Autenticação recusada.',forbidden:'Acesso recusado.',upstream_http:'Falha HTTP no PACS.',redirect:'Redirecionamento recusado.',invalid_response:'Resposta inválida.',not_orthanc:'Destino não confirmado como PACS.',unavailable:'PACS indisponível.',invalid_target:'Destino inválido.',blocked_target:'Destino bloqueado.'};
  if(results[outcome])return results[outcome];
  const allowed=['configuração criada','configuração atualizada','nome','URL','usuário','endpoint DICOMweb','timeout','verificação TLS','credencial removida','credencial alterada','sem alteração efetiva'];
  if(outcome.split(', ').every(p=>allowed.includes(p)))return 'Configuração administrativa atualizada; valores omitidos.';
 }
 return 'Sem contexto adicional disponível.';
}
