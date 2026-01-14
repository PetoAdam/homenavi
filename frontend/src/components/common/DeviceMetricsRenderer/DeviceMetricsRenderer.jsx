/**
 * Shared device metrics display component.
 * Used by both DeviceTile (Devices page) and DeviceWidget (Dashboard).
 */
import React, { useMemo, useState } from 'react';
import {
  faBatteryThreeQuarters,
  faBolt,
  faDroplet,
  faDoorOpen,
  faGaugeHigh,
  faSignal,
  faThermometerHalf,
  faWaveSquare,
  faLightbulb,
} from '@fortawesome/free-solid-svg-icons';
import GlassMetric from '../GlassMetric/GlassMetric';
import GlassPill from '../GlassPill/GlassPill';
import { formatBinaryStateValue, formatMetricValueAndUnitForKey } from '../../../utils/stateFormat';
import './DeviceMetricsRenderer.css';

// ────────────────────────────────────────────────────────────────────
// Helper utilities
// ────────────────────────────────────────────────────────────────────

const ICON_BY_CAP = {
  temperature: faThermometerHalf,
  humidity: faDroplet,
  battery: faBatteryThreeQuarters,
  voltage: faBolt,
  linkquality: faWaveSquare,
  brightness: faLightbulb,
  power: faBolt,
  contact: faDoorOpen,
  signal: faSignal,
};

const BINARY_PILL_BLOCKLIST = new Set(['contact', 'battery_low', 'low_battery']);

function normalizeValueLabel(value) {
  if (value === undefined || value === null || value === '') {
    return '—';
  }
  if (typeof value === 'number') {
    const isInt = Number.isInteger(value);
    if (!isInt) {
      return Number.parseFloat(value.toFixed(2));
    }
  }
  if (typeof value === 'object') {
    return formatObjectValue(value);
  }
  return value;
}

function formatObjectValue(obj) {
  if (!obj || typeof obj !== 'object') return '—';
  if ('x' in obj && 'y' in obj) {
    const x = Number.isFinite(obj.x) ? Number(obj.x).toFixed(2) : obj.x;
    const y = Number.isFinite(obj.y) ? Number(obj.y).toFixed(2) : obj.y;
    return `x:${x} y:${y}`;
  }
  if ('r' in obj && 'g' in obj && 'b' in obj) {
    return `rgb(${obj.r},${obj.g},${obj.b})`;
  }
  if ('h' in obj && 's' in obj) {
    const h = Number.isFinite(obj.h) ? Math.round(obj.h) : obj.h;
    const s = Number.isFinite(obj.s) ? Math.round(obj.s) : obj.s;
    const v = 'v' in obj ? obj.v : null;
    return `h:${h} s:${s}${v !== null && v !== undefined ? ` v:${v}` : ''}`;
  }
  return Object.entries(obj)
    .map(([key, value]) => `${key}:${value}`)
    .join(' ');
}

function formatDisplayLabel(value) {
  if (!value) return '';
  return value
    .toString()
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, ch => ch.toUpperCase());
}

function toControlBoolean(value) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (typeof value === 'string') {
    const lowered = value.trim().toLowerCase();
    return ['on', 'true', '1', 'yes', 'enabled'].includes(lowered);
  }
  return Boolean(value);
}

function resolveBinaryTone(key, raw) {
  const bool = toControlBoolean(raw);
  const normalizedKey = (key || '').toString().toLowerCase();
  switch (normalizedKey) {
  case 'contact':
    return bool ? 'success' : 'warning';
  case 'tamper':
  case 'smoke':
  case 'water_leak':
  case 'leak':
    return bool ? 'danger' : 'success';
  case 'battery_low':
  case 'low_battery':
    return bool ? 'warning' : 'success';
  default:
    return bool ? 'success' : 'warning';
  }
}

function getIconForKey(key) {
  const lowerKey = (key || '').toLowerCase();
  return ICON_BY_CAP[lowerKey] || faGaugeHigh;
}

// ────────────────────────────────────────────────────────────────────
// Build metrics from device state
// ────────────────────────────────────────────────────────────────────

/**
 * Builds metric objects from device state.
 * @param {Object} state - The device.state object
 * @param {Array} selectedFields - Optional array of field keys to display
 * @returns {{ binaryMetrics: Array, numericMetrics: Array, allMetrics: Array }}
 */
