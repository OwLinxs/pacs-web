import { Icon, type IconName } from '../design-system/Icon';
import type { ViewerTool } from './tools';

const tools: { name: ViewerTool; label: string; icon: IconName }[] = [
  { name: 'WindowLevel', label: 'Window/Level', icon: 'sun' },
  { name: 'Zoom', label: 'Zoom', icon: 'zoom' },
  { name: 'Pan', label: 'Pan', icon: 'move' },
  { name: 'Length', label: 'Length', icon: 'ruler' },
  { name: 'Angle', label: 'Angle', icon: 'angle' },
  { name: 'Probe', label: 'Probe', icon: 'probe' },
  { name: 'RectangleROI', label: 'Rectangle ROI', icon: 'g1' },
];

type Props = {
  selected: ViewerTool;
  inverted: boolean;
  disabled: boolean;
  onSelect: (tool: ViewerTool) => void;
  onInvert: () => void;
  onReset: () => void;
};

export function ViewerToolbar({ selected, inverted, disabled, onSelect, onInvert, onReset }: Props) {
  const button = (label: string, icon: IconName, active: boolean | undefined, onClick: () => void) => (
    <button key={label} type="button" title={label} aria-label={label} aria-pressed={active} disabled={disabled} onClick={onClick}
      style={{ display: 'flex', alignItems: 'center', gap: 6, flex: 'none', height: 36, padding: '0 10px', font: 'inherit', fontSize: 12, border: '1px solid transparent', borderBottomColor: active ? 'var(--color-accent-400)' : 'transparent', background: active ? 'rgba(255,255,255,.08)' : 'transparent', color: active ? 'var(--color-accent-300)' : 'var(--color-neutral-300)', opacity: disabled ? .4 : 1, cursor: disabled ? 'default' : 'pointer' }}>
      <Icon name={icon} size={16} /><span>{label}</span>
    </button>
  );
  return (
    <div role="toolbar" aria-label="Ferramentas do Viewer" style={{ height: 50, flex: 'none', display: 'flex', alignItems: 'center', gap: 2, padding: '0 10px', overflowX: 'auto', borderBottom: '1px solid rgba(255,255,255,.08)', background: 'color-mix(in srgb, var(--color-neutral-900) 70%, black)' }}>
      {tools.map((tool) => button(tool.label, tool.icon, selected === tool.name, () => onSelect(tool.name)))}
      <span style={{ height: 22, width: 1, margin: '0 6px', background: 'rgba(255,255,255,.12)', flex: 'none' }} />
      {button('Invert', 'contrast', inverted, onInvert)}
      {button('Reset', 'reset', undefined, onReset)}
      <span style={{ marginLeft: 'auto', paddingLeft: 12, fontSize: 11, whiteSpace: 'nowrap', color: 'var(--color-neutral-500)' }}>Scroll / setas: imagens</span>
    </div>
  );
}
