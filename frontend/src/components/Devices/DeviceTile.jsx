import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBatteryThreeQuarters,
  faBolt,
  faDroplet,
  faDoorOpen,
  faFan,
  faGaugeHigh,
  faLightbulb,
  faMicrochip,
  faMusic,
  faPalette,
  faPlug,
  faShieldHalved,
  faSignal,
  faSliders,
  faThermometerHalf,
  faTicket,
  faVideo,
  faWaveSquare,
  faPen,
  faCheck,
  faXmark,
  faTrash,
} from '@fortawesome/free-solid-svg-icons';
import GlassCard from '../common/GlassCard/GlassCard';
import GlassSwitch from '../common/GlassSwitch/GlassSwitch';
import GlassMetric from '../common/GlassMetric/GlassMetric';
import GlassPill from '../common/GlassPill/GlassPill';
import { HexColorPicker } from 'react-colorful';
import { DEVICE_ICON_CHOICES, DEVICE_ICON_MAP } from './deviceIconChoices';
import './DeviceTile.css';

const ICON_BY_CAP = {
  temperature: faThermometerHalf,
  humidity: faDroplet,
  battery: faBatteryThreeQuarters,
  voltage: faBolt,
  linkquality: faWaveSquare,
  brightness: faLightbulb,
  power: faBolt,
  contact: faDoorOpen,
};

const ICON_BY_INPUT_TYPE = {
  toggle: faBolt,
  slider: faSliders,
  number: faGaugeHigh,
  select: faTicket,
  color: faPalette,
};

const DESCRIPTION_LIMIT = 140;
const CONTROL_COLLAPSE_COUNT = 5;
const BINARY_PILL_BLOCKLIST = new Set(['contact', 'battery_low', 'low_battery']);

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

function formatRelativeTime(date) {
  if (!date || Number.isNaN(date.getTime())) return 'never';
  const diff = Date.now() - date.getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 20) return 'just now';
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 14) return `${days}d ago`;
  return date.toLocaleDateString();
}

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

function resolveStateValue(state, cap) {
  if (!state) return undefined;
  const candidates = [cap.id, cap.property, cap.name]
    .filter(Boolean)
    .map(key => key.toString());
  for (const key of candidates) {
    if (state[key] !== undefined) {
      return state[key];
    }
    const lower = key.toLowerCase();
    if (state[lower] !== undefined) {
      return state[lower];
    }
    const camel = lower.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
    if (state[camel] !== undefined) {
      return state[camel];
    }
  }
  return undefined;
}

function getCapabilityKeyParts(cap) {
  if (!cap || typeof cap !== 'object') return [];
  return [cap.id, cap.property, cap.name]
    .filter(part => part !== undefined && part !== null)
    .map(part => part.toString().toLowerCase());
}

function collectCapabilities(device) {
  const lists = [
    Array.isArray(device?.capabilities) ? device.capabilities : [],
    Array.isArray(device?.state?.capabilities) ? device.state.capabilities : [],
  ];
  const seen = new Set();
  const result = [];
  lists.flat().forEach(cap => {
    if (!cap || typeof cap !== 'object') return;
    const keys = getCapabilityKeyParts(cap);
    const primary = keys[0];
    if (primary && seen.has(primary)) return;
    if (keys.length === 0) {
      result.push(cap);
      return;
    }
    keys.forEach(key => seen.add(key));
    result.push(cap);
  });
  return result;
}

function buildCapabilityLookup(capabilities) {
  const map = new Map();
  capabilities.forEach(cap => {
    getCapabilityKeyParts(cap).forEach(key => {
      if (key && !map.has(key)) {
        map.set(key, cap);
      }
    });
  });
  return map;
}

function resolveCapabilityForInput(capabilityLookup, input) {
  if (!input || !capabilityLookup) return null;
  const candidates = [
    input.capability_id,
    input.capabilityId,
    input.capabilityID,
    input.id,
    input.property,
  ];
  for (const candidate of candidates) {
    if (!candidate) continue;
    const key = candidate.toString().toLowerCase();
    if (capabilityLookup.has(key)) {
      return capabilityLookup.get(key);
    }
  }
  return null;
}

function isCapabilityWritable(cap) {
  if (!cap || typeof cap !== 'object') return false;
  const access = cap.access || {};
  if (access.readOnly === true) return false;
  const negativeFlags = [access.write, access.set, access.command, access.toggle]
    .filter(flag => flag === false);
  if (negativeFlags.length > 0) {
    return false;
  }
  const positiveFlags = [access.write, access.set, access.command, access.toggle]
    .filter(flag => flag === true);
  if (positiveFlags.length > 0) {
    return true;
  }
  if ('write' in access || 'set' in access || 'command' in access || 'toggle' in access) {
    return Boolean(access.write || access.set || access.command || access.toggle);
  }
  return false;
}

function formatBinaryStateValue(key, raw) {
  if (raw === undefined || raw === null || raw === '') {
    return '—';
  }
  let bool;
  if (typeof raw === 'boolean') {
    bool = raw;
  } else if (typeof raw === 'number') {
    bool = raw !== 0;
  } else if (typeof raw === 'string') {
    const lowered = raw.trim().toLowerCase();
    if (['true', 'on', '1', 'yes', 'enabled', 'detected', 'active', 'closed', 'present'].includes(lowered)) {
      bool = true;
    } else if (['false', 'off', '0', 'no', 'disabled', 'clear', 'inactive', 'open', 'absent'].includes(lowered)) {
      bool = false;
    }
  }
  if (bool === undefined) {
    bool = toControlBoolean(raw);
  }
  const normalizedKey = (key || '').toString().toLowerCase();
  switch (normalizedKey) {
  case 'contact':
    return bool ? 'Closed' : 'Open';
  case 'open':
    return bool ? 'Open' : 'Closed';
  case 'closed':
    return bool ? 'Closed' : 'Open';
  case 'occupancy':
  case 'motion':
  case 'presence':
    return bool ? 'Detected' : 'Clear';
  case 'water_leak':
  case 'leak':
  case 'moisture':
    return bool ? 'Leak' : 'Dry';
  case 'smoke':
    return bool ? 'Smoke' : 'Clear';
  case 'tamper':
    return bool ? 'Tamper' : 'Secure';
  case 'battery_low':
  case 'low_battery':
    return bool ? 'Low' : 'OK';
  default:
    return bool ? 'On' : 'Off';
  }
}

