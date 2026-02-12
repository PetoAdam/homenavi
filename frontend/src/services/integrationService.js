import http from './httpClient';

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

export async function getIntegrationMarketplace() {
  const params = new URLSearchParams();
  params.set('ts', String(Date.now()));
  const base = (import.meta.env?.VITE_MARKETPLACE_API_BASE || 'https://marketplace.homenavi.org').replace(/\/+$/, '');
  return http.get(`${base}/api/integrations?${params.toString()}`);
}

export async function incrementMarketplaceDownloads(id) {
  if (!id) return { success: false, error: 'Missing integration id' };
  const base = (import.meta.env?.VITE_MARKETPLACE_API_BASE || 'https://marketplace.homenavi.org').replace(/\/+$/, '');
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
  return http.post('/integrations/install', payload);
}

export async function uninstallIntegration(id) {
  return http.post('/integrations/uninstall', { id });
}

export async function getIntegrationInstallStatus(id) {
  return http.get(`/integrations/install-status/${id}`);
}
