import { useEffect, useRef, useState, type FormEvent } from 'react';
import { api, ApiError, type ManagedUser, type Unit, type UsersPage, type Validity } from '../api/client';
import { useSession } from '../auth/SessionProvider';
import { BlueprintCorners } from '../design-system/Blueprint';
import { Icon } from '../design-system/Icon';

type Form = { mode: 'create' | 'edit' | 'renew' | 'reset'; role: 'MEDICO' | 'GESTOR'; target?: ManagedUser; name: string; username: string; email: string; unitIds: string[]; validity: Validity; password: string };
const periods: [Validity,string][] = [['1m','1 mês'],['3m','3 meses'],['6m','6 meses'],['1y','1 ano'],['unlimited','Sem expiração']];
const date = (value: string | null) => value ? value.split('-').reverse().join('/') : 'Sem expiração';
const statusLabel = { active: 'Ativo', inactive: 'Inativo', expired: 'Expirado' };
const fresh = (role: Form['role']): Form => ({mode:'create',role,name:'',username:'',email:'',unitIds:[],validity:'3m',password:''});

export function UsuariosScreen({ onSessionExpired, onToast, modalInicial = false }: { onSessionExpired: () => void; onToast: (text:string) => void; modalInicial?: boolean }) {
 const {user}=useSession();const admin=user?.role==='ADMIN';
 const [page,setPage]=useState<UsersPage|null>(null),[loading,setLoading]=useState(true),[saving,setSaving]=useState(false);
 const [error,setError]=useState(''),[formError,setFormError]=useState('');
 const [search,setSearch]=useState(''),[role,setRole]=useState(''),[unitId,setUnitId]=useState(''),[status,setStatus]=useState(''),[offset,setOffset]=useState(0),[revision,setRevision]=useState(0);
 const [form,setForm]=useState<Form|null>(modalInicial?fresh('MEDICO'):null);
 const [units,setUnits]=useState<Unit[]>([]),[unitOffset,setUnitOffset]=useState(0),[nextUnits,setNextUnits]=useState<number|null>(null),[unitError,setUnitError]=useState(''),[unitsLoading,setUnitsLoading]=useState(true);
 const mutation=useRef<AbortController|null>(null);
 useEffect(()=>()=>{mutation.current?.abort();},[]);
 useEffect(()=>{
  const c=new AbortController();setLoading(true);setError('');
  const timer=setTimeout(()=>{void api.getUsers({search,role,unitId,status,offset},c.signal).then(result=>{if(!c.signal.aborted){setPage(result);setLoading(false);}}).catch((e:unknown)=>{if(c.signal.aborted)return;if(e instanceof ApiError&&e.status===401){onSessionExpired();return;}setError(e instanceof ApiError?e.message:'Não foi possível carregar os usuários.');setLoading(false);});},300);
  return()=>{clearTimeout(timer);c.abort();};
 },[search,role,unitId,status,offset,revision,onSessionExpired]);
 useEffect(()=>{
  const c=new AbortController();setUnitsLoading(true);setUnitError('');
  void api.getUnits(false,unitOffset,c.signal).then(p=>{if(c.signal.aborted)return;setUnits(old=>unitOffset===0?p.items:[...old,...p.items.filter(u=>!old.some(v=>v.id===u.id))]);setNextUnits(p.nextOffset);setUnitsLoading(false);}).catch((e:unknown)=>{if(c.signal.aborted)return;if(e instanceof ApiError&&e.status===401){onSessionExpired();return;}setUnitError('Não foi possível carregar as unidades.');setUnitsLoading(false);});
  return()=>c.abort();
 },[unitOffset,revision,onSessionExpired]);
 async function mutate(operation:(signal:AbortSignal)=>Promise<ManagedUser>,message:string){
  if(mutation.current)return;const c=new AbortController();mutation.current=c;setSaving(true);setFormError('');
  try{await operation(c.signal);if(c.signal.aborted)return;setForm(null);setOffset(0);setRevision(v=>v+1);onToast(message);}
  catch(e){if(c.signal.aborted)return;if(e instanceof ApiError&&e.status===401){onSessionExpired();return;}setFormError(e instanceof ApiError?e.message:'Não foi possível salvar o usuário.');setForm(f=>f?{...f,password:''}:null);}
  finally{if(!c.signal.aborted){mutation.current=null;setSaving(false);}}
 }
 function open(mode:Form['mode'],target?:ManagedUser,createRole:Form['role']='MEDICO'){
  setFormError('');setForm(target?{...fresh(target.role==='GESTOR'?'GESTOR':'MEDICO'),mode,target,name:target.name,username:target.username,email:target.email??'',unitIds:target.units.map(u=>u.id)}:fresh(createRole));
 }
 function submit(e:FormEvent){e.preventDefault();if(!form)return;
  if((form.mode==='create'||form.mode==='edit')&&(!form.name.trim()||!form.unitIds.length)){setFormError('Informe nome e ao menos uma unidade.');return;}
  if(form.mode==='create'&&!/^[a-z0-9][a-z0-9_-]{2,63}$/.test(form.username)){setFormError('Usuário: 3 a 64 caracteres, iniciando por letra ou número; sem ponto, espaços ou acentos.');return;}
  if((form.mode==='create'||form.mode==='reset')&&[...form.password].length<12){setFormError('A senha deve ter ao menos 12 caracteres.');return;}
  const f=form;
  if(f.mode==='create')void mutate(signal=>api.createUser(f.role,{name:f.name,username:f.username,email:f.email,unitIds:f.unitIds,validity:f.validity,initialPassword:f.password},signal),'Usuário criado. Entregue a senha inicial por um canal seguro.');
  else if(f.target){const id=f.target.id;
   if(f.mode==='edit')void mutate(signal=>api.editUser(id,{name:f.name,email:f.email,unitIds:f.unitIds},signal),'Usuário atualizado.');
   if(f.mode==='renew')void mutate(signal=>api.renewUser(id,f.validity,signal),'Acesso renovado.');
   if(f.mode==='reset'&&window.confirm('Redefinir a senha e encerrar todas as sessões deste usuário?'))void mutate(signal=>api.resetUserPassword(id,f.password,signal),'Senha redefinida. Nova troca será obrigatória.');
  }
 }
 const busy=saving||loading;
 const linked=form?.target?.units??[];
 const options=[...units,...linked.filter(u=>!units.some(v=>v.id===u.id))];
 const title=form?.mode==='edit'?'Editar usuário':form?.mode==='renew'?'Renovar acesso':form?.mode==='reset'?'Redefinir senha':form?.role==='GESTOR'?'Novo gestor':'Novo médico';
 return <div style={{padding:'32px 40px 48px',display:'flex',flexDirection:'column',gap:20}}>
  <div style={{display:'flex',alignItems:'center',justifyContent:'space-between'}}><h1 style={{margin:0,fontSize:40}}>Usuários</h1><div style={{display:'flex',gap:10}}><button className="btn btn-primary" disabled={busy||!!form} onClick={()=>open('create')}><Icon name="plus"/> Novo médico</button>{admin&&<button className="btn btn-secondary" disabled={busy||!!form} onClick={()=>open('create',undefined,'GESTOR')}>Novo gestor</button>}</div></div>
  <p style={{margin:0,color:'var(--color-neutral-700)'}}>Gerencie acessos, unidades e validade.{!admin&&' Seu perfil administra somente médicos.'}</p>
  <div style={{display:'flex',gap:12,alignItems:'end',flexWrap:'wrap'}}>
   <div className="field" style={{flex:1,minWidth:220}}><label htmlFor="users-search">Buscar nome ou usuário</label><input id="users-search" className="input" maxLength={120} value={search} onChange={e=>{setSearch(e.target.value);setOffset(0);}}/></div>
   {admin&&<div className="field"><label htmlFor="users-role">Papel</label><select id="users-role" className="input" value={role} onChange={e=>{setRole(e.target.value);setOffset(0);}}><option value="">Todos</option><option value="MEDICO">Médico</option><option value="GESTOR">Gestor</option><option value="ADMIN">Administrador</option></select></div>}
   <div className="field"><label htmlFor="users-unit">Unidade</label><select id="users-unit" className="input" value={unitId} onChange={e=>{setUnitId(e.target.value);setOffset(0);}}><option value="">Todas</option>{units.map(u=><option key={u.id} value={u.id}>{u.name}</option>)}</select></div>
   <div className="field"><label htmlFor="users-status">Status</label><select id="users-status" className="input" value={status} onChange={e=>{setStatus(e.target.value);setOffset(0);}}><option value="">Todos</option><option value="active">Ativo</option><option value="inactive">Inativo</option><option value="expired">Expirado</option></select></div>
   <button className="btn btn-secondary" disabled={saving} onClick={()=>setRevision(v=>v+1)}>Atualizar</button>
  </div>
  {unitError&&<p role="alert">{unitError} <button className="btn btn-secondary" onClick={()=>setRevision(v=>v+1)}>Tentar carregar unidades</button></p>}
  {nextUnits!==null&&<button className="btn btn-secondary" disabled={unitsLoading} onClick={()=>setUnitOffset(nextUnits)}>Carregar mais unidades</button>}
  {!form&&formError&&<p role="alert">{formError}</p>}
  <div className="blueprint" aria-busy={loading} style={{position:'relative',overflowX:'auto'}}><BlueprintCorners/>
   <table style={{width:'100%',borderCollapse:'collapse',textAlign:'left',fontSize:14}}><thead><tr>{['Nome','Usuário','Papel','Unidades','Status','Validade','Último acesso','Ações'].map(h=><th key={h} scope="col" style={{padding:12,fontSize:11,textTransform:'uppercase',borderBottom:'1px solid var(--color-divider)'}}>{h}</th>)}</tr></thead>
   <tbody>{!loading&&!error&&page?.items.map(u=><tr key={u.id} style={{borderBottom:'1px solid var(--color-divider)'}}>
    <td style={{padding:12}}>{u.name}</td><td style={{padding:12}}>{u.username}</td><td style={{padding:12}}>{u.role==='ADMIN'?'Administrador':u.role==='GESTOR'?'Gestor':'Médico'}</td>
    <td style={{padding:12}}>{u.units.map(v=>`${v.name}${v.active?'':' (inativa)'}`).join(', ')||'—'}</td><td style={{padding:12}}><span className={`tag ${u.status==='active'?'tag-accent':''}`}>{statusLabel[u.status]}</span></td>
    <td style={{padding:12,whiteSpace:'nowrap'}}>{date(u.accessValidUntil)}</td><td style={{padding:12}}>{u.lastLoginAt?new Date(u.lastLoginAt).toLocaleString('pt-BR'):'—'}</td>
    <td style={{padding:12}}>{u.role==='ADMIN'?<span>Protegido · CLI</span>:<div style={{display:'flex',gap:6,flexWrap:'wrap'}}>
     <button className="btn btn-secondary" disabled={busy||!!form} onClick={()=>open('edit',u)}>Editar</button><button className="btn btn-secondary" disabled={busy||!!form} onClick={()=>open('renew',u)}>Renovar acesso</button><button className="btn btn-secondary" disabled={busy||!!form} onClick={()=>open('reset',u)}>Redefinir senha</button>
     <button className="btn btn-secondary" disabled={busy||!!form} onClick={()=>{if(!u.active||window.confirm('Desativar este usuário e encerrar todas as suas sessões?'))void mutate(signal=>api.setUserActive(u.id,!u.active,signal),u.active?'Usuário desativado.':'Usuário reativado. A validade foi preservada.');}}>{u.active?'Desativar':'Reativar'}</button>
    </div>}</td>
   </tr>)}</tbody></table>
   {loading&&<p role="status" style={{padding:24}}>Carregando usuários…</p>}{!loading&&!error&&page?.items.length===0&&<p style={{padding:24}}>Nenhum usuário encontrado.</p>}
   {error&&<div role="alert" style={{padding:24}}>{error} <button className="btn btn-secondary" onClick={()=>setRevision(v=>v+1)}>Tentar novamente</button></div>}
  </div>
  <div style={{display:'flex',gap:16,alignItems:'center'}}><button className="btn btn-secondary" disabled={busy||offset===0||!!form} onClick={()=>setOffset(v=>Math.max(0,v-25))}>Anterior</button><span>Página {Math.floor(offset/25)+1}</span><button className="btn btn-secondary" disabled={busy||page?.nextOffset==null||!!form} onClick={()=>{if(page?.nextOffset!=null)setOffset(page.nextOffset);}}>Próxima</button></div>
  {form&&<div role="dialog" aria-modal="true" aria-labelledby="user-form-title" style={{position:'fixed',inset:0,zIndex:30,display:'grid',placeItems:'center',background:'color-mix(in srgb,var(--color-neutral-900) 50%,transparent)'}}>
   <form onSubmit={submit} className="blueprint" style={{position:'relative',width:560,maxHeight:'90vh',overflowY:'auto',padding:28,background:'var(--color-bg)',display:'flex',flexDirection:'column',gap:16}}><BlueprintCorners/><h2 id="user-form-title" style={{margin:0}}>{title}</h2>
    {form.target&&<p style={{margin:0}}>{form.target.username}</p>}
    {(form.mode==='create'||form.mode==='edit')&&<>
     <div className="field"><label htmlFor="user-name">Nome completo *</label><input id="user-name" className="input" required maxLength={120} autoFocus disabled={saving} value={form.name} onChange={e=>setForm({...form,name:e.target.value})}/></div>
     {form.mode==='create'&&<div className="field"><label htmlFor="user-username">Nome de usuário *</label><input id="user-username" className="input" required minLength={3} maxLength={64} autoComplete="off" disabled={saving} value={form.username} onChange={e=>setForm({...form,username:e.target.value.toLowerCase()})}/><small>3–64 caracteres. Letras, números, _ e -. Sem ponto.</small></div>}
     <div className="field"><label htmlFor="user-email">E-mail (opcional)</label><input id="user-email" type="email" className="input" maxLength={254} disabled={saving} value={form.email} onChange={e=>setForm({...form,email:e.target.value})}/></div>
     <fieldset disabled={saving} style={{border:'1px solid var(--color-divider)',display:'flex',flexDirection:'column',gap:8,maxHeight:170,overflowY:'auto'}}><legend>Unidades *</legend>
      {options.map(u=><label key={u.id}><input type="checkbox" checked={form.unitIds.includes(u.id)} disabled={!u.active&&!linked.some(v=>v.id===u.id)} onChange={e=>setForm({...form,unitIds:e.target.checked?[...form.unitIds,u.id]:form.unitIds.filter(id=>id!==u.id)})}/> {u.name}{!u.active?' (inativa · vínculo existente)':''}</label>)}
      {unitsLoading&&<span>Carregando unidades…</span>}{!unitsLoading&&!options.length&&<span>Nenhuma unidade ativa disponível.</span>}{nextUnits!==null&&<button type="button" className="btn btn-secondary" disabled={unitsLoading} onClick={()=>setUnitOffset(nextUnits)}>Mais unidades</button>}
     </fieldset>
    </>}
    {(form.mode==='create'||form.mode==='renew')&&<div className="field"><label htmlFor="user-validity">Validade *</label><select id="user-validity" className="input" disabled={saving} value={form.validity} onChange={e=>setForm({...form,validity:e.target.value as Validity})}>{periods.map(([value,label])=><option key={value} value={value}>{label}</option>)}</select>{form.mode==='renew'&&<small>Acrescenta ao vencimento atual se ainda válido; caso contrário, conta de hoje. Sem prazo anterior: conta de hoje.</small>}</div>}
    {(form.mode==='create'||form.mode==='reset')&&<div className="field"><label htmlFor="user-password">{form.mode==='reset'?'Nova senha inicial *':'Senha inicial *'}</label><input id="user-password" type="password" className="input" required minLength={12} maxLength={1024} autoComplete="new-password" disabled={saving} value={form.password} onChange={e=>setForm({...form,password:e.target.value})}/><small>Mínimo de 12 caracteres. Troca obrigatória no próximo acesso. Sem envio por e-mail.</small></div>}
    {formError&&<p role="alert">{formError}</p>}
    <div style={{display:'flex',justifyContent:'end',gap:10}}><button type="button" className="btn btn-secondary" disabled={saving} onClick={()=>{setForm(null);setFormError('');}}>Cancelar</button><button className="btn btn-primary" disabled={saving}>{saving?'Salvando…':form.mode==='create'?'Criar usuário':'Salvar'}</button></div>
   </form>
  </div>}
 </div>;
}
