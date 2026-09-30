import {useEffect,useRef,useState} from 'react';
import {api,ApiError} from '../api/client';
import {exportFilename,mayExportWithoutIdentification,type ExportFormat,type ExportSnapshot} from './exportModel';
import {generateExport} from './exportImage';

export function ExportDialog({viewport,capture,close,expired,signal}:{viewport:number;capture:()=>ExportSnapshot;close:()=>void;expired:()=>void;signal:AbortSignal}) {
 const dialog=useRef<HTMLDialogElement>(null);const pending=useRef(false);
 const [format,setFormat]=useState<ExportFormat>('PNG');const [identified,setIdentified]=useState<boolean|null>(null);
 const [confirmed,setConfirmed]=useState(false);const [busy,setBusy]=useState(false);const [message,setMessage]=useState('');
 const lifetime=useRef<AbortController|null>(null);
 useEffect(()=>{
  const controller=new AbortController();lifetime.current=controller;
  const abort=()=>controller.abort();signal.addEventListener('abort',abort,{once:true});if(signal.aborted)abort();
  dialog.current?.showModal();
  return()=>{controller.abort();signal.removeEventListener('abort',abort);};
 },[signal]);
 async function save(){
  if(pending.current||!lifetime.current)return;
  pending.current=true;setBusy(true);setMessage('');let snapshot:ExportSnapshot|null=null;
  const current=lifetime.current.signal;
  try {
   current.throwIfAborted();if(identified===null){setMessage('Selecione o modo de identificação.');return;}snapshot=capture();
   if(!identified && (!mayExportWithoutIdentification(snapshot.metadata)||!confirmed)){setMessage('Sem identificação exige BurnedInAnnotation=NO e confirmação visual de que não há identificação nos pixels.');return;}
   const blob=await generateExport(snapshot,format,identified);snapshot.dispose();snapshot=null;current.throwIfAborted();
   let warning=false;
   try{await api.recordViewerExport(format,identified,current);}catch(e){
    if(current.aborted)throw e;
    if(e instanceof ApiError && (e.status===401 || e.code==='PASSWORD_CHANGE_REQUIRED')){expired();return;}
    if(e instanceof ApiError && e.status===403){setMessage('A sessão não permite esta exportação. Recarregue a página.');return;}
    // A failed server response is not evidence of a live session: revalidate before download.
    const user=await api.me();current.throwIfAborted();if(!user||user.mustChangePassword){expired();return;}warning=true;
   }
   current.throwIfAborted();
   const url=URL.createObjectURL(blob);const link=document.createElement('a');link.href=url;link.download=exportFilename(format);try {link.click();} finally {window.setTimeout(()=>URL.revokeObjectURL(url),1000);}
   // Browser has consumed the click. Release on a short bounded timer, including after dialog close.

   setMessage(warning?'Arquivo gerado, mas a auditoria não pôde ser confirmada.':'Arquivo gerado. Verifique o download no navegador.');
  }catch{if(!current.aborted)setMessage('Não foi possível exportar a imagem atual. Aguarde o carregamento e tente novamente.');}
  finally{snapshot?.dispose();pending.current=false;if(!current.aborted)setBusy(false);}
 }
 return <dialog ref={dialog} aria-labelledby="export-title" onCancel={event=>{event.preventDefault();if(!busy)close();}} style={{background:'#171c22',color:'#e2e8f0',border:'1px solid #536171',maxWidth:500,padding:24}}>
  <h2 id="export-title">Exportar imagem</h2><p>Imagem atual · viewport {viewport+1}. Somente o resultado renderizado deste viewport.</p>
  <label htmlFor="export-format">Formato</label><select id="export-format" className="input" value={format} disabled={busy} onChange={e=>setFormat(e.target.value as ExportFormat)}>{(['PNG','JPEG','PDF'] as const).map(f=><option key={f}>{f}</option>)}</select>
  <fieldset disabled={busy} style={{marginTop:16}}><legend>Identificação</legend><label><input type="radio" name="export-identification" checked={identified===true} onChange={()=>{setIdentified(true);setConfirmed(false);}}/> Com identificação</label><br/><label><input type="radio" name="export-identification" checked={identified===false} onChange={()=>setIdentified(false)}/> Sem identificação</label></fieldset>
  {identified===null?<p>Selecione explicitamente o modo de identificação.</p>:identified?<p>O arquivo exportado conterá dados de identificação do paciente.</p>:<><p>Sem metadados de identificação. Não é anonimização DICOM. Identificação já gravada nos pixels não é removida; requer BurnedInAnnotation=NO.</p><label><input type="checkbox" checked={confirmed} disabled={busy} onChange={e=>setConfirmed(e.target.checked)}/> Conferi a imagem e não há identificação visível nos pixels.</label></>}
  <p>Medições e overlays da interface não são incluídos. O enquadramento atual é mantido.</p>
  {message && <p role="status">{message}</p>}
  <div style={{display:'flex',gap:12,marginTop:20}}><button type="button" className="btn btn-primary" disabled={busy||identified===null||(!identified&&!confirmed)} onClick={()=>void save()}>{busy?'Gerando…':'Gerar arquivo'}</button><button type="button" className="btn btn-secondary" disabled={busy} onClick={close}>Fechar</button></div>
 </dialog>;
}
