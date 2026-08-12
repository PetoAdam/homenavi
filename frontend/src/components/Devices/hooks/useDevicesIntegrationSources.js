import { useCallback, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { listIntegrations as listDeviceHubIntegrations } from '../../../services/deviceHubService';
import { queryKeys } from '../../../state/queryKeys';
import { useIntegrationRegistryQuery } from '../../../features/integrations/hooks/useIntegrationQueries';

function normalizeProtocolKey(protocol) {
  return (protocol || '').toString().trim().toLowerCase();
}

function scopeFromAccessToken(accessToken) {
  return accessToken ? accessToken.slice(-16) : 'anonymous';
}

export function deviceHubIntegrationsQueryOptions(accessToken, { enabled = true } = {}) {
  return {
    queryKey: queryKeys.deviceHub.integrations(scopeFromAccessToken(accessToken)),
    queryFn: async () => {
      const response = await listDeviceHubIntegrations(accessToken);
      if (!response?.success) {
        throw new Error(response?.error || 'Unable to load integrations');
      }
      return Array.isArray(response.data) ? response.data : [];
    },
    enabled,
  };
}

export function buildProtocolDisplayNames(registryData) {
  const next = {};
  const list = Array.isArray(registryData?.integrations) ? registryData.integrations : [];
  list.forEach((integration) => {
    const protocol = normalizeProtocolKey(integration?.device_extension?.protocol);
    const name = typeof integration?.display_name === 'string' ? integration.display_name.trim() : '';
    if (protocol && name) {
      next[protocol] = name;
    }
  });
  return next;
}

export async function reloadDevicesIntegrationSources({ deviceHubIntegrationsQuery, integrationRegistryQuery }) {
  return Promise.all([
    deviceHubIntegrationsQuery.refetch(),
    integrationRegistryQuery.refetch(),
  ]);
}

export function useDevicesIntegrationSources({ enabled, accessToken }) {
  const deviceHubIntegrationsQuery = useQuery(
    deviceHubIntegrationsQueryOptions(accessToken, { enabled }),
  );

  const integrationRegistryQuery = useIntegrationRegistryQuery(
    { page: 1, pageSize: 250 },
    { enabled },
  );

  const integrations = useMemo(
    () => (Array.isArray(deviceHubIntegrationsQuery.data) ? deviceHubIntegrationsQuery.data : []),
    [deviceHubIntegrationsQuery.data],
  );

  const protocolDisplayNames = useMemo(
    () => buildProtocolDisplayNames(integrationRegistryQuery.data),
    [integrationRegistryQuery.data],
  );

  const reload = useCallback(
    async () => reloadDevicesIntegrationSources({ deviceHubIntegrationsQuery, integrationRegistryQuery }),
    [deviceHubIntegrationsQuery, integrationRegistryQuery],
  );

  return {
    integrations,
    integrationsLoading: deviceHubIntegrationsQuery.isFetching || integrationRegistryQuery.isFetching,
    integrationsError: deviceHubIntegrationsQuery.error?.message || null,
    protocolDisplayNames,
    reload,
    deviceHubIntegrationsQuery,
    integrationRegistryQuery,
  };
}
