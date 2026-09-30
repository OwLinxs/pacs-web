import { useEffect, useState } from 'react';
import { api, ApiError, type AuditPage, type AuditQuery, type AuditReference, type ManagedUser } from '../api/client';
import { eventLabels, eventLabel, auditDate, resultLabel, detailLabel } from '../audit/model';
import { BlueprintCorners } from '../design-system/Blueprint';

function reference(ref:AuditReference|null) {
 if(!ref)return 'Não identificado';
 return ref.name ? `${ref.name}${ref.username ? ` / ${ref.username}`:''}` : ref.username || `${ref.kind==='unit'?'Unidade':'Usuário'} indisponível (${ref.id})`;
}

/** Read-only administrative history. Filters live only in React memory. */
export function AuditoriaScreen({onSessionExpired}:{onSessionExpired:()=>void}) {
 const [draft,setDraft]=useState<AuditQuery>({});
 const [query,setQuery]=useState<AuditQuery>({});
 const [revision,setRevision]=useState(0);
 const [page,setPage]=useState<AuditPage|null>(null);
 const [loading,setLoading]=useState(true);
 const [error,setError]=useState('');
 const [validation,setValidation]=useState('');
 const [pickerKey,setPickerKey]=useState(0);
 useEffect(()=>{
  const controller=new AbortController();setLoading(true);setError('');
  api.getAudit(query,controller.signal).then(value=>{if(!controller.signal.aborted)setPage(value);}).catch(err=>{
   if(controller.signal.aborted)return;
   if(err instanceof ApiError && (err.status===401 || err.code==='PASSWORD_CHANGE_REQUIRED')){onSessionExpired();return;}
   setError(err instanceof ApiError && err.status===403?'Seu perfil não pode consultar a auditoria.':'Não foi possível consultar a auditoria. Tente novamente.');
  }).finally(()=>{if(!controller.signal.aborted)setLoading(false);});
  return()=>controller.abort();
 },[query,revision,onSessionExpired]);
 function apply(){
  if(draft.dateFrom && draft.dateTo && draft.dateFrom>draft.dateTo){setValidation('A data inicial deve ser anterior ou igual à final.');return;}
  setValidation('');setQuery({...draft,offset:0});
 }
 const field=(key:keyof AuditQuery,value:string)=>setDraft(v=>({...v,[key]:value}));
 return <section style={{padding:'32px 40px 48px',display:'flex',flexDirection:'column',gap:20,maxWidth:1600,minWidth:980}}>
  <header style={{display:'flex',alignItems:'center',gap:20}}><h1 style={{margin:0,fontSize:40}}>Auditoria</h1><span style={{fontSize:13,color:'var(--color-neutral-700)'}}>Histórico administrativo · horário de Brasília</span><button className="btn btn-secondary" style={{marginLeft:'auto'}} disabled={loading} onClick={()=>{setQuery(q=>({...q,offset:0}));setRevision(v=>v+1);}}>Atualizar</button></header>
  <form onSubmit={e=>{e.preventDefault();apply();}} style={{display:'grid',gridTemplateColumns:'repeat(4,minmax(0,1fr))',gap:12,alignItems:'start'}}>
   <div className="field"><label htmlFor="audit-from">Data inicial</label><input className="input" id="audit-from" type="date" value={draft.dateFrom??''} onChange={e=>field('dateFrom',e.target.value)}/></div>
   <div className="field"><label htmlFor="audit-to">Data final</label><input className="input" id="audit-to" type="date" value={draft.dateTo??''} onChange={e=>field('dateTo',e.target.value)}/></div>
   <div className="field"><label htmlFor="audit-event">Ação</label><select className="input" id="audit-event" value={draft.event??''} onChange={e=>field('event',e.target.value)}><option value="">Todas as ações</option>{Object.entries(eventLabels).map(([value,label])=><option key={value} value={value}>{label}</option>)}</select></div>
   <div className="field"><label htmlFor="audit-category">Categoria</label><select className="input" id="audit-category" value={draft.category??''} onChange={e=>field('category',e.target.value)}><option value="">Todas</option><option value="users">Usuários</option><option value="units">Unidades</option><option value="auth">Autenticação</option><option value="settings">Configuração PACS</option><option value="viewer">Exportação do Viewer</option></select></div>
   <UserFilter key={`actor-${pickerKey}`} id="audit-actor" label="Ator" onChange={id=>field('actorId',id)} onSessionExpired={onSessionExpired}/>
   <UserFilter key={`target-${pickerKey}`} id="audit-target" label="Usuário alvo" onChange={id=>field('targetUserId',id)} onSessionExpired={onSessionExpired}/>
   <div style={{display:'flex',gap:8,paddingTop:22}}><button type="submit" className="btn btn-primary">Aplicar filtros</button><button type="button" className="btn btn-secondary" onClick={()=>{setDraft({});setQuery({});setValidation('');setPickerKey(k=>k+1);}}>Limpar filtros</button></div>
  </form>
  {validation && <p role="alert">{validation}</p>}
  <div aria-busy={loading} aria-live="polite">{loading?(page?'Atualizando eventos…':'Carregando eventos…'):''}</div>
  {error && <div role="alert">{error} <button className="btn btn-secondary" onClick={()=>setRevision(v=>v+1)}>Tentar novamente</button></div>}
  {!error && page && <div className="blueprint" style={{position:'relative',overflowX:'auto'}}><BlueprintCorners/>
   <table style={{width:'100%',borderCollapse:'collapse',fontSize:13,textAlign:'left'}}>
    <thead><tr>{['Data / Hora','Ator','Ação','Alvo','Resultado','Detalhes'].map(text=><th key={text} style={{padding:12,borderBottom:'1px solid var(--color-divider)',fontWeight:600}}>{text}</th>)}</tr></thead>
    <tbody>{page.items.map(event=><tr key={event.id} style={{borderBottom:'1px solid var(--color-divider)',verticalAlign:'top'}}>
     <td style={{padding:12,whiteSpace:'nowrap'}}>{auditDate(event.occurredAt)}</td><td style={{padding:12}}>{reference(event.actor)}</td><td style={{padding:12}}>{eventLabel(event.event)}</td><td style={{padding:12}}>{event.target?reference(event.target):event.category==='settings'?'Configuração PACS':'—'}</td><td style={{padding:12}}>{resultLabel(event.result)}</td>
     <td style={{padding:12}}><details><summary style={{cursor:'pointer'}}>Ver detalhes</summary><p>{detailLabel(event.detail)}</p><p>Registro: {event.id}</p>{event.origin && <p>Origem (IP): {event.origin}</p>}{event.actor && <p>ID do ator: {event.actor.id}</p>}{event.target && <p>ID do alvo: {event.target.id}</p>}<small>Nomes resolvidos pelo cadastro atual.</small></details></td>
    </tr>)}</tbody>
   </table>{page.items.length===0 && <p style={{padding:20}}>Nenhum evento encontrado para os filtros selecionados.</p>}
  </div>}
  {page && !error && <footer style={{display:'flex',alignItems:'center',gap:12}}><span>Página {Math.floor(page.offset/page.limit)+1} · até {page.limit} eventos por página</span><button className="btn btn-secondary" disabled={loading || page.offset===0} onClick={()=>setQuery(q=>({...q,offset:Math.max(0,page.offset-page.limit)}))}>Anterior</button><button className="btn btn-secondary" disabled={loading || page.nextOffset===null} onClick={()=>setQuery(q=>({...q,offset:page.nextOffset??0}))}>Próxima</button>{!page.hasMore && <span>Fim dos resultados.</span>}{page.hasMore && page.nextOffset===null && <span>Refine o período para consultar mais eventos.</span>}</footer>}
 </section>;
}