function formatBinaryCapabilityValue(cap, raw) {
  if (!cap || typeof cap !== 'object') {
    return formatBinaryStateValue('', raw);
  }
  const key = cap.property || cap.id || cap.name || '';
  return formatBinaryStateValue(key, raw);
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

function buildMetrics(device, capabilities = []) {
  const binaryMetrics = [];
  const numericMetrics = [];
  const state = device.state || {};
  const usedKeys = new Set();

  capabilities.forEach((cap, idx) => {
    if (!cap || typeof cap !== 'object') return;
    const rawId = cap.id || cap.property || cap.name || `cap-${idx}`;
    const id = rawId ? rawId.toString().toLowerCase() : `cap-${idx}`;
    if (usedKeys.has(id)) return;
    const kind = (cap.kind || cap.type || '').toString().toLowerCase();
    const icon = ICON_BY_CAP[id] || ICON_BY_CAP[cap.property?.toLowerCase?.()] || faGaugeHigh;
    const value = resolveStateValue(state, cap);
    if (kind === 'binary') {
      usedKeys.add(id);
      const formatted = formatBinaryCapabilityValue(cap, value);
      if (BINARY_PILL_BLOCKLIST.has(id)) {
        numericMetrics.push({
          key: id,
          label: cap.name || cap.property || rawId,
          value: formatted,
          unit: '',
          icon,
        });
      } else if (!isCapabilityWritable(cap)) {
        binaryMetrics.push({
          key: id,
          label: cap.name || cap.property || rawId,
          value: formatted,
          rawValue: value,
          unit: '',
          icon,
        });
      }
      return;
    }
    usedKeys.add(id);
    const metricValue = normalizeValueLabel(value);
    numericMetrics.push({
      key: id,
      label: cap.name || cap.property || rawId,
      value: metricValue,
      unit: cap.unit || cap.units || '',
      icon,
    });
  });

  const binaryFallbacks = [
    { key: 'contact', label: 'Contact', icon: ICON_BY_CAP.contact || faGaugeHigh },
  ];

  binaryFallbacks.forEach(meta => {
    if (usedKeys.has(meta.key)) return;
    if (BINARY_PILL_BLOCKLIST.has(meta.key)) {
      usedKeys.add(meta.key);
      return;
    }
    const value = state[meta.key];
    if (value === undefined || value === null) return;
    binaryMetrics.push({
      key: meta.key,
      label: meta.label,
      value: formatBinaryStateValue(meta.key, value),
      rawValue: value,
      unit: '',
      icon: meta.icon,
    });
    usedKeys.add(meta.key);
  });

  const fallbackMetrics = [
    { key: 'temperature', label: 'Temperature', unit: '°C', icon: faThermometerHalf },
    { key: 'humidity', label: 'Humidity', unit: '%', icon: faDroplet },
    { key: 'battery', label: 'Battery', unit: '%', icon: faBatteryThreeQuarters },
    { key: 'linkquality', label: 'Link quality', unit: '', icon: faSignal },
    { key: 'contact', label: 'Contact', unit: '', icon: ICON_BY_CAP.contact || faGaugeHigh },
    { key: 'battery_low', label: 'Battery Low', unit: '', icon: ICON_BY_CAP.battery || faBatteryThreeQuarters },
  ];

  fallbackMetrics.forEach(metric => {
    if (usedKeys.has(metric.key)) return;
    const value = state[metric.key];
    if (value === undefined || value === null) return;
    numericMetrics.push({ ...metric, value: normalizeValueLabel(value) });
    usedKeys.add(metric.key);
  });

  const allMetrics = [...binaryMetrics, ...numericMetrics];
  return { allMetrics, binaryMetrics, numericMetrics };
}

function sanitizeInputKey(input) {
  if (!input) return '';
  return input.id || input.capability_id || input.capabilityId || input.property || '';
}

function resolveInputLabel(input) {
  if (!input) return 'Control';
  if (input.label) return input.label;
  const source = input.property || input.capability_id || input.capabilityId || input.id;
  if (!source) return 'Control';
  return source
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, ch => ch.toUpperCase());
}

function formatDisplayLabel(value) {
  if (!value) return '';
  return value
    .toString()
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, ch => ch.toUpperCase());
}

function inferInputTypeFromCapability(cap) {
  const kind = (cap.kind || '').toString().toLowerCase();
  const valueType = (cap.value_type || cap.valueType || '').toString().toLowerCase();
  const name = (cap.name || '').toString().toLowerCase();
  const property = (cap.property || '').toString().toLowerCase();
  if (property === 'color' || name.includes('color')) {
    return 'color';
  }
  if (kind === 'binary' || valueType === 'boolean') {
    return 'toggle';
  }
  if (kind === 'enum' || valueType === 'enum' || Array.isArray(cap.enum)) {
    return 'select';
  }
  if (kind === 'numeric' || valueType === 'number') {
    if (typeof cap.range?.min === 'number' && typeof cap.range?.max === 'number') {
      return 'slider';
    }
    return 'number';
  }
  if (valueType === 'object' && (name.includes('color') || property === 'color')) {
    return 'color';
  }
  return null;
}

