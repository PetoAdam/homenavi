import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/dashboardService', () => ({
  getDashboard: vi.fn(),
  getWidgetCatalog: vi.fn(),
  updateDashboard: vi.fn(),
}));

vi.mock('../components/Home/Dashboard/widgetRegistry', () => ({
  getWidgetDefaultHeight: vi.fn(),
  listLocalWidgetCatalog: vi.fn(() => [
    { id: 'local.weather', label: 'Weather' },
    { id: 'local.map', label: 'Map' },
  ]),
}));

import { getDashboard, getWidgetCatalog } from '../services/dashboardService';
import {
  applyPendingDashboardDoc,
  dashboardQueryOptions,
  fetchWidgetCatalogData,
  getDashboardVersionForSave,
  mergeWidgetCatalogData,
  parseDashboardDoc,
  shouldPersistDashboardCache,
} from './useDashboard';
import { queryKeys } from '../state/queryKeys';

describe('parseDashboardDoc', () => {
  it('parses string docs and falls back safely', () => {
    expect(parseDashboardDoc({ doc: '{"layouts_by_cols":{"4":[]},"items":[{"instance_id":"1"}]}' })).toEqual({
      layoutsByCols: { '4': [], '3': [], '2': [], '1': [] },
      items: [{ instance_id: '1' }],
    });
    expect(parseDashboardDoc({ doc: '{"layouts":{"lg":[{"i":"1","x":0,"y":0,"w":1,"h":4}]},"items":[{"instance_id":"1"}]}' })).toEqual({
      layoutsByCols: {
        '4': [{ i: '1', x: 0, y: 0, w: 1, h: 4, minW: 1, minH: 2 }],
        '3': [{ i: '1', x: 0, y: 0, w: 1, h: 4, minW: 1, minH: 2 }],
        '2': [],
        '1': [],
      },
      items: [{ instance_id: '1' }],
    });
    expect(parseDashboardDoc({ doc: 'not-json' })).toEqual({
      layoutsByCols: { '4': [], '3': [], '2': [], '1': [] },
      items: [],
    });
  });
});

describe('dashboardQueryOptions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('builds a scoped query key and resolves dashboard data', async () => {
    getDashboard.mockResolvedValue({ success: true, data: { id: 'dash-1' } });

    const options = dashboardQueryOptions('abcdefghijklmnopqrstuvwxyz', { enabled: false });
    expect(options.queryKey).toEqual(queryKeys.dashboard.me('klmnopqrstuvwxyz'));
    expect(options.enabled).toBe(false);
    await expect(options.queryFn()).resolves.toEqual({ id: 'dash-1' });
  });
});

describe('widget catalog helpers', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('merges remote and local widget catalogs without duplicates', () => {
    expect(mergeWidgetCatalogData([{ id: 'local.weather', label: 'Remote Weather' }, { id: 'remote.device', label: 'Device' }])).toEqual([
      { id: 'local.weather', label: 'Remote Weather' },
      { id: 'remote.device', label: 'Device' },
      { id: 'local.map', label: 'Map' },
    ]);
  });

  it('falls back to the local catalog when the backend catalog is unavailable', async () => {
    getWidgetCatalog.mockResolvedValue({ success: false, error: 'nope' });

    await expect(fetchWidgetCatalogData('token')).resolves.toEqual([
      { id: 'local.weather', label: 'Weather' },
      { id: 'local.map', label: 'Map' },
    ]);
  });
});

describe('dashboard conflict and cache helpers', () => {
  it('reapplies a pending doc onto a freshly loaded dashboard snapshot', () => {
    expect(applyPendingDashboardDoc(
      { layout_version: 4, doc: { layouts_by_cols: { '4': [] }, items: [] }, title: 'Main' },
      { layouts_by_cols: { '4': [{ i: 'widget-1' }], '3': [], '2': [], '1': [] }, items: [{ instance_id: 'widget-1' }] },
    )).toEqual({
      layout_version: 4,
      doc: { layouts_by_cols: { '4': [{ i: 'widget-1' }], '3': [], '2': [], '1': [] }, items: [{ instance_id: 'widget-1' }] },
      title: 'Main',
    });
  });

  it('avoids persisting optimistic dashboard docs while a save is still pending', () => {
    expect(shouldPersistDashboardCache({
      queryEnabled: true,
      dashboard: { id: 'dash-1' },
      pendingDoc: { layouts_by_cols: { '4': [], '3': [], '2': [], '1': [] }, items: [] },
    })).toBe(false);

    expect(shouldPersistDashboardCache({
      queryEnabled: true,
      dashboard: { id: 'dash-1' },
      pendingDoc: null,
    })).toBe(true);
  });

  it('prefers the freshest cached dashboard version when saving pending docs', () => {
    expect(getDashboardVersionForSave(
      { layout_version: 5 },
      { layout_version: 4 },
    )).toBe(5);

    expect(getDashboardVersionForSave(
      null,
      { layout_version: 4 },
    )).toBe(4);

    expect(getDashboardVersionForSave(null, null)).toBeNull();
  });
});
