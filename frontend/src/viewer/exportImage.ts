import { exportFields, type ExportFormat, type ExportSnapshot } from './exportModel';
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
 ctx.fillText(rendered,x,y);
}
/** Separate, detached composition. Never draws on the Cornerstone canvas. */
export async function generateExport(snapshot:ExportSnapshot,format:ExportFormat,identified:boolean):Promise<Blob> {
 const source=snapshot.canvas;
 let composed:HTMLCanvasElement|null=null;
 try {
  if(format==='PDF') {
   // A4 at 144 dpi; separate document layout, proportional viewport image.
   const landscape=source.width>source.height;const w=landscape?1684:1190,h=landscape?1190:1684;
   const {canvas,ctx}=surface(w,h);composed=canvas;ctx.fillStyle='white';ctx.fillRect(0,0,w,h);ctx.fillStyle='#111';ctx.font='bold 30px sans-serif';
   const margin=48;let top=margin;
   if(identified){ctx.fillText('PACS Municipal',margin,top+30);ctx.font='20px sans-serif';ctx.fillText('Prefeitura Municipal de Francisco Beltrão',margin,top+60);top+=98;
    ctx.font='22px sans-serif';for(const [label,value] of exportFields(snapshot.metadata)){text(ctx,`${label}: ${value}`,margin,top,w-margin*2);top+=30;}top+=16;
   }
   const bottom=identified?75:margin;
   const ratio=Math.min((w-2*margin)/source.width,(h-top-bottom)/source.height);
   ctx.drawImage(source,(w-source.width*ratio)/2,top+(h-top-bottom-source.height*ratio)/2,source.width*ratio,source.height*ratio);
   if(identified){ctx.font='18px sans-serif';ctx.fillText('Imagem derivada para documentação. Não substitui o DICOM original.',margin,h-35);}
   const png=await encode(canvas,'image/png');
   const {PDFDocument}=await import('pdf-lib');const doc=await PDFDocument.create();
   const embedded=await doc.embedPng(await png.arrayBuffer());const page=doc.addPage([w/2,h/2]);page.drawImage(embedded,{x:0,y:0,width:w/2,height:h/2});
   return new Blob([new Uint8Array(await doc.save())],{type:'application/pdf'});
  }
  let canvas=source;
  if(identified){
   // Never downsample original viewport pixels. Minimum width accommodates labels.
   const width=Math.max(1000,source.width),font=Math.max(22,Math.round(width/55)),line=font+10,header=line*6+36;
   const dest=surface(width,source.height+header);composed=dest.canvas;canvas=dest.canvas;
   dest.ctx.fillStyle='black';dest.ctx.fillRect(0,0,canvas.width,canvas.height);dest.ctx.drawImage(source,(width-source.width)/2,header);
   dest.ctx.fillStyle='white';dest.ctx.font=`bold ${font}px sans-serif`;dest.ctx.fillText('PACS Municipal · Imagem atual',20,line);
   dest.ctx.font=`${font}px sans-serif`;
   exportFields(snapshot.metadata).forEach(([label,value],i)=>text(dest.ctx,`${label}: ${value}`,20+(i%2)*width/2,line*(Math.floor(i/2)+2.3),width/2-40));
  }
  return await encode(canvas,format==='PNG'?'image/png':'image/jpeg',format==='JPEG'?JPEG_QUALITY:undefined);
 } finally {if(composed){composed.width=0;composed.height=0;}}
}
