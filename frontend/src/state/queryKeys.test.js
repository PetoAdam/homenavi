import { describe, expect, it } from 'vitest';
import { normalizeRegistryParams, queryKeys } from './queryKeys';

describe('normalizeRegistryParams', () => {
  it('normalizes blanks and invalid pagination values to defaults', () => {
    expect(normalizeRegistryParams({ q: '   ', page: 0, pageSize: 'x' })).toEqual({
      q: '',
      page: 1,
      pageSize: 10,
    });
  });

  it('keeps valid values and trims query text', () => {
    expect(normalizeRegistryParams({ q: '  zigbee  ', page: '3', pageSize: '20' })).toEqual({
      q: 'zigbee',
      page: 3,
      pageSize: 20,
    });
  });
});

describe('queryKeys.integrations', () => {
  it('builds stable registry keys for semantically equal params', () => {
    const left = queryKeys.integrations.registry({ q: 'abc', page: 1, pageSize: 10 });
    const right = queryKeys.integrations.registry({ q: ' abc ', page: '1', pageSize: '10' });
    expect(left).toEqual(right);
  });

  it('builds install status keys with normalized ids', () => {
    expect(queryKeys.integrations.installStatus('  spotify ')).toEqual([
      'integrations',
      'install-status',
      'spotify',
    ]);
  });
});

describe('queryKeys.dashboard', () => {
  it('normalizes dashboard scope fragments', () => {
    expect(queryKeys.dashboard.me('  token-fragment  ')).toEqual(['dashboard', 'me', 'token-fragment']);
    expect(queryKeys.dashboard.catalog('')).toEqual(['dashboard', 'catalog', 'current']);
  });
});

describe('queryKeys.ers', () => {
  it('builds inventory keys with a normalized scope', () => {
    expect(queryKeys.ers.inventory('  resident-1  ')).toEqual(['ers', 'inventory', 'resident-1']);
  });
});
