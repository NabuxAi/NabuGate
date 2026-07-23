import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// NabuGate console SPA. Built to ./dist as a static bundle; the gateway (or any
// static host / Coolify) can serve it. `base: './'` keeps asset URLs relative so
// it works whether mounted at / or under a sub-path.
export default defineConfig({
  base: './',
  plugins: [react()],
  server: { port: 5173 },
});
