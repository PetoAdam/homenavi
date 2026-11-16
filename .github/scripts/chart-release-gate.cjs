const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const yaml = require('../../frontend/node_modules/js-yaml');

function chartImages(rendered, owner, tag) {
  const images = new Set();
  const visit = (value) => {
    if (!value || typeof value !== 'object') return;
    for (const [key, child] of Object.entries(value)) {
      if ((key === 'containers' || key === 'initContainers') && Array.isArray(child)) {
        for (const container of child) {
          if (typeof container.image === 'string' && /^ghcr\.io\/[^/]+\/homenavi-/.test(container.image)) {
            images.add(container.image);
          }
        }
      }
      visit(child);
    }
  };
  yaml.loadAll(rendered).forEach(visit);
  if (!images.size) throw new Error('Rendered chart contains no HomeNavi images');
  const prefix = `ghcr.io/${owner.toLowerCase()}/homenavi-`;
  return [...images].sort().map((image) => {
    if (!image.startsWith(prefix) || !image.endsWith(`:${tag}`)) {
      throw new Error(`Chart image does not match release owner and tag: ${image}`);
    }
    const service = image.slice(prefix.length, -(tag.length + 1));
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(service)) throw new Error(`Invalid service image: ${image}`);
    return { image, workflow: `${service.replaceAll('-', '_')}_release.yaml` };
  });
}

async function waitForImages({ images, lookup, inspect, pause, now, timeoutMs, log }) {
  const deadline = now() + timeoutMs;
  const ready = new Set();
  while (true) {
    for (const { image, workflow } of images) {
      if (ready.has(image)) continue;
      const run = await lookup(workflow);
      if (!run || run.status !== 'completed') continue;
      if (run.conclusion !== 'success') {
        throw new Error(`${workflow} finished with ${run.conclusion || 'no conclusion'}`);
      }
      if (await inspect(image)) {
        ready.add(image);
        log(`Verified ${image} after ${workflow}`);
      }
    }
    if (ready.size === images.length) return;
    if (now() >= deadline) {
      throw new Error(`Timed out waiting for release images: ${images.filter(({ image }) => !ready.has(image)).map(({ image }) => image).join(', ')}`);
    }
    await pause();
  }
}

async function run({ github, context, core, renderedPath, tag, sha }) {
  const images = chartImages(fs.readFileSync(renderedPath, 'utf8'), context.repo.owner, tag);
  await waitForImages({
    images,
    lookup: async (workflow) => {
      const response = await github.rest.actions.listWorkflowRuns({
        ...context.repo, workflow_id: workflow, event: 'push', branch: tag, head_sha: sha, per_page: 100,
      });
      return response.data.workflow_runs.find((candidate) => candidate.head_sha === sha && candidate.head_branch === tag);
    },
    inspect: (image) => {
      try {
        execFileSync('docker', ['buildx', 'imagetools', 'inspect', image], { stdio: 'ignore' });
        return true;
      } catch {
        return false;
      }
    },
    pause: () => new Promise((resolve) => setTimeout(resolve, 30000)),
    now: Date.now,
    timeoutMs: 60 * 60 * 1000,
    log: core.info,
  });
}

module.exports = { chartImages, waitForImages, run };