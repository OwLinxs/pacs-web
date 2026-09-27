import { api, viewerDICOMPath, type ViewerSeries } from '../api/client';

export function initialSeries(series: ViewerSeries[]): ViewerSeries | null {
  return series.find((item) => item.instanceCount > 0) ?? series[0] ?? null;
}

/** O backend já ordena por InstanceNumber; conserva integralmente essa ordem. */
export async function loadSeriesStack(studyId: string, seriesId: string, signal: AbortSignal) {
  const { items } = await api.getViewerInstances(studyId, seriesId, signal);
  return items.map((item) => viewerDICOMPath(studyId, seriesId, item.orthancInstanceId));
}

export function navigationDelta(event: KeyboardEvent): number {
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || event.defaultPrevented) return 0;
  const target = event.target;
  if (target instanceof HTMLElement && (target.isContentEditable || target.closest('input, textarea, select, [role="textbox"]'))) return 0;
  if (event.key === 'ArrowDown' || event.key === 'ArrowRight') return 1;
  if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') return -1;
  return 0;
}
