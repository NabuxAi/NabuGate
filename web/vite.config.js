import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// NabuGate console SPA. Built to ./dist as a static bundle; the gateway (or any
// static host / Coolify) can serve it. `base: './'` keeps asset URLs relative so
// it works whether mounted at / or under a sub-path.
// Fills in the hashed names of each language's main text font, which the
// pre-paint script in index.html preloads. Font files are only named once the
// bundle is written, so this runs after it; failing the build beats silently
// shipping a page that preloads nothing (or a stale name that 404s).
function preloadFonts() {
  const FONTS = {
    __FONT_FA__: /^assets\/vazirmatn-arabic-wght-normal-[\w-]+\.woff2$/,
    __FONT_EN__: /^assets\/inter-latin-wght-normal-[\w-]+\.woff2$/,
  };
  return {
    name: 'nabugate-preload-fonts',
    apply: 'build',
    transformIndexHtml: {
      order: 'post',
      handler(html, ctx) {
        for (const [placeholder, re] of Object.entries(FONTS)) {
          const file = Object.keys(ctx.bundle || {}).find((f) => re.test(f));
          if (!file) throw new Error(`preload-fonts: no bundled font matches ${re}`);
          html = html.replace(placeholder, '/' + file);
        }
        return html;
      },
    },
  };
}

export default defineConfig({
  base: '/',
  plugins: [react(), preloadFonts()],
  server: { port: 5173 },
  build: {
    rollupOptions: {
      output: {
        // React changes a few times a year; the app changes every deploy. In
        // their own chunk, a returning visitor keeps React from cache (assets
        // are immutable) and downloads only the app code that changed.
        manualChunks(id) {
          if (/[\\/]node_modules[\\/](react|react-dom|scheduler)[\\/]/.test(id)) return 'react';
        },
      },
    },
  },
});
