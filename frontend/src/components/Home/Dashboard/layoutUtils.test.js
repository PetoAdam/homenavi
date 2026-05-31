import { describe, expect, it } from 'vitest';

import { normalizeLayoutHeights, pickSourceBreakpoint } from './layoutUtils';

describe('pickSourceBreakpoint', () => {
  it('prefers the widest populated breakpoint', () => {
    expect(pickSourceBreakpoint({ sm: [{ i: 'a' }], xl: [{ i: 'a' }] })).toBe('xl');
  });
});

describe('normalizeLayoutHeights', () => {
  it('applies the preferred breakpoint height to every layout', () => {
    const layouts = {
      lg: [{ i: 'weather', x: 0, y: 0, w: 1, h: 5 }],
      xxs: [{ i: 'weather', x: 0, y: 0, w: 1, h: 8 }],
    };

    expect(normalizeLayoutHeights(layouts, { preferredBreakpoint: 'lg' })).toEqual({
      lg: [{ i: 'weather', x: 0, y: 0, w: 1, h: 5 }],
      xxs: [{ i: 'weather', x: 0, y: 0, w: 1, h: 5 }],
    });
  });

  it('falls back to the first available breakpoint when the preferred one is missing', () => {
    const layouts = {
      sm: [{ i: 'device', x: 0, y: 0, w: 1, h: 4 }],
      xxs: [{ i: 'device', x: 0, y: 0, w: 1, h: 6 }],
    };

    expect(normalizeLayoutHeights(layouts, { preferredBreakpoint: 'lg' })).toEqual({
      sm: [{ i: 'device', x: 0, y: 0, w: 1, h: 4 }],
      xxs: [{ i: 'device', x: 0, y: 0, w: 1, h: 4 }],
    });
  });

  it('respects minH when the canonical height is smaller', () => {
    const layouts = {
      lg: [{ i: 'map', x: 0, y: 0, w: 2, h: 3 }],
      xxs: [{ i: 'map', x: 0, y: 0, w: 1, h: 7, minH: 5 }],
    };

    expect(normalizeLayoutHeights(layouts, { preferredBreakpoint: 'lg' })).toEqual({
      lg: [{ i: 'map', x: 0, y: 0, w: 2, h: 3 }],
      xxs: [{ i: 'map', x: 0, y: 0, w: 1, h: 5, minH: 5 }],
    });
  });
});