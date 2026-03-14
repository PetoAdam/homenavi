import http from './httpClient';

const DEFAULT_INTEGRATION_OPERATION_TIMEOUT_MS = 600000;

function parsePositiveTimeoutMs(value) {
  const parsed = Number.parseInt(String(value ?? '').trim(), 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

function getRuntimeConfig() {
  if (typeof window === 'undefined') return {};
  return window.__HOMENAVI_RUNTIME_CONFIG__ || {};
}

function getIntegrationOperationTimeoutMs() {
  const runtimeConfig = getRuntimeConfig();
  return (
    parsePositiveTimeoutMs(runtimeConfig.integrationOperationTimeoutMs)
    ?? parsePositiveTimeoutMs(runtimeConfig.integrationsInstallTimeoutMs)
    ?? parsePositiveTimeoutMs(import.meta.env?.VITE_INTEGRATION_OPERATION_TIMEOUT_MS)
    ?? parsePositiveTimeoutMs(import.meta.env?.VITE_INTEGRATIONS_INSTALL_TIMEOUT_MS)
    ?? DEFAULT_INTEGRATION_OPERATION_TIMEOUT_MS
  );
}

function getMarketplaceApiBase() {
  const runtimeConfig = getRuntimeConfig();
  return (
    runtimeConfig.marketplaceApiBase
    || import.meta.env?.VITE_MARKETPLACE_API_BASE
    || 'https://marketplace.homenavi.org'
  ).replace(/\/+$/, '');
}

export async function getIntegrationRegistry({ q, page, pageSize } = {}) {
  const params = new URLSearchParams();
  params.set('ts', String(Date.now()));
  if (q) params.set('q', q);
  if (page) params.set('page', String(page));
  if (pageSize) params.set('page_size', String(pageSize));
  return http.get(`/integrations/registry.json?${params.toString()}`);
}

export async function reloadIntegrations() {
  return http.post('/integrations/reload');
}

export async function restartAllIntegrations() {
  return http.post('/integrations/restart-all');
}

export async function restartIntegration(id) {
  return http.post(`/integrations/restart/${id}`);
}

export async function setIntegrationSecrets(id, secrets) {
  return http.put(`/integrations/${id}/api/admin/secrets`, { secrets });
}

export async function detectIntegrationSetupCapability(id) {
  if (!id) return { success: false, capable: false, error: 'Missing integration id' };
  const res = await http.get(`/integrations/${id}/api/admin/setup`);
  if (res.success) {
    return { success: true, capable: true };
  }
  if (res.status === 401 || res.status === 403) {
    return { success: true, capable: true };
  }
  if (res.status === 404) {
    return { success: true, capable: false };
  }
  return { success: false, capable: false, error: res.error, status: res.status };
}

export async function getIntegrationMarketplace() {
  const params = new URLSearchParams();
  params.set('ts', String(Date.now()));
  const base = getMarketplaceApiBase();
  return http.get(`${base}/api/integrations?${params.toString()}`);
}

export async function incrementMarketplaceDownloads(id) {
  if (!id) return { success: false, error: 'Missing integration id' };
  const base = getMarketplaceApiBase();
  return http.post(`${base}/api/integrations/${encodeURIComponent(id)}/downloads`, {});
}

export async function installIntegration(id, upstream, compose) {
  const payload = { id };
  if (upstream) {
    payload.upstream = upstream;
  }
  if (compose?.compose_file) {
    payload.compose_file = compose.compose_file;
  }
  if (compose?.version) {
    payload.version = compose.version;
  }
  if (typeof compose?.auto_update === 'boolean') {
    payload.auto_update = compose.auto_update;
  }
  return http.post('/integrations/install', payload, { timeout: getIntegrationOperationTimeoutMs() });
}

export async function uninstallIntegration(id) {
  return http.post('/integrations/uninstall', { id });
}

export async function getIntegrationInstallStatus(id) {
  return http.get(`/integrations/install-status/${id}`);
}

export async function getIntegrationUpdates(refresh = false) {
  const params = new URLSearchParams();
  if (refresh) params.set('refresh', 'true');
  const query = params.toString();
  return http.get(`/integrations/updates${query ? `?${query}` : ''}`);
}

export async function updateIntegration(id) {
  return http.post('/integrations/update', { id }, { timeout: getIntegrationOperationTimeoutMs() });
}

export async function setIntegrationAutoUpdate(id, autoUpdate) {
  return http.post('/integrations/update-policy', { id, auto_update: Boolean(autoUpdate) });
}