function buildInputsFromCapabilities(capabilities) {
  const results = [];
  capabilities.forEach(cap => {
    if (!cap || typeof cap !== 'object') return;
    if (!isCapabilityWritable(cap)) return;
    const inferredType = inferInputTypeFromCapability(cap);
    if (!inferredType) return;
    const rawId = cap.id || cap.property || cap.name;
    if (!rawId) return;
    const capabilityId = cap.id || cap.property || rawId;
    const property = cap.property || capabilityId;
    const range = cap.range && typeof cap.range === 'object'
      ? {
        min: typeof cap.range.min === 'number' ? cap.range.min : undefined,
        max: typeof cap.range.max === 'number' ? cap.range.max : undefined,
        step: typeof cap.range.step === 'number' ? cap.range.step : (typeof cap.step === 'number' ? cap.step : undefined),
      }
      : null;
    const options = Array.isArray(cap.enum)
      ? cap.enum.map(value => ({
        value,
        label: formatDisplayLabel(value),
      }))
      : [];
    results.push({
      id: rawId,
      type: inferredType,
      property,
      capability_id: capabilityId,
      label: cap.name || cap.property || rawId,
      range,
      options,
      metadata: { ...(cap.metadata || {}), fromCapability: true },
      access: cap.access || {},
      capability: cap,
      readOnly: false,
    });
  });
  return results;
}

function resolveDeviceIcon(device, capabilities) {
  const manualKey = typeof device?.icon === 'string' ? device.icon.toLowerCase() : '';
  if (manualKey && manualKey !== 'auto' && DEVICE_ICON_MAP[manualKey]) {
    return DEVICE_ICON_MAP[manualKey];
  }
  const keywords = [device?.type, device?.description, device?.model, device?.displayName, device?.manufacturer]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  const hasCapability = key => capabilities.some(cap => {
    const parts = getCapabilityKeyParts(cap);
    return parts.includes(key);
  });
  if (hasCapability('contact') || keywords.includes('door')) {
    return faDoorOpen;
  }
  if (hasCapability('brightness') || hasCapability('color') || keywords.includes('light') || keywords.includes('lamp')) {
    return faLightbulb;
  }
  if (hasCapability('power') || keywords.includes('plug') || keywords.includes('socket') || keywords.includes('outlet')) {
    return faPlug;
  }
  if (hasCapability('temperature') || keywords.includes('thermo') || keywords.includes('heating')) {
    return faThermometerHalf;
  }
  if (hasCapability('humidity')) {
    return faDroplet;
  }
  if (hasCapability('battery')) {
    return faBatteryThreeQuarters;
  }
  if (hasCapability('voltage') || hasCapability('power')) {
    return faBolt;
  }
  return faMicrochip;
}

function clampColorByte(v) {
  if (Number.isNaN(v)) return 0;
  return Math.min(255, Math.max(0, Math.round(Number(v))));
}

function rgbToHex(r, g, b) {
  const toHex = value => clampColorByte(value).toString(16).padStart(2, '0');
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`.toUpperCase();
}

function extractHexColor(value) {
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (/^#[0-9a-f]{6}$/i.test(trimmed)) return trimmed.toUpperCase();
    if (/^[0-9a-f]{6}$/i.test(trimmed)) return `#${trimmed.toUpperCase()}`;
    return null;
  }
  if (value && typeof value === 'object') {
    if (typeof value.hex === 'string') {
      return extractHexColor(value.hex);
    }
    if (
      typeof value.r === 'number'
      && typeof value.g === 'number'
      && typeof value.b === 'number'
    ) {
      return rgbToHex(value.r, value.g, value.b);
    }
  }
  return null;
}

function normalizeColorHex(value, fallback = '#FFFFFF') {
  return extractHexColor(value) || fallback;
}

