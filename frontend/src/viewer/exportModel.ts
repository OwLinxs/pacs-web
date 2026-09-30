/** Explicit presentation DTO: no accession, UIDs, URLs or arbitrary DICOM metadata. */
export type ExportMetadata = {
 patientName:string; patientId:string; studyDate:string; modality:string;
 studyDescription:string; institution:string; seriesNumber:string; instanceNumber:string;
 burnedIn:string; index:number; total:number;
};
export type ExportFormat='PNG'|'JPEG'|'PDF';
export type ExportSnapshot={canvas:HTMLCanvasElement; metadata:ExportMetadata; dispose:()=>void};
export function patientName(value:string):string {
 return (value.split('=')[0]?.trim() || value.split('=').find(p=>p.trim()) || '').split('^').map(v=>v.trim()).filter(Boolean).join(' ');
}
export function exportFields(m:ExportMetadata):[string,string][] {
 let date='—';
 if(/^\d{8}$/.test(m.studyDate)) {
  const y=Number(m.studyDate.slice(0,4)),mo=Number(m.studyDate.slice(4,6)),d=Number(m.studyDate.slice(6));
  const t=new Date(0);t.setUTCFullYear(y,mo-1,d);
  if(y>0&&t.getUTCFullYear()===y&&t.getUTCMonth()===mo-1&&t.getUTCDate()===d)date=`${m.studyDate.slice(6)}/${m.studyDate.slice(4,6)}/${m.studyDate.slice(0,4)}`;
 }
 return [['Paciente',patientName(m.patientName)||'—'],['ID',m.patientId||'—'],['Data do exame',date],['Modalidade',m.modality||'—'],['Exame',m.studyDescription||'—'],['Instituição',m.institution||'—'],['Série',m.seriesNumber||'—'],['Imagem',`${m.index+1} / ${m.total}`]];
}
export function exportFilename(format:ExportFormat,now=new Date()) {
 const pad=(n:number)=>String(n).padStart(2,'0');
 return `pacs-imagem-${now.getFullYear()}${pad(now.getMonth()+1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}.${format==='JPEG'?'jpg':format.toLowerCase()}`;
}
export function mayExportWithoutIdentification(m:ExportMetadata){return m.burnedIn.trim().toUpperCase()==='NO';}
