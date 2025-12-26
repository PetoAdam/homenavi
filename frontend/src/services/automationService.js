import http from './httpClient';

export async function listWorkflows(token) {
  return http.get('/api/automation/workflows', { token });
}

export async function getNodes(token) {
  return http.get('/api/automation/nodes', { token });
}

export async function createWorkflow(payload, token) {
  return http.post('/api/automation/workflows', payload, { token });
}

export async function updateWorkflow(id, payload, token) {
  return http.put(`/api/automation/workflows/${id}`, payload, { token });
}

export async function enableWorkflow(id, token) {
  return http.post(`/api/automation/workflows/${id}/enable`, {}, { token });
}

export async function disableWorkflow(id, token) {
  return http.post(`/api/automation/workflows/${id}/disable`, {}, { token });
}

export async function runWorkflow(id, token) {
  return http.post(`/api/automation/workflows/${id}/run`, {}, { token });
}

export async function listRuns(id, token, limit) {
  const q = Number.isFinite(limit) && limit > 0 ? `?limit=${encodeURIComponent(limit)}` : '';
  return http.get(`/api/automation/workflows/${id}/runs${q}`, { token });
}

export async function getRun(runId, token) {
  return http.get(`/api/automation/runs/${runId}`, { token });
}

export async function deleteWorkflow(id, token) {
  return http.del(`/api/automation/workflows/${id}`, { token });
}
