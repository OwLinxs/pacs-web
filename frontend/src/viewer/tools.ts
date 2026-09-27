import { eventTarget, utilities, type StackViewport } from '@cornerstonejs/core';
import {
  init, addTool, ToolGroupManager, Enums, annotation, cancelActiveManipulations, state,
  WindowLevelTool, ZoomTool, PanTool, LengthTool, AngleTool, ProbeTool, RectangleROITool,
} from '@cornerstonejs/tools';

export type ViewerTool = 'WindowLevel' | 'Zoom' | 'Pan' | 'Length' | 'Angle' | 'Probe' | 'RectangleROI';
const toolClasses = [WindowLevelTool, ZoomTool, PanTool, LengthTool, AngleTool, ProbeTool, RectangleROITool];
let initialized = false;

export function initializeViewerTools() {
  if (initialized) return;
  init();
  for (const tool of toolClasses) addTool(tool);
  initialized = true;
}

/** Um grupo por viewport; registro das classes uma vez por aplicação. */
export function createViewerTools(element: HTMLDivElement, viewport: StackViewport, engineId: string) {
  const groupId = `${engineId}-${viewport.id}-tools`;
  const group = ToolGroupManager.createToolGroup(groupId);
  if (!group) throw new Error('Ferramentas indisponíveis.');
  for (const tool of toolClasses) group.addTool(tool.toolName, tool === ZoomTool ? { minZoomScale: 0.1, maxZoomScale: 20, zoomToCenter: true } : {});
  group.addViewport(viewport.id, engineId);
  let selected: ViewerTool = 'WindowLevel';
  let suspended = true;
  const apply = () => {
    for (const tool of toolClasses) {
      if (suspended) group.setToolEnabled(tool.toolName);
      else group.setToolPassive(tool.toolName, { removeAllBindings: true });
    }
    if (!suspended) group.setToolActive(selected, { bindings: [{ mouseButton: Enums.MouseBindings.Primary }] });
  };
  apply();
  return {
    select(tool: ViewerTool) {
      if (suspended) return false;
      cancelActiveManipulations(element);
      selected = tool; apply();
      return true;
    },
    cancel: () => cancelActiveManipulations(element),
    interacting: () => state.isInteractingWithTool || state.isMultiPartToolActive,
    suspend(value: boolean) {
      if (value) cancelActiveManipulations(element);
      suspended = value; apply();
    },
    dispose() {
      cancelActiveManipulations(element);
      group.removeViewports(engineId, viewport.id);
      ToolGroupManager.destroyToolGroup(groupId);
    },
  };
}

/** Propriedade das annotations é da sessão, não do viewport que as criou. */
export function createAnnotationSession(engineId: string, render: () => void) {
  const owned = new Set<string>();
  const added = (event: Event) => {
    const detail = (event as CustomEvent).detail;
    if (detail.renderingEngineId === engineId && detail.annotation?.annotationUID) owned.add(detail.annotation.annotationUID);
  };
  eventTarget.addEventListener(Enums.Events.ANNOTATION_ADDED, added);
  const applicable = (imageIds: string[]) => [...owned].filter((uid) => {
    const item = annotation.state.getAnnotation(uid);
    return item && imageIds.includes(item.metadata?.referencedImageId ?? '');
  });
  const remove = (ids: string[]) => {
    for (const uid of ids) { annotation.state.removeAnnotation(uid); owned.delete(uid); }
    render();
  };
  return {
    clear(imageIds: string[]) {
      const ids = applicable(imageIds);
      if (ids.length && window.confirm(`Limpar ${ids.length} medição(ões) desta série? Isso também afeta outros viewports que exibam esta mesma série. Outras séries serão preservadas.`)) remove(ids);
    },
    deleteSelected(imageId: string) {
      const selected = annotation.selection.getAnnotationsSelected();
      const ids = applicable([imageId]).filter((id) => selected.includes(id));
      // Uma seleção ambígua nunca apaga várias medições por acidente.
      if (ids.length === 1) remove(ids);
    },
    dispose() {
      eventTarget.removeEventListener(Enums.Events.ANNOTATION_ADDED, added);
      for (const uid of owned) annotation.state.removeAnnotation(uid);
      owned.clear();
      const history = utilities.HistoryMemo.DefaultHistoryMemo;
      history.size = history.size;
    },
  };
}
