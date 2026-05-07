import fs from 'node:fs'
import path from 'node:path'
import { execSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function readAppVersion() {
  try {
    const chartPath = path.resolve(__dirname, '../helm/homenavi/Chart.yaml');
    const chart = fs.readFileSync(chartPath, 'utf8');
    const match = chart.match(/^appVersion:\s*["']?([^\s"']+)["']?\s*$/m);
    return match?.[1]?.trim() || 'dev';
  } catch {
    return 'dev';
  }
}

function readGitCommit() {
  try {
    return execSync('git rev-parse --short HEAD', {
      cwd: path.resolve(__dirname, '..'),
      stdio: ['ignore', 'pipe', 'ignore'],
    }).toString().trim();
  } catch {
    return '';
  }
}

const chartVersion = String(process.env.HOMENAVI_APP_VERSION || readAppVersion()).replace(/^v(?=\d)/, '');
const buildCommit = String(process.env.GITHUB_SHA || readGitCommit()).trim();
const buildMeta = {
  version: chartVersion,
  releaseTag: chartVersion && chartVersion !== 'dev' ? `v${chartVersion}` : chartVersion,
  commit: buildCommit,
  builtAt: new Date().toISOString(),
};

// https://vite.dev/config/
export default defineConfig({
  define: {
    __HOMENAVI_BUILD__: JSON.stringify(buildMeta),
  },
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
