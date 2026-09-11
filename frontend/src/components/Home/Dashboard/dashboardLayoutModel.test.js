import { describe, expect, it } from 'vitest';

import {
  appendLayoutItem,
  buildDenseLayout,
  createDashboardDocSnapshot,
  createEmptyLayoutsByCols,
  ensureLayoutsByCols,
  getDashboardColumnMode,
  normalizeLegacyLayoutsToCols,
  normalizeLayoutHeightsByMode,
  parseDashboardDocShape,
  resolveNearestSourceMode,
  serializeDashboardDoc,
} from './dashboardLayoutModel';

function buildWidgetTypeMap(items) {
  return new Map(items.map((item) => [item.instance_id, item.widget_type]));
}

function buildHeightMap(items) {
  return new Map(items.map((item) => [item.instance_id, item.defaultH || 4]));
}

function preferredWidthForType(widgetType, cols) {
  if (widgetType === 'homenavi.weather' || widgetType === 'homenavi.map') {
    return Math.min(cols, cols >= 2 ? 2 : 1);
  }
  return 1;
}

describe('dashboardLayoutModel', () => {
  it('maps viewport width to monotonic column modes', () => {
    expect(getDashboardColumnMode(1900)).toBe('4');
    expect(getDashboardColumnMode(1400)).toBe('3');
    expect(getDashboardColumnMode(900)).toBe('2');
    expect(getDashboardColumnMode(500)).toBe('1');
  });

  it('normalizes legacy breakpoint layouts into column-mode layouts', () => {
    const next = normalizeLegacyLayoutsToCols({
      lg: [{ i: 'a', x: 0, y: 0, w: 1, h: 4 }, { i: 'b', x: 1, y: 0, w: 1, h: 4 }, { i: 'c', x: 2, y: 0, w: 1, h: 4 }, { i: 'd', x: 3, y: 0, w: 1, h: 4 }],
      md: [{ i: 'a', x: 0, y: 0, w: 1, h: 4 }, { i: 'b', x: 1, y: 0, w: 1, h: 4 }, { i: 'c', x: 2, y: 0, w: 1, h: 4 }],
      sm: [{ i: 'a', x: 0, y: 0, w: 1, h: 4 }, { i: 'b', x: 1, y: 0, w: 1, h: 4 }],
      xxs: [{ i: 'a', x: 0, y: 0, w: 1, h: 4 }],
    });

    expect(next['4']).toHaveLength(4);
    expect(next['3']).toHaveLength(3);
    expect(next['2']).toHaveLength(2);
    expect(next['1']).toHaveLength(1);
  });

  it('builds dense layouts while preserving top-left priority ordering', () => {
    const items = [
      { instance_id: 'weather', widget_type: 'homenavi.weather', defaultH: 5 },
      { instance_id: 'device', widget_type: 'homenavi.device', defaultH: 4 },
      { instance_id: 'map', widget_type: 'homenavi.map', defaultH: 5 },
      { instance_id: 'automation', widget_type: 'homenavi.automation.manual_trigger', defaultH: 3 },
    ];
    const layout = buildDenseLayout({
      sourceLayout: [
        { i: 'weather', x: 0, y: 0, w: 2, h: 5 },
        { i: 'device', x: 2, y: 0, w: 1, h: 4 },
        { i: 'map', x: 0, y: 5, w: 2, h: 5 },
        { i: 'automation', x: 2, y: 5, w: 1, h: 3 },
      ],
      instanceIds: items.map((item) => item.instance_id),
      columnMode: '3',
      widgetTypeByInstanceId: buildWidgetTypeMap(items),
      preferredWidthForType,
      defaultHeightByInstanceId: buildHeightMap(items),
    });

    expect(layout[0].i).toBe('weather');
    expect(layout[0].w).toBe(2);
    expect(layout[1].i).toBe('device');
    expect(layout[2].i).toBe('map');
  });

  it('can auto-layout without resizing widget widths or heights', () => {
    const items = [
      { instance_id: 'weather', widget_type: 'homenavi.weather', defaultH: 5 },
      { instance_id: 'device', widget_type: 'homenavi.device', defaultH: 4 },
    ];
    const layout = buildDenseLayout({
      sourceLayout: [
        { i: 'weather', x: 0, y: 0, w: 1, h: 7 },
        { i: 'device', x: 1, y: 0, w: 1, h: 6 },
      ],
      instanceIds: items.map((item) => item.instance_id),
      columnMode: '3',
      widgetTypeByInstanceId: buildWidgetTypeMap(items),
      preferredWidthForType,
      defaultHeightByInstanceId: buildHeightMap(items),
      resizeWidths: false,
      resizeHeights: false,
    });

    expect(layout[0].w).toBe(1);
    expect(layout[0].h).toBe(7);
    expect(layout[1].w).toBe(1);
    expect(layout[1].h).toBe(6);
  });

  it('can auto-layout while growing widgets into open gaps', () => {
    const items = [
      { instance_id: 'weather', widget_type: 'homenavi.weather', defaultH: 5 },
      { instance_id: 'device', widget_type: 'homenavi.device', defaultH: 4 },
      { instance_id: 'map', widget_type: 'homenavi.map', defaultH: 5 },
    ];
    const layout = buildDenseLayout({
      sourceLayout: [
        { i: 'weather', x: 0, y: 0, w: 2, h: 5 },
        { i: 'device', x: 2, y: 0, w: 1, h: 4 },
        { i: 'map', x: 0, y: 6, w: 2, h: 2 },
      ],
      instanceIds: items.map((item) => item.instance_id),
      columnMode: '3',
      widgetTypeByInstanceId: buildWidgetTypeMap(items),
      preferredWidthForType,
      defaultHeightByInstanceId: buildHeightMap(items),
      resizeWidths: false,
      resizeHeights: false,
      growWidths: true,
      growHeights: true,
    });

    expect(layout[0]).toMatchObject({ i: 'weather', x: 0, y: 0, w: 2, h: 5 });
    expect(layout[1]).toMatchObject({ i: 'device', x: 2, y: 0, w: 1, h: 4 });
    expect(layout[2]).toMatchObject({ i: 'map', x: 0, y: 5, w: 3, h: 2 });
  });

  it('can auto-layout by shrinking oversized widgets before reorganizing', () => {
    const items = [
      { instance_id: 'weather', widget_type: 'homenavi.weather', defaultH: 5 },
      { instance_id: 'device', widget_type: 'homenavi.device', defaultH: 4 },
    ];
    const layout = buildDenseLayout({
      sourceLayout: [
        { i: 'weather', x: 0, y: 0, w: 3, h: 8 },
        { i: 'device', x: 0, y: 8, w: 2, h: 6 },
      ],
      instanceIds: items.map((item) => item.instance_id),
      columnMode: '3',
      widgetTypeByInstanceId: buildWidgetTypeMap(items),
      preferredWidthForType,
      defaultHeightByInstanceId: buildHeightMap(items),
      resizeWidths: false,
      resizeHeights: false,
      shrinkWidths: true,
      shrinkHeights: true,
      reorganize: true,
    });

    expect(layout[0]).toMatchObject({ i: 'weather', x: 0, y: 0, w: 2, h: 5 });
    expect(layout[1]).toMatchObject({ i: 'device', x: 2, y: 0, w: 1, h: 4 });
  });

  it('appends a new widget without changing existing geometry', () => {
    const next = appendLayoutItem({
      layout: [
        { i: 'weather', x: 0, y: 0, w: 2, h: 5 },
        { i: 'device', x: 2, y: 0, w: 1, h: 4 },
      ],
      columnMode: '3',
      instanceId: 'automation',
      width: 1,
      height: 3,
    });

    expect(next[0]).toMatchObject({ i: 'weather', x: 0, y: 0, w: 2, h: 5 });
    expect(next[1]).toMatchObject({ i: 'device', x: 2, y: 0, w: 1, h: 4 });
    expect(next[2]).toMatchObject({ i: 'automation', x: 0, y: 5, w: 1, h: 3 });
  });

  it('picks the nearest stored source mode when generating a missing layout', () => {
    expect(resolveNearestSourceMode({ '4': [], '3': [{ i: 'a', x: 0, y: 0, w: 1, h: 4 }], '2': [], '1': [] }, '4')).toBe('3');
    expect(resolveNearestSourceMode({ '4': [], '3': [], '2': [{ i: 'a', x: 0, y: 0, w: 1, h: 4 }], '1': [] }, '1')).toBe('2');
  });

  it('ensures all column modes exist from the nearest stored layout', () => {
    const items = [
      { instance_id: 'weather', widget_type: 'homenavi.weather', defaultH: 5 },
      { instance_id: 'device', widget_type: 'homenavi.device', defaultH: 4 },
      { instance_id: 'map', widget_type: 'homenavi.map', defaultH: 5 },
    ];
    const layouts = ensureLayoutsByCols({
      layoutsByCols: {
        ...createEmptyLayoutsByCols(),
        '4': [
          { i: 'weather', x: 0, y: 0, w: 2, h: 5 },
          { i: 'device', x: 2, y: 0, w: 1, h: 4 },
          { i: 'map', x: 0, y: 5, w: 2, h: 5 },
        ],
      },
      items,
      widgetTypeByInstanceId: buildWidgetTypeMap(items),
      preferredWidthForType,
      defaultHeightByInstanceId: buildHeightMap(items),
    });

    expect(layouts['4']).toHaveLength(3);
    expect(layouts['3']).toHaveLength(3);
    expect(layouts['2']).toHaveLength(3);
    expect(layouts['1']).toHaveLength(3);
  });

  it('keeps stored heights per mode when ensuring layouts', () => {
    const items = [
      { instance_id: 'weather', widget_type: 'homenavi.weather', defaultH: 5 },
    ];
    const layouts = ensureLayoutsByCols({
      layoutsByCols: {
        '4': [{ i: 'weather', x: 0, y: 0, w: 2, h: 5 }],
        '3': [{ i: 'weather', x: 0, y: 0, w: 2, h: 3 }],
        '2': [],
        '1': [],
      },
      items,
      widgetTypeByInstanceId: buildWidgetTypeMap(items),
      preferredWidthForType,
      defaultHeightByInstanceId: buildHeightMap(items),
    });

    expect(layouts['4'][0].h).toBe(5);
    expect(layouts['3'][0].h).toBe(3);
  });

  it('normalizes canonical heights across column modes', () => {
    const next = normalizeLayoutHeightsByMode({
      '4': [{ i: 'weather', x: 0, y: 0, w: 2, h: 5 }],
      '3': [{ i: 'weather', x: 0, y: 0, w: 2, h: 2 }],
      '2': [],
      '1': [],
    }, { preferredMode: '4' });

    expect(next['3'][0].h).toBe(5);
  });

  it('parses and serializes the column-mode dashboard doc shape', () => {
    const parsed = parseDashboardDocShape({
      items: [{ instance_id: 'weather', widget_type: 'homenavi.weather' }],
      layouts_by_cols: {
        '4': [{ i: 'weather', x: 0, y: 0, w: 2, h: 5 }],
      },
    });

    expect(parsed.layoutsByCols['4']).toHaveLength(1);
    expect(serializeDashboardDoc(parsed)).toEqual({
      items: [{ instance_id: 'weather', widget_type: 'homenavi.weather' }],
      layouts_by_cols: {
        '4': [{ i: 'weather', x: 0, y: 0, w: 2, h: 5, minW: 1, minH: 2 }],
        '3': [],
        '2': [],
        '1': [],
      },
    });
    expect(createDashboardDocSnapshot(parsed)).toContain('layouts_by_cols');
  });
});