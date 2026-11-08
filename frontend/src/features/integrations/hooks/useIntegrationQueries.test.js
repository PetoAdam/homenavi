import { QueryClient } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';
import { getIntegrationRegistry } from '../../../services/integrationService';
import {
  integrationMarketplaceQueryOptions,
  integrationRegistryQueryOptions,
  integrationUpdatesQueryOptions,
} from './useIntegrationQueries';
import { queryKeys } from '../../../state/queryKeys';

vi.mock('../../../services/integrationService', () => ({
  getIntegrationMarketplace: vi.fn(),
  getIntegrationRegistry: vi.fn(),
  getIntegrationUpdates: vi.fn(),
}));

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
    expect(options.refetchInterval).toBe(60000);
  });

  it('fetches updated registry data even when cached data is fresh', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { staleTime: 5000 } } });
    const params = { page: 1, pageSize: 10 };
    const oldData = { integrations: [{ id: 'emodul', update_available: false }] };
    const newData = { integrations: [{ id: 'emodul', update_available: true }] };
    client.setQueryData(queryKeys.integrations.registry(params), oldData);
    getIntegrationRegistry.mockResolvedValueOnce({ success: true, data: newData });

    const result = await client.fetchQuery(integrationRegistryQueryOptions(params, { forceFresh: true }));

    expect(result).toEqual(newData);
    expect(client.getQueryData(queryKeys.integrations.registry(params))).toEqual(newData);
    client.clear();
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
