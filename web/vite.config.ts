import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

/**
 * `go:embed all:dist` needs the directory to contain at least one file, so a
 * placeholder is committed. Vite empties the output directory on every build,
 * which would delete it — put it back.
 */
function keepEmbedPlaceholder() {
  return {
    name: 'keep-embed-placeholder',
    closeBundle() {
      writeFileSync(join(__dirname, 'dist', '.gitkeep'), '')
    },
  }
}

export default defineConfig({
  plugins: [react(), keepEmbedPlaceholder()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:5006',
    },
  },
})
