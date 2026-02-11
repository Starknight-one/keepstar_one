import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// All CSS is imported via ?inline in widget.jsx and injected into Shadow DOM.
// This plugin suppresses regular CSS imports from components so CSS isn't duplicated.
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

export default defineConfig({
  plugins: [react(), shadowDomCss()],
  define: {
    'process.env.NODE_ENV': '"production"',
  },
  build: {
    lib: {
      entry: 'src/widget.jsx',
      name: 'KeestarWidget',
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
})
