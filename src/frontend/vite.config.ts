import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'url';

const DEFAULT_BACKEND_URL = 'http://10.11.221.79:8080';

function normalizeBaseURL(value?: string): string {
  return (value || DEFAULT_BACKEND_URL).replace(/\/+$/, '');
}

function websocketTarget(baseURL: string): string {
  const url = new URL(baseURL);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString().replace(/\/+$/, '');
}

const backendURL = normalizeBaseURL(process.env.VITE_AGENTHUB_BACKEND_URL);

export default defineConfig({
  base: './',
  plugins: [react()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  cacheDir: '.vite-cache',
  server: {
    host: '0.0.0.0',
    proxy: {
      '/api': {
        target: backendURL,
        changeOrigin: true,
      },
      '/ws': {
        target: websocketTarget(backendURL),
        ws: true,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-react': ['react', 'react-dom', 'react-router-dom'],
          'vendor-antd': ['antd', '@ant-design/icons'],
        },
      },
    },
  },
});
