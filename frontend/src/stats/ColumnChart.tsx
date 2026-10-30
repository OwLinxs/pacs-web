import { useState } from 'react';
import { compact, integer, niceScale } from './model';

export type Column = { key: string; label: string; title: string; value: number };

const PLOT_HEIGHT = 200;
const AXIS = 'var(--color-neutral-700)';

/**
 * Colunas de uma única série (sem legenda: o título nomeia o dado). Rótulo
 * direto apenas no pico; demais valores no tooltip (hover/foco) e na tabela.
 */
export function ColumnChart({ title, subtitle, columns, unit }: { title: string; subtitle: string; columns: Column[]; unit: string }) {
  const [active, setActive] = useState<number | null>(null);
  const max = Math.max(0, ...columns.map(c => c.value));
  const scale = niceScale(max), step = scale.step;
  // Folga acima do pico para o rótulo direto não sair da área do gráfico.
  const top = max > 0 && scale.top === max ? scale.top + step : scale.top;
  const ticks = Array.from({ length: Math.round(top / step) + 1 }, (_, i) => i * step);
  const peak = max > 0 ? columns.findIndex(c => c.value === max) : -1;
  const every = Math.max(1, Math.ceil(columns.length / 12));
  const shown = active === null ? null : columns[active];
  return <figure style={{ margin: 0, display: 'flex', flexDirection: 'column', gap: 10 }}>
    <figcaption><h2 style={{ margin: 0, fontSize: 22 }}>{title}</h2><span style={{ fontSize: 13, color: AXIS }}>{subtitle}</span></figcaption>
    <div style={{ display: 'grid', gridTemplateColumns: '40px 1fr', columnGap: 8 }}>
      <div aria-hidden="true" style={{ position: 'relative', height: PLOT_HEIGHT }}>
        {ticks.map(t => <span key={t} style={{ position: 'absolute', right: 0, bottom: `${(t / top) * 100}%`, transform: 'translateY(50%)', fontSize: 11, color: AXIS }}>{compact(t)}</span>)}
      </div>
      <div style={{ position: 'relative', height: PLOT_HEIGHT }} onPointerLeave={() => setActive(null)}>
        {ticks.map(t => <div key={t} aria-hidden="true" style={{ position: 'absolute', left: 0, right: 0, bottom: `${(t / top) * 100}%`, borderTop: '1px solid var(--color-divider)' }}/>)}
        <div role="list" aria-label={title} style={{ position: 'absolute', inset: 0, display: 'flex', gap: 2, alignItems: 'flex-end' }}>
          {columns.map((c, i) => <div role="listitem" key={c.key} tabIndex={0} aria-label={`${c.title}: ${integer(c.value)} ${unit}`}
            onPointerEnter={() => setActive(i)} onFocus={() => setActive(i)} onBlur={() => setActive(null)}
            style={{ flex: 1, height: '100%', position: 'relative', display: 'flex', justifyContent: 'center', alignItems: 'flex-end', outlineOffset: -2, cursor: 'default' }}>
            {c.value > 0 && <div style={{ width: '100%', maxWidth: 24, height: `${(c.value / top) * 100}%`, background: active === i ? 'var(--color-accent-700)' : 'var(--color-accent-600)', borderRadius: '4px 4px 0 0' }}/>}
            {i === peak && <span aria-hidden="true" style={{ position: 'absolute', bottom: `calc(${(c.value / top) * 100}% + 4px)`, fontSize: 12, fontWeight: 600, color: 'var(--color-text)' }}>{integer(c.value)}</span>}
          </div>)}
        </div>
        {shown && active !== null && <div role="status" style={{ position: 'absolute', top: -6, left: `${((active + 0.5) / columns.length) * 100}%`, transform: `translate(${active > columns.length / 2 ? '-100%' : '0'}, -100%)`, background: 'var(--color-neutral-900)', color: 'white', padding: '6px 10px', fontSize: 12, whiteSpace: 'nowrap', pointerEvents: 'none', zIndex: 2 }}>
          {shown.title} · <strong>{integer(shown.value)}</strong> {unit}
        </div>}
      </div>
      <div/>
      <div aria-hidden="true" style={{ display: 'flex', gap: 2 }}>
        {columns.map((c, i) => <span key={c.key} style={{ flex: 1, textAlign: 'center', fontSize: 11, color: AXIS, whiteSpace: 'nowrap', overflow: 'visible' }}>{i % every === 0 ? c.label : ''}</span>)}
      </div>
    </div>
    <details><summary style={{ cursor: 'pointer', fontSize: 13 }}>Ver tabela</summary>
      <table style={{ borderCollapse: 'collapse', fontSize: 13, marginTop: 8 }}><thead><tr><th scope="col" style={{ textAlign: 'left', padding: '4px 16px 4px 0' }}>Período</th><th scope="col" style={{ textAlign: 'right', padding: 4 }}>{unit[0]!.toUpperCase() + unit.slice(1)}</th></tr></thead>
        <tbody>{columns.map(c => <tr key={c.key} style={{ borderTop: '1px solid var(--color-divider)' }}><td style={{ padding: '4px 16px 4px 0' }}>{c.title}</td><td style={{ textAlign: 'right', padding: 4 }}>{integer(c.value)}</td></tr>)}</tbody></table>
    </details>
  </figure>;
}