function extractStateColorHex(raw) {
  return extractHexColor(raw);
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

function getStateValueForInput(device, input) {
  if (!device || !input) return undefined;
  const state = device.state || {};
  const capabilityId = input.capability_id || input.capabilityId || input.capabilityID || input.id;
  const pseudoCap = {
    id: capabilityId || input.id || input.property,
    property: input.property || capabilityId || input.id,
    name: input.label || input.property,
  };
  return resolveStateValue(state, pseudoCap);
}

function buildInitialControlValues(device, inputs) {
  const values = {};
  const list = Array.isArray(inputs) ? inputs : [];
  list.forEach(input => {
    const key = sanitizeInputKey(input);
    if (!key) return;
    let raw = getStateValueForInput(device, input);
    if (raw === undefined && input.type === 'toggle' && typeof device.toggleState === 'boolean') {
      raw = device.toggleState;
    }
    if (raw === undefined && input.type === 'slider' && input.range && typeof input.range.min === 'number') {
      raw = input.range.min;
    }
    if (raw === undefined && input.type === 'select' && input.options?.length) {
      raw = input.options[0].value;
    }
    switch (input.type) {
    case 'toggle':
      values[key] = toControlBoolean(raw);
      break;
    case 'slider':
    case 'number':
      values[key] = typeof raw === 'number' ? raw : Number(raw ?? 0);
      break;
    case 'color':
      values[key] = extractStateColorHex(raw) || '#FFFFFF';
      break;
    case 'select':
      values[key] = raw ?? '';
      break;
    default:
      if (raw !== undefined) {
        values[key] = raw;
      }
    }
  });
  return values;
}

function buildPayloadForInput(input, value) {
  const key = sanitizeInputKey(input);
  if (!key) {
    return null;
  }
  const payload = { input: { id: key, value } };
  const property = (input.property || '').toLowerCase();
  if (input.type === 'slider' && property.startsWith('brightness')) {
    const numeric = Number(value);
    if (!Number.isNaN(numeric)) {
      payload.state = { on: numeric > 0 };
    }
  }
  if (input.type === 'color') {
    const hex = typeof value === 'string' ? normalizeColorHex(value) : normalizeColorHex(value?.hex);
    payload.input.value = hex;
    payload.state = { ...(payload.state || {}), on: true };
  }
  if (input.type === 'select') {
    const nextState = { ...(payload.state || {}) };
    if (property) {
      nextState[property] = value;
    }
    if (property.includes('effect')) {
      nextState.on = true;
    }
    if (Object.keys(nextState).length > 0) {
      payload.state = nextState;
    }
  }
  return payload;
}

function formatControlValue(input, value) {
  if (value === undefined || value === null || value === '') {
    return '';
  }
  switch (input.type) {
  case 'toggle':
    return value ? 'On' : 'Off';
  case 'color':
    return String(value).toUpperCase();
  default:
    return value;
  }
}

export default function DeviceTile({ device, onCommand, onRename, onUpdateIcon, pending, onDelete }) {
  const capabilities = useMemo(
    () => collectCapabilities(device),
    [device],
  );
  const capabilityLookup = useMemo(() => buildCapabilityLookup(capabilities), [capabilities]);
  const {
    allMetrics,
    binaryMetrics,
  } = useMemo(
    () => buildMetrics(device, capabilities),
    [device, capabilities],
  );
  const deviceIcon = useMemo(() => resolveDeviceIcon(device, capabilities), [device, capabilities]);

  const normalizedInputs = useMemo(() => {
    const rawList = Array.isArray(device.inputs) ? device.inputs : [];
    const hasCompositeColor = rawList.some(input => (input?.type || '').toLowerCase() === 'color' && input?.metadata?.mode === 'composite');
    const suppressedProperties = new Set();
    if (hasCompositeColor) {
      ['x', 'y', 'hue', 'saturation', 'color_x', 'color_y', 'color_hs', 'color_xy']
        .forEach(key => suppressedProperties.add(key));
    }
    const normalized = rawList
      .map(input => {
        const type = (input.type || '').toLowerCase();
        const capabilityId = input.capability_id || input.capabilityId || input.capabilityID || '';
        const property = input.property || capabilityId || input.id || '';
        const id = input.id || capabilityId || property;
        const capability = resolveCapabilityForInput(capabilityLookup, input);
        const access = {
          ...(capability?.access || {}),
          ...(input.metadata?.access || {}),
          ...(input.access || {}),
        };
        const readOnly = Boolean(access?.readOnly)
          || access.write === false
          || access.set === false
          || access.toggle === false
          || (capability ? !isCapabilityWritable(capability) : false);
        return {
          ...input,
          id,
          type,
          property,
          capability_id: capabilityId,
          capability,
          access,
          readOnly,
          options: Array.isArray(input.options) ? input.options : [],
          metadata: input.metadata || {},
          range: input.range || input.Range || null,
        };
      })
      .filter(input => {
        if (!input.id || input.readOnly) return false;
        if (!['toggle', 'slider', 'number', 'select', 'color'].includes(input.type)) return false;
        const propertyKey = (input.property || input.id || '').toString().toLowerCase();
        if (suppressedProperties.has(propertyKey)) return false;
        return true;
      });
    const seenKeys = new Set(normalized.map(item => sanitizeInputKey(item).toLowerCase()));
    const fallbackInputs = buildInputsFromCapabilities(capabilities);
    fallbackInputs.forEach(fallback => {
      const key = sanitizeInputKey(fallback).toLowerCase();
      if (!key || seenKeys.has(key)) return;
      if (!['toggle', 'slider', 'number', 'select', 'color'].includes(fallback.type)) return;
      const propertyKey = (fallback.property || fallback.id || '').toString().toLowerCase();
      if (suppressedProperties.has(propertyKey)) return;
      seenKeys.add(key);
      normalized.push({
        ...fallback,
        options: Array.isArray(fallback.options) ? fallback.options : [],
        metadata: fallback.metadata || {},
        range: fallback.range || null,
        capability: fallback.capability,
        capability_id: fallback.capability_id,
        access: fallback.access || {},
        readOnly: false,
      });
    });
    return normalized;
  }, [device.inputs, capabilityLookup, capabilities]);

  const primaryToggleInput = useMemo(() => {
    return normalizedInputs.find(input => {
      if (input.type !== 'toggle') return false;
      const key = sanitizeInputKey(input).toLowerCase();
      return key === 'state' || key === 'on' || key === 'power';
    }) || null;
  }, [normalizedInputs]);

  const primaryToggleKey = primaryToggleInput ? sanitizeInputKey(primaryToggleInput) : null;

  const showHeaderToggle = Boolean(primaryToggleInput && onCommand && device?.id);
  const toggleDisabled = pending;

  const interactiveInputs = useMemo(() => {
    if (!showHeaderToggle || !primaryToggleInput) {
      return normalizedInputs;
    }
    return normalizedInputs.filter(input => input !== primaryToggleInput);
  }, [normalizedInputs, primaryToggleInput, showHeaderToggle]);

  const [showAllControls, setShowAllControls] = useState(false);
  const hasExtraControls = interactiveInputs.length > CONTROL_COLLAPSE_COUNT;
  const visibleControls = useMemo(
    () => (showAllControls ? interactiveInputs : interactiveInputs.slice(0, CONTROL_COLLAPSE_COUNT)),
    [interactiveInputs, showAllControls],
  );
  const hiddenControlsCount = Math.max(interactiveInputs.length - CONTROL_COLLAPSE_COUNT, 0);

  const [controlValues, setControlValues] = useState(() => buildInitialControlValues(device, normalizedInputs));
  const [activeColorInput, setActiveColorInput] = useState(null);
  const [colorDrafts, setColorDrafts] = useState({});
  const [isEditingName, setIsEditingName] = useState(false);
  const [nameDraft, setNameDraft] = useState(device.displayName || '');
  const [renamePending, setRenamePending] = useState(false);
  const [renameError, setRenameError] = useState(null);
  const [iconMenuOpen, setIconMenuOpen] = useState(false);
  const [iconPending, setIconPending] = useState(false);
  const [iconError, setIconError] = useState(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState(null);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [forceDelete, setForceDelete] = useState(false);
  const [actionMenuOpen, setActionMenuOpen] = useState(false);
  const iconMenuRef = useRef(null);
  const actionMenuRef = useRef(null);
  const actionMenuButtonRef = useRef(null);
  const canEditIcon = Boolean(isEditingName && onUpdateIcon);
  const showIconPicker = Boolean(canEditIcon && iconMenuOpen);
  const iconGlyph = (
    <FontAwesomeIcon icon={deviceIcon} className="device-title-icon" />
  );
  const deleteDialogTitleId = useMemo(() => `device-delete-title-${device.id || 'unknown'}`, [device.id]);
  const deleteDialogDescId = useMemo(() => `device-delete-desc-${device.id || 'unknown'}`, [device.id]);
  const deleteForceNoteId = useMemo(() => `device-delete-note-${device.id || 'unknown'}`, [device.id]);
  const deleteForceCheckboxId = useMemo(() => `device-delete-force-checkbox-${device.id || 'unknown'}`, [device.id]);

  const stateVersion = device.stateUpdatedAt instanceof Date ? device.stateUpdatedAt.getTime() : (device.stateUpdatedAt || 0);
  const metadataVersion = device.metadataUpdatedAt instanceof Date ? device.metadataUpdatedAt.getTime() : (device.metadataUpdatedAt || 0);

  useEffect(() => {
    setControlValues(prev => {
      if (pending) {
        // Keep the optimistic value while a command is in flight to avoid flicker.
        return prev;
      }
      const baseline = buildInitialControlValues(device, normalizedInputs);
      if (!prev || typeof prev !== 'object') {
        return baseline;
      }
      const merged = { ...baseline };
      normalizedInputs.forEach(input => {
        if (input.type !== 'color') {
          return;
        }
        const key = sanitizeInputKey(input);
        if (!key) {
          return;
        }
        const stateRaw = getStateValueForInput(device, input);
        const stateHex = extractStateColorHex(stateRaw);
        if (!stateHex && prev[key]) {
          merged[key] = prev[key];
        }
      });
      return merged;
    });
    setActiveColorInput(null);
    setColorDrafts({});
    setIsEditingName(false);
    setNameDraft(device.displayName || '');
    setRenamePending(false);
    setRenameError(null);
    setShowAllControls(false);
    setIconMenuOpen(false);
    setIconError(null);
    setIconPending(false);
    setDeletePending(false);
    setDeleteError(null);
    setDeleteModalOpen(false);
    setForceDelete(false);
    setActionMenuOpen(false);
  }, [device, device.id, stateVersion, metadataVersion, device.toggleState, normalizedInputs, pending]);

  useEffect(() => {
    if (!isEditingName) {
      setIconMenuOpen(false);
      setActionMenuOpen(false);
    }
  }, [isEditingName]);

  const toggleValue = primaryToggleKey ? toControlBoolean(controlValues[primaryToggleKey]) : null;

  const updateControlValue = useCallback((input, value) => {
    const key = sanitizeInputKey(input);
    if (!key) return;
    setControlValues(prev => ({ ...prev, [key]: value }));
  }, []);

  const handleInputCommand = useCallback((input, rawValue) => {
    if (!onCommand) return Promise.resolve();
    const payload = buildPayloadForInput(input, rawValue);
    if (!payload) return Promise.resolve();
    return onCommand(device, payload);
  }, [device, onCommand]);

  const handleToggleInput = useCallback((input, next) => {
    const value = toControlBoolean(next);
    updateControlValue(input, value);
    return handleInputCommand(input, value);
  }, [handleInputCommand, updateControlValue]);

  const handleSliderChange = useCallback((input, raw) => {
    const numeric = Number(raw);
    if (Number.isNaN(numeric)) return;
    updateControlValue(input, numeric);
  }, [updateControlValue]);

  const handleSliderCommit = useCallback((input, raw) => {
    const numeric = Number(raw);
    if (Number.isNaN(numeric)) return;
    handleInputCommand(input, numeric);
  }, [handleInputCommand]);

  const handleSelectChange = useCallback((input, optionValue) => {
    updateControlValue(input, optionValue);
    handleInputCommand(input, optionValue);
  }, [handleInputCommand, updateControlValue]);

  const handleNumberCommit = useCallback((input, raw) => {
    if (raw === '' || raw === null || raw === undefined) {
      return;
    }
    const numeric = Number(raw);
    if (Number.isNaN(numeric)) return;
    updateControlValue(input, numeric);
    handleInputCommand(input, numeric);
  }, [handleInputCommand, updateControlValue]);

  const handlePrimaryToggle = useCallback((next) => {
    if (!primaryToggleInput) return;
    handleToggleInput(primaryToggleInput, next);
  }, [handleToggleInput, primaryToggleInput]);

  const beginRename = useCallback(() => {
    if (!onRename) return;
    setNameDraft(device.displayName || '');
    setRenameError(null);
    setIsEditingName(true);
    setIconMenuOpen(false);
    setActionMenuOpen(false);
  }, [device.displayName, onRename]);

  const cancelRename = useCallback(() => {
    setIsEditingName(false);
    setNameDraft(device.displayName || '');
    setRenameError(null);
    setIconMenuOpen(false);
    setActionMenuOpen(false);
  }, [device.displayName]);

  const handleRenameSubmit = useCallback(async () => {
    if (!onRename) {
      setIsEditingName(false);
      return;
    }
    const trimmed = nameDraft.trim();
    if (!trimmed) {
      setRenameError('Name cannot be empty');
      return;
    }
    if (trimmed === (device.displayName || '').trim()) {
      setIsEditingName(false);
      setIconMenuOpen(false);
      setActionMenuOpen(false);
      return;
    }
    setRenamePending(true);
    try {
      await onRename(device, trimmed);
      setNameDraft(trimmed);
      setRenameError(null);
      setIsEditingName(false);
      setIconMenuOpen(false);
      setActionMenuOpen(false);
    } catch (err) {
      setRenameError(err?.message || 'Unable to rename device');
    } finally {
      setRenamePending(false);
    }
  }, [onRename, nameDraft, device]);

  const [showAllMetrics, setShowAllMetrics] = useState(false);
  useEffect(() => { setShowAllMetrics(false); }, [device.id]);
  const hasExtraMetrics = allMetrics.length > 5;
  const visibleMetrics = useMemo(
    () => (showAllMetrics ? allMetrics : allMetrics.slice(0, 5)),
    [allMetrics, showAllMetrics],
  );

  const activeIconKey = device?.icon && device.icon !== '' ? device.icon : 'auto';

  useEffect(() => {
    if (!iconMenuOpen || !canEditIcon) return undefined;
    const handleClick = event => {
      if (!iconMenuRef.current) return;
      if (!iconMenuRef.current.contains(event.target)) {
        setIconMenuOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [canEditIcon, iconMenuOpen]);

  useEffect(() => {
    if (!actionMenuOpen) return undefined;
    const handleClick = event => {
      if (actionMenuRef.current && actionMenuRef.current.contains(event.target)) {
        return;
      }
      if (actionMenuButtonRef.current && actionMenuButtonRef.current.contains(event.target)) {
        return;
      }
      setActionMenuOpen(false);
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [actionMenuOpen]);

  const handleIconSelection = useCallback(async (nextKey) => {
    if (!onUpdateIcon) {
      setIconMenuOpen(false);
      return;
    }
    const normalizedKey = nextKey || 'auto';
    if (normalizedKey === activeIconKey) {
      setIconMenuOpen(false);
      return;
    }
    setIconPending(true);
    setIconError(null);
    try {
      await onUpdateIcon(device, normalizedKey === 'auto' ? null : normalizedKey);
      setIconMenuOpen(false);
    } catch (err) {
      setIconError(err?.message || 'Unable to update icon');
    } finally {
      setIconPending(false);
    }
  }, [activeIconKey, device, onUpdateIcon]);

  const handleDelete = useCallback(async (options = {}) => {
    if (!onDelete) return;
    setDeletePending(true);
    setDeleteError(null);
    try {
      await onDelete(device, options);
      setDeleteModalOpen(false);
      setForceDelete(false);
    } catch (err) {
      setDeleteError(err?.message || 'Unable to delete device');
    } finally {
      setDeletePending(false);
    }
  }, [device, onDelete]);

  const openDeleteModal = useCallback(() => {
    setForceDelete(false);
    setDeleteError(null);
    setDeleteModalOpen(true);
    setActionMenuOpen(false);
  }, []);

  const closeDeleteModal = useCallback(() => {
    if (deletePending) return;
    setDeleteModalOpen(false);
    setForceDelete(false);
    setDeleteError(null);
  }, [deletePending]);

  const confirmDelete = useCallback(() => {
    handleDelete({ force: forceDelete });
  }, [forceDelete, handleDelete]);

  const toggleActionMenu = useCallback(() => {
    if (!onRename && !onDelete) return;
    setActionMenuOpen(prev => !prev);
  }, [onDelete, onRename]);

  const handleMenuRename = useCallback(() => {
    setActionMenuOpen(false);
    beginRename();
  }, [beginRename]);

  const handleMenuDelete = useCallback(() => {
    openDeleteModal();
  }, [openDeleteModal]);

  const description = useMemo(() => {
    if (!device.description) return '';
    if (device.description.length <= DESCRIPTION_LIMIT) return device.description;
    return `${device.description.slice(0, DESCRIPTION_LIMIT - 1)}…`;
  }, [device.description]);

  const deleteModalElement = deleteModalOpen ? (
    <div className="device-delete-modal-backdrop">
      <div
        className="device-delete-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={deleteDialogTitleId}
        aria-describedby={deleteDialogDescId}
      >
        <div className="device-delete-modal-body">
          <p className="device-delete-eyebrow">Device → Edit</p>
          <h3 id={deleteDialogTitleId}>Delete {device.displayName || device.name || 'this device'}?</h3>
          <p id={deleteDialogDescId}>
            Removing this device clears its historical state from dashboards and automations. This action cannot be undone.
          </p>
          <label className="device-delete-force-toggle" htmlFor={deleteForceCheckboxId}>
            <input
              type="checkbox"
              id={deleteForceCheckboxId}
              checked={forceDelete}
              onChange={event => setForceDelete(event.target.checked)}
              disabled={deletePending}
              aria-describedby={deleteForceNoteId}
            />
            <div>
              <span>Force delete</span>
              <p id={deleteForceNoteId}>
                Force delete bypasses adapter acknowledgements and purges the record immediately. Use this when the bridge
                or coordinator no longer recognizes the hardware or the removal queue is stuck.
              </p>
            </div>
          </label>
          {deleteError ? <div className="device-delete-error">{deleteError}</div> : null}
          <div className="device-delete-actions">
            <button type="button" className="device-delete-cancel" onClick={closeDeleteModal} disabled={deletePending}>
              Cancel
            </button>
            <button type="button" className="device-delete-confirm" onClick={confirmDelete} disabled={deletePending}>
              {deletePending ? 'Removing…' : forceDelete ? 'Force delete' : 'Delete'}
            </button>
          </div>
        </div>
      </div>
    </div>
  ) : null;

  const deleteModal = deleteModalElement
    ? (typeof document !== 'undefined'
      ? createPortal(deleteModalElement, document.body)
      : deleteModalElement)
    : null;

  return (
    <>
      <GlassCard className={`device-tile-card ${device.online ? 'device-online' : 'device-offline'}`}>
        <div className="device-tile">
        <div className="device-tile-header">
          <div className="device-title-container">
            <div className="device-title-row">
              <div className="device-title-leading" ref={iconMenuRef}>
                {canEditIcon ? (
                  <>
                    <button
                      type="button"
                      className="device-title-icon-button editable"
                      onClick={() => setIconMenuOpen(prev => !prev)}
                      title="Change icon"
                      aria-label="Change icon"
                    >
                      {iconGlyph}
                    </button>
                    <span className="device-icon-hint">Tap to change</span>
                  </>
                ) : (
                  iconGlyph
                )}
                {showIconPicker ? (
                  <div className="device-icon-picker">
                    <div className="device-icon-picker-grid">
                      {DEVICE_ICON_CHOICES.map(choice => (
                        <button
                          key={choice.key}
                          type="button"
                          className={`device-icon-choice${activeIconKey === choice.key ? ' active' : ''}`}
                          onClick={() => handleIconSelection(choice.key)}
                          disabled={iconPending}
                        >
                          <FontAwesomeIcon icon={choice.icon} />
                          <span>{choice.label}</span>
                        </button>
                      ))}
                    </div>
                    {iconError ? <div className="device-icon-error">{iconError}</div> : null}
                  </div>
                ) : null}
              </div>
              {isEditingName ? (
                <input
                  className="device-title-edit-input"
                  value={nameDraft}
                  onChange={e => setNameDraft(e.target.value)}
                  onKeyDown={e => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                      handleRenameSubmit();
                    }
                    if (e.key === 'Escape') {
                      e.preventDefault();
                      cancelRename();
                    }
                  }}
                  disabled={renamePending}
                  placeholder="Device name"
                  autoFocus
                />
              ) : (
                <span className="device-title">{device.displayName}</span>
              )}
              {onRename || onDelete ? (
                <div className="device-title-actions">
                  {isEditingName ? (
                    <>
                      <button
                        type="button"
                        className="device-title-action device-title-cancel"
                        onClick={cancelRename}
                        disabled={renamePending}
                        title="Cancel rename"
                      >
                        <FontAwesomeIcon icon={faXmark} />
                      </button>
                      <button
                        type="button"
                        className="device-title-action device-title-save"
                        onClick={handleRenameSubmit}
                        disabled={renamePending}
                        title="Save name"
                      >
                        <FontAwesomeIcon icon={faCheck} />
                      </button>
                    </>
                  ) : (
                    <div className="device-title-menu-wrapper">
                      <button
                        type="button"
                        className="device-title-action device-title-edit"
                        onClick={toggleActionMenu}
                        aria-haspopup="true"
                        aria-expanded={actionMenuOpen}
                        aria-label="device -> Edit"
                        ref={actionMenuButtonRef}
                        title="device -> Edit"
                        disabled={!onRename && !onDelete}
                      >
                        <FontAwesomeIcon icon={faPen} />
                      </button>
                      {actionMenuOpen ? (
                        <div className="device-title-menu" ref={actionMenuRef}>
                          {onRename ? (
                            <button type="button" onClick={handleMenuRename}>
                              <FontAwesomeIcon icon={faPen} />
                              <span>Edit</span>
                            </button>
                          ) : null}
                          {onDelete ? (
                            <button
                              type="button"
                              className="danger"
                              onClick={handleMenuDelete}
                              disabled={deletePending}
                            >
                              <FontAwesomeIcon icon={faTrash} />
                              <span>{deletePending ? 'Deleting…' : 'Delete'}</span>
                            </button>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                  )}
                </div>
              ) : null}
            </div>
            <div className="device-meta">
              {device.manufacturer || 'Unknown'}
              {device.model ? <span className="device-meta-dot">•</span> : null}
              {device.model || null}
              {device.type ? <span className="device-meta-dot">•</span> : null}
              {device.type || null}
            </div>
            {renameError ? <div className="device-rename-error">{renameError}</div> : null}
            {deleteError ? <div className="device-rename-error">{deleteError}</div> : null}
            {iconError && !iconMenuOpen ? (
              <div className="device-rename-error">{iconError}</div>
            ) : null}
          </div>
          {showHeaderToggle ? (
            <GlassSwitch checked={Boolean(toggleValue)} disabled={toggleDisabled} onChange={handlePrimaryToggle} />
          ) : (
            <span className={`device-status ${device.online ? 'online' : 'offline'}`}>
              <span className="device-status-dot" />
              {device.online ? 'Online' : 'Offline'}
            </span>
          )}
        </div>

        <div className="device-pill-row">
          <GlassPill icon={faMicrochip} text={device.protocol?.toUpperCase() || 'Unknown'} />
          <GlassPill icon={faTicket} text={`${capabilities.length} capabilities`} />
          {device.firmware ? <GlassPill text={`FW ${device.firmware}`} tone="default" /> : null}
        </div>

        {binaryMetrics.length > 0 ? (
          <div className="device-state-pills">
            {binaryMetrics.map(metric => (
              <GlassPill
                key={metric.key}
                icon={metric.icon}
                tone={resolveBinaryTone(metric.key, metric.rawValue)}
                text={`${formatDisplayLabel(metric.label)}: ${metric.value}`}
              />
            ))}
          </div>
        ) : null}

        {description ? <p className="device-description">{description}</p> : null}

        {interactiveInputs.length > 0 ? (
          <div className="device-controls">
            {visibleControls.map(input => {
              const key = sanitizeInputKey(input);
              const value = controlValues[key];
              const icon = ICON_BY_INPUT_TYPE[input.type] || faGaugeHigh;
              const displayValue = formatControlValue(input, value);
              const range = input.range || {};
              const min = typeof range?.min === 'number' ? range.min : 0;
              const max = typeof range?.max === 'number' ? range.max : (input.type === 'slider' ? 255 : undefined);
              const step = typeof range?.step === 'number' ? range.step : (input.type === 'slider' ? 1 : undefined);
              const label = resolveInputLabel(input);
              switch (input.type) {
              case 'toggle':
                return (
                  <div className="device-control" key={key}>
                    <div className="device-control-label">
                      <FontAwesomeIcon icon={icon} />
                      <span>{label}</span>
                      {displayValue ? <span className="device-control-value">{displayValue}</span> : null}
                    </div>
                    <GlassSwitch
                      checked={Boolean(value)}
                      disabled={pending}
                      onChange={next => handleToggleInput(input, next)}
                    />
                  </div>
                );
              case 'slider':
                return (
                  <div className="device-control device-control-wide" key={key}>
                    <div className="device-control-label">
                      <FontAwesomeIcon icon={icon} />
                      <span>{label}</span>
                      <span className="device-control-value">{Math.round(value ?? min)}</span>
                    </div>
                    <div className="device-control-slider">
                      <input
                        type="range"
                        min={min}
                        max={max}
                        step={step}
                        value={value ?? min}
                        disabled={pending}
                        onChange={e => handleSliderChange(input, e.target.value)}
                        onMouseUp={e => handleSliderCommit(input, e.target.value)}
                        onTouchEnd={e => handleSliderCommit(input, e.target.value)}
                        onKeyUp={e => { if (e.key === 'Enter') handleSliderCommit(input, e.target.value); }}
                        onBlur={e => handleSliderCommit(input, e.target.value)}
                        className="device-control-range"
                      />
                      <div className="device-control-slider-scale">
                        <span>{min}</span>
                        <span>{max}</span>
                      </div>
                    </div>
                  </div>
                );
              case 'select':
                return (
                  <div className="device-control" key={key}>
                    <div className="device-control-label">
                      <FontAwesomeIcon icon={icon} />
                      <span>{label}</span>
                    </div>
                    <select
                      className="device-control-select"
                      value={value ?? (input.options[0]?.value || '')}
                      disabled={pending}
                      onChange={e => handleSelectChange(input, e.target.value)}
                    >
                      {(input.options || []).map(option => (
                        <option key={option.value} value={option.value}>
                          {option.label || option.value}
                        </option>
                      ))}
                    </select>
                  </div>
                );
              case 'number':
                return (
                  <div className="device-control" key={key}>
                    <div className="device-control-label">
                      <FontAwesomeIcon icon={icon} />
                      <span>{label}</span>
                    </div>
                    <input
                      type="number"
                      className="device-control-number"
                      min={min}
                      max={max}
                      step={step || 1}
                      value={value === '' ? '' : value ?? ''}
                      disabled={pending}
                      onChange={e => {
                        const raw = e.target.value;
                        if (raw === '') {
                          updateControlValue(input, '');
                          return;
                        }
                        const numeric = Number(raw);
                        if (!Number.isNaN(numeric)) {
                          updateControlValue(input, numeric);
                        }
                      }}
                      onBlur={e => handleNumberCommit(input, e.target.value)}
                      onKeyUp={e => { if (e.key === 'Enter') handleNumberCommit(input, e.target.value); }}
                    />
                  </div>
                );
              case 'color': {
                const draft = colorDrafts[key];
                const normalizedValue = normalizeColorHex(draft?.current ?? value);
                const isActive = activeColorInput === key;
                const openPicker = () => {
                  if (pending) return;
                  setActiveColorInput(key);
                  setColorDrafts(prev => ({
                    ...prev,
                    [key]: {
                      initial: normalizeColorHex(value ?? '#FFFFFF'),
                      current: normalizeColorHex(value ?? '#FFFFFF'),
                    },
                  }));
                };
                const closePicker = () => {
                  setActiveColorInput(prev => (prev === key ? null : prev));
                  setColorDrafts(prev => {
                    const next = { ...prev };
                    delete next[key];
                    return next;
                  });
                };
                const updateDraft = hex => {
                  const normalized = normalizeColorHex(hex);
                  setColorDrafts(prev => ({
                    ...prev,
                    [key]: {
                      initial: prev[key]?.initial ?? normalizeColorHex(value ?? '#FFFFFF'),
                      current: normalized,
                    },
                  }));
                };
                const applyDraft = () => {
                  const finalHex = normalizeColorHex(colorDrafts[key]?.current ?? value ?? '#FFFFFF');
                  updateControlValue(input, finalHex);
                  handleInputCommand(input, finalHex);
                  closePicker();
                };
                return (
                  <div
                    className={`device-control device-color-control device-control-wide${isActive ? ' device-color-active' : ''}`}
                    key={key}
                  >
                    <div className="device-control-label">
                      <FontAwesomeIcon icon={icon} />
                      <span>{label}</span>
                      <span className="device-control-value">{normalizedValue}</span>
                    </div>
                    <div className="device-color-summary">
                      <button
                        type="button"
                        className="device-color-toggle"
                        onClick={() => (isActive ? closePicker() : openPicker())}
                        disabled={pending}
                      >
                        <span className="device-color-swatch" style={{ backgroundColor: normalizedValue }} />
                        <span>{isActive ? 'Close picker' : 'Adjust color'}</span>
                      </button>
                    </div>
                    {isActive ? (
                      <div className="device-color-popover">
                        <HexColorPicker color={normalizedValue} onChange={updateDraft} />
                        <div className="device-color-actions">
                          <input
                            type="text"
                            className="device-color-input"
                            value={normalizedValue}
                            onChange={e => updateDraft(e.target.value)}
                          />
                          <div className="device-color-buttons">
                            <button type="button" className="device-color-cancel" onClick={closePicker}>Cancel</button>
                            <button
                              type="button"
                              className="device-color-apply"
                              onClick={applyDraft}
                              disabled={pending}
                            >
                              Apply
                            </button>
                          </div>
                        </div>
                      </div>
                    ) : null}
                  </div>
                );
              }
              default:
                return null;
              }
            })}
            {hasExtraControls ? (
              <button
                type="button"
                className="device-controls-toggle"
                onClick={() => setShowAllControls(prev => !prev)}
              >
                {showAllControls ? 'Show fewer controls' : `Show ${hiddenControlsCount} more`}
              </button>
            ) : null}
          </div>
        ) : null}

        <div className="device-metrics">
          {visibleMetrics.map(metric => (
            <GlassMetric
              key={metric.key}
              icon={metric.icon}
              label={metric.label}
              value={metric.value}
              unit={metric.unit}
            />
          ))}
          {hasExtraMetrics ? (
            <button
              type="button"
              className="device-metrics-toggle"
              onClick={() => setShowAllMetrics(prev => !prev)}
            >
              {showAllMetrics ? 'Show fewer details' : `Show ${Math.max(allMetrics.length - 5, 0)} more`}
            </button>
          ) : null}
        </div>

        <div className="device-footer">
          <span className="device-last-seen">Last seen {formatRelativeTime(device.lastSeen || device.stateUpdatedAt || device.updatedAt)}</span>
          {pending ? <span className="device-command-pending">Updating…</span> : null}
        </div>
      </div>
      </GlassCard>
      {deleteModal}
    </>
  );
}
