import { rm, mkdir, cp, rename, access } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));

function p(...parts) {
  return resolve(here, ...parts);
}

async function exists(path) {
  try {
    await access(path);
    return true;
  } catch {
    return false;
  }
}

async function exportTarget(target, destDir, destIndexName) {
  const distDir = p('..', 'dist', target);
  const srcHtml = p('..', 'dist', target, `${target}.html`);

  if (!(await exists(distDir))) {
    throw new Error(`Missing build output: ${distDir}`);
  }
  if (!(await exists(srcHtml))) {
    throw new Error(`Missing HTML entry: ${srcHtml}`);
  }

  // Clean destination
  await rm(destDir, { recursive: true, force: true });
  await mkdir(destDir, { recursive: true });

  // Copy everything
  await cp(distDir, destDir, { recursive: true });

  // Rename tab.html/widget.html -> index.html
  const destHtml = resolve(destDir, destIndexName);
  await rename(resolve(destDir, `${target}.html`), destHtml);
}

await exportTarget('tab', p('..', '..', '..', 'web', 'ui'), 'index.html');
await exportTarget('widget', p('..', '..', '..', 'web', 'widgets', 'player'), 'index.html');

console.log('OK: exported tab -> web/ui and widget -> web/widgets/player');
