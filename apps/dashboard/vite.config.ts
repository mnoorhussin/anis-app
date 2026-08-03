import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],

  server: {
    port: 5173,
    strictPort: true,
    // The PocketBase dev server runs on 8090. Proxying it means the dev SPA
    // talks to a same-origin /api, so cookies and CORS behave in development
    // the way they will in production behind Caddy — rather than working in
    // dev because of a permissive CORS setting and breaking on deploy.
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8090',
        changeOrigin: true,
        // Chat replies stream token by token. Without this the proxy buffers
        // the whole response and the "typing" effect disappears in dev only.
        ws: true,
      },
    },
  },

  build: {
    outDir: 'dist',
    sourcemap: true,
    // Caddy serves these with a long cache lifetime, so filenames must be
    // content-hashed — which is Vite's default. Do not disable it.
    target: 'es2022',
  },
});
