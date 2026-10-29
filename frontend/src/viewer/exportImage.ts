import { exportOverlay, type ExportFormat, type ExportSnapshot } from './exportModel';
export const JPEG_QUALITY=0.95;
const MAX_PIXELS=32*1024*1024;
function surface(w:number,h:number) {
 if(!w||!h||w*h>MAX_PIXELS || w>16384 || h>16384)throw new Error('Dimensões de exportação indisponíveis.');
 const canvas=document.createElement('canvas');canvas.width=w;canvas.height=h;
 const ctx=canvas.getContext('2d');if(!ctx)throw new Error('Canvas indisponível.');return {canvas,ctx};
}
function encode(canvas:HTMLCanvasElement,type:string,quality?:number):Promise<Blob> {
 return new Promise((resolve,reject)=>canvas.toBlob(blob=>blob&&blob.type===type?resolve(blob):reject(new Error('Não foi possível gerar o arquivo.')),type,quality));
}
function text(ctx:CanvasRenderingContext2D,value:string,x:number,y:number,width:number) {
 let rendered=value;if(ctx.measureText(rendered).width>width){while(rendered.length&&ctx.measureText(rendered+'…').width>width)rendered=rendered.slice(0,-1);rendered+='…';}
 // Contorno escuro mantém o texto legível sobre áreas claras da anatomia.
 ctx.strokeText(rendered,x,y);ctx.fillText(rendered,x,y);
 return ctx.measureText(rendered).width;
}
/** Régua em cm: traço a cada 1 cm (5 cm quando denso), maior a cada 5 cm. */
function ruler(ctx:CanvasRenderingContext2D,mmPerPixel:number,from:number,to:number,at:number,vertical:boolean,tick:number) {
 const available=(to-from)*mmPerPixel*0.9;
 const step=10/mmPerPixel>=4?10:50;
 const length=Math.floor(available/step)*step;
 if(length<step||step/mmPerPixel<4)return;
 const start=(from+to)/2-length/mmPerPixel/2;
 const point=(along:number,across:number):[number,number]=>vertical?[at+across,along]:[along,at+across];
 ctx.beginPath();ctx.moveTo(...point(start,0));ctx.lineTo(...point(start+length/mmPerPixel,0));
 for(let mm=0;mm<=length;mm+=step){const p=start+mm/mmPerPixel,size=mm===0||mm===length||mm%50===0?tick:tick/2;ctx.moveTo(...point(p,0));ctx.lineTo(...point(p,-size));}
 ctx.stroke();
}
/** Overlays nos quatro cantos e réguas, como no PACS atual. Desenha apenas sobre a cópia. */
function drawOverlay(ctx:CanvasRenderingContext2D,snapshot:ExportSnapshot,w:number,h:number) {
 const o=exportOverlay(snapshot.metadata,snapshot.view,w,h);
 const font=Math.max(11,Math.min(40,Math.round(Math.min(w,h)/48))),line=Math.round(font*1.2),margin=Math.round(font*0.5),half=w/2-margin*2;
 ctx.save();ctx.fillStyle='white';ctx.strokeStyle='rgba(0,0,0,0.85)';ctx.lineWidth=Math.max(2,font/6);ctx.lineJoin='round';ctx.textBaseline='top';
 const block=(lines:string[],right:boolean,bottom:boolean,bold:boolean)=>{
  ctx.font=`${bold?'bold ':''}${font}px sans-serif`;ctx.textAlign=right?'right':'left';let widest=0;
  lines.forEach((value,i)=>{const y=bottom?h-margin-(lines.length-i)*line:margin+i*line;widest=Math.max(widest,text(ctx,value,right?w-margin:margin,y,half));});
  return widest;
 };
 block(o.topLeft,false,false,true);block(o.topRight,true,false,false);
 const left=block(o.bottomLeft,false,true,false),right=block(o.bottomRight,true,true,false);
 const mm=snapshot.view.mmPerPixel;
 if(mm){
  ctx.strokeStyle='white';ctx.lineWidth=Math.max(1,Math.round(font/12));ctx.shadowColor='black';ctx.shadowBlur=2;
  ruler(ctx,mm,margin+o.topRight.length*line+font,h-margin-o.bottomRight.length*line-font,w-margin-Math.round(font*0.2),true,font*0.6);
  ruler(ctx,mm,margin*2+left+font,w-margin*2-right-font,h-margin-Math.round(font*0.3),false,font*0.6);
 }
 ctx.restore();
}
/** Separate, detached composition. Never draws on the Cornerstone canvas. */
export async function generateExport(snapshot:ExportSnapshot,format:ExportFormat,identified:boolean):Promise<Blob> {
 const source=snapshot.canvas;
 const composed:HTMLCanvasElement[]=[];
 // Copy with overlays at the given scale; scale 1 keeps the viewport pixels untouched.
 const overlay=(scale:number)=>{
  const w=Math.round(source.width*scale),h=Math.round(source.height*scale);
  const dest=surface(w,h);composed.push(dest.canvas);dest.ctx.imageSmoothingQuality='high';dest.ctx.drawImage(source,0,0,w,h);
  const mm=snapshot.view.mmPerPixel;
  drawOverlay(dest.ctx,{...snapshot,view:{...snapshot.view,mmPerPixel:mm?mm/scale:null}},w,h);
  return dest.canvas;
 };
 try {
  if(format==='PDF') {
   // A4 at 144 dpi; proportional image centered on the page, overlays at page resolution.
   const landscape=source.width>source.height;const w=landscape?1684:1190,h=landscape?1190:1684;
   const {canvas,ctx}=surface(w,h);composed.push(canvas);ctx.fillStyle='white';ctx.fillRect(0,0,w,h);
   const margin=48,bottom=identified?75:margin;
   const ratio=Math.min((w-2*margin)/source.width,(h-margin-bottom)/source.height);
   const image=identified?overlay(ratio):source;
   const iw=identified?image.width:source.width*ratio,ih=identified?image.height:source.height*ratio;
   ctx.drawImage(image,(w-iw)/2,margin+(h-margin-bottom-ih)/2,iw,ih);
   if(identified){ctx.fillStyle='#111';ctx.font='18px sans-serif';ctx.fillText('Imagem derivada para documentação. Não substitui o DICOM original.',margin,h-35);}
   const png=await encode(canvas,'image/png');
   const {PDFDocument}=await import('pdf-lib');const doc=await PDFDocument.create();
   const embedded=await doc.embedPng(await png.arrayBuffer());const page=doc.addPage([w/2,h/2]);page.drawImage(embedded,{x:0,y:0,width:w/2,height:h/2});
   return new Blob([new Uint8Array(await doc.save())],{type:'application/pdf'});
  }
  return await encode(identified?overlay(1):source,format==='PNG'?'image/png':'image/jpeg',format==='JPEG'?JPEG_QUALITY:undefined);
 } finally {for(const canvas of composed){canvas.width=0;canvas.height=0;}}
}
