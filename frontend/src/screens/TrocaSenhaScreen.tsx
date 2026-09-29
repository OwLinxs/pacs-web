import { useEffect, useRef, useState, type FormEvent } from 'react';
import { api, ApiError } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';

export function TrocaSenhaScreen({onChanged,onLogout,onSessionExpired}:{onChanged:()=>Promise<void>;onLogout:()=>void;onSessionExpired:()=>void}) {
 const [current,setCurrent]=useState(''),[next,setNext]=useState(''),[confirmation,setConfirmation]=useState(''),[error,setError]=useState(''),[busy,setBusy]=useState(false);
 const pending=useRef<AbortController|null>(null);useEffect(()=>()=>pending.current?.abort(),[]);
 async function submit(e:FormEvent){e.preventDefault();if(pending.current)return;
  if(next!==confirmation||[...next].length<12||next===current){setError('Informe uma senha diferente, com ao menos 12 caracteres, e confirme-a corretamente.');return;}
  const c=new AbortController();pending.current=c;setBusy(true);setError('');
  try{await api.changePassword(current,next,c.signal);if(c.signal.aborted)return;setCurrent('');setNext('');setConfirmation('');await onChanged();}
  catch(cause){if(c.signal.aborted)return;setCurrent('');setNext('');setConfirmation('');if(cause instanceof ApiError&&cause.status===401){onSessionExpired();return;}setError(cause instanceof ApiError?cause.message:'Não foi possível alterar a senha.');}
  finally{if(!c.signal.aborted){pending.current=null;setBusy(false);}}
 }
 return <main className="pacs-grid-ground" style={{minHeight:'100vh',display:'grid',placeItems:'center'}}><form onSubmit={submit} className="blueprint" style={{position:'relative',width:460,padding:32,background:'var(--color-bg)',display:'flex',flexDirection:'column',gap:18}}><BlueprintCorners/>
  <h1 style={{fontSize:28,margin:0}}>Troca obrigatória de senha</h1><p style={{margin:0}}>Defina sua senha pessoal para continuar. Após salvar, entre novamente com a nova senha.</p>
  <div className="field"><label htmlFor="password-current">Senha atual</label><input id="password-current" className="input" type="password" autoComplete="current-password" required maxLength={1024} disabled={busy} value={current} onChange={e=>setCurrent(e.target.value)}/></div>
  <div className="field"><label htmlFor="password-new">Nova senha</label><input id="password-new" className="input" type="password" autoComplete="new-password" required minLength={12} maxLength={1024} disabled={busy} value={next} onChange={e=>setNext(e.target.value)}/></div>
  <div className="field"><label htmlFor="password-confirm">Confirmar nova senha</label><input id="password-confirm" className="input" type="password" autoComplete="new-password" required disabled={busy} value={confirmation} onChange={e=>setConfirmation(e.target.value)}/></div>
  {error&&<p role="alert">{error}</p>}<button className="btn btn-primary" disabled={busy}>{busy?'Salvando…':'Alterar senha'}</button><button type="button" className="btn btn-secondary" disabled={busy} onClick={onLogout}>Sair</button>
 </form></main>;
}
