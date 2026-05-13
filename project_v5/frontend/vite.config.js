import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// shadowDomCss — same trick V4 frontend uses: suppress regular .css imports
// from React components so the only CSS that ends up in the bundle is what
// widget.jsx imports via ?inline. CSS goes into the shadow root as one
// <style> element rather than being injected into document.head.
function shadowDomCss() {
  return {
    name: 'shadow-dom-css',
    enforce: 'pre',
    resolveId(source) {
      if (source.endsWith('.css') && !source.includes('?')) {
        return '\0empty-css'
      }
    },
    load(id) {
      if (id === '\0empty-css') {
        return ''
      }
    },
  }
}

export default defineConfig(({ mode }) => ({
  plugins: [react(), shadowDomCss()],
  // Bake NODE_ENV=production only for the actual prod build. Vitest
  // sets mode='test'; under test we want development React so the act()
  // helper @testing-library/react needs is available.
  define:
    mode === 'production'
      ? { 'process.env.NODE_ENV': '"production"' }
      : {},
  build: {
    lib: {
      entry: 'src/widget.jsx',
      name: 'KeepstarV5Widget',
      fileName: () => 'widget.js',
      formats: ['iife'],
    },
    cssCodeSplit: false,
    rollupOptions: {
      output: {
        inlineDynamicImports: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./tests/setup.js'],
  },
}))
