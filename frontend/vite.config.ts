import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// O backend Go responde em /api e /health. Em desenvolvimento o Vite faz proxy
// para ele, de modo que navegador e API compartilhem a origem http://localhost:5173.
// Isso mantém os cookies de sessão funcionando sem CORS e sem SameSite=None.
// Endereço padrão do backend em desenvolvimento (HTTP_ADDR do .env do backend).
const BACKEND = 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: BACKEND, changeOrigin: false },
      '/health': { target: BACKEND, changeOrigin: false },
    },
  },
});
