const fs = require('fs');

const baseUrl = (process.env.MARKETPLACE_API_URL || '').replace(/\/+$/, '');
const metadataPath = process.env.METADATA_PATH || '';

if (!baseUrl) {
  throw new Error('MARKETPLACE_API_URL is required.');
}
if (!metadataPath) {
  throw new Error('METADATA_PATH is required.');
}

const rawMetadata = fs.readFileSync(metadataPath, 'utf8');
let metadata = JSON.parse(rawMetadata);
if (typeof metadata === 'string') {
  metadata = JSON.parse(metadata);
}

const id = String(metadata.id || '').trim();
const name = String(metadata.name || '').trim();
const listenPath = String(metadata.listen_path || '').trim();

if (!id || !name || !listenPath) {
  throw new Error('metadata must include id, name, and listen_path.');
}

const fetchJSON = async (url) => {
  const res = await fetch(url, { headers: { 'Accept': 'application/json' } });
  if (!res.ok) {
    throw new Error(`Failed to fetch ${url}: ${res.status}`);
  }
  return res.json();
};

const main = async () => {
  const listUrl = `${baseUrl}/api/integrations?latest=true`;
  const payload = await fetchJSON(listUrl);
  const integrations = Array.isArray(payload.integrations) ? payload.integrations : [];

  const conflicts = [];
  for (const entry of integrations) {
    if (!entry || !entry.id) continue;
    if (String(entry.id) === id) continue;
    const entryName = String(entry.name || '').trim();
    const entryListenPath = String(entry.listen_path || '').trim();
    if (entryName === name) {
      conflicts.push({
        field: 'name',
        id: entry.id,
        value: entryName,
        conflicting: name,
      });
    }
    if (entryListenPath === listenPath) {
      conflicts.push({
        field: 'listen_path',
        id: entry.id,
        value: entryListenPath,
        conflicting: listenPath,
      });
    }
  }

  if (conflicts.length) {
    console.error('Marketplace uniqueness check failed:');
    for (const conflict of conflicts) {
      console.error(`- ${conflict.field} conflict with ${conflict.id}: ${conflict.value}`);
    }
    console.error(`Requested name: ${name}`);
    console.error(`Requested listen_path: ${listenPath}`);
    process.exit(1);
  }

  console.log('Marketplace uniqueness check passed.');
};

main().catch((err) => {
  console.error(err.message || err);
  process.exit(1);
});
