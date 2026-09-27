import { api, viewerDICOMPath, type ViewerSeries, type ViewerInstance } from '../api/client';

/** Somente a primeira série não vazia; nenhuma busca ou prefetch de pixels. */
export async function selectInitialImage(studyId: string, signal: AbortSignal) {
  const { items: series } = await api.getViewerSeries(studyId, signal);
  for (const item of series) {
    if (item.instanceCount < 1) continue;
    const { items } = await api.getViewerInstances(studyId, item.orthancSeriesId, signal);
    const instance = items[0];
    if (!instance) continue; // série ficou vazia entre as duas consultas
    return { series, selected: item, instance, path: viewerDICOMPath(studyId, item.orthancSeriesId, instance.orthancInstanceId) };
  }
  return { series, selected: null as ViewerSeries | null, instance: null as ViewerInstance | null, path: null };
}
