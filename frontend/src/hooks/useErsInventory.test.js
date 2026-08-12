import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/entityRegistryService', () => ({
  listErsDevices: vi.fn(),
  listErsGroups: vi.fn(),
  listErsRooms: vi.fn(),
  listErsTags: vi.fn(),
}));

import {
  listErsDevices,
  listErsGroups,
  listErsRooms,
  listErsTags,
} from '../services/entityRegistryService';
import {
  ersInventoryQueryOptions,
  fetchErsInventorySnapshot,
} from './useErsInventory';
import { queryKeys } from '../state/queryKeys';

describe('ersInventoryQueryOptions', () => {
  it('builds a scoped ERS inventory key', () => {
    const options = ersInventoryQueryOptions('abcdefghijklmnopqrstuvwxyz', { enabled: false });
    expect(options.queryKey).toEqual(queryKeys.ers.inventory('klmnopqrstuvwxyz'));
    expect(options.enabled).toBe(false);
  });
});

describe('fetchErsInventorySnapshot', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns devices and tolerates partial failures on secondary resources', async () => {
    listErsDevices.mockResolvedValue({ success: true, data: [{ id: 'device-1' }] });
    listErsGroups.mockResolvedValue({ success: false, error: 'groups' });
    listErsRooms.mockResolvedValue({ success: true, data: [{ id: 'room-1' }] });
    listErsTags.mockResolvedValue({ success: false, error: 'tags' });

    await expect(fetchErsInventorySnapshot('token')).resolves.toEqual({
      devices: [{ id: 'device-1' }],
      groups: [],
      rooms: [{ id: 'room-1' }],
      tags: [],
    });
  });

  it('throws when the primary ERS devices request fails', async () => {
    listErsDevices.mockResolvedValue({ success: false, error: 'devices failed' });
    listErsGroups.mockResolvedValue({ success: true, data: [] });
    listErsRooms.mockResolvedValue({ success: true, data: [] });
    listErsTags.mockResolvedValue({ success: true, data: [] });

    await expect(fetchErsInventorySnapshot('token')).rejects.toThrow('devices failed');
  });
});
