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
  return http.get(`/integrations/marketplace.json?${params.toString()}`);
}

export async function installIntegration(id) {
  return http.post('/integrations/install', { id });
}

export async function uninstallIntegration(id) {
  return http.post('/integrations/uninstall', { id });
}

export async function getIntegrationInstallStatus(id) {
  return http.get(`/integrations/install-status/${id}`);
}