function buildMetricsFromState(state = {}, selectedFields = null, capabilities = null) {
  const binaryMetrics = [];
  const numericMetrics = [];
  const usedKeys = new Set();

  const keysToCheck = selectedFields 
    ? selectedFields.filter(k => typeof k === 'string')
    : Object.keys(state);

  keysToCheck.forEach(key => {
    if (usedKeys.has(key)) return;
    
    const value = state[key];
    if (value === undefined || value === null) return;
    
    // Skip objects that look like nested capability data
    if (typeof value === 'object' && !Array.isArray(value) && value !== null) {
      // Allow color objects and coordinate objects
      if (!('x' in value || 'r' in value || 'h' in value)) {
        return;
      }
    }

    usedKeys.add(key);
    const icon = getIconForKey(key);
    const label = formatDisplayLabel(key);

    // Detect binary values
    const isBinary = typeof value === 'boolean' || 
      (typeof value === 'string' && ['on', 'off', 'true', 'false', 'open', 'closed'].includes(value.toLowerCase()));

    if (isBinary && !BINARY_PILL_BLOCKLIST.has(key.toLowerCase())) {
      binaryMetrics.push({
        key,
        label,
        value: formatBinaryStateValue(key, value),
        rawValue: value,
        unit: '',
        icon,
      });
      return;
    }

    // For blocklisted binary keys (e.g. contact), keep them in the main metric grid
    // but format the value using the shared binary formatter (so we don't show true/false).
    const { valueText, unit } = formatMetricValueAndUnitForKey(key, value, capabilities);
    numericMetrics.push({
      key,
      label,
      value: valueText === '' ? normalizeValueLabel(value) : valueText,
      unit: unit || (isBinary ? '' : getUnitForKey(key)),
      icon,
    });
  });

  return {
    binaryMetrics,
    numericMetrics,
    allMetrics: [...binaryMetrics, ...numericMetrics],
  };
}

function getUnitForKey(key) {
  const lowerKey = (key || '').toLowerCase();
  switch (lowerKey) {
  case 'temperature':
    return '°C';
  case 'humidity':
  case 'battery':
    return '%';
  default:
    return '';
  }
}

// ────────────────────────────────────────────────────────────────────
// Metric Components
// ────────────────────────────────────────────────────────────────────

/**
 * Renders binary metrics as colored pills.
 */
export function DeviceBinaryMetrics({ metrics = [] }) {
  if (metrics.length === 0) return null;

  return (
    <div className="dmr-binary-metrics">
      {metrics.map(metric => (
        <GlassPill
          key={metric.key}
          icon={metric.icon}
          tone={resolveBinaryTone(metric.key, metric.rawValue)}
          text={`${metric.label}: ${metric.value}`}
        />
      ))}
    </div>
  );
}

/**
 * Renders numeric/general metrics as GlassMetric cards.
 */
export function DeviceNumericMetrics({ metrics = [], layout = 'cards', collapseAfter = 5 }) {
  const [showAll, setShowAll] = useState(false);
  const hasExtra = metrics.length > collapseAfter;
  const visibleMetrics = useMemo(
    () => (showAll ? metrics : metrics.slice(0, collapseAfter)),
    [metrics, showAll, collapseAfter],
  );
  const hiddenCount = Math.max(metrics.length - collapseAfter, 0);

  if (metrics.length === 0) return null;

  return (
    <div className={`dmr-numeric-metrics dmr-layout-${layout}`}>
      {visibleMetrics.map(metric => (
        <GlassMetric
          key={metric.key}
          icon={metric.icon}
          label={metric.label}
          value={metric.value}
          unit={metric.unit}
        />
      ))}
      {hasExtra && (
        <button
          type="button"
          className="dmr-metrics-toggle"
          onClick={() => setShowAll(prev => !prev)}
        >
          {showAll ? 'Show fewer details' : `Show ${hiddenCount} more`}
        </button>
      )}
    </div>
  );
}

/**
 * Main component that renders all device metrics.
 */
export default function DeviceMetricsRenderer({
  device,
  selectedFields = null,
  layout = 'cards',
  collapseAfter = 5,
  showBinaryPills = true,
}) {
  const capabilities = device?.capabilities || device?.state?.capabilities || null;
  const { binaryMetrics, numericMetrics } = useMemo(
    () => buildMetricsFromState(device?.state || {}, selectedFields, capabilities),
    [device?.state, selectedFields, capabilities],
  );

  // Stop propagation to prevent parent onClick handlers from firing
  const handleContainerClick = (e) => {
    e.stopPropagation();
  };

  return (
    <div className="dmr-container" onClick={handleContainerClick}>
      {showBinaryPills && <DeviceBinaryMetrics metrics={binaryMetrics} />}
      <DeviceNumericMetrics 
        metrics={numericMetrics} 
        layout={layout} 
        collapseAfter={collapseAfter} 
      />
    </div>
  );
}
