import http from './httpClient';

export async function getIntegrationRegistry() {
  return http.get('/integrations/registry.json');
}
