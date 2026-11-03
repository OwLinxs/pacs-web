/**
 * Aviso de privacidade do PACS Web. RASCUNHO: texto e contato do encarregado
 * devem ser validados/preenchidos pela Prefeitura (encarregado de dados e
 * Procuradoria) antes do uso definitivo.
 */
export const PRIVACY_DRAFT = true;

/** Encarregado pelo tratamento de dados pessoais (LGPD, art. 41). A preencher. */
export const ENCARREGADO = { nome: '[a definir pela Prefeitura]', contato: '[e-mail ou telefone a definir]' };

export const PRIVACY_SECTIONS: { heading: string; paragraphs: string[] }[] = [
  { heading: 'Quem trata os dados', paragraphs: [
    'Município de Francisco Beltrão, por meio da Secretaria Municipal de Saúde, controlador dos dados tratados no PACS Web.',
  ] },
  { heading: 'Quais dados', paragraphs: [
    'Pacientes: imagens e informações de exames (como nome, identificação, data e descrição do exame), recebidas dos equipamentos da rede municipal.',
    'Profissionais usuários: nome, usuário, perfil, unidades vinculadas e registros de acesso (data, hora, endereço de rede e ações relevantes, como exportações).',
  ] },
  { heading: 'Para quê', paragraphs: [
    'Guarda e visualização de exames para assistência à saúde; segurança da informação, auditoria e prevenção de acesso indevido; estatísticas operacionais somente agregadas, sem identificação de pacientes.',
  ] },
  { heading: 'Base legal', paragraphs: [
    'Execução de políticas públicas e tutela da saúde (Lei nº 13.709/2018 — LGPD, arts. 7º, III, e 11, II, "b" e "f"). Por isso o tratamento não depende de consentimento.',
  ] },
  { heading: 'Compartilhamento e guarda', paragraphs: [
    'Os dados não são compartilhados fora da rede municipal de saúde, salvo obrigação legal ou requisição de autoridade competente.',
    'O prazo de guarda segue as normas aplicáveis a registros de saúde e a tabela de temporalidade do Município.',
  ] },
  { heading: 'Segurança', paragraphs: [
    'Acesso individual e restrito por perfil, sessão com expiração por inatividade, termo de responsabilidade e sigilo e registro de auditoria dos acessos.',
  ] },
  { heading: 'Direitos e contato', paragraphs: [
    'Titulares podem solicitar informações sobre o tratamento e exercer os direitos previstos na LGPD junto ao encarregado de dados do Município.',
  ] },
];
