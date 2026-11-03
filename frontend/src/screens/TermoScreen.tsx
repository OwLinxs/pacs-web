import { useEffect, useRef, useState } from 'react';
import { api, ApiError, type TermsDocument } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';
import { PrivacyDialog } from '../legal/PrivacyDialog';

/** Termo de responsabilidade e sigilo: obrigatório no primeiro acesso e a cada nova versão. */
export function TermoScreen({ onAccepted, onLogout, onSessionExpired }: { onAccepted: () => Promise<void>; onLogout: () => void; onSessionExpired: () => void }) {
  const [doc, setDoc] = useState<TermsDocument | null>(null);
  const [error, setError] = useState('');
  const [read, setRead] = useState(false);
  const [busy, setBusy] = useState(false);
  const [privacy, setPrivacy] = useState(false);
  const [revision, setRevision] = useState(0);
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  useEffect(() => {
    const controller = new AbortController();
    setError('');
    api.getTerms(controller.signal).then(value => { if (!controller.signal.aborted) setDoc(value); }).catch(cause => {
      if (controller.signal.aborted) return;
      if (cause instanceof ApiError && cause.status === 401) { onSessionExpired(); return; }
      setError('Não foi possível carregar o termo. Tente novamente.');
    });
    return () => controller.abort();
  }, [revision, onSessionExpired]);

  async function accept() {
    if (!doc || pending.current) return;
    const controller = new AbortController();
    pending.current = controller; setBusy(true); setError('');
    try {
      await api.acceptTerms(doc.version, controller.signal);
      if (!controller.signal.aborted) await onAccepted();
    } catch (cause) {
      if (controller.signal.aborted) return;
      if (cause instanceof ApiError && cause.status === 401) { onSessionExpired(); return; }
      if (cause instanceof ApiError && cause.status === 409) { setDoc(null); setRead(false); setRevision(v => v + 1); }
      setError(cause instanceof ApiError ? cause.message : 'Não foi possível registrar o aceite. Tente novamente.');
    } finally {
      if (!controller.signal.aborted) { pending.current = null; setBusy(false); }
    }
  }

  return <main className="pacs-grid-ground" style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: '32px 16px' }}>
    <section className="blueprint" aria-labelledby="terms-title" style={{ position: 'relative', width: 'min(720px, 100%)', padding: 32, background: 'var(--color-bg)', display: 'flex', flexDirection: 'column', gap: 16 }}>
      <BlueprintCorners/>
      <h1 id="terms-title" style={{ fontSize: 28, margin: 0 }}>{doc?.title ?? 'Termo de responsabilidade e sigilo'}</h1>
      <p style={{ margin: 0 }}>Antes de acessar exames, leia e aceite o termo abaixo. O aceite é registrado com data, hora e versão.</p>
      {doc?.draft && <p role="note" style={{ margin: 0, fontSize: 13, padding: '8px 12px', border: '1px dashed var(--color-neutral-700)' }}>Rascunho — texto a validar pela Procuradoria do Município. Versão {doc.version}.</p>}
      <div tabIndex={0} aria-label="Texto do termo" style={{ maxHeight: '48vh', overflowY: 'auto', padding: '4px 16px 4px 0', borderTop: '1px solid var(--color-divider)', borderBottom: '1px solid var(--color-divider)' }}>
        {!doc && !error && <p>Carregando termo…</p>}
        {doc?.sections.map(section => <section key={section.heading}>
          <h2 style={{ fontSize: 17, margin: '16px 0 6px' }}>{section.heading}</h2>
          {section.paragraphs.length > 1 && section.heading === 'Compromissos'
            ? <ul style={{ margin: 0, paddingLeft: 20 }}>{section.paragraphs.map(p => <li key={p} style={{ marginBottom: 6, lineHeight: 1.5 }}>{p}</li>)}</ul>
            : section.paragraphs.map(p => <p key={p} style={{ margin: '0 0 8px', lineHeight: 1.5 }}>{p}</p>)}
        </section>)}
      </div>
      <button type="button" className="btn btn-ghost" style={{ alignSelf: 'flex-start', padding: 0 }} onClick={() => setPrivacy(true)}>Ler o aviso de privacidade</button>
      <label style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
        <input type="checkbox" checked={read} disabled={!doc || busy} onChange={e => setRead(e.target.checked)}/>
        <span>Li o termo e compreendi minhas responsabilidades.</span>
      </label>
      {error && <p role="alert" style={{ margin: 0 }}>{error} {!doc && <button type="button" className="btn btn-secondary" onClick={() => setRevision(v => v + 1)}>Tentar novamente</button>}</p>}
      <div style={{ display: 'flex', gap: 12 }}>
        <button type="button" className="btn btn-primary" disabled={!doc || !read || busy} onClick={() => void accept()}>{busy ? 'Registrando…' : 'Li e concordo'}</button>
        <button type="button" className="btn btn-secondary" disabled={busy} onClick={onLogout}>Sair</button>
      </div>
    </section>
    <PrivacyDialog open={privacy} onClose={() => setPrivacy(false)}/>
  </main>;
}
