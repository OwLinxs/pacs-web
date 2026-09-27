import { useEffect, useRef, useState } from 'react';
import { iniciaisDe, useSession } from '../auth/SessionProvider';
import { Icon, type IconName } from '../design-system/Icon';
import { serieAt, type Exame } from '../mocks/dados';

/** Estados do visualizador mostrados no design (3d–3e). */
export type ViewerState = 'ok' | 'loading' | 'error';

/** Ferramentas de mouse — exclusivas: uma ativa por vez. */
type ToolKey = 'wl' | 'zoom' | 'pan' | 'scroll' | 'measure';

const TOOLS: { key: ToolKey; icon: IconName; label: string; kb: string }[] = [
  { key: 'wl', icon: 'sun', label: 'Janela / Nível', kb: 'W' },
  { key: 'zoom', icon: 'zoom', label: 'Zoom', kb: 'Z' },
  { key: 'pan', icon: 'move', label: 'Mover', kb: 'P' },
  { key: 'scroll', icon: 'layers', label: 'Rolar imagens', kb: 'S' },
  { key: 'measure', icon: 'ruler', label: 'Medição', kb: 'M' },
];

/** Ações — executam na hora; só "Inverter" mantém estado. */
const ACTIONS: { key: 'invert' | 'rotate' | 'reset' | 'fit'; icon: IconName; label: string; kb: string }[] = [
  { key: 'invert', icon: 'contrast', label: 'Inverter', kb: 'I' },
  { key: 'rotate', icon: 'rotate', label: 'Girar 90°', kb: 'R' },
  { key: 'reset', icon: 'reset', label: 'Restaurar', kb: 'Esc' },
  { key: 'fit', icon: 'fit', label: 'Ajustar à tela', kb: 'F' },
];

type LayoutKey = '1x1' | '1x2' | '2x2';

const LAYOUTS: { key: LayoutKey; icon: IconName }[] = [
  { key: '1x1', icon: 'g1' },
  { key: '1x2', icon: 'g12' },
  { key: '2x2', icon: 'g22' },
];

const VIEWPORTS_POR_LAYOUT: Record<LayoutKey, number> = { '1x1': 1, '1x2': 2, '2x2': 4 };

const CURSOR: Record<ToolKey, string> = {
  wl: 'ns-resize',
  zoom: 'zoom-in',
  pan: 'grab',
  scroll: 'row-resize',
  measure: 'crosshair',
};

/**
 * Radiografia fictícia desenhada em CSS — placeholder da área que o
 * Cornerstone3D vai renderizar (Fase 5). Não é imagem médica real.
 */
const RADIOGRAFIA_BASE = [
  'radial-gradient(ellipse 20% 33% at 32% 46%, rgba(24,24,24,.8), transparent 72%)',
  'radial-gradient(ellipse 20% 33% at 68% 46%, rgba(24,24,24,.8), transparent 72%)',
  'radial-gradient(ellipse 30% 6% at 34% 22%, rgba(200,200,200,.28), transparent 70%)',
  'radial-gradient(ellipse 30% 6% at 66% 22%, rgba(200,200,200,.28), transparent 70%)',
  'radial-gradient(ellipse 15% 20% at 57% 62%, rgba(210,210,210,.5), transparent 70%)',
  'linear-gradient(90deg, transparent 46%, rgba(200,200,200,.22) 49%, rgba(215,215,215,.3) 50%, rgba(200,200,200,.22) 51%, transparent 54%)',
  'radial-gradient(ellipse 60% 20% at 50% 94%, rgba(210,210,210,.6), transparent 70%)',
  'radial-gradient(ellipse 54% 60% at 50% 50%, #6a6a6a, #444 55%, #1c1c1c 80%, #000 100%)',
].join(',');

