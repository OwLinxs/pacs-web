import { useEffect, useRef } from 'react';
import { ENCARREGADO, PRIVACY_DRAFT, PRIVACY_SECTIONS } from './privacy';

/** Aviso de privacidade em diálogo modal nativo (foco preso e Esc fecham). */
export function PrivacyDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = dialog.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);
  return <dialog ref={dialog} aria-labelledby="privacy-title" onClose={onClose}
    style={{ maxWidth: 640, width: 'calc(100% - 32px)', maxHeight: '85vh', padding: 28, border: '1px solid var(--color-divider)', background: 'var(--color-bg)', color: 'var(--color-text)' }}>
    <h2 id="privacy-title" style={{ marginTop: 0, fontSize: 24 }}>Aviso de privacidade</h2>
    {PRIVACY_DRAFT && <p role="note" style={{ fontSize: 13, padding: '8px 12px', border: '1px dashed var(--color-neutral-700)' }}>Rascunho — texto a validar pela Prefeitura.</p>}
    {PRIVACY_SECTIONS.map(section => <section key={section.heading}>
      <h3 style={{ fontSize: 16, margin: '16px 0 6px' }}>{section.heading}</h3>
      {section.paragraphs.map(p => <p key={p} style={{ margin: '0 0 8px', fontSize: 14, lineHeight: 1.5 }}>{p}</p>)}
    </section>)}
    <p style={{ fontSize: 14 }}><strong>Encarregado de dados:</strong> {ENCARREGADO.nome} · {ENCARREGADO.contato}</p>
    <form method="dialog"><button className="btn btn-primary" autoFocus>Fechar</button></form>
  </dialog>;
}
