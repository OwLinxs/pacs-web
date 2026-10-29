/** Explicit presentation DTO: no accession, UIDs, URLs or arbitrary DICOM metadata. */
export type ExportMetadata = {
 patientName:string; patientId:string; patientBirthDate:string; patientSex:string; patientAge:string;
 studyDate:string; acquisitionTime:string; institution:string; station:string;
 seriesNumber:string; laterality:string; viewPosition:string;
 kvp:string; exposure:string; filter:string; calibration:string;
 burnedIn:string; index:number; total:number;
};
/** Rendered viewport state at capture time; null when not determinable. */
export type ExportView = { windowWidth:number|null; windowCenter:number|null; zoom:number|null; mmPerPixel:number|null };
export type ExportFormat='PNG'|'JPEG'|'PDF';
export type ExportSnapshot={canvas:HTMLCanvasElement; metadata:ExportMetadata; view:ExportView; dispose:()=>void};
export type ExportOverlay={topLeft:string[]; topRight:string[]; bottomLeft:string[]; bottomRight:string[]};
export function patientName(value:string):string {
 return (value.split('=')[0]?.trim() || value.split('=').find(p=>p.trim()) || '').split('^').map(v=>v.trim()).filter(Boolean).join(' ');
}
const MONTHS=['jan','fev','mar','abr','mai','jun','jul','ago','set','out','nov','dez'];
/** DICOM DA → 11-fev-1965; inválida ou ausente → ''. */
export function overlayDate(value:string):string {
 if(!/^\d{8}$/.test(value))return '';
 const y=Number(value.slice(0,4)),mo=Number(value.slice(4,6)),d=Number(value.slice(6));
 const t=new Date(0);t.setUTCFullYear(y,mo-1,d);
 if(y<=0||t.getUTCFullYear()!==y||t.getUTCMonth()!==mo-1||t.getUTCDate()!==d)return '';
 return `${value.slice(6)}-${MONTHS[mo-1]}-${value.slice(0,4)}`;
}
/** DICOM TM → HH:MM[:SS]; inválida ou ausente → ''. */
export function overlayTime(value:string):string {
 const m=/^(\d{2})(\d{2})(\d{2})?/.exec(value.trim());
 if(!m||Number(m[1])>23||Number(m[2])>59||(m[3]!==undefined&&Number(m[3])>60))return '';
 return [m[1],m[2],m[3]].filter(Boolean).join(':');
}
const decimal=(n:number,digits:number)=>n.toLocaleString('pt-BR',{minimumFractionDigits:digits,maximumFractionDigits:digits});
/** Number-like DICOM value (IS/DS) shown without inventing precision; non-numeric → ''. */
const numeric=(value:string)=>{const n=Number(value);return value.trim()&&Number.isFinite(n)?decimal(n,Number.isInteger(n)?0:Math.min(2,(value.split('.')[1]??'').length)):'';};
const or=(value:string)=>value||'—';
/** Corner layout of the current PACS: only tags present; labeled absences shown as dash. */
export function exportOverlay(m:ExportMetadata,v:ExportView,width:number,height:number):ExportOverlay {
 const demographics=[m.patientAge,m.patientSex,overlayDate(m.patientBirthDate)];
 const time=overlayTime(m.acquisitionTime);
 const lateral=[m.laterality,m.viewPosition].filter(Boolean).join('/');
 const kvp=numeric(m.kvp),exposure=numeric(m.exposure);
 const fov=v.mmPerPixel?`DFOV: ${decimal(width*v.mmPerPixel/10,1)} x ${decimal(height*v.mmPerPixel/10,1)} cm`:'';
 return {
  topLeft:[m.patientId,patientName(m.patientName),demographics.some(Boolean)?demographics.map(or).join('/'):''].filter(Boolean),
  topRight:[overlayDate(m.studyDate),time&&`Acq: ${time}`,m.seriesNumber&&`Se: ${m.seriesNumber}`,`Im: ${m.index+1}/${m.total}`,`Lat/Pos: ${or(lateral)}`].filter(Boolean),
  bottomLeft:[`KVP: ${kvp?`${kvp} kV`:'—'}`,`Exp: ${exposure?`${exposure} mAs`:'—'}`,`Filter: ${or(m.filter)}`,
   `Zoom: ${v.zoom?`${decimal(v.zoom,2)}x`:'—'}`,`W:${v.windowWidth===null?'—':Math.round(v.windowWidth)} L:${v.windowCenter===null?'—':Math.round(v.windowCenter)}`],
  bottomRight:[m.station,m.institution,fov,m.calibration&&`Calibration: ${m.calibration}`].filter(Boolean),
 };
}
export function exportFilename(format:ExportFormat,now=new Date()) {
 const pad=(n:number)=>String(n).padStart(2,'0');
 return `pacs-imagem-${now.getFullYear()}${pad(now.getMonth()+1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}.${format==='JPEG'?'jpg':format.toLowerCase()}`;
}
export function mayExportWithoutIdentification(m:ExportMetadata){return m.burnedIn.trim().toUpperCase()==='NO';}
