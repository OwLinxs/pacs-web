import { Icon } from '../design-system/Icon';

/** Aviso de confirmação no canto inferior direito, como no design. */
export function Toast({ mensagem }: { mensagem: string }) {
  return (
    <div
      role="status"
      style={{
        position: 'fixed',
        right: 24,
        bottom: 24,
        zIndex: 40,
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        padding: '14px 18px',
        background: 'var(--color-neutral-900)',
        color: 'var(--color-neutral-100)',
        fontSize: 14,
        boxShadow: 'var(--shadow-lg)',
      }}
    >
      <span
        style={{
          width: 22,
          height: 22,
          flex: 'none',
          display: 'grid',
          placeItems: 'center',
          background: 'var(--color-accent)',
          color: 'var(--color-bg)',
        }}
      >
        <Icon name="check" />
      </span>
      {mensagem}
    </div>
  );
}
