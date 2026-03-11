const fs = require('fs');

const tag = process.env.TAG || '';
const rawBase = process.env.RAW_BASE || '';
const manifestUrl = process.env.MANIFEST_URL || '';
const metadataPath = process.env.METADATA_PATH || '';
const manifestPath = process.env.MANIFEST_PATH || '';

const readJson = (pathValue) => JSON.parse(fs.readFileSync(pathValue, 'utf8'));

let metadata = readJson(metadataPath);
if (typeof metadata === 'string') {
  metadata = JSON.parse(metadata);
}

if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) {
  throw new Error('Marketplace metadata must be a JSON object.');
}

const manifest = readJson(manifestPath);

const normalizeUrl = (value) => {
  if (value === null || value === undefined) {
    return value;
  }
  const strValue = typeof value === 'string' ? value : String(value);
  const lowerValue = strValue.toLowerCase();
  if (lowerValue.startsWith('fa:')) {
    return strValue;
  }
  if (strValue.startsWith('http://') || strValue.startsWith('https://')) {
    return strValue;
  }
  return `${rawBase}/${strValue}`;
};

metadata.version = tag;
metadata.release_tag = tag;
metadata.manifest_url = manifestUrl;
metadata.manifest = manifest;

if (typeof metadata.image === 'string' && metadata.image.length > 0) {
  metadata.image = `${metadata.image}:${tag}`;
}

if (!metadata.deployment_artifacts || typeof metadata.deployment_artifacts !== 'object' || Array.isArray(metadata.deployment_artifacts)) {
  metadata.deployment_artifacts = {};
}

if (!metadata.deployment_artifacts.compose || typeof metadata.deployment_artifacts.compose !== 'object') {
  metadata.deployment_artifacts.compose = {};
}

const composeSource = metadata.deployment_artifacts.compose.file || metadata.compose_file;
if (composeSource) {
  const composeValueStr = typeof composeSource === 'string' ? composeSource : '';
  const composeName = composeValueStr.split('/').pop() || '';
  if (composeName !== 'docker-compose.integration.yml') {
    throw new Error('compose artifact must point to compose/docker-compose.integration.yml');
  }
  metadata.deployment_artifacts.compose.file = normalizeUrl(composeValueStr);
  delete metadata.compose_file;
}

if (metadata.deployment_artifacts.helm && typeof metadata.deployment_artifacts.helm === 'object') {
  const helm = metadata.deployment_artifacts.helm;
  if (typeof helm.chart_ref === 'string' && helm.chart_ref.length > 0) {
    helm.chart_ref = helm.chart_ref;
  }
  if (!helm.version || String(helm.version).trim().length === 0) {
    helm.version = tag;
  }
}

if (metadata.deployment_artifacts.k8s_generated && typeof metadata.deployment_artifacts.k8s_generated === 'object') {
  const generated = metadata.deployment_artifacts.k8s_generated;
  if (!generated.version || String(generated.version).trim().length === 0) {
    generated.version = tag;
  }
}

const hasCompose = Boolean(metadata.deployment_artifacts.compose && metadata.deployment_artifacts.compose.file);
const hasHelm = Boolean(metadata.deployment_artifacts.helm && metadata.deployment_artifacts.helm.chart_ref);
const hasGenerated = Boolean(metadata.deployment_artifacts.k8s_generated && metadata.deployment_artifacts.k8s_generated.chart_ref);
if (!hasCompose && !hasHelm && !hasGenerated) {
  throw new Error('deployment_artifacts must include compose.file, helm.chart_ref, or k8s_generated.chart_ref');
}

metadata.assets = metadata.assets || {};
for (const [key, value] of Object.entries(metadata.assets)) {
  metadata.assets[key] = normalizeUrl(value);
}

metadata.images = Array.isArray(metadata.images) ? metadata.images : [];
metadata.images = metadata.images.map(normalizeUrl);

process.stdout.write(JSON.stringify(metadata));
