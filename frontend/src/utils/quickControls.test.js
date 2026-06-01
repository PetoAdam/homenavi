import { describe, expect, it } from 'vitest';

import { resolveQuickControlItems } from './quickControls';

describe('resolveQuickControlItems', () => {
  it('keeps selected groups as a single quick control entity', () => {
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
      {
        id: 'zigbee/device-b',
        ersId: 'ers-device-b',
        hdpId: 'zigbee/device-b',
        state: { on: true },
        capabilities: [
          { id: 'on', property: 'on', kind: 'binary', value_type: 'boolean', access: { write: true } },
        ],
      },
    ];

    const ersGroups = [
      {
        id: 'group-1',
        name: 'Living room',
        deviceIds: ['ers-device-a', 'ers-device-b'],
        devices: [
          {
            id: 'ers-device-a',
            ersId: 'ers-device-a',
          },
          {
            id: 'ers-device-b',
            ersId: 'ers-device-b',
          },
        ],
      },
    ];

    const result = resolveQuickControlItems({
      selectedIds: [],
      selectedGroupIds: ['group-1'],
      ersDevices,
      ersGroups,
      realtimeDevices: [],
    });

    expect(result).toHaveLength(1);
    expect(result[0]).toMatchObject({
      kind: 'group',
      id: 'group-1',
      group: { name: 'Living room' },
      toggleValue: true,
      mixed: false,
    });
  });

  it('keeps a direct device tile and a group tile as separate quick controls', () => {
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

    const result = resolveQuickControlItems({
      selectedIds: ['zigbee/device-a'],
      selectedGroupIds: ['group-1'],
      ersDevices,
      ersGroups,
      realtimeDevices: [],
    });

    expect(result).toHaveLength(2);
    expect(result[0]).toMatchObject({ kind: 'device', commandId: 'zigbee/device-a' });
    expect(result[1]).toMatchObject({ kind: 'group', id: 'group-1' });
  });
});