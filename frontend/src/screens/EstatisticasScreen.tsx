import { useEffect, useState, type ReactNode } from 'react';
import { api, ApiError, type StatsPeriod, type StatsResponse } from '../api/client';
import { BlueprintCorners } from '../design-system/Blueprint';
import { ColumnChart } from '../stats/ColumnChart';
import { bucketLabel, bucketTitle, delta, diskLabel, integer, periodLabel, shiftDate, todaySaoPaulo } from '../stats/model';

const PERIODS: [StatsPeriod, string][] = [['day', 'Dia'], ['week', 'Semana'], ['month', 'Mês']];

function Tile({ label, value, note, hero = false }: { label: string; value: string; note?: string; hero?: boolean }) {
  return <div className="blueprint" style={{ position: 'relative', padding: 20, display: 'flex', flexDirection: 'column', gap: 6, background: 'var(--color-bg)' }}>
    <BlueprintCorners/>
    <span style={{ fontSize: 13, color: 'var(--color-neutral-700)' }}>{label}</span>
    <strong style={{ fontSize: hero ? 52 : 34, lineHeight: 1, fontWeight: 600 }}>{value}</strong>
    {note && <span style={{ fontSize: 13, color: 'var(--color-neutral-700)' }}>{note}</span>}
  </div>;
}

function Section({ children }: { children: ReactNode }) {
  return <div className="blueprint" style={{ position: 'relative', padding: 24, background: 'var(--color-bg)' }}><BlueprintCorners/>{children}</div>;
}

/** Somente contagens agregadas; nenhum dado de paciente chega a esta tela. */
export function EstatisticasScreen({ onSessionExpired }: { onSessionExpired: () => void }) {
  const today = todaySaoPaulo();
  const [period, setPeriod] = useState<StatsPeriod>('day');
  const [date, setDate] = useState(today);
  const [revision, setRevision] = useState(0);
  const [data, setData] = useState<StatsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError('');
    api.getStats(period, date, controller.signal).then(value => { if (!controller.signal.aborted) setData(value); }).catch(err => {
      if (controller.signal.aborted) return;
      if (err instanceof ApiError && (err.status === 401 || err.code === 'PASSWORD_CHANGE_REQUIRED')) { onSessionExpired(); return; }
      setError(err instanceof ApiError && err.status === 403 ? 'Seu perfil não pode consultar estatísticas.' : 'Não foi possível carregar as estatísticas. Tente novamente.');
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [period, date, revision, onSessionExpired]);

  // Mantém o último resultado visível enquanto o próximo carrega, sem misturar períodos.
  const current = data && data.period === period ? data : null;
  const exams = current?.exams, usage = current?.usage;
  const canAdvance = current ? current.to < today : false;
  const columns = exams ? exams.buckets.map(b => ({ key: b.label, label: bucketLabel(period, b.label), title: bucketTitle(period, b.label), value: b.count })) : [];
  const hours = exams ? exams.byHour.map((value, h) => { const label = String(h).padStart(2, '0'); return { key: label, label: `${label}h`, title: bucketTitle('day', label), value }; }) : [];
  return <section style={{ padding: '32px 40px 48px', display: 'flex', flexDirection: 'column', gap: 20, maxWidth: 1600, minWidth: 980 }}>
    <header style={{ display: 'flex', alignItems: 'center', gap: 20, flexWrap: 'wrap' }}>
      <h1 style={{ margin: 0, fontSize: 40 }}>Estatísticas</h1>
      <span style={{ fontSize: 13, color: 'var(--color-neutral-700)' }}>Contagens agregadas · horário de Brasília</span>
      <div role="group" aria-label="Período" style={{ display: 'flex', gap: 4, marginLeft: 'auto' }}>
        {PERIODS.map(([value, label]) => <button key={value} type="button" aria-pressed={period === value} className={`btn ${period === value ? 'btn-primary' : 'btn-secondary'}`} onClick={() => setPeriod(value)}>{label}</button>)}
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <button type="button" className="btn btn-secondary" aria-label="Período anterior" onClick={() => setDate(d => shiftDate(period, d, -1, today))}>‹</button>
        <span aria-live="polite" style={{ minWidth: 190, textAlign: 'center', fontWeight: 600 }}>{current ? periodLabel(period, current.from, current.to) : '—'}</span>
        <button type="button" className="btn btn-secondary" aria-label="Próximo período" disabled={!canAdvance} onClick={() => setDate(d => shiftDate(period, d, 1, today))}>›</button>
        <button type="button" className="btn btn-secondary" disabled={date === today} onClick={() => setDate(today)}>Hoje</button>
        <button type="button" className="btn btn-secondary" disabled={loading} onClick={() => setRevision(v => v + 1)}>Atualizar</button>
      </div>
    </header>
    <div aria-busy={loading} aria-live="polite" style={{ minHeight: 18, fontSize: 13 }}>{loading ? (current ? 'Atualizando…' : 'Carregando estatísticas…') : ''}</div>
    {error && <div role="alert">{error} <button className="btn btn-secondary" onClick={() => setRevision(v => v + 1)}>Tentar novamente</button></div>}
    {current && !error && <>
      <div style={{ display: 'grid', gridTemplateColumns: '1.4fr repeat(3, 1fr)', gap: 16 }}>
        <Tile hero label="Exames realizados" value={exams ? integer(exams.total) : '—'} note={exams ? delta(exams.total, exams.previousTotal, period) : 'Indisponível'}/>
        <Tile label="Usuários ativos" value={usage ? integer(usage.activeUsers) : '—'} note={usage ? delta(usage.activeUsers, usage.previousActiveUsers, period) : 'Indisponível'}/>
        <Tile label="Logins" value={usage ? integer(usage.logins) : '—'} note={usage ? `${delta(usage.logins, usage.previousLogins, period)} · ${integer(usage.loginFailures)} falha(s)` : 'Indisponível'}/>
        <Tile label="Exportações de imagem" value={usage ? integer(usage.exports) : '—'} note={usage ? delta(usage.exports, usage.previousExports, period) : 'Indisponível'}/>
      </div>
      {!usage && <p role="alert" style={{ margin: 0 }}>Estatísticas de uso indisponíveis no momento.</p>}
      {exams ? <>
        <Section><ColumnChart unit="exames" title={period === 'day' ? 'Exames por hora' : 'Exames por dia'} subtitle="Pela data e hora do exame (StudyDate/StudyTime)" columns={columns}/></Section>
        {period !== 'day' && <Section><ColumnChart unit="exames" title="Horário de pico" subtitle="Exames por hora do dia, somando o período" columns={hours}/></Section>}
        <div style={{ fontSize: 13, color: 'var(--color-neutral-700)', display: 'flex', flexDirection: 'column', gap: 4 }}>
          {exams.withoutTime > 0 && <span>{integer(exams.withoutTime)} exame(s) sem horário válido: contados no total, fora da distribuição por hora.</span>}
          {exams.truncated && <span role="alert">O período excede o limite de leitura; os números de exames estão incompletos. Use um período menor.</span>}
        </div>
      </> : <p role="alert" style={{ margin: 0 }}>{current.examsError ?? 'Estatísticas de exames indisponíveis.'}</p>}
      <Section>
        <h2 style={{ margin: '0 0 4px', fontSize: 22 }}>Acervo do PACS</h2>
        <span style={{ fontSize: 13, color: 'var(--color-neutral-700)' }}>Totais atuais do Orthanc, independentes do período selecionado</span>
        {current.archive ? <div style={{ display: 'flex', gap: 48, marginTop: 16 }}>
          {[['Exames', integer(current.archive.studies)], ['Séries', integer(current.archive.series)], ['Imagens', integer(current.archive.instances)], ['Espaço em disco', diskLabel(current.archive.diskMB)]].map(([label, value]) =>
            <div key={label} style={{ display: 'flex', flexDirection: 'column', gap: 4 }}><span style={{ fontSize: 13, color: 'var(--color-neutral-700)' }}>{label}</span><strong style={{ fontSize: 26, fontWeight: 600 }}>{value}</strong></div>)}
        </div> : <p style={{ marginBottom: 0 }}>Indisponível no momento.</p>}
      </Section>
    </>}
  </section>;
}
