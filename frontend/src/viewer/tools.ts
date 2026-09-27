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
  const groupId = `${engineId}-tools`;
  const group = ToolGroupManager.createToolGroup(groupId);
  if (!group) throw new Error('Ferramentas indisponíveis.');
  for (const tool of toolClasses) group.addTool(tool.toolName, tool === ZoomTool ? { minZoomScale: 0.1, maxZoomScale: 20, zoomToCenter: true } : {});
  group.addViewport(viewport.id, engineId);
  let selected: ViewerTool = 'WindowLevel';
  let suspended = true;
  const owned = new Set<string>();
  const added = (event: Event) => {
    const detail = (event as CustomEvent).detail;
    if (detail.renderingEngineId === engineId && detail.viewportId === viewport.id && detail.annotation?.annotationUID) owned.add(detail.annotation.annotationUID);
  };
  eventTarget.addEventListener(Enums.Events.ANNOTATION_ADDED, added);
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
    interacting: () => state.isInteractingWithTool || state.isMultiPartToolActive,
    suspend(value: boolean) {
      if (value) cancelActiveManipulations(element);
      suspended = value; apply();
    },
    dispose() {
      cancelActiveManipulations(element);
      eventTarget.removeEventListener(Enums.Events.ANNOTATION_ADDED, added);
      for (const uid of owned) annotation.state.removeAnnotation(uid);
      owned.clear();
      // A aplicação possui somente um Viewer. Remove também referências retidas
      // pelo histórico interno de gestos, sem adicionar undo/redo à interface.
      const history = utilities.HistoryMemo.DefaultHistoryMemo;
      history.size = history.size;
      group.removeViewports(engineId, viewport.id);
      ToolGroupManager.destroyToolGroup(groupId);
    },
  };
}
