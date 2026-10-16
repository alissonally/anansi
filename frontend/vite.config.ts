/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'
import { writeFileSync } from 'node:fs'

// Vite empties dist/ on build; the Go binary embeds it with go:embed, which
// needs the directory to exist in fresh checkouts, so keep a placeholder.
const keepDist = {
  name: 'keep-dist-placeholder',
  closeBundle() {
    writeFileSync(fileURLToPath(new URL('./dist/gitkeep', import.meta.url)), '')
  },
}

export default defineConfig({
  plugins: [vue(), keepDist],
  resolve: {
    alias: [
      // monaco-editor's package "exports" do not expose its CSS files.
      {
        find: /^monaco-css\/(.*)$/,
        replacement: fileURLToPath(new URL('./node_modules/monaco-editor/esm/vs/$1', import.meta.url)),
      },
    ],
  },
  build: {
    target: 'es2022',
    // Monaco is large; assets are served locally by the desktop app.
    chunkSizeWarningLimit: 5000,
    reportCompressedSize: false,
    rollupOptions: {
      output: {
        manualChunks: (id) => (id.includes('node_modules/monaco-editor') ? 'monaco' : undefined),
      },
    },
  },
  worker: { format: 'es' },
  test: {
    environment: 'jsdom',
    include: ['tests/**/*.test.ts'],
  },
})
