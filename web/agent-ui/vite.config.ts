import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// The agent serves these files itself, from a random port on loopback, so
// assets are referenced relatively. Nothing is fetched from a CDN: the page has
// to work on a machine with no network at all.
export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // A source map would ship the whole interface source inside the binary for
    // no benefit to the person running it.
    sourcemap: false,
  },
});
