const fs = require('fs');

const metadataPath = process.env.METADATA_PATH || '';

if (!metadataPath) {
  throw new Error('METADATA_PATH is required');
}

const metadata = JSON.parse(fs.readFileSync(metadataPath, 'utf8'));
const deployment = metadata && typeof metadata === 'object' ? metadata.deployment_artifacts || {} : {};
const compose = deployment && typeof deployment === 'object' ? deployment.compose || {} : {};
const composePath = String(compose.file || metadata.compose_file || '').trim();

if (composePath) {
  const fileName = composePath.split('/').pop() || '';
  if (fileName !== 'docker-compose.integration.yml') {
    throw new Error('compose artifact must point to compose/docker-compose.integration.yml');
  }
}

process.stdout.write(composePath);
