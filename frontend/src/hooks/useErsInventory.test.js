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
  mergeErsDeviceWithRealtime,
} from './useErsInventory';
import { queryKeys } from '../state/queryKeys';

describe('ersInventoryQueryOptions', () => {
  it('builds a scoped ERS inventory key', () => {
    const options = ersInventoryQueryOptions('abcdefghijklmnopqrstuvwxyz', { enabled: false });
    expect(options.queryKey).toEqual(queryKeys.ers.inventory('klmnopqrstuvwxyz'));
    expect(options.enabled).toBe(false);
  });
});

describe('device presentation preferences', () => {
  it('applies compatible saved choices without mutating integration defaults', () => {
    const realtime = {
      inputs: [{ id: 'target', type: 'slider', range: { min: 0, max: 100 }, ui: { control: 'slider' } }],
      capabilities: [{ id: 'target', value_type: 'number', range: { min: 0, max: 100 }, ui: { display: 'metric' } }],
      state: { target: 42 },
    };
    const device = { id: 'ers-1', hdp_external_ids: ['demo/1'], meta: { presentation: { controls: { target: 'stepper' }, displays: { target: 'gauge' } } } };
    const result = mergeErsDeviceWithRealtime(device, new Map([['demo/1', realtime]]), new Map());
    expect(result.inputs[0].ui.control).toBe('stepper');
    expect(result.capabilities[0].ui.display).toBe('gauge');
    expect(realtime.inputs[0].ui.control).toBe('slider');
    expect(realtime.capabilities[0].ui.display).toBe('metric');
  });

  it('falls back to integration defaults for invalid or reset preferences', () => {
    const realtime = { inputs: [{ id: 'mode', type: 'select', options: [{ value: 'on' }], ui: { control: 'radio' } }], capabilities: [{ id: 'mode', value_type: 'enum', ui: { display: 'badge' } }] };
    const device = { id: 'ers-1', hdp_external_ids: ['demo/1'], meta: { presentation: { controls: { mode: 'stepper' }, displays: { mode: 'gauge' } } } };
    const result = mergeErsDeviceWithRealtime(device, new Map([['demo/1', realtime]]), new Map());
    expect(result.inputs[0].ui.control).toBe('radio');
    expect(result.capabilities[0].ui.display).toBe('badge');
    device.meta.presentation.controls.mode = null;
    expect(mergeErsDeviceWithRealtime(device, new Map([['demo/1', realtime]]), new Map()).inputs[0].ui.control).toBe('radio');
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
