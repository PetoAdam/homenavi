import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { dirname } from 'node:path';
import net from 'node:net';

const target = process.env.HN_TARGET === 'widget' ? 'widget' : 'tab';

const here = dirname(fileURLToPath(import.meta.url));
const frontendRoot = here;

const inputHtml = target === 'widget'
  ? resolve(frontendRoot, 'widget.html')
  : resolve(frontendRoot, 'tab.html');

function isPortOpen(port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once('error', () => resolve(false));
    server.once('listening', () => {
      server.close(() => resolve(true));
    });
    server.listen(port, '127.0.0.1');
  });
}

async function findFreePort(start) {
  let port = start;
  while (port < start + 1000) {
    // eslint-disable-next-line no-await-in-loop
    if (await isPortOpen(port)) return port;
    port += 1;
  }
  return start;
}

export default defineConfig(async () => {
  const envPort = Number(process.env.HN_PORT);
  const basePort = Number.isFinite(envPort) && envPort > 0
    ? envPort
    : (target === 'widget' ? 10001 : 10000);

  const port = await findFreePort(basePort);

  return {
    plugins: [react()],
    root: frontendRoot,
    base: './',
    build: {
      outDir: resolve(here, 'dist', target),
      emptyOutDir: true,
      rollupOptions: {
        input: inputHtml
      }
    },
    server: {
      port,
      strictPort: false
    }
  };
});
