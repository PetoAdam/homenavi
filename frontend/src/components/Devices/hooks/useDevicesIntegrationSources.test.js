import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../../services/deviceHubService', () => ({
  listIntegrations: vi.fn(),
}));

import { listIntegrations } from '../../../services/deviceHubService';
import {
  buildProtocolDisplayNames,
  deviceHubIntegrationsQueryOptions,
  reloadDevicesIntegrationSources,
} from './useDevicesIntegrationSources';
import { queryKeys } from '../../../state/queryKeys';

describe('deviceHubIntegrationsQueryOptions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('builds a stable query key and resolves integrations data', async () => {
    listIntegrations.mockResolvedValue({ success: true, data: [{ protocol: 'zigbee' }] });

    const options = deviceHubIntegrationsQueryOptions('abcdefghijklmnopqrstuvwxyz', { enabled: false });

    expect(options.queryKey).toEqual(queryKeys.deviceHub.integrations('klmnopqrstuvwxyz'));
    expect(options.enabled).toBe(false);
    await expect(options.queryFn()).resolves.toEqual([{ protocol: 'zigbee' }]);
  });
});

describe('buildProtocolDisplayNames', () => {
  it('maps registry protocol metadata into display names', () => {
    expect(buildProtocolDisplayNames({
      integrations: [
        { device_extension: { protocol: ' ZigBee ' }, display_name: 'Zigbee Bridge' },
        { device_extension: { protocol: 'matter' }, display_name: 'Matter Hub' },
      ],
    })).toEqual({
      zigbee: 'Zigbee Bridge',
      matter: 'Matter Hub',
    });
  });
});

describe('reloadDevicesIntegrationSources', () => {
  it('refreshes both device-hub and registry integration sources', async () => {
    const deviceHubIntegrationsQuery = { refetch: vi.fn().mockResolvedValue({ data: [] }) };
    const integrationRegistryQuery = { refetch: vi.fn().mockResolvedValue({ data: { integrations: [] } }) };

    await reloadDevicesIntegrationSources({ deviceHubIntegrationsQuery, integrationRegistryQuery });

    expect(deviceHubIntegrationsQuery.refetch).toHaveBeenCalledTimes(1);
    expect(integrationRegistryQuery.refetch).toHaveBeenCalledTimes(1);
  });
});