const RADIOGRAFIA_MINIATURA = [
  'radial-gradient(ellipse 22% 34% at 34% 44%, rgba(18,18,18,.9), transparent 70%)',
  'radial-gradient(ellipse 22% 34% at 66% 44%, rgba(18,18,18,.9), transparent 70%)',
  'radial-gradient(ellipse 15% 20% at 57% 62%, rgba(210,210,210,.5), transparent 70%)',
  'linear-gradient(90deg, transparent 46%, rgba(200,200,200,.22) 49%, rgba(215,215,215,.3) 50%, rgba(200,200,200,.22) 51%, transparent 54%)',
  'radial-gradient(ellipse 60% 20% at 50% 94%, rgba(210,210,210,.6), transparent 70%)',
  'radial-gradient(ellipse 54% 60% at 50% 50%, #6a6a6a, #444 55%, #1c1c1c 80%, #000 100%)',
].join(',');

const FUNDO_ESCURO = 'color-mix(in srgb, var(--color-neutral-900) 40%, black)';
const FUNDO_HEADER = 'color-mix(in srgb, var(--color-neutral-900) 80%, black)';
const FUNDO_TOOLBAR = 'color-mix(in srgb, var(--color-neutral-900) 70%, black)';
const FUNDO_PAINEL = 'color-mix(in srgb, var(--color-neutral-900) 60%, black)';
const LINHA_CLARA = 'color-mix(in srgb, white 8%, transparent)';
const LINHA_CLARA_12 = 'color-mix(in srgb, white 12%, transparent)';

type Arraste = {
  x: number;
  y: number;
  wb: number;
  wc: number;
  px: number;
  py: number;
  zoom: number;
  passo: number;
  ultimo: number;
};

type ViewerScreenProps = {
  exame: Exame;
  viewerState: ViewerState;
  onVoltar: () => void;
  onLogout: () => void;
};

/** Tela 1c do design — "Visualizador". Principal tela clínica. */
export function ViewerScreen({ exame, viewerState, onVoltar, onLogout }: ViewerScreenProps) {
  const [tool, setTool] = useState<ToolKey>('wl');
  const [invert, setInvert] = useState(false);
  const [rot, setRot] = useState(0);
  const [zoom, setZoom] = useState(1);
  const [px, setPx] = useState(0);
  const [py, setPy] = useState(0);
  const [wb, setWb] = useState(1);
  const [wc, setWc] = useState(1);
  const [serieIndex, setSerieIndex] = useState(0);
  const [img, setImg] = useState(0);
  const [layout, setLayout] = useState<LayoutKey>('1x1');
  const [full, setFull] = useState(false);
  const [painel, setPainel] = useState(true);
  const [tip, setTip] = useState<string | null>(null);
  const [maisAberto, setMaisAberto] = useState(false);

  const { user } = useSession();
  const nomeDoMedico = user?.name ?? '—';

  const arraste = useRef<Arraste | null>(null);
  const serie = serieAt(exame, Math.min(serieIndex, exame.series.length - 1));

  function passoImagem(delta: number) {
    setImg((atual) => Math.max(0, Math.min(serie.count - 1, atual + delta)));
  }

  useEffect(() => {
    function onMove(event: MouseEvent) {
      const d = arraste.current;
      if (!d) return;
      const dx = event.clientX - d.x;
      const dy = event.clientY - d.y;
      if (tool === 'wl') {
        setWb(Math.max(0.3, Math.min(2.2, d.wb - dy / 300)));
        setWc(Math.max(0.3, Math.min(3, d.wc + dx / 300)));
      } else if (tool === 'pan') {
        setPx(d.px + dx);
        setPy(d.py + dy);
      } else if (tool === 'zoom') {
        setZoom(Math.max(0.3, Math.min(6, d.zoom * (1 - dy / 250))));
      } else if (tool === 'scroll') {
        const passo = Math.round(dy / 40);
        if (passo !== d.passo) {
          d.passo = passo;
          passoImagem(passo > d.ultimo ? 1 : -1);
          d.ultimo = passo;
        }
      }
    }
    function onUp() {
      arraste.current = null;
    }
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
  });

  function restaurarVista() {
    setZoom(1);
    setPx(0);
    setPy(0);
    setWb(1);
    setWc(1);
    setRot(0);
    setInvert(false);
  }

  function executarAcao(key: (typeof ACTIONS)[number]['key']) {
    if (key === 'invert') setInvert((valor) => !valor);
    if (key === 'rotate') setRot((valor) => valor + 90);
    if (key === 'reset') restaurarVista();
    if (key === 'fit') {
      setZoom(1);
      setPx(0);
      setPy(0);
    }
  }

  const nViewports = VIEWPORTS_POR_LAYOUT[layout];
  const transform = `translate(${px}px, ${py}px) scale(${zoom}) rotate(${rot}deg)`;
  const filtro = `brightness(${wb.toFixed(2)}) contrast(${wc.toFixed(2)})${invert ? ' invert(1)' : ''}`;
  const wlText = `W ${Math.round(wc * 2048)}  L ${Math.round(wb * 1024)}`;

  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        display: 'flex',
        flexDirection: 'column',
        background: FUNDO_ESCURO,
        color: 'var(--color-neutral-300)',
        userSelect: 'none',
      }}
    >
      {!full && (
        <header
          style={{
            height: 46,
            flex: 'none',
            display: 'flex',
            alignItems: 'center',
            gap: 16,
            padding: '0 12px 0 8px',
            borderBottom: `1px solid ${LINHA_CLARA}`,
            background: FUNDO_HEADER,
          }}
        >
          <button
            type="button"
            className="pacs-hover-white"
            onClick={onVoltar}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              height: 32,
              padding: '0 12px 0 8px',
              border: 0,
              background: 'transparent',
              color: 'var(--color-neutral-200)',
              font: 'inherit',
              fontSize: 14,
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            <Icon name="arrowLeft" />
            Exames
          </button>
          <div style={{ width: 1, height: 20, background: LINHA_CLARA_12 }} />
          <div
            style={{
              display: 'flex',
              alignItems: 'baseline',
              gap: 14,
              fontSize: 13.5,
              minWidth: 0,
              whiteSpace: 'nowrap',
              overflow: 'hidden',
            }}
          >
            <span style={{ fontWeight: 600, color: 'var(--color-neutral-100)' }}>{exame.name}</span>
            <span style={{ color: 'var(--color-neutral-500)' }}>{exame.sexAge}</span>
            <span
              style={{
                fontFamily: 'ui-monospace,Menlo,monospace',
                fontSize: 12.5,
                color: 'var(--color-neutral-400)',
              }}
            >
              {exame.pront}
            </span>
            <span style={{ color: 'var(--color-neutral-300)' }}>{exame.exam}</span>
            <span style={{ color: 'var(--color-neutral-500)' }}>
              {exame.dayLabel}, {exame.time} · {exame.unit}
            </span>
          </div>
          <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 10, flex: 'none' }}>
            <div
              style={{
                width: 26,
                height: 26,
                display: 'grid',
                placeItems: 'center',
                background: 'var(--color-accent-800)',
                color: 'var(--color-accent-100)',
                fontFamily: 'var(--font-heading)',
                fontWeight: 600,
                fontSize: 12,
              }}
            >
              {iniciaisDe(nomeDoMedico)}
            </div>
            <span style={{ fontSize: 13.5, color: 'var(--color-neutral-200)', fontWeight: 500 }}>
              {nomeDoMedico}
            </span>
            <button
              type="button"
              className="pacs-hover-white"
              onClick={onLogout}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                height: 30,
                padding: '0 12px',
                marginLeft: 6,
                border: '1px solid color-mix(in srgb, white 14%, transparent)',
                background: 'transparent',
                color: 'var(--color-neutral-200)',
                font: 'inherit',
                fontSize: 13,
                cursor: 'pointer',
              }}
            >
              <Icon name="logout" />
              Sair
            </button>
          </div>
        </header>
      )}

      <div
        style={{
          height: 50,
          flex: 'none',
          display: 'flex',
          alignItems: 'center',
          gap: 4,
          padding: '0 10px',
          borderBottom: `1px solid ${LINHA_CLARA}`,
          background: FUNDO_TOOLBAR,
          position: 'relative',
          zIndex: 6,
        }}
      >
        <button
          type="button"
          className="pacs-hover-white"
          onClick={() => setPainel((aberto) => !aberto)}
          title="Séries"
          aria-label="Séries"
          style={{
            width: 38,
            height: 38,
            display: 'grid',
            placeItems: 'center',
            border: 0,
            background: 'transparent',
            color: painel ? 'var(--color-accent-300)' : 'var(--color-neutral-300)',
            cursor: 'pointer',
          }}
        >
          <Icon name="panel" size={20} />
        </button>

        <div style={{ flex: 1 }} />

        <div style={{ display: 'flex', gap: 2 }}>
          {TOOLS.map((item) => {
            const ativo = tool === item.key;
            return (
              <button
                key={item.key}
                type="button"
                className="pacs-hover-white"
                onClick={() => setTool(item.key)}
                onMouseEnter={() => setTip(`${item.label} · ${item.kb}`)}
                onMouseLeave={() => setTip(null)}
                aria-label={item.label}
                aria-pressed={ativo}
                style={{
                  width: 40,
                  height: 38,
                  display: 'grid',
                  placeItems: 'center',
                  border: `1px solid ${ativo ? 'var(--color-accent-500)' : 'transparent'}`,
                  background: ativo
                    ? 'color-mix(in srgb, var(--color-accent-400) 22%, transparent)'
                    : 'transparent',
                  color: ativo ? 'var(--color-accent-300)' : 'var(--color-neutral-300)',
                  cursor: 'pointer',
                }}
              >
                <Icon name={item.icon} size={20} />
              </button>
            );
          })}
        </div>

        <div style={{ width: 1, height: 22, background: LINHA_CLARA_12, margin: '0 8px' }} />

        <div style={{ display: 'flex', gap: 2 }}>
          {ACTIONS.map((item) => {
            const ligada = item.key === 'invert' && invert;
            return (
              <button
                key={item.key}
                type="button"
                className="pacs-hover-white"
                onClick={() => executarAcao(item.key)}
                onMouseEnter={() => setTip(`${item.label} · ${item.kb}`)}
                onMouseLeave={() => setTip(null)}
                aria-label={item.label}
                style={{
                  width: 40,
                  height: 38,
                  display: 'grid',
                  placeItems: 'center',
                  border: 0,
                  background: ligada
                    ? 'color-mix(in srgb, var(--color-accent-400) 22%, transparent)'
                    : 'transparent',
                  color: ligada ? 'var(--color-accent-300)' : 'var(--color-neutral-300)',
                  cursor: 'pointer',
                }}
              >
                <Icon name={item.icon} size={20} />
              </button>
            );
          })}
        </div>

        <div style={{ width: 1, height: 22, background: LINHA_CLARA_12, margin: '0 8px' }} />

        <div style={{ position: 'relative' }}>
          <button
            type="button"
            className="pacs-hover-white"
            onClick={() => setMaisAberto((aberto) => !aberto)}
            aria-expanded={maisAberto}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              height: 38,
              padding: '0 10px',
              border: 0,
              background: 'transparent',
              color: 'var(--color-neutral-300)',
              font: 'inherit',
              fontSize: 13,
              cursor: 'pointer',
            }}
          >
            <Icon name="more" size={20} />
            Mais
          </button>
          {maisAberto && (
            <div
              style={{
                position: 'absolute',
                left: '50%',
                transform: 'translateX(-50%)',
                top: 44,
                width: 230,
                padding: 6,
                background: 'color-mix(in srgb, var(--color-neutral-900) 85%, black)',
                border: `1px solid ${LINHA_CLARA_12}`,
                boxShadow: '0 12px 32px rgba(0,0,0,.5)',
                display: 'flex',
                flexDirection: 'column',
                fontSize: 13.5,
              }}
            >
              {['Espelhar horizontal', 'Espelhar vertical', 'Medir ângulo', 'Lupa', 'Limpar medições'].map(
                (label) => (
                  <ItemMais key={label} label={label} />
                ),
              )}
              <div style={{ height: 1, background: 'color-mix(in srgb, white 10%, transparent)', margin: '6px 0' }} />
              <ItemMais label="Informações da imagem" />
              <ItemMais label="Atalhos de teclado" />
            </div>
          )}
        </div>

        <div style={{ flex: 1 }} />

        <div style={{ display: 'flex', border: '1px solid color-mix(in srgb, white 10%, transparent)', marginRight: 6 }}>
          {LAYOUTS.map((item) => {
            const ativo = layout === item.key;
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => setLayout(item.key)}
                title={`Layout ${item.key}`}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  height: 32,
                  padding: '0 10px',
                  border: 0,
                  background: ativo
                    ? 'color-mix(in srgb, var(--color-accent-400) 22%, transparent)'
                    : 'transparent',
                  color: ativo ? 'var(--color-accent-300)' : 'var(--color-neutral-400)',
                  font: 'inherit',
                  fontSize: 12,
                  fontVariantNumeric: 'tabular-nums',
                  cursor: 'pointer',
                }}
              >
                <Icon name={item.icon} />
                {item.key}
              </button>
            );
          })}
        </div>

        <button
          type="button"
          className="pacs-hover-white"
          onClick={() => setFull((valor) => !valor)}
          title="Tela cheia · F11"
          aria-label="Tela cheia"
          style={{
            width: 40,
            height: 38,
            display: 'grid',
            placeItems: 'center',
            border: 0,
            background: 'transparent',
            color: 'var(--color-neutral-300)',
            cursor: 'pointer',
          }}
        >
          <Icon name="full" size={20} />
        </button>

        {tip && (
          <div
            style={{
              position: 'absolute',
              left: '50%',
              top: 54,
              transform: 'translateX(-50%)',
              padding: '5px 10px',
              background: 'var(--color-neutral-100)',
              color: 'var(--color-neutral-900)',
              fontSize: 12,
              fontWeight: 500,
              whiteSpace: 'nowrap',
              boxShadow: '0 4px 12px rgba(0,0,0,.4)',
              pointerEvents: 'none',
            }}
          >
            {tip}
          </div>
        )}
      </div>

      <div style={{ flex: 1, display: 'flex', minHeight: 0 }}>
        {painel && (
          <aside
            style={{
              width: 152,
              flex: 'none',
              borderRight: `1px solid ${LINHA_CLARA}`,
              padding: '14px 12px',
              display: 'flex',
              flexDirection: 'column',
              gap: 14,
              overflow: 'auto',
              background: FUNDO_PAINEL,
            }}
          >
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'baseline',
                fontSize: 10.5,
                letterSpacing: '.1em',
                textTransform: 'uppercase',
                color: 'var(--color-neutral-500)',
              }}
            >
              <span>Séries</span>
              <span>{exame.series.length}</span>
            </div>
            {exame.series.map((item, j) => {
              const ativa = serieIndex === j;
              const espelhada = item.label === 'PERFIL' || item.label === 'OBL' || item.label === 'AXIAL';
              return (
                <button
                  key={item.label}
                  type="button"
                  onClick={() => {
                    setSerieIndex(j);
                    setImg(0);
                  }}
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 6,
                    padding: 0,
                    border: 0,
                    background: 'transparent',
                    cursor: 'pointer',
                    textAlign: 'left',
                    font: 'inherit',
                  }}
                >
                  <div
                    style={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      width: '100%',
                      fontSize: 12,
                      color: ativa ? 'var(--color-accent-300)' : 'var(--color-neutral-400)',
                    }}
                  >
                    <span style={{ fontWeight: 600, letterSpacing: '.04em' }}>{item.label}</span>
                    <span style={{ color: 'var(--color-neutral-500)' }}>
                      S{item.n} · {item.count}
                    </span>
                  </div>
                  <div
                    style={{
                      width: 128,
                      height: 128,
                      border: `1px solid ${ativa ? 'var(--color-accent-400)' : 'color-mix(in srgb, white 10%, transparent)'}`,
                      background: 'black',
                      display: 'grid',
                      placeItems: 'center',
                      overflow: 'hidden',
                    }}
                  >
                    <div
                      style={{
                        height: '88%',
                        aspectRatio: '.82',
                        transform: espelhada ? 'scaleX(.78)' : 'none',
                        filter: 'blur(1.5px)',
                        background: RADIOGRAFIA_MINIATURA,
                      }}
                    />
                  </div>
                </button>
              );
            })}
          </aside>
        )}

        <div
          style={{
            flex: 1,
            minWidth: 0,
            display: 'grid',
            gridTemplateColumns: nViewports === 1 ? '1fr' : '1fr 1fr',
            gridTemplateRows: nViewports === 4 ? '1fr 1fr' : '1fr',
            gap: 2,
            background: 'black',
          }}
        >
          {Array.from({ length: nViewports }, (_, k) => (
            <div
              key={k}
              data-cornerstone-viewport="true"
              onMouseDown={(event) => {
                event.preventDefault();
                arraste.current = { x: event.clientX, y: event.clientY, wb, wc, px, py, zoom, passo: 0, ultimo: 0 };
              }}
              onWheel={(event) => {
                if (tool === 'scroll' || !event.ctrlKey) passoImagem(event.deltaY > 0 ? 1 : -1);
              }}
              style={{
                position: 'relative',
                overflow: 'hidden',
                background: 'black',
                outline: `1px solid ${k === 0 && nViewports > 1 ? 'var(--color-accent-500)' : LINHA_CLARA}`,
                outlineOffset: -1,
                cursor: CURSOR[tool],
                display: 'grid',
                placeItems: 'center',
              }}
            >
              {viewerState === 'ok' && (
                <>
                  <div
                    style={{
                      position: 'relative',
                      height: '92%',
                      aspectRatio: '.82',
                      transform,
                      filter: filtro,
                      transition: 'transform .12s ease-out',
                    }}
                  >
                    <div style={{ width: '100%', height: '100%', filter: 'blur(3px)', background: RADIOGRAFIA_BASE }} />
                    <CostelasOverlay lado="esquerda" />
                    <CostelasOverlay lado="direita" />
                  </div>
                  <div
                    style={{
                      position: 'absolute',
                      left: '50%',
                      bottom: 64,
                      transform: 'translateX(-50%)',
                      fontFamily: 'ui-monospace,Menlo,monospace',
                      fontSize: 10,
                      letterSpacing: '.12em',
                      textTransform: 'uppercase',
                      color: 'color-mix(in srgb, white 22%, transparent)',
                      pointerEvents: 'none',
                    }}
                  >
                    Imagem demonstrativa · área do viewport Cornerstone3D
                  </div>
                </>
              )}

              {viewerState === 'loading' && (
                <div
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    gap: 14,
                    color: 'var(--color-neutral-400)',
                  }}
                >
                  <div
                    style={{
                      width: 28,
                      height: 28,
                      border: '1.5px solid color-mix(in srgb, white 15%, transparent)',
                      borderTopColor: 'var(--color-accent-400)',
                      borderRadius: '50%',
                      animation: 'pacsSpin .9s linear infinite',
                    }}
                  />
                  <span style={{ fontSize: 13.5 }}>Carregando imagem 1 de 3…</span>
                  <div
                    style={{
                      width: 180,
                      height: 2,
                      background: 'color-mix(in srgb, white 10%, transparent)',
                      overflow: 'hidden',
                    }}
                  >
                    <div
                      style={{
                        width: '40%',
                        height: '100%',
                        background: 'var(--color-accent-400)',
                        animation: 'pacsBar 1.3s ease-in-out infinite',
                      }}
                    />
                  </div>
                </div>
              )}

              {viewerState === 'error' && (
                <div
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    gap: 10,
                    textAlign: 'center',
                    maxWidth: 360,
                  }}
                >
                  <span style={{ color: 'oklch(0.72 0.12 25)' }}>
                    <Icon name="alert" size={32} />
                  </span>
                  <span
                    style={{
                      fontFamily: 'var(--font-heading)',
                      fontWeight: 600,
                      fontSize: 22,
                      color: 'var(--color-neutral-100)',
                    }}
                  >
                    Não foi possível carregar a imagem
                  </span>
                  <span style={{ fontSize: 13.5, color: 'var(--color-neutral-400)', lineHeight: 1.5 }}>
                    A transferência foi interrompida. As demais imagens do estudo podem estar
                    disponíveis no painel de séries.
                  </span>
                  <button
                    type="button"
                    className="pacs-hover-accent-dark"
                    style={{
                      marginTop: 8,
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      height: 36,
                      padding: '0 16px',
                      border: '1px solid var(--color-accent-500)',
                      background: 'transparent',
                      color: 'var(--color-accent-300)',
                      font: 'inherit',
                      fontSize: 14,
                      cursor: 'pointer',
                    }}
                  >
                    <Icon name="rotate" />
                    Tentar novamente
                  </button>
                </div>
              )}

              <Sobreposicao posicao="tl">
                <span style={{ color: 'color-mix(in srgb, var(--color-neutral-100) 80%, transparent)' }}>
                  {exame.short}
                </span>
                <span>ID {exame.pront}</span>
                <span>{exame.sexAge}</span>
              </Sobreposicao>

              <Sobreposicao posicao="tr">
                <span>{exame.unit}</span>
                <span>
                  {exame.date} {exame.time}
                </span>
                <span>{exame.exam}</span>
              </Sobreposicao>

              <Sobreposicao posicao="bl">
                <span>{wlText}</span>
                <span>
                  Zoom {Math.round(zoom * 100)}% · Rot {rot % 360}°
                </span>
              </Sobreposicao>

              <Sobreposicao posicao="br">
                <span>
                  Série {serieIndex + 1} · {serie.label}
                </span>
                <span style={{ color: 'color-mix(in srgb, var(--color-neutral-100) 80%, transparent)' }}>
                  Imagem {img + 1} / {serie.count}
                </span>
              </Sobreposicao>

              <MarcaLateral lado="left">R</MarcaLateral>
              <MarcaLateral lado="right">L</MarcaLateral>
            </div>
          ))}
        </div>
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          bottom: 22,
          transform: 'translateX(-50%)',
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          padding: 4,
          background: 'color-mix(in srgb, var(--color-neutral-900) 80%, transparent)',
          border: '1px solid color-mix(in srgb, white 10%, transparent)',
          zIndex: 4,
        }}
      >
        <button
          type="button"
          className="pacs-hover-white"
          onClick={() => passoImagem(-1)}
          title="Imagem anterior · ↑"
          aria-label="Imagem anterior"
          style={{
            width: 30,
            height: 28,
            display: 'grid',
            placeItems: 'center',
            border: 0,
            background: 'transparent',
            color: 'var(--color-neutral-200)',
            opacity: img > 0 ? 1 : 0.35,
            cursor: 'pointer',
          }}
        >
          <Icon name="left" />
        </button>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, padding: '0 4px' }}>
          {Array.from({ length: serie.count }, (_, k) => (
            <button
              key={k}
              type="button"
              onClick={() => setImg(k)}
              aria-label={`Imagem ${k + 1}`}
              style={{
                width: k === img ? 18 : 6,
                height: 6,
                padding: 0,
                border: 0,
                background: k === img ? 'var(--color-accent-300)' : 'color-mix(in srgb, white 25%, transparent)',
                cursor: 'pointer',
                transition: 'width .15s',
              }}
            />
          ))}
        </div>
        <span
          style={{
            fontSize: 12,
            fontVariantNumeric: 'tabular-nums',
            color: 'var(--color-neutral-300)',
            padding: '0 6px',
          }}
        >
          {img + 1} / {serie.count}
        </span>
        <button
          type="button"
          className="pacs-hover-white"
          onClick={() => passoImagem(1)}
          title="Próxima imagem · ↓"
          aria-label="Próxima imagem"
          style={{
            width: 30,
            height: 28,
            display: 'grid',
            placeItems: 'center',
            border: 0,
            background: 'transparent',
            color: 'var(--color-neutral-200)',
            opacity: img < serie.count - 1 ? 1 : 0.35,
            cursor: 'pointer',
          }}
        >
          <Icon name="right" />
        </button>
      </div>

      {full && (
        <button
          type="button"
          onClick={() => setFull(false)}
          style={{
            position: 'absolute',
            right: 16,
            bottom: 22,
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            height: 30,
            padding: '0 12px',
            border: '1px solid color-mix(in srgb, white 14%, transparent)',
            background: 'color-mix(in srgb, var(--color-neutral-900) 80%, transparent)',
            color: 'var(--color-neutral-200)',
            font: 'inherit',
            fontSize: 12.5,
            cursor: 'pointer',
            zIndex: 4,
          }}
        >
          Sair da tela cheia · Esc
        </button>
      )}
    </div>
  );
}

