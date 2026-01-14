import WeatherWidget from './widgets/WeatherWidget';
import DeviceWidget from './widgets/DeviceWidget';
import DeviceGraphWidget from './widgets/DeviceGraphWidget';
import MapWidget from './widgets/MapWidget';
import AutomationWidget from './widgets/AutomationWidget';

export function clampWidgetHeight(h) {
  const n = Number(h);
  if (!Number.isFinite(n)) return 4;
  return Math.max(1, Math.min(10, Math.round(n)));
}

// Temporary local catalog for first-party widgets.
// Later, this should be replaced/augmented by GET /api/widgets/catalog.
const LOCAL_WIDGET_CATALOG = [
  {
    id: 'homenavi.weather',
    display_name: 'Weather',
    description: 'Current weather and forecast.',
    default_height: 5,
    source: 'first_party',
    verified: true,
  },
  {
    id: 'homenavi.map',
    display_name: 'Map',
    description: 'Home map and device locations.',
    default_height: 5,
    source: 'first_party',
    verified: true,
  },
  {
    id: 'homenavi.device',
    display_name: 'Device',
    description: 'A single device tile.',
    default_height: 4,
    source: 'first_party',
    verified: true,
  },
  {
    id: 'homenavi.device.graph',
    display_name: 'Device Graph',
    description: 'Graph a device metric over time.',
    default_height: 5,
    source: 'first_party',
    verified: true,
  },
  {
    id: 'homenavi.automation.manual_trigger',
    display_name: 'Automation',
    description: 'Manually trigger an automation workflow.',
    default_height: 3,
    source: 'first_party',
    verified: true,
  },
];

const WIDGET_RENDERERS = {
  'homenavi.weather': WeatherWidget,
  'homenavi.device': DeviceWidget,
  'homenavi.device.graph': DeviceGraphWidget,
  'homenavi.map': MapWidget,
  'homenavi.automation.manual_trigger': AutomationWidget,
};

export function getWidgetComponent(widgetType) {
  return WIDGET_RENDERERS[widgetType] || null;
}

export function listLocalWidgetCatalog() {
  return LOCAL_WIDGET_CATALOG.slice();
}

function findCatalogEntry(widgetType, catalog) {
  const type = (widgetType || '').toString();
  if (!type) return null;

  const fromRemote = Array.isArray(catalog) ? catalog.find((w) => w?.id === type) : null;
  if (fromRemote) return fromRemote;

  return LOCAL_WIDGET_CATALOG.find((w) => w.id === type) || null;
}

export function getWidgetTypeMeta(widgetType, catalog) {
  return findCatalogEntry(widgetType, catalog);
}

export function getWidgetDefaultHeight(widgetType, catalog) {
  const meta = findCatalogEntry(widgetType, catalog);
  const metaHeight = meta?.default_height ?? meta?.defaultHeight;
  if (metaHeight !== undefined) return clampWidgetHeight(metaHeight);

  // Back-compat: if metadata isn't available yet, fall back to component defaults.
  const C = WIDGET_RENDERERS[widgetType];
  if (!C) return 4;
  return clampWidgetHeight(C.defaultHeight);
}

export function listKnownWidgetTypes(catalog) {
  const list = Array.isArray(catalog) ? catalog : LOCAL_WIDGET_CATALOG;
  return list.map((w) => w?.id).filter(Boolean);
}
