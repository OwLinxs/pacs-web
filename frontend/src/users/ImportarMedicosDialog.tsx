import { useEffect, useRef, useState, type ChangeEvent } from 'react';
import type { Unit } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';
import { readXlsxPreview } from './readXlsxPreview';
import type { ImportPreview } from './importPreview';

type Props = {
  units: Unit[];
  unitsLoading: boolean;
  nextUnits: number | null;
  loadMoreUnits: () => void;
  close: () => void;
};

export function ImportarMedicosDialog({ units, unitsLoading, nextUnits, loadMoreUnits, close }: Props) {
  const [unitId, setUnitId] = useState('');
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [reading, setReading] = useState(false);
  const [error, setError] = useState('');
  const generation = useRef(0);
  useEffect(() => () => { generation.current++; }, []);

  async function chooseFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    const current = ++generation.current;
    setReading(true); setPreview(null); setError('');
    try {
      const result = await readXlsxPreview(file);
      if (generation.current === current) setPreview(result);
    } catch (cause) {
      if (generation.current === current) setError(cause instanceof Error ? cause.message : 'Não foi possível ler o arquivo .xlsx.');
    } finally {
      if (generation.current === current) setReading(false);
    }
  }

  const unit = units.find(value => value.id === unitId && value.active);
  return <div role="dialog" aria-modal="true" aria-labelledby="import-medicos-title" style={{ position:'fixed', inset:0, zIndex:30, display:'grid', placeItems:'center', background:'color-mix(in srgb,var(--color-neutral-900) 50%,transparent)' }}>
    <div className="blueprint" style={{ position:'relative', width:900, maxWidth:'96vw', maxHeight:'90vh', overflowY:'auto', padding:28, background:'var(--color-bg)', display:'flex', flexDirection:'column', gap:16 }}>
      <BlueprintCorners/>
      <h2 id="import-medicos-title" style={{ margin:0 }}>Importar médicos · pré-visualização</h2>
      <p style={{ margin:0 }}>Esta etapa apenas lê e valida a planilha neste navegador. Nenhum usuário, senha ou vínculo será criado.</p>
      <div className="field"><label htmlFor="import-unit">Unidade existente</label><select id="import-unit" className="input" value={unitId} onChange={event => setUnitId(event.target.value)}>
        <option value="">Selecione uma unidade</option>{units.filter(value => value.active).map(value => <option key={value.id} value={value.id}>{value.name}</option>)}
      </select></div>
      {unitsLoading && <span>Carregando unidades…</span>}
      {nextUnits !== null && <button type="button" className="btn btn-secondary" disabled={unitsLoading} onClick={loadMoreUnits}>Carregar mais unidades</button>}
      <div className="field"><label htmlFor="import-file">Planilha Excel (.xlsx, até 2 MiB)</label><input id="import-file" className="input" type="file" accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" disabled={reading} onChange={event => void chooseFile(event)}/></div>
      {reading && <p role="status">Lendo planilha localmente…</p>}
      {error && <p role="alert">{error}</p>}
      {preview && <>
        <p style={{ margin:0 }}>Aba: {preview.sheet} · Perfil: Médico · Unidade: {unit?.name ?? '—'}</p>
        <div aria-label="Contadores da pré-visualização" style={{ display:'flex', gap:18 }}><span>Válidos: {preview.valid}</span><span>Duplicados: {preview.duplicate}</span><span>Inválidos: {preview.invalid}</span></div>
        {preview.rows.length === 0 ? <p>Nenhuma linha preenchida encontrada.</p> : <div className="blueprint" style={{ maxHeight:420, overflow:'auto' }}>
          <table style={{ width:'100%', borderCollapse:'collapse', textAlign:'left', fontSize:14 }}><thead><tr>{['Linha','Nome','Perfil','Unidade','Situação'].map(title => <th key={title} scope="col" style={{ padding:12, fontSize:11, textTransform:'uppercase', borderBottom:'1px solid var(--color-divider)' }}>{title}</th>)}</tr></thead>
          <tbody>{preview.rows.slice(0,200).map(row => <tr key={row.line} style={{ borderBottom:'1px solid var(--color-divider)' }}><td style={{ padding:12 }}>{row.line}</td><td style={{ padding:12 }}>{row.name}</td><td style={{ padding:12 }}>Médico</td><td style={{ padding:12 }}>{unit?.name ?? '—'}</td><td style={{ padding:12 }}><span className={`tag ${row.status === 'valid' ? 'tag-accent' : ''}`}>{row.reason}</span></td></tr>)}</tbody></table>
        </div>}
        {preview.rows.length > 200 && <small>Exibindo as primeiras 200 linhas; os contadores consideram todas as linhas da aba.</small>}
      </>}
      <div style={{ display:'flex', justifyContent:'end' }}><button type="button" className="btn btn-secondary" onClick={close}>Fechar</button></div>
    </div>
  </div>;
}