/** Textura de costelas sobre um dos campos pulmonares. */
function CostelasOverlay({ lado }: { lado: 'esquerda' | 'direita' }) {
  const angulo = lado === 'esquerda' ? '166deg' : '194deg';
  const centro = lado === 'esquerda' ? '32%' : '68%';
  const mascara = `radial-gradient(ellipse 22% 36% at ${centro} 46%, black 40%, transparent 75%)`;
  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        background: `repeating-linear-gradient(${angulo}, transparent 0 16px, rgba(255,255,255,.08) 16px 22px)`,
        WebkitMaskImage: mascara,
        maskImage: mascara,
        filter: 'blur(1.5px)',
      }}
    />
  );
}

/** Sobreposição de canto do viewport — texto mono sobre a imagem. */
function Sobreposicao({
  posicao,
  children,
}: {
  posicao: 'tl' | 'tr' | 'bl' | 'br';
  children: React.ReactNode;
}) {
  const direita = posicao === 'tr' || posicao === 'br';
  const baixo = posicao === 'bl' || posicao === 'br';
  return (
    <div
      style={{
        position: 'absolute',
        [baixo ? 'bottom' : 'top']: 12,
        [direita ? 'right' : 'left']: 14,
        display: 'flex',
        flexDirection: 'column',
        alignItems: direita ? 'flex-end' : undefined,
        gap: 1,
        fontFamily: 'ui-monospace,Menlo,monospace',
        fontSize: 11.5,
        lineHeight: 1.5,
        whiteSpace: 'nowrap',
        color: 'color-mix(in srgb, var(--color-neutral-300) 72%, transparent)',
        pointerEvents: 'none',
      }}
    >
      {children}
    </div>
  );
}

/** Marcadores de lateralidade R / L. */
function MarcaLateral({ lado, children }: { lado: 'left' | 'right'; children: React.ReactNode }) {
  return (
    <span
      style={{
        position: 'absolute',
        [lado]: 14,
        top: '50%',
        fontFamily: 'ui-monospace,Menlo,monospace',
        fontSize: 13,
        color: 'color-mix(in srgb, var(--color-neutral-300) 60%, transparent)',
        pointerEvents: 'none',
      }}
    >
      {children}
    </span>
  );
}

function ItemMais({ label }: { label: string }) {
  return (
    <button
      type="button"
      className="pacs-hover-white"
      style={{
        height: 34,
        padding: '0 10px',
        border: 0,
        background: 'transparent',
        color: 'var(--color-neutral-200)',
        font: 'inherit',
        textAlign: 'left',
        cursor: 'pointer',
      }}
    >
      {label}
    </button>
  );
}
