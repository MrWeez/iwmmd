import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { writeFileSync } from 'node:fs'
import { defineConfig, type Plugin } from 'vite'

function preserveGitkeep(): Plugin {
  return {
    name: 'preserve-gitkeep',
    closeBundle() {
      writeFileSync(new URL('../internal/webui/dist/.gitkeep', import.meta.url), '')
    },
  }
}

const apiTarget = process.env.IWMMD_DEV_API ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [react(), tailwindcss(), preserveGitkeep()],
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': apiTarget,
      '/healthz': apiTarget,
      '/readyz': apiTarget,
    },
  },
})
