const fs = require('fs');

const tag = process.env.TAG || '';
const rawBase = process.env.RAW_BASE || '';
const manifestUrl = process.env.MANIFEST_URL || '';
const metadataPath = process.env.METADATA_PATH || '';
const manifestPath = process.env.MANIFEST_PATH || '';

const readJson = (path) => JSON.parse(fs.readFileSync(path, 'utf8'));

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

if (metadata.compose_file) {
  metadata.compose_file = normalizeUrl(metadata.compose_file);
}

metadata.assets = metadata.assets || {};
for (const [key, value] of Object.entries(metadata.assets)) {
  metadata.assets[key] = normalizeUrl(value);
}

metadata.images = Array.isArray(metadata.images) ? metadata.images : [];
metadata.images = metadata.images.map(normalizeUrl);

process.stdout.write(JSON.stringify(metadata));
