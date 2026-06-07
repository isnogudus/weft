import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// The build output (dist/) is embedded into the Go binary. During development,
// `npm run dev` proxies the JSON API to a locally running `weft -dev`.
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8099',
    },
  },
})
