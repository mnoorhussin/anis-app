import preact from '@preact/preset-vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

/**
 * The widget ships as ONE file that a business pastes into their site.
 *
 * Constraints that drive this config:
 *  - No separate .css request. The stylesheet has to end up inside the shadow
 *    root, so it is imported with `?inline` in the source and shipped as a
 *    string in the JS. That is why there is no CSS-injection plugin here — the
 *    usual ones inject into `document.head`, which is exactly where the styles
 *    must NOT go.
 *  - No externals, no chunks. It runs on somebody else's page with no
 *    bundler and no import map.
 *  - IIFE, not ESM, so a plain `<script src>` works without `type="module"` on
 *    sites that still target older browsers.
 */
export default defineConfig({
  plugins: [preact(), tailwindcss()],

  server: {
    port: 5174,
    strictPort: true,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8090', changeOrigin: true },
    },
  },

  build: {
    outDir: 'dist',
    target: 'es2020',
    sourcemap: true,
    // Never inline assets as data URIs — an inlined font or image would blow
    // the budget silently.
    assetsInlineLimit: 0,
    cssCodeSplit: false,
    lib: {
      entry: 'src/embed.tsx',
      name: 'AnisWidget',
      formats: ['iife'],
      // Stable filename: the install snippet on thousands of sites points at
      // it. Cache-busting is handled by Caddy's ETag plus a short max-age on
      // this one file, not by renaming it.
      fileName: () => 'widget.js',
    },
    // Nothing is externalised: Preact and the shared packages are bundled in,
    // because the host page has no module loader and no import map. Library
    // mode already disables code splitting, so there is no rollupOptions
    // override needed to get a single file.
  },
});
