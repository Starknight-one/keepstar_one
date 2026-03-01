import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Same CSS suppression plugin as main config — CSS is loaded via ?inline
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
    outDir: '../../project_admin/frontend/public/lib',
    emptyOutDir: false,
    lib: {
      entry: 'src/preview.jsx',
      name: 'KeestarPreview',
      fileName: () => 'preview.js',
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
