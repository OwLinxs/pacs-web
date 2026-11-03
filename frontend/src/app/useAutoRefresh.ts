import { useEffect, useRef, useState } from 'react';

/** Sem interação por este tempo, a atualização automática pausa. */
export const IDLE_PAUSE_MS = 5 * 60_000;
const ACTIVITY = ['pointermove', 'pointerdown', 'keydown', 'wheel', 'touchstart'] as const;

/**
 * Chama `tick` a cada `intervalMs` enquanto `enabled`, a aba está visível e houve
 * interação recente. Cada requisição renova a sessão no servidor: sem a pausa por
 * inatividade, uma estação esquecida aberta nunca expiraria. Ao retomar (interação
 * ou volta à aba), atualiza na hora.
 */
export function useAutoRefresh(tick: () => void, intervalMs: number, enabled: boolean): { paused: boolean } {
  const tickRef = useRef(tick);
  useEffect(() => { tickRef.current = tick; });
  const [paused, setPaused] = useState(false);
  useEffect(() => {
    if (!enabled) { setPaused(false); return; }
    let last = Date.now(), idle = false;
    const activity = () => {
      last = Date.now();
      if (idle) { idle = false; setPaused(false); if (!document.hidden) tickRef.current(); }
    };
    const visibility = () => { if (!document.hidden && !idle && Date.now() - last <= IDLE_PAUSE_MS) tickRef.current(); };
    ACTIVITY.forEach(event => window.addEventListener(event, activity, { passive: true }));
    document.addEventListener('visibilitychange', visibility);
    const timer = window.setInterval(() => {
      if (document.hidden) return;
      if (Date.now() - last > IDLE_PAUSE_MS) { if (!idle) { idle = true; setPaused(true); } return; }
      tickRef.current();
    }, intervalMs);
    return () => {
      window.clearInterval(timer);
      ACTIVITY.forEach(event => window.removeEventListener(event, activity));
      document.removeEventListener('visibilitychange', visibility);
    };
  }, [enabled, intervalMs]);
  return { paused };
}
