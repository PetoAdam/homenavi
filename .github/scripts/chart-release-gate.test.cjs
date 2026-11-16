const { test } = require('node:test');
const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const yaml = require('../../frontend/node_modules/js-yaml');
const { chartImages, waitForImages } = require('./chart-release-gate.cjs');

test('discovers tagged HomeNavi images from the rendered chart', () => {
  const rendered = execFileSync('helm', ['template', 'homenavi', 'helm/homenavi', '--set', 'global.imageTag=v1.2.3'], { encoding: 'utf8' });
  const images = chartImages(rendered, 'PetoAdam', 'v1.2.3');
  assert.ok(images.length > 1);
  assert.ok(images.some(({ workflow }) => workflow === 'frontend_release.yaml'));
  assert.ok(images.some(({ workflow }) => workflow === 'api_gateway_release.yaml'));
  for (const { image, workflow } of images) {
    const release = yaml.load(fs.readFileSync(`.github/workflows/${workflow}`, 'utf8'));
    const service = image.split('/').pop().replace(/^homenavi-/, '').split(':')[0];
    assert.equal(release.jobs.release.with.service, service, workflow);
    assert.equal(release.jobs.release.uses, './.github/workflows/service_release.yml', workflow);
  }
  assert.throws(() => chartImages(rendered, 'another-owner', 'v1.2.3'), /release owner and tag/);
  assert.throws(() => chartImages(rendered, 'PetoAdam', 'v9.9.9'), /release owner and tag/);
  assert.throws(() => chartImages('kind: ConfigMap\ndata: {}', 'PetoAdam', 'v1.2.3'), /no HomeNavi images/);
});

test('the chart push is ordered after the image gate', () => {
  const workflow = yaml.load(fs.readFileSync('.github/workflows/helm_chart_release.yaml', 'utf8'));
  const steps = workflow.jobs['release-chart'].steps.map((step) => step.name);
  assert.equal(workflow.permissions.actions, 'read');
  assert.ok(steps.indexOf('Wait for release images') >= 0);
  assert.ok(steps.indexOf('Push chart to GHCR') > steps.indexOf('Wait for release images'));
});

test('waits for successful workflows and available images', async () => {
  let time = 0;
  let attempts = 0;
  const images = [{ image: 'first', workflow: 'first.yaml' }, { image: 'second', workflow: 'second.yaml' }];
  await waitForImages({
    images,
    lookup: async (workflow) => workflow === 'second.yaml' && time === 0 ? undefined : { status: 'completed', conclusion: 'success' },
    inspect: async (image) => image !== 'second' || ++attempts > 1,
    pause: async () => { time += 1; },
    now: () => time,
    timeoutMs: 3,
    log: () => {},
  });
  assert.equal(time, 2);
});

test('fails on failed workflows or an image that never appears', async () => {
  const images = [{ image: 'missing', workflow: 'missing.yaml' }];
  let time = 0;
  const options = {
    images, lookup: async () => ({ status: 'completed', conclusion: 'success' }),
    inspect: async () => false, pause: async () => { time += 1; },
    now: () => time, timeoutMs: 2, log: () => {},
  };
  await assert.rejects(waitForImages({ ...options, lookup: async () => ({ status: 'completed', conclusion: 'failure' }) }), /finished with failure/);
  await assert.rejects(waitForImages(options), /Timed out.*missing/);
});