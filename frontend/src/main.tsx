import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './design-system/industry.css';
import './styles/app.css';
import { App } from './App';
import { SessionProvider } from './auth/SessionProvider';

const container = document.getElementById('root');
if (!container) {
  throw new Error('Elemento #root não encontrado em index.html.');
}

createRoot(container).render(
  <StrictMode>
    <SessionProvider>
      <App />
    </SessionProvider>
  </StrictMode>,
);
