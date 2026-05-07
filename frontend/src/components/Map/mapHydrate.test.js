import { describe, expect, it } from 'vitest';

import { mergeRoomsFromErs } from './mapHydrate';

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