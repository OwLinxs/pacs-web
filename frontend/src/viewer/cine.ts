/** Sem interval/backlog: próximo tick somente após terminar a imagem anterior. */
export function createCine(advance: () => Promise<void>, changed: (playing: boolean) => void) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let generation = 0;
  let playing = false;
  const stop = () => {
    generation++; playing = false; clearTimeout(timer); timer = undefined; changed(false);
  };
  return {
    stop,
    play(fps: number) {
      stop(); playing = true; changed(true);
      const token = generation;
      const delay = 1000 / Math.max(1, Math.min(30, fps || 10));
      const tick = async () => {
        if (!playing || token !== generation) return;
        try { await advance(); } catch { if (token === generation) stop(); return; }
        if (playing && token === generation) timer = setTimeout(() => { void tick(); }, delay);
      };
      timer = setTimeout(() => { void tick(); }, delay);
    },
  };
}
