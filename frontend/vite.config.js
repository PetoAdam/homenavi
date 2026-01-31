import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa';

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    VitePWA({
      registerType: 'autoUpdate',
      workbox: {
        // IMPORTANT: exclude platform endpoints from SPA navigation fallback.
        // Otherwise the service worker can respond with index.html for these URLs,
        // which breaks integration iframes and API calls.
        navigateFallbackDenylist: [/^\/api\//, /^\/integrations\//, /^\/ws\//, /^\/uploads\//],
        runtimeCaching: [
          {
            urlPattern: /^\/api\//,
            handler: 'NetworkOnly'
          },
          {
            urlPattern: /^\/integrations\//,
            handler: 'NetworkOnly'
          }
        ]
      },
      manifest: {
        name: 'Homenavi',
        short_name: 'Homenavi',
        description: 'Homenavi platform web app',
        theme_color: '#ffffff',
        background_color: '#ffffff',
        display: 'standalone',
        start_url: '/',
        icons: [
          {
            src: '/icons/icon-192x192.png',
            sizes: '192x192',
            type: 'image/png'
          },
          {
            src: '/icons/icon-512x512.png',
            sizes: '512x512',
            type: 'image/png'
          }
        ]
      }
    })
  ],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      // When running the frontend dev server, proxy integration content through nginx
      // so that /integrations/* does not fall back to index.html.
      '/integrations': {
        target: 'http://localhost',
        changeOrigin: true
      },
      '/ws/automation': {
        target: 'ws://localhost:8080',
        changeOrigin: true,
        ws: true
      },
      '/ws/hdp': {
        target: 'ws://localhost:8080',
        changeOrigin: true,
        ws: true
      }
    }
  }
})
