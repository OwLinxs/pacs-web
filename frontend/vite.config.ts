import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { viteCommonjs } from '@originjs/vite-plugin-commonjs';

// O backend Go responde em /api e /health. Em desenvolvimento o Vite faz proxy
// para ele, de modo que navegador e API compartilhem a origem http://localhost:5173.
// Isso mantém os cookies de sessão funcionando sem CORS e sem SameSite=None.
// Endereço padrão do backend em desenvolvimento (HTTP_ADDR do .env do backend).
const BACKEND = 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [react(), viteCommonjs()],
  optimizeDeps: {
    exclude: ['@cornerstonejs/dicom-image-loader'],
    include: ['dicom-parser'],
  },
  worker: { format: 'es' },
  // xmlbuilder2, transitivo do metadata/dcmjs, usa EventEmitter no browser.
  resolve: { alias: [{ find: /^events$/, replacement: 'events/' }] },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: BACKEND, changeOrigin: false },
      '/health': { target: BACKEND, changeOrigin: false },
    },
  },
});
