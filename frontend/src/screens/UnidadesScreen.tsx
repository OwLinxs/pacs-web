import { useEffect, useRef, useState, type FormEvent } from 'react';
import { api, ApiError, type Unit, type UnitsPage } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';
import { Icon } from '../design-system/Icon';

/** Administração V1: entidades próprias, sem associação/alteração de usuários. */
export function UnidadesScreen({ onSessionExpired }: { onSessionExpired: () => void }) {
  const [page, setPage] = useState<UnitsPage | null>(null);
  const [offset, setOffset] = useState(0);
  const [revision, setRevision] = useState(0);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [formError, setFormError] = useState('');
  const [message, setMessage] = useState('');
  const [form, setForm] = useState<{ id: string | null; name: string } | null>(null);
  const pending = useRef<AbortController | null>(null);

  useEffect(() => () => { pending.current?.abort(); }, []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError('');
    void api.getUnits(true, offset, controller.signal).then(result => {
      if (controller.signal.aborted) return;
      setPage(result); setLoading(false);
    }).catch((cause: unknown) => {
      if (controller.signal.aborted) return;
      if (cause instanceof ApiError && cause.status === 401) { onSessionExpired(); return; }
      setError(cause instanceof ApiError ? cause.message : 'Não foi possível carregar as unidades.');
      setLoading(false); setPage(null);
    });
    return () => controller.abort();
  }, [offset, revision, onSessionExpired]);

  async function mutate(operation: (signal: AbortSignal) => Promise<Unit>, success: string) {
    if (pending.current) return;
    const controller = new AbortController(); pending.current = controller;
    setSaving(true); setFormError(''); setMessage('');
    try {
      await operation(controller.signal);
      if (controller.signal.aborted) return;
      setForm(null); setMessage(success); setOffset(0); setRevision(value => value + 1);
    } catch (cause) {
      if (controller.signal.aborted) return;
      if (cause instanceof ApiError && cause.status === 401) { onSessionExpired(); return; }
      setFormError(cause instanceof ApiError ? cause.message : 'Não foi possível salvar a unidade.');
    } finally {
      if (!controller.signal.aborted) { pending.current = null; setSaving(false); }
    }
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    if (!form) return;
    const name = form.name.trim();
    if (!name || [...name].length > 120 || /[\u0000-\u001f\u007f-\u009f]/.test(form.name)) { setFormError('Informe um nome de 1 a 120 caracteres, sem caracteres de controle.'); return; }
    const id = form.id;
    void mutate(signal => id ? api.updateUnit(id, { name }, signal) : api.createUnit(name, signal), id ? 'Unidade atualizada.' : 'Unidade criada.');
  }
  function toggle(unit: Unit) {
    if (!unit.active || window.confirm('Desativar esta unidade? Ela permanecerá cadastrada e poderá ser reativada.')) {
      void mutate(signal => api.updateUnit(unit.id, { active: !unit.active }, signal), unit.active ? 'Unidade desativada.' : 'Unidade reativada.');
    }
  }
  const busy = saving || loading;
  return (
    <div style={{ padding: '32px 40px 48px', maxWidth: 1200, display: 'flex', flexDirection: 'column', gap: 20 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <h1 style={{ margin: 0, fontSize: 40 }}>Unidades</h1>
        <button className="btn btn-primary" disabled={busy || form !== null} onClick={() => { setForm({ id: null, name: '' }); setFormError(''); setMessage(''); }}><Icon name="plus" /> Nova unidade</button>
      </div>
      <p style={{ margin: 0, color: 'var(--color-neutral-700)', fontSize: 14 }}>Cadastro de unidades de saúde. Unidades inativas permanecem no cadastro.</p>
      {message && <p role="status" style={{ margin: 0 }}>{message}</p>}
      {formError && <p role="alert" style={{ margin: 0 }}>{formError}</p>}
      {form && <form onSubmit={submit} className="blueprint" style={{ position: 'relative', padding: 24, display: 'flex', alignItems: 'end', gap: 16 }}>
        <BlueprintCorners />
        <div className="field" style={{ flex: 1 }}>
          <label htmlFor="unit-name">{form.id ? 'Editar nome da unidade' : 'Nome da unidade'}</label>
          <input id="unit-name" className="input" value={form.name} maxLength={120} required autoFocus disabled={saving}
            onChange={event => setForm({ ...form, name: event.target.value })} />
        </div>
        <button className="btn btn-primary" type="submit" disabled={saving}>{saving ? 'Salvando…' : 'Salvar unidade'}</button>
        <button className="btn btn-secondary" type="button" disabled={saving} onClick={() => { setForm(null); setFormError(''); }}>Cancelar</button>
      </form>}
      <div className="blueprint" aria-busy={loading} style={{ position: 'relative' }}>
        <BlueprintCorners />
        <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: 14 }}>
          <thead><tr style={{ borderBottom: '1px solid var(--color-divider)', fontSize: 11, textTransform: 'uppercase', letterSpacing: '.06em' }}>
            <th scope="col" style={{ padding: 16 }}>Nome</th><th scope="col" style={{ padding: 16, width: 100 }}>Status</th><th scope="col" style={{ padding: 16, width: 220 }}>Ações</th>
          </tr></thead>
          <tbody>{!loading && page?.items.map(unit => <tr key={unit.id} style={{ borderBottom: '1px solid var(--color-divider)' }}>
            <td style={{ padding: 16, overflowWrap: 'anywhere' }}>{unit.name}</td>
            <td style={{ padding: 16 }}><span className={`tag ${unit.active ? 'tag-accent' : ''}`}>{unit.active ? 'Ativa' : 'Inativa'}</span></td>
            <td style={{ padding: 16 }}><div style={{ display: 'flex', gap: 8 }}>
              <button className="btn btn-secondary" disabled={busy || form !== null} onClick={() => { setForm({ id: unit.id, name: unit.name }); setFormError(''); setMessage(''); }}>Editar</button>
              <button className="btn btn-secondary" disabled={busy || form !== null} onClick={() => toggle(unit)}>{unit.active ? 'Desativar' : 'Reativar'}</button>
            </div></td>
          </tr>)}</tbody>
        </table>
        {loading && <p role="status" style={{ padding: 24 }}>Carregando unidades…</p>}
        {!loading && !error && page?.items.length === 0 && <p style={{ padding: 24 }}>Nenhuma unidade cadastrada.</p>}
        {error && <div role="alert" style={{ padding: 24 }}><p>{error}</p><button className="btn btn-secondary" onClick={() => setRevision(value => value + 1)}>Tentar novamente</button></div>}
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, fontSize: 14 }}>
        <button className="btn btn-secondary" disabled={busy || form !== null || offset === 0} onClick={() => setOffset(value => Math.max(0, value - 50))}>Anterior</button>
        <span>Página {Math.floor(offset / 50) + 1} · até 50 unidades</span>
        <button className="btn btn-secondary" disabled={busy || form !== null || page?.nextOffset == null} onClick={() => { if (page?.nextOffset != null) setOffset(page.nextOffset); }}>Próxima</button>
        {page?.hasMore && page.nextOffset === null && <span>Limite de navegação atingido.</span>}
      </div>
    </div>
  );
}
