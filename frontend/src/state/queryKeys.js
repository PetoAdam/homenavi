function cleanText(value, fallback = '') {
  const normalized = typeof value === 'string' ? value.trim() : '';
  return normalized || fallback;
}

function cleanPositiveInt(value, fallback) {
  const parsed = Number.parseInt(String(value ?? ''), 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function normalizeRegistryParams(params = {}) {
  return {
    q: cleanText(params.q, ''),
    page: cleanPositiveInt(params.page, 1),
    pageSize: cleanPositiveInt(params.pageSize, 10),
  };
}

export const queryKeys = {
  dashboard: {
    root: ['dashboard'],
    me: (scope = 'current') => ['dashboard', 'me', cleanText(scope, 'current')],
    catalog: (scope = 'current') => ['dashboard', 'catalog', cleanText(scope, 'current')],
  },
  ers: {
    root: ['ers'],
    inventory: (scope = 'current') => ['ers', 'inventory', cleanText(scope, 'current')],
  },
  deviceHub: {
    root: ['deviceHub'],
    integrations: (scope = 'current') => ['deviceHub', 'integrations', cleanText(scope, 'current')],
  },
  integrations: {
    root: ['integrations'],
    registry: (params = {}) => ['integrations', 'registry', normalizeRegistryParams(params)],
    marketplace: () => ['integrations', 'marketplace'],
    updates: () => ['integrations', 'updates'],
    installStatus: (id) => ['integrations', 'install-status', cleanText(id)],
  },
};

export { normalizeRegistryParams };
