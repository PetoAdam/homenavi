import {
  faBatteryHalf,
  faCircleInfo,
  faDoorClosed,
  faDroplet,
  faLightbulb,
  faSun,
  faTemperatureHalf,
} from '@fortawesome/free-solid-svg-icons';

import { safeString } from './mapErsMeta';

export function lowerKey(value) {
  return safeString(value).trim().toLowerCase();
}

export function uniqueRoomName(desired, existingNamesLower) {
  const base = safeString(desired).trim() || 'Room';
  const baseLower = base.toLowerCase();
  const taken = existingNamesLower || new Set();
  if (!taken.has(baseLower)) return base;
  let n = 2;
  while (n < 5000) {
    const candidate = `${base}(${n})`;
    if (!taken.has(candidate.toLowerCase())) return candidate;
    n += 1;
  }
  return `${base}(${Date.now()})`;
}

export function formatRelativeTimeShort(date) {
  if (!(date instanceof Date) || Number.isNaN(date.getTime())) return '';
  const diff = Date.now() - date.getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 20) return 'just now';
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function iconForFactLabel(label) {
  const k = typeof label === 'string' ? label.trim().toLowerCase() : '';
  if (!k) return faCircleInfo;
  if (k.includes('temp')) return faTemperatureHalf;
  if (k.includes('humid')) return faDroplet;
  if (k.includes('battery')) return faBatteryHalf;
  if (k === 'on' || k.includes('power') || k.includes('state')) return faLightbulb;
  if (k.includes('bright')) return faSun;
  if (k.includes('contact') || k.includes('door')) return faDoorClosed;
  return faCircleInfo;
}

export function getFaSvgPath(iconDef) {
  const icon = iconDef?.icon;
  if (!Array.isArray(icon) || icon.length < 5) return null;
  const width = icon[0];
  const height = icon[1];
  const raw = icon[4];
  const path = Array.isArray(raw) ? raw[0] : raw;
  if (!path || !Number.isFinite(width) || !Number.isFinite(height)) return null;
  return { width, height, path };
}

export function pickStateValue(state, key) {
  if (!state || typeof state !== 'object') return undefined;
  if (state[key] !== undefined) return state[key];
  const lower = key.toLowerCase();
  if (state[lower] !== undefined) return state[lower];
  const camel = lower.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
  if (state[camel] !== undefined) return state[camel];
  return undefined;
}

export function collectFavoriteFieldOptionsFromState(state) {
  if (!state || typeof state !== 'object' || Array.isArray(state)) return [];
  const reserved = new Set([
    'schema', 'device_id', 'deviceid', 'external_id', 'externalid', 'protocol', 'topic', 'retained',
    'ts', 'timestamp', 'time', 'received_at', 'receivedat',
    'capabilities',
  ]);
  return Object.keys(state)
    .filter(k => k && !reserved.has(k.toLowerCase()))
    .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
}