function UserFilter({id,label,onChange,onSessionExpired}:{id:string;label:string;onChange:(id:string)=>void;onSessionExpired:()=>void}) {
 const [search,setSearch]=useState('');const [selected,setSelected]=useState<ManagedUser|null>(null);
 const [open,setOpen]=useState(false);const [items,setItems]=useState<ManagedUser[]>([]);const [offset,setOffset]=useState(0);const [next,setNext]=useState<number|null>(null);const [loading,setLoading]=useState(false);const [error,setError]=useState('');
 useEffect(()=>{
  if(!open)return;
  const ctrl=new AbortController();setLoading(true);setError('');
  const timer=setTimeout(()=>{api.getUsers({search,offset},ctrl.signal).then(p=>{if(!ctrl.signal.aborted){setItems(p.items);setNext(p.nextOffset);}}).catch(e=>{if(ctrl.signal.aborted)return;if(e instanceof ApiError && (e.status===401 || e.code==='PASSWORD_CHANGE_REQUIRED'))onSessionExpired();else setError('Não foi possível localizar usuários.');}).finally(()=>{if(!ctrl.signal.aborted)setLoading(false);});},250);
  return()=>{clearTimeout(timer);ctrl.abort();};
 },[open,search,offset,onSessionExpired]);
 return <div className="field"><label htmlFor={id}>{label}</label>{selected?<div>{selected.name} / {selected.username} <button type="button" className="btn btn-secondary" onClick={()=>{setSelected(null);onChange('');}}>Remover filtro</button></div>:<input className="input" id={id} placeholder="Buscar nome ou usuário" value={search} onFocus={()=>setOpen(true)} onChange={e=>{setSearch(e.target.value);setOffset(0);setOpen(true);}}/>}
 {open && !selected && <div style={{border:'1px solid var(--color-divider)',padding:8,maxHeight:220,overflowY:'auto'}}>{loading?'Buscando…':error||items.length===0?'Nenhum usuário disponível.':items.map(u=><button key={u.id} type="button" className="btn btn-secondary" style={{display:'block',width:'100%',textAlign:'left',marginBottom:4}} onClick={()=>{setSelected(u);setOpen(false);onChange(u.id);}}>{u.name} / {u.username}</button>)}{error && <span role="alert">{error}</span>}<div style={{display:'flex',gap:8}}>{offset>0 && <button type="button" disabled={loading} onClick={()=>setOffset(Math.max(0,offset-25))}>Anteriores</button>}{next!==null && <button type="button" disabled={loading} onClick={()=>setOffset(next)}>Mais usuários</button>}<button type="button" onClick={()=>setOpen(false)}>Fechar</button></div></div>}
 </div>;
}
