import { describe, expect, it } from 'vitest';

import { resolveQuickControlDevices } from './quickControls';

describe('resolveQuickControlDevices', () => {
  it('resolves group members even when the group references ers ids but inventory is keyed by hdp ids', () => {
    const ersDevices = [
      {
        id: 'zigbee/device-a',
        ersId: 'ers-device-a',
        hdpId: 'zigbee/device-a',
        state: { on: true },
        capabilities: [
          { id: 'on', property: 'on', kind: 'binary', value_type: 'boolean', access: { write: true } },
        ],
      },
    ];

    const ersGroups = [
      {
        id: 'group-1',
        deviceIds: ['ers-device-a'],
        devices: [
          {
            id: 'ers-device-a',
            ersId: 'ers-device-a',
          },
        ],
      },
    ];

    const result = resolveQuickControlDevices({
      selectedIds: [],
      selectedGroupIds: ['group-1'],
      ersDevices,
      ersGroups,
      realtimeDevices: [],
    });

    expect(result).toHaveLength(1);
    expect(result[0]).toMatchObject({ ersId: 'ers-device-a', hdpId: 'zigbee/device-a' });
  });

  it('deduplicates the same command target across direct device and group selection', () => {
    const ersDevices = [
      {
        id: 'zigbee/device-a',
        ersId: 'ers-device-a',
        hdpId: 'zigbee/device-a',
        state: { on: true },
        capabilities: [
          { id: 'on', property: 'on', kind: 'binary', value_type: 'boolean', access: { write: true } },
        ],
      },
    ];

    const ersGroups = [
      {
        id: 'group-1',
        devices: [{ id: 'ers-device-a', ersId: 'ers-device-a' }],
      },
    ];

    const result = resolveQuickControlDevices({
      selectedIds: ['zigbee/device-a'],
      selectedGroupIds: ['group-1'],
      ersDevices,
      ersGroups,
      realtimeDevices: [],
    });

    expect(result).toHaveLength(1);
  });
});