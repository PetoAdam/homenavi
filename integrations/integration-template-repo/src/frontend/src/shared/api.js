function resolveIntegrationBasePath() {
  if (typeof window === 'undefined') return '';
  const path = window.location.pathname || '';
  const parts = path.split('/').filter(Boolean);
  const idx = parts.indexOf('integrations');
  if (idx >= 0 && parts[idx + 1]) {
    return `/${['integrations', parts[idx + 1]].join('/')}`;
  }
  return '';
}

function buildUrl(path) {
  const base = resolveIntegrationBasePath();
  if (!path.startsWith('/')) return `${base}/${path}`;
  return `${base}${path}`;
}

export async function fetchCounter() {
  const resp = await fetch(buildUrl('/api/counter'), { method: 'GET' });
  if (!resp.ok) throw new Error('Failed to load counter');
  return resp.json();
}

export async function incrementCounter(delta = 1) {
  const resp = await fetch(buildUrl('/api/counter'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ delta })
  });
  if (!resp.ok) throw new Error('Failed to update counter');
  return resp.json();
}
