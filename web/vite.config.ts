import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8443',
      '/health': 'http://localhost:8443',
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id: string) {
          if (id.includes('node_modules')) {
            if (id.includes('react-dom') || id.includes('react-router') || id.match(/\/react\//)) {
              return 'vendor-react'
            }
            if (id.includes('@tanstack/react-query')) {
              return 'vendor-query'
            }
            if (id.includes('@xyflow') || id.includes('dagre')) {
              return 'vendor-graph'
            }
            if (id.includes('lucide-react')) {
              return 'vendor-ui'
            }
            if (id.includes('zustand')) {
              return 'vendor-state'
            }
          }
        },
        chunkFileNames: 'chunks/[name]-[hash].js',
      },
    },
  },
})
