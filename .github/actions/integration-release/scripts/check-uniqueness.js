const fs = require('fs');

const baseUrl = (process.env.MARKETPLACE_API_URL || '').replace(/\/+$/, '');
const metadataPath = process.env.METADATA_PATH || '';
const retryCount = Number.parseInt(process.env.MARKETPLACE_RETRY_COUNT || '3', 10);
const retryDelayMs = Number.parseInt(process.env.MARKETPLACE_RETRY_DELAY_MS || '1500', 10);

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

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const isRetryableStatus = (status) => status >= 500;

const fetchJSON = async (url) => {
  let lastError = null;
  const attempts = Number.isFinite(retryCount) && retryCount > 0 ? retryCount : 3;

  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      const res = await fetch(url, { headers: { 'Accept': 'application/json' } });
      if (res.ok) {
        return res.json();
      }

      const error = new Error(`Failed to fetch ${url}: ${res.status}`);
      error.status = res.status;
      error.retryable = isRetryableStatus(res.status);
      lastError = error;
    } catch (err) {
      const wrapped = new Error(err && err.message ? err.message : String(err));
      wrapped.retryable = true;
      lastError = wrapped;
    }

    if (!lastError.retryable || attempt === attempts) {
      break;
    }

    const delay = retryDelayMs * attempt;
    console.warn(`Marketplace lookup failed (attempt ${attempt}/${attempts}). Retrying in ${delay}ms...`);
    await sleep(delay);
  }

  throw lastError || new Error('Unknown marketplace lookup error');
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
  const status = Number(err && err.status);
  if ((Number.isFinite(status) && status >= 500) || err.retryable) {
    console.warn(`Marketplace uniqueness check skipped due to transient marketplace error: ${err.message || err}`);
    process.exit(0);
  }

  console.error(err.message || err);
  process.exit(1);
});
