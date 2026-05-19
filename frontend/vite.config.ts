/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(__dirname, './src') } },
  server: {
    port: 3001,
    proxy: {
      '/api': { target: 'http://localhost:80', changeOrigin: true },
    },
  },
  test: {
    environment: 'happy-dom',
    globals: true,
    setupFiles: ['./src/__tests__/setup.ts'],
    // Exclude the Playwright e2e specs from Vitest's collection
    exclude: ['node_modules', 'dist', 'tests/e2e/**', 'playwright-report/**'],
  },
})
