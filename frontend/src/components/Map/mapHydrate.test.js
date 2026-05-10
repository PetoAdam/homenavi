import { describe, expect, it } from 'vitest';

import { mergeDevicePlacementsFromErs, mergeRoomsFromErs } from './mapHydrate';

describe('mergeRoomsFromErs', () => {
  it('replaces stale local-only rooms with ERS rooms to avoid duplicate map overlays', () => {
    const prevLayout = {
      rooms: [
        {
          id: 'old-local-room',
          name: 'Old Room',
          points: [{ x: 0, y: 0 }, { x: 10, y: 0 }, { x: 10, y: 10 }],
          wallLengths: [],
        },
      ],
    };

    const ersRooms = [
      {
        id: 'ers-room-1',
        name: 'Living Room',
        meta: {
          map: {
            points: [{ x: 50, y: 50 }, { x: 90, y: 50 }, { x: 90, y: 90 }],
            wall_lengths: [40, 40, 56.6],
          },
        },
      },
    ];

    expect(mergeRoomsFromErs(prevLayout, ersRooms)).toEqual({
      rooms: [
        {
          id: 'ers-room-1',
          name: 'Living Room',
          points: [{ x: 50, y: 50 }, { x: 90, y: 50 }, { x: 90, y: 90 }],
          wallLengths: [40, 40, 56.6],
        },
      ],
    });
  });

  it('keeps ERS room ids stable while filling missing geometry from local layout', () => {
    const prevLayout = {
      rooms: [
        {
          id: 'room-1',
          name: 'Kitchen',
          points: [{ x: 5, y: 5 }, { x: 25, y: 5 }, { x: 25, y: 20 }],
          wallLengths: [20, 15, 25],
        },
      ],
    };

    const ersRooms = [
      {
        id: 'room-1',
        name: 'Kitchen',
        meta: {},
      },
    ];

    expect(mergeRoomsFromErs(prevLayout, ersRooms)).toEqual(prevLayout);
  });
});

describe('mergeDevicePlacementsFromErs', () => {
  it('drops orphaned local placements that no longer exist in ERS inventory', () => {
    const prevLayout = {
      devicePlacements: {
        'old-long-hdp-id': { roomId: 'room-1', x: 10, y: 20 },
      },
    };

    const devicesForPalette = [
      { ersId: 'ers-device-1', hdpId: 'emodul/module-a/zone/1', hdpIds: ['emodul/module-a/zone/1'] },
    ];

    expect(mergeDevicePlacementsFromErs(prevLayout, devicesForPalette)).toEqual({
      devicePlacements: {},
    });
  });

  it('migrates legacy hdp-keyed placements onto the canonical ers device key', () => {
    const prevLayout = {
      devicePlacements: {
        'emodul/module-a/zone/1': { roomId: 'room-1', x: 25, y: 35 },
      },
    };

    const devicesForPalette = [
      {
        ersId: 'ers-device-1',
        id: 'ers-device-1',
        hdpId: 'emodul/module-a/zone/1',
        hdpIds: ['emodul/module-a/zone/1'],
        room_id: 'room-1',
      },
    ];

    expect(mergeDevicePlacementsFromErs(prevLayout, devicesForPalette)).toEqual({
      devicePlacements: {
        'ers-device-1': { roomId: 'room-1', x: 25, y: 35 },
      },
    });
  });

  it('prefers ERS placement metadata over stale local coordinates', () => {
    const prevLayout = {
      devicePlacements: {
        'ers-device-1': { roomId: 'room-1', x: 10, y: 20 },
      },
    };

    const devicesForPalette = [
      {
        ersId: 'ers-device-1',
        id: 'ers-device-1',
        hdpId: 'emodul/module-a/zone/1',
        hdpIds: ['emodul/module-a/zone/1'],
        room_id: 'room-2',
        meta: { map: { x: 50, y: 60 } },
      },
    ];

    expect(mergeDevicePlacementsFromErs(prevLayout, devicesForPalette)).toEqual({
      devicePlacements: {
        'ers-device-1': { roomId: 'room-2', x: 50, y: 60 },
      },
    });
  });
});