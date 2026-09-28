import type { ViewerSeries } from '../api/client';

export type LayoutCount = 1 | 2 | 4;
export type SeriesSlot = { series: ViewerSeries | null; manual: boolean };

/** Mantém slots visíveis, restaura escolhas manuais e só então preenche vazios.
 * A ordem é a lista do backend, sem semântica clínica ou hanging protocol.
 */
export function fillLayout(previous: SeriesSlot[], series: ViewerSeries[], count: LayoutCount, previousCount: LayoutCount): SeriesSlot[] {
  const valid = new Map(series.map((item) => [item.orthancSeriesId, item]));
  const slots = Array.from({ length: 4 }, (_, index): SeriesSlot => {
    const before = previous[index];
    const item = before?.series && valid.get(before.series.orthancSeriesId);
    return { series: item || null, manual: !!item && !!before?.manual };
  });
  const used = new Set<string>();
  // Duplicações deliberadamente escolhidas pelo usuário são preservadas.
  for (let i = 0; i < count; i++) {
    const slot = slots[i]!;
    if (slot.series && (i < previousCount || slot.manual)) used.add(slot.series.orthancSeriesId);
  }
  for (let i = previousCount; i < count; i++) {
    const slot = slots[i]!;
    if (!slot.series || slot.manual) continue;
    if (used.has(slot.series.orthancSeriesId)) slot.series = null;
    else used.add(slot.series.orthancSeriesId);
  }
  for (let i = 0; i < count; i++) {
    const slot = slots[i]!;
    if (slot.series) continue;
    slot.series = series.find((item) => !used.has(item.orthancSeriesId)) ?? null;
    if (slot.series) used.add(slot.series.orthancSeriesId);
  }
  return slots;
}
