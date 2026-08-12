import { describe, expect, it } from 'vitest';
import {
  integrationMarketplaceQueryOptions,
  integrationRegistryQueryOptions,
  integrationUpdatesQueryOptions,
} from './useIntegrationQueries';
import { queryKeys } from '../../../state/queryKeys';

describe('integrationRegistryQueryOptions', () => {
  it('produces normalized query keys and honors enabled flags', () => {
    const options = integrationRegistryQueryOptions(
      { q: '  test ', page: '2', pageSize: '25' },
      { enabled: false }
    );

    expect(options.queryKey).toEqual(
      queryKeys.integrations.registry({ q: 'test', page: 2, pageSize: 25 })
    );
    expect(options.enabled).toBe(false);
  });
});

describe('integrationMarketplaceQueryOptions', () => {
  it('uses the marketplace key and a longer stale time', () => {
    const options = integrationMarketplaceQueryOptions({ enabled: true });
    expect(options.queryKey).toEqual(queryKeys.integrations.marketplace());
    expect(options.enabled).toBe(true);
    expect(options.staleTime).toBe(30000);
  });
});

describe('integrationUpdatesQueryOptions', () => {
  it('adds refresh suffix for forced refresh query', () => {
    const refreshOptions = integrationUpdatesQueryOptions({ refresh: true, enabled: true });
    const normalOptions = integrationUpdatesQueryOptions({ refresh: false, enabled: true });

    expect(refreshOptions.queryKey).toEqual([...queryKeys.integrations.updates(), 'refresh']);
    expect(normalOptions.queryKey).toEqual(queryKeys.integrations.updates());
  });
});
