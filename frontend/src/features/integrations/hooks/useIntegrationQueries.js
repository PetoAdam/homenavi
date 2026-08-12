import { useQuery } from '@tanstack/react-query';
import {
  getIntegrationMarketplace,
  getIntegrationRegistry,
  getIntegrationUpdates,
} from '../../../services/integrationService';
import { normalizeRegistryParams, queryKeys } from '../../../state/queryKeys';

function asQueryData(response, fallbackMessage) {
  if (response?.success) return response.data;
  throw new Error(response?.error || fallbackMessage);
}

export function integrationRegistryQueryOptions(params = {}, { enabled = true } = {}) {
  const normalized = normalizeRegistryParams(params);
  return {
    queryKey: queryKeys.integrations.registry(normalized),
    queryFn: async () => asQueryData(
      await getIntegrationRegistry(normalized),
      'Failed to load integrations'
    ),
    enabled,
  };
}

export function integrationMarketplaceQueryOptions({ enabled = true } = {}) {
  return {
    queryKey: queryKeys.integrations.marketplace(),
    queryFn: async () => asQueryData(
      await getIntegrationMarketplace(),
      'Failed to load marketplace'
    ),
    enabled,
    staleTime: 30 * 1000,
  };
}

export function integrationUpdatesQueryOptions({ refresh = false, enabled = true } = {}) {
  return {
    queryKey: refresh
      ? [...queryKeys.integrations.updates(), 'refresh']
      : queryKeys.integrations.updates(),
    queryFn: async () => asQueryData(
      await getIntegrationUpdates(refresh),
      'Failed to load integration updates'
    ),
    enabled,
  };
}

export function useIntegrationRegistryQuery(params = {}, options = {}) {
  return useQuery(integrationRegistryQueryOptions(params, options));
}

export function useIntegrationMarketplaceQuery(options = {}) {
  return useQuery(integrationMarketplaceQueryOptions(options));
}

export function useIntegrationUpdatesQuery(options = {}) {
  return useQuery(integrationUpdatesQueryOptions(options));
}
