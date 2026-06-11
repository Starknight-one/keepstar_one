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
      // GUARD: benign today because widget.jsx has no top-level exports
      // (rollup emits a bare IIFE). If an export is ever added, rollup
      // will emit `var KeepstarV5Widget = ...` in the classic script and
      // clobber the window.KeepstarV5Widget global the entry assigns —
      // keep exports out of widget.jsx (put them in widget-preview.jsx).
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
    // Process CSS in tests so `widget.css?inline` yields the real
    // stylesheet text (widget-preview test asserts it lands in the
    // shadow root). Vitest default stubs CSS imports to ''.
    css: true,
  },
}))
