import { useEffect, useRef, useState, type ChangeEvent } from 'react';
import { api, ApiError, type Unit, type Validity } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';
import { readXlsxPreview } from './readXlsxPreview';
import { MAX_IMPORT_ACCOUNTS, USERNAME_PATTERN, type ImportPreview } from './importPreview';

type Props = {
  units: Unit[];
  unitsLoading: boolean;
  nextUnits: number | null;
  loadMoreUnits: () => void;
  periods: [Validity, string][];
  onCreated: (count: number) => void;
  onSessionExpired: () => void;
  close: () => void;
};
/** Estado editável por linha válida da prévia, indexado pela linha de origem. */
type Entry = { include: boolean; username: string; created: boolean; message: string };

export function ImportarMedicosDialog({ units, unitsLoading, nextUnits, loadMoreUnits, periods, onCreated, onSessionExpired, close }: Props) {
  const [unitId, setUnitId] = useState('');
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [entries, setEntries] = useState<Record<number, Entry>>({});
  const [validity, setValidity] = useState<Validity>('3m');
  const [password, setPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [reading, setReading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [formError, setFormError] = useState('');
  const generation = useRef(0);
  const mutation = useRef<AbortController | null>(null);
  useEffect(() => () => { generation.current++; mutation.current?.abort(); }, []);

  async function chooseFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    const current = ++generation.current;
    setReading(true); setPreview(null); setEntries({}); setError(''); setFormError('');
    try {
      const result = await readXlsxPreview(file);
      if (generation.current !== current) return;
      setPreview(result);
      setEntries(Object.fromEntries(result.rows.filter(row => row.status === 'valid').map(row => [row.line, { include: true, username: row.username, created: false, message: '' }])));
    } catch (cause) {
      if (generation.current === current) setError(cause instanceof Error ? cause.message : 'Não foi possível ler o arquivo .xlsx.');
    } finally {
      if (generation.current === current) setReading(false);
    }
  }

  const unit = units.find(value => value.id === unitId && value.active);
  const update = (line: number, change: Partial<Entry>) => setEntries(old => ({ ...old, [line]: { ...old[line]!, ...change } }));
  const pending = preview?.rows.filter(row => entries[row.line]?.include && !entries[row.line]?.created) ?? [];
  const createdCount = Object.values(entries).filter(entry => entry.created).length;
  const tooMany = (preview?.rows.length ?? 0) > MAX_IMPORT_ACCOUNTS;

  async function create() {
    if (!preview || mutation.current) return;
    setFormError('');
    if (!unit) { setFormError('Selecione uma unidade ativa.'); return; }
    if (!pending.length) { setFormError('Nenhum médico selecionado para criação.'); return; }
    const usernames = pending.map(row => entries[row.line]!.username);
    if (usernames.some(username => !USERNAME_PATTERN.test(username))) { setFormError('Usuário: 3 a 64 caracteres, iniciando por letra ou número; somente a-z, 0-9, _ e -.'); return; }
    if (new Set(usernames).size !== usernames.length) { setFormError('Há nomes de usuário repetidos entre os selecionados.'); return; }
    if ([...password].length < 12) { setFormError('A senha inicial deve ter ao menos 12 caracteres.'); return; }
    if (password !== confirmation) { setFormError('A confirmação não confere com a senha inicial.'); return; }
    if (!window.confirm(`Criar ${pending.length} conta(s) de médico em ${unit.name}? Todas usarão a mesma senha inicial, com troca obrigatória no primeiro acesso.`)) return;
    const c = new AbortController(); mutation.current = c; setSaving(true);
    let created = 0;
    try {
      for (let i = 0; i < pending.length; i++) {
        const row = pending[i]!;
        try {
          await api.createUser('MEDICO', { name: row.name, username: entries[row.line]!.username, email: '', unitIds: [unit.id], validity, initialPassword: password }, c.signal);
          if (c.signal.aborted) return;
          created++; update(row.line, { created: true, message: 'Criado' });
        } catch (cause) {
          if (c.signal.aborted) return;
          if (cause instanceof ApiError && cause.status === 401) { onSessionExpired(); return; }
          if (cause instanceof ApiError && cause.status === 409) { update(row.line, { message: 'Usuário já cadastrado; ajuste e tente novamente.' }); continue; }
          if (cause instanceof ApiError && cause.status === 400) { update(row.line, { message: cause.message }); continue; }
          // Falha geral (perfil, indisponibilidade): interrompe sem tentar as linhas restantes.
          for (const rest of pending.slice(i)) update(rest.line, { message: 'Não processado.' });
          setFormError(cause instanceof ApiError ? cause.message : 'Não foi possível criar os usuários.');
          break;
        }
      }
    } finally {
      if (!c.signal.aborted) {
        mutation.current = null; setSaving(false); setPassword(''); setConfirmation('');
        if (created) onCreated(created);
      }
    }
  }

  const cell = { padding: 12 } as const;
  return <div role="dialog" aria-modal="true" aria-labelledby="import-medicos-title" style={{ position:'fixed', inset:0, zIndex:30, display:'grid', placeItems:'center', background:'color-mix(in srgb,var(--color-neutral-900) 50%,transparent)' }}>
    <div className="blueprint" style={{ position:'relative', width:1000, maxWidth:'96vw', maxHeight:'90vh', overflowY:'auto', padding:28, background:'var(--color-bg)', display:'flex', flexDirection:'column', gap:16 }}>
      <BlueprintCorners/>
      <h2 id="import-medicos-title" style={{ margin:0 }}>Importar médicos</h2>
      <p style={{ margin:0 }}>A planilha é lida somente neste navegador. Aceita coluna “Nome” com cabeçalho ou uma única coluna só com nomes. Ao criar, apenas nome e usuário de cada linha selecionada são enviados.</p>
      <div className="field"><label htmlFor="import-unit">Unidade existente</label><select id="import-unit" className="input" value={unitId} disabled={saving} onChange={event => setUnitId(event.target.value)}>
        <option value="">Selecione uma unidade</option>{units.filter(value => value.active).map(value => <option key={value.id} value={value.id}>{value.name}</option>)}
      </select></div>
      {unitsLoading && <span>Carregando unidades…</span>}
      {nextUnits !== null && <button type="button" className="btn btn-secondary" disabled={unitsLoading || saving} onClick={loadMoreUnits}>Carregar mais unidades</button>}
      <div className="field"><label htmlFor="import-file">Planilha Excel (.xlsx, até 2 MiB)</label><input id="import-file" className="input" type="file" accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" disabled={reading || saving} onChange={event => void chooseFile(event)}/></div>
      {reading && <p role="status">Lendo planilha localmente…</p>}
      {error && <p role="alert">{error}</p>}
      {preview && <>
        <p style={{ margin:0 }}>Aba: {preview.sheet}{preview.headerless ? ' · sem cabeçalho' : ''} · Perfil: Médico · Unidade: {unit?.name ?? '—'}</p>
        <div aria-label="Contadores da pré-visualização" style={{ display:'flex', gap:18 }}><span>Válidos: {preview.valid}</span><span>Duplicados: {preview.duplicate}</span><span>Inválidos: {preview.invalid}</span><span>Criados: {createdCount}</span></div>
        {preview.rows.length === 0 ? <p>Nenhuma linha preenchida encontrada.</p> : <div className="blueprint" style={{ maxHeight:420, overflow:'auto' }}>
          <table style={{ width:'100%', borderCollapse:'collapse', textAlign:'left', fontSize:14 }}><thead><tr>{['Incluir','Linha','Nome','Usuário','Unidade','Situação'].map(title => <th key={title} scope="col" style={{ padding:12, fontSize:11, textTransform:'uppercase', borderBottom:'1px solid var(--color-divider)' }}>{title}</th>)}</tr></thead>
          <tbody>{preview.rows.slice(0, MAX_IMPORT_ACCOUNTS).map(row => { const entry = entries[row.line]; return <tr key={row.line} style={{ borderBottom:'1px solid var(--color-divider)' }}>
            <td style={cell}>{entry && <input type="checkbox" aria-label={`Incluir linha ${row.line}`} checked={entry.include} disabled={saving || entry.created} onChange={event => update(row.line, { include: event.target.checked })}/>}</td>
            <td style={cell}>{row.line}</td><td style={cell}>{row.name}</td>
            <td style={cell}>{entry ? <input className="input" aria-label={`Usuário da linha ${row.line}`} maxLength={64} autoComplete="off" value={entry.username} disabled={saving || entry.created || !entry.include} onChange={event => update(row.line, { username: event.target.value.toLowerCase(), message: '' })}/> : '—'}</td>
            <td style={cell}>{unit?.name ?? '—'}</td>
            <td style={cell}><span className={`tag ${entry?.created ? 'tag-accent' : ''}`}>{entry?.message || row.reason}</span></td>
          </tr>; })}</tbody></table>
        </div>}
        {tooMany ? <p role="alert">A planilha tem mais de {MAX_IMPORT_ACCOUNTS} linhas preenchidas. Divida o arquivo para criar as contas.</p> : preview.valid > 0 && <>
          <div style={{ display:'flex', gap:12, flexWrap:'wrap', alignItems:'start' }}>
            <div className="field"><label htmlFor="import-validity">Validade *</label><select id="import-validity" className="input" disabled={saving} value={validity} onChange={event => setValidity(event.target.value as Validity)}>{periods.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div>
            <div className="field"><label htmlFor="import-password">Senha inicial do lote *</label><input id="import-password" type="password" className="input" minLength={12} maxLength={1024} autoComplete="new-password" disabled={saving} value={password} onChange={event => setPassword(event.target.value)}/></div>
            <div className="field"><label htmlFor="import-password-confirm">Confirmar senha *</label><input id="import-password-confirm" type="password" className="input" minLength={12} maxLength={1024} autoComplete="new-password" disabled={saving} value={confirmation} onChange={event => setConfirmation(event.target.value)}/></div>
          </div>
          <small>Mínimo de 12 caracteres. A mesma senha vale para todos os selecionados; troca obrigatória no primeiro acesso. Entregue por canal seguro.</small>
        </>}
      </>}
      {formError && <p role="alert">{formError}</p>}
      <div style={{ display:'flex', justifyContent:'end', gap:10 }}>
        <button type="button" className="btn btn-secondary" disabled={saving} onClick={close}>Fechar</button>
        {preview && !tooMany && preview.valid > 0 && <button type="button" className="btn btn-primary" disabled={saving || !pending.length} onClick={() => void create()}>{saving ? 'Criando…' : `Criar ${pending.length} médico(s)`}</button>}
      </div>
    </div>
  </div>;
}
