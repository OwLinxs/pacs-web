import { useState } from 'react';
import type { DemoState } from '../App';

/**
 * ANDAIME DE DESENVOLVIMENTO — NÃO FAZ PARTE DO DESIGN.
 *
 * Existe só para conferir cada tela e cada estado do canvas
 * "PACS Web Municipal - Telas.dc.html" sem precisar navegar até eles.
 * Só é montado em `import.meta.env.DEV`; não vai para o build de produção.
 */

type Item = { id: string; label: string; estado: Partial<DemoState> };

const ITENS: { grupo: string; itens: Item[] }[] = [
  {
    grupo: '1 · Fluxo principal',
    itens: [
      { id: '1a', label: 'Login', estado: { screen: 'login' } },
      { id: '1b', label: 'Exames / Worklist', estado: { screen: 'exames' } },
    ],
  },
  {
    grupo: '2 · Administração',
    itens: [
      { id: '2a', label: 'Usuários', estado: { screen: 'usuarios', modalNovoMedico: false } },
      { id: '2b', label: 'Novo médico (modal)', estado: { screen: 'usuarios', modalNovoMedico: true } },
      { id: '2c', label: 'Auditoria', estado: { screen: 'auditoria' } },
      { id: '2d', label: 'Configurações · PACS', estado: { screen: 'configuracoes' } },
    ],
  },
  {
    grupo: '3 · Estados',
    itens: [
      { id: '3f', label: 'Sistema indisponível', estado: { screen: 'indisponivel' } },
      { id: '3g', label: 'Sessão expirada', estado: { screen: 'expirada' } },
    ],
  },
];

type PainelDeTelasProps = {
  demo: DemoState;
  onMudar: (estado: DemoState) => void;
};

export function PainelDeTelas({ demo, onMudar }: PainelDeTelasProps) {
  const [aberto, setAberto] = useState(false);

  function aplicar(item: Item) {
    onMudar({ ...demo, modalNovoMedico: false, ...item.estado });
  }

  return (
    <div
      style={{
        position: 'fixed',
        left: 12,
        bottom: 12,
        zIndex: 100,
        fontFamily: 'ui-monospace,Menlo,monospace',
        fontSize: 11,
      }}
    >
      {aberto && (
        <div
          style={{
            width: 232,
            marginBottom: 6,
            padding: 8,
            background: '#111',
            color: '#ddd',
            border: '1px solid #333',
            maxHeight: '70vh',
            overflow: 'auto',
          }}
        >
          <div style={{ color: '#888', marginBottom: 8, lineHeight: 1.4 }}>
            Andaime de desenvolvimento.
            <br />
            Telas do canvas de design.
          </div>
          {ITENS.map((grupo) => (
            <div key={grupo.grupo} style={{ marginBottom: 8 }}>
              <div style={{ color: '#6f8fb0', margin: '0 0 4px' }}>{grupo.grupo}</div>
              {grupo.itens.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => aplicar(item)}
                  style={{
                    display: 'block',
                    width: '100%',
                    textAlign: 'left',
                    padding: '3px 6px',
                    border: 0,
                    background: 'transparent',
                    color: '#ddd',
                    font: 'inherit',
                    cursor: 'pointer',
                  }}
                >
                  <span style={{ color: '#888', marginRight: 6 }}>{item.id}</span>
                  {item.label}
                </button>
              ))}
            </div>
          ))}
        </div>
      )}
      <button
        type="button"
        onClick={() => setAberto((valor) => !valor)}
        style={{
          padding: '4px 8px',
          background: '#111',
          color: '#ddd',
          border: '1px solid #333',
          font: 'inherit',
          cursor: 'pointer',
        }}
      >
        {aberto ? '× telas' : 'telas ▴'}
      </button>
    </div>
  );
}
