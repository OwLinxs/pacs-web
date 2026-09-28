import { utilities, type StackViewport } from '@cornerstonejs/core';

export type PresentationState = { inverted: boolean; rotation: number; flipHorizontal: boolean; flipVertical: boolean };
export function readPresentation(viewport: StackViewport): PresentationState {
  const camera = viewport.getCamera();
  return {
    inverted: !!viewport.getProperties().invert,
    rotation: Math.round(viewport.getRotation()) % 360,
    flipHorizontal: !!camera.flipHorizontal,
    flipVertical: !!camera.flipVertical,
  };
}

/** resetCamera em 5.11 também desfaz orientação: restaura-a explicitamente.
 * Projeção dos cantos usa APIs públicas, para encaixar também imagens não quadradas
 * após rotação de 90°. Não toca VOI/invert nem annotations.
 */
export function fitToWindow(viewport: StackViewport) {
  const { rotation, flipHorizontal, flipVertical } = readPresentation(viewport);
  viewport.resetCamera();
  viewport.setViewPresentation({ rotation, flipHorizontal, flipVertical, zoom: 1, pan: [0, 0] });
  const data = viewport.getImageData();
  if (!data?.imageData || !data.dimensions) return;
  const [width, height] = data.dimensions;
  const corners = [[-0.5, -0.5], [width - 0.5, -0.5], [-0.5, height - 0.5], [width - 0.5, height - 0.5]]
    .map(([x, y]) => viewport.worldToCanvas(utilities.transformIndexToWorld(data.imageData, [x!, y!, 0])));
  const xs = corners.map((p) => p[0]), ys = corners.map((p) => p[1]);
  const spanX = Math.max(...xs) - Math.min(...xs), spanY = Math.max(...ys) - Math.min(...ys);
  const scale = Math.min(viewport.element.clientWidth / spanX, viewport.element.clientHeight / spanY) * 0.95;
  if (Number.isFinite(scale) && scale > 0) viewport.setZoom(viewport.getZoom() * scale);
}
