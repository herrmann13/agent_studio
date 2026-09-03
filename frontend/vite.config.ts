import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  // Wails serves the embedded production bundle from its root asset handler.
  // Relative URLs also keep the same bundle usable from the local Vite server.
  base: './',
  plugins: [react()]
})
