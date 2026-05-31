import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBatteryThreeQuarters,
  faBolt,
  faDoorOpen,
  faDroplet,
  faLayerGroup,
  faLightbulb,
  faMicrochip,
  faPen,
  faPlus,
  faPlug,
  faTag,
  faThermometerHalf,
  faTrash,
  faUsers,
} from '@fortawesome/free-solid-svg-icons';
import PageHeader from '../common/PageHeader/PageHeader';
import GlassCard from '../common/GlassCard/GlassCard';
import GlassPill from '../common/GlassPill/GlassPill';
import LoadingView from '../common/LoadingView/LoadingView';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import BaseModal from '../common/BaseModal/BaseModal';
import SearchBar from '../common/SearchBar/SearchBar';
import Button from '../common/Button/Button';
import DeviceTile from '../Devices/DeviceTile';
import DeviceControlList from '../common/DeviceControlRenderer/DeviceControlRenderer';
import { sanitizeInputKey, toControlBoolean } from '../common/DeviceControlRenderer/deviceControlUtils';
import { DEVICE_ICON_MAP } from '../Devices/deviceIconChoices';
import useDeviceHubDevices from '../../hooks/useDeviceHubDevices';
import useErsInventory from '../../hooks/useErsInventory';
import { useAuth } from '../../context/AuthContext';
import { sendDeviceCommand } from '../../services/deviceHubService';
import {
  createErsGroup,
  deleteErsGroup,
  patchErsGroup,
  setErsGroupMembers,
} from '../../services/entityRegistryService';
import { resolveCommandDeviceId } from '../../utils/deviceIdentity';
import { normalizeColorHex } from '../../utils/colorHex';
import '../Devices/Devices.css';
import '../Devices/DeviceTile.css';
import '../Devices/DeviceDetail.css';
import '../Devices/AddDeviceModal.css';
import '../Auth/AuthModal/AuthModal.css';
import './Groups.css';

const GROUP_SHARED_MIXED_GRACE_MS = 2800;

function arrayOrEmpty(value) {
  return Array.isArray(value) ? value : [];
}

function stringOrEmpty(value) {
  return typeof value === 'string' ? value.trim() : '';
}

function relativeTime(value) {
  const parsed = Date.parse(stringOrEmpty(value));
  if (!Number.isFinite(parsed)) return 'recently';
  const seconds = Math.max(0, Math.round((Date.now() - parsed) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  if (days < 7) return `${days}d ago`;
  return new Date(parsed).toLocaleDateString();
}

function resolveGroupDeviceIcon(device) {
  const manualKey = typeof device?.icon === 'string' ? device.icon.toLowerCase() : '';
  if (manualKey && manualKey !== 'auto' && DEVICE_ICON_MAP[manualKey]) {
    return DEVICE_ICON_MAP[manualKey];
  }
  const keywords = [device?.type, device?.description, device?.model, device?.displayName, device?.manufacturer]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  const inputs = arrayOrEmpty(device?.inputs);
  const hasProperty = (key) => inputs.some((input) => {
    const property = String(input?.property || input?.capability_id || input?.id || '').toLowerCase();
    return property.includes(key);
  });
  if (hasProperty('contact') || keywords.includes('door')) return faDoorOpen;
  if (hasProperty('brightness') || hasProperty('color') || keywords.includes('light') || keywords.includes('lamp')) return faLightbulb;
  if (hasProperty('power') || keywords.includes('plug') || keywords.includes('socket') || keywords.includes('outlet')) return faPlug;
  if (hasProperty('temperature') || keywords.includes('thermo') || keywords.includes('heating')) return faThermometerHalf;
  if (hasProperty('humidity')) return faDroplet;
  if (hasProperty('battery')) return faBatteryThreeQuarters;
  if (hasProperty('voltage') || hasProperty('power')) return faBolt;
  return faMicrochip;
}

function getCapabilityKeyParts(cap) {
  if (!cap || typeof cap !== 'object') return [];
  return [cap.id, cap.property, cap.name]
    .filter((part) => part !== undefined && part !== null)
    .map((part) => part.toString().toLowerCase());
}

function collectCapabilities(device) {
  const lists = [
    arrayOrEmpty(device?.capabilities),
    arrayOrEmpty(device?.state?.capabilities),
  ];
  const seen = new Set();
  const result = [];
  lists.flat().forEach((cap) => {
    if (!cap || typeof cap !== 'object') return;
    const keys = getCapabilityKeyParts(cap);
    const primary = keys[0];
    if (primary && seen.has(primary)) return;
    if (keys.length === 0) {
      result.push(cap);
      return;
    }
    keys.forEach((key) => seen.add(key));
    result.push(cap);
  });
  return result;
}

function isCapabilityWritable(cap) {
  if (!cap || typeof cap !== 'object') return false;
  const access = cap.access || {};
  if (access.readOnly === true) return false;
  const negativeFlags = [access.write, access.set, access.command, access.toggle].filter((flag) => flag === false);
  if (negativeFlags.length > 0) return false;
  const positiveFlags = [access.write, access.set, access.command, access.toggle].filter((flag) => flag === true);
  if (positiveFlags.length > 0) return true;
  if ('write' in access || 'set' in access || 'command' in access || 'toggle' in access) {
    return Boolean(access.write || access.set || access.command || access.toggle);
  }
  return false;
}

function inferInputTypeFromCapability(cap) {
  const kind = (cap.kind || '').toString().toLowerCase();
  const valueType = (cap.value_type || cap.valueType || '').toString().toLowerCase();
  const name = (cap.name || '').toString().toLowerCase();
  const property = (cap.property || '').toString().toLowerCase();
  if (property === 'color' || name.includes('color')) return 'color';
  if (kind === 'binary' || valueType === 'boolean') return 'toggle';
  if (kind === 'enum' || valueType === 'enum' || Array.isArray(cap.enum)) return 'select';
  if (kind === 'numeric' || valueType === 'number') {
    if (typeof cap.range?.min === 'number' && typeof cap.range?.max === 'number') return 'slider';
    return 'number';
  }
  if (valueType === 'object' && (name.includes('color') || property === 'color')) return 'color';
  return null;
}

function formatDisplayLabel(value) {
  if (!value) return '';
  return value
    .toString()
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function buildInputsFromCapabilities(capabilities) {
  const results = [];
  capabilities.forEach((cap) => {
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
      ? cap.enum.map((value) => ({ value, label: formatDisplayLabel(value) }))
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

function buildCapabilityLookup(capabilities) {
  const map = new Map();
  capabilities.forEach((cap) => {
    getCapabilityKeyParts(cap).forEach((key) => {
      if (key && !map.has(key)) {
        map.set(key, cap);
      }
    });
  });
  return map;
}

function resolveCapabilityForInput(capabilityLookup, input) {
  if (!input || !capabilityLookup) return null;
  const candidates = [input.capability_id, input.capabilityId, input.capabilityID, input.id, input.property];
  for (const candidate of candidates) {
    if (!candidate) continue;
    const key = candidate.toString().toLowerCase();
    if (capabilityLookup.has(key)) return capabilityLookup.get(key);
  }
  return null;
}

function resolveStateValue(state, cap) {
  if (!state) return undefined;
  const candidates = [cap.id, cap.property, cap.name].filter(Boolean).map((key) => key.toString());
  for (const key of candidates) {
    if (state[key] !== undefined) return state[key];
    const lower = key.toLowerCase();
    if (state[lower] !== undefined) return state[lower];
    const camel = lower.replace(/_([a-z])/g, (_, char) => char.toUpperCase());
    if (state[camel] !== undefined) return state[camel];
  }
  return undefined;
}

function extractStateColorHex(raw) {
  const normalized = normalizeColorHex(raw, '');
  return normalized || null;
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
  arrayOrEmpty(inputs).forEach((input) => {
    const key = sanitizeInputKey(input);
    if (!key) return;
    let raw = getStateValueForInput(device, input);
    if (raw === undefined && input.type === 'toggle' && typeof device.toggleState === 'boolean') raw = device.toggleState;
    if (raw === undefined && input.type === 'slider' && input.range && typeof input.range.min === 'number') raw = input.range.min;
    if (raw === undefined && input.type === 'select' && input.options?.length) raw = input.options[0].value;
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
      if (raw !== undefined) values[key] = raw;
    }
  });
  return values;
}

function normalizeToggleProperty(property) {
  const key = (property ?? '').toString().trim();
  const lower = key.toLowerCase();
  if (lower === 'state' || lower === 'power' || lower === 'on') return 'on';
  return key;
}

function buildPayloadForInput(input, value) {
  const key = sanitizeInputKey(input);
  if (!key) return null;
  const stateKeyRaw = (input.property || key || '').toString().trim();
  if (!stateKeyRaw) return null;
  const state = {};
  const propertyLower = stateKeyRaw.toLowerCase();
  switch (input.type) {
  case 'toggle':
    if (input?.metadata?.togglePowerString === true || propertyLower === 'power') {
      state.power = toControlBoolean(value) ? 'on' : 'off';
    } else {
      state[normalizeToggleProperty(stateKeyRaw)] = toControlBoolean(value);
    }
    break;
  case 'slider':
  case 'number': {
    const numeric = Number(value);
    if (Number.isNaN(numeric)) return null;
    state[stateKeyRaw] = numeric;
    if (propertyLower.startsWith('brightness')) state.on = numeric > 0;
    break;
  }
  case 'color':
    state[stateKeyRaw] = value;
    state.on = true;
    break;
  case 'select':
    state[stateKeyRaw] = value;
    if (propertyLower.includes('effect')) state.on = true;
    break;
  default:
    state[stateKeyRaw] = value;
  }
  return { state };
}

function buildNormalizedInputs(device) {
  const capabilities = collectCapabilities(device);
  const capabilityLookup = buildCapabilityLookup(capabilities);
  const rawList = arrayOrEmpty(device?.inputs);
  const hasCompositeColor = rawList.some((input) => (input?.type || '').toLowerCase() === 'color' && input?.metadata?.mode === 'composite');
  const suppressedProperties = new Set();
  if (hasCompositeColor) {
    ['x', 'y', 'hue', 'saturation', 'color_x', 'color_y', 'color_hs', 'color_xy'].forEach((key) => suppressedProperties.add(key));
  }
  const normalized = rawList
    .map((input) => {
      const originalType = (input.type || '').toLowerCase();
      const capabilityId = input.capability_id || input.capabilityId || input.capabilityID || '';
      const property = input.property || capabilityId || input.id || '';
      const propertyLower = property.toString().toLowerCase();
      const options = Array.isArray(input.options) ? input.options : [];
      const hasOn = options.some((opt) => String(opt?.value || '').toLowerCase() === 'on');
      const hasOff = options.some((opt) => String(opt?.value || '').toLowerCase() === 'off');
      const isPowerSelect = originalType === 'select' && propertyLower === 'power' && hasOn && hasOff;
      const type = isPowerSelect ? 'toggle' : originalType;
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
        options,
        metadata: {
          ...(input.metadata || {}),
          ...(isPowerSelect ? { togglePowerString: true } : {}),
        },
        range: input.range || input.Range || null,
      };
    })
    .filter((input) => {
      if (!input.id || input.readOnly) return false;
      if (!['toggle', 'slider', 'number', 'select', 'color'].includes(input.type)) return false;
      const propertyKey = (input.property || input.id || '').toString().toLowerCase();
      if (suppressedProperties.has(propertyKey)) return false;
      return true;
    });
  const seenKeys = new Set(normalized.map((item) => sanitizeInputKey(item).toLowerCase()));
  buildInputsFromCapabilities(capabilities).forEach((fallback) => {
    const key = sanitizeInputKey(fallback).toLowerCase();
    if (!key || seenKeys.has(key)) return;
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
}

function valuesEqualByType(type, left, right) {
  if (type === 'color') return extractStateColorHex(left) === extractStateColorHex(right);
  if (type === 'toggle') return toControlBoolean(left) === toControlBoolean(right);
  return String(left ?? '') === String(right ?? '');
}

function formatMixedValuePreview(type, value) {
  if (value === undefined || value === null || value === '') return null;
  if (type === 'toggle') return toControlBoolean(value) ? 'on' : 'off';
  if (type === 'color') return extractStateColorHex(value)?.toUpperCase() || null;
  if (typeof value === 'number') return Number.isInteger(value) ? String(value) : String(Math.round(value * 10) / 10);
  return String(value).trim() || null;
}

function buildMixedControlAnnotation(input, values) {
  const label = stringOrEmpty(input?.label) || formatDisplayLabel(input?.property || input?.id || 'value');
  const prefix = `Mixed ${label.toLowerCase()}`;
  const previews = Array.from(new Set(values
    .map((value) => formatMixedValuePreview(input?.type, value))
    .filter(Boolean)));
  if (previews.length >= 2 && previews.length <= 3) {
    return `${prefix}: ${previews.join(' / ')}`;
  }
  return prefix;
}

function intersectSharedInputs(devices) {
  const members = arrayOrEmpty(devices);
  if (!members.length) return { inputs: [], values: {}, mixedKeys: [] };
  const normalizedByDevice = members.map((device) => ({
    device,
    inputs: buildNormalizedInputs(device),
  }));
  if (normalizedByDevice.some((entry) => entry.inputs.length === 0)) {
    return { inputs: [], values: {}, mixedKeys: [] };
  }
  const maps = normalizedByDevice.map((entry) => {
    const map = new Map();
    entry.inputs.forEach((input) => {
      const key = sanitizeInputKey(input).toLowerCase();
      if (key) map.set(key, input);
    });
    return map;
  });
  const sharedKeys = Array.from(maps[0].keys()).filter((key) => maps.every((map) => map.has(key)));
  const sharedInputs = [];
  const sharedValues = {};
  const mixedKeys = [];
  const mixedAnnotations = {};
  sharedKeys.forEach((key) => {
    const candidates = maps.map((map) => map.get(key)).filter(Boolean);
    if (!candidates.length) return;
    const base = candidates[0];
    if (!candidates.every((candidate) => candidate.type === base.type)) return;
    if (base.type === 'select') {
      const optionSets = candidates.map((candidate) => new Map(arrayOrEmpty(candidate.options).map((option) => [String(option.value).toLowerCase(), option])));
      const intersectedOptions = arrayOrEmpty(base.options).filter((option) => optionSets.every((set) => set.has(String(option.value).toLowerCase())));
      if (!intersectedOptions.length) return;
      base.options = intersectedOptions;
    }
    if (base.type === 'slider' || base.type === 'number') {
      const mins = candidates.map((candidate) => candidate.range?.min).filter((value) => typeof value === 'number');
      const maxes = candidates.map((candidate) => candidate.range?.max).filter((value) => typeof value === 'number');
      const steps = candidates.map((candidate) => candidate.range?.step).filter((value) => typeof value === 'number');
      const min = mins.length ? Math.max(...mins) : undefined;
      const max = maxes.length ? Math.min(...maxes) : undefined;
      if (typeof min === 'number' && typeof max === 'number' && max < min) return;
      base.range = {
        ...(base.range || {}),
        ...(typeof min === 'number' ? { min } : {}),
        ...(typeof max === 'number' ? { max } : {}),
        ...(steps.length ? { step: Math.max(...steps) } : {}),
      };
    }
    sharedInputs.push(base);
    const memberValues = normalizedByDevice.map(({ device }) => getStateValueForInput(device, base));
    const comparableValues = memberValues.filter((value) => value !== undefined);
    const initialValues = buildInitialControlValues(normalizedByDevice[0].device, [base]);
    const baseValue = comparableValues.length ? comparableValues[0] : initialValues[sanitizeInputKey(base)];
    sharedValues[sanitizeInputKey(base)] = base.type === 'color'
      ? extractStateColorHex(baseValue) || initialValues[sanitizeInputKey(base)]
      : base.type === 'toggle'
        ? toControlBoolean(baseValue)
        : baseValue;
    if (comparableValues.length > 1 && !comparableValues.every((value) => valuesEqualByType(base.type, comparableValues[0], value))) {
      const inputKey = sanitizeInputKey(base);
      mixedKeys.push(inputKey);
      mixedAnnotations[inputKey] = buildMixedControlAnnotation(base, comparableValues);
      base.mixed = true;
      base.annotation = mixedAnnotations[inputKey];
    }
  });
  return { inputs: sharedInputs, values: sharedValues, mixedKeys, mixedAnnotations };
}

function getCommandDeviceId(device) {
  return resolveCommandDeviceId(device);
}

function normalizeGroup(group) {
  const devices = arrayOrEmpty(group?.devices);
  return {
    ...group,
    id: stringOrEmpty(group?.id),
    slug: stringOrEmpty(group?.slug),
    name: stringOrEmpty(group?.name) || stringOrEmpty(group?.slug) || stringOrEmpty(group?.id),
    description: stringOrEmpty(group?.description),
    devices,
    deviceIds: devices
      .map((device) => stringOrEmpty(device?.ersId || device?.id))
      .filter(Boolean),
    updatedAt: stringOrEmpty(group?.updatedAt || group?.updated_at || group?.createdAt || group?.created_at),
  };
}

function buildGroupStats(group) {
  const members = arrayOrEmpty(group?.devices);
  const roomNames = Array.from(new Set(members.map((device) => stringOrEmpty(device?.roomName)).filter(Boolean)));
  return {
    memberCount: members.length,
    roomNames,
  };
}

function useGroupSharedControls(members, { accessToken, enabled }) {
  const [sharedValues, setSharedValues] = useState({});
  const [sharedError, setSharedError] = useState('');
  const [pendingCount, setPendingCount] = useState(0);
  const graceRef = useRef(new Map());
  const graceTimersRef = useRef(new Map());
  const [graceVersion, setGraceVersion] = useState(0);

  const sharedControls = useMemo(() => intersectSharedInputs(members), [members]);
  const sharedInputIds = useMemo(
    () => sharedControls.inputs.map((input) => sanitizeInputKey(input)).filter(Boolean).join('|'),
    [sharedControls.inputs],
  );

  const clearGrace = useCallback((key) => {
    const timer = graceTimersRef.current.get(key);
    if (timer) {
      window.clearTimeout(timer);
      graceTimersRef.current.delete(key);
    }
    if (graceRef.current.delete(key)) {
      setGraceVersion((value) => value + 1);
    }
  }, []);

  const applyGrace = useCallback((key, value) => {
    if (!key) return;
    clearGrace(key);
    graceRef.current.set(key, {
      value,
      expiresAt: Date.now() + GROUP_SHARED_MIXED_GRACE_MS,
    });
    const timer = window.setTimeout(() => {
      graceTimersRef.current.delete(key);
      if (graceRef.current.delete(key)) {
        setGraceVersion((current) => current + 1);
      }
    }, GROUP_SHARED_MIXED_GRACE_MS + 25);
    graceTimersRef.current.set(key, timer);
    setGraceVersion((current) => current + 1);
  }, [clearGrace]);

  useEffect(() => {
    setSharedValues(() => {
      const next = { ...sharedControls.values };
      graceRef.current.forEach((entry, key) => {
        if (entry?.expiresAt > Date.now()) {
          next[key] = entry.value;
        }
      });
      return next;
    });
  }, [sharedControls.values, sharedInputIds]);

  useEffect(() => () => {
    graceTimersRef.current.forEach((timer) => window.clearTimeout(timer));
    graceTimersRef.current.clear();
    graceRef.current.clear();
  }, []);

  const resolvedControls = useMemo(() => {
    const nextValues = { ...sharedControls.values };
    const nextInputs = sharedControls.inputs.map((input) => {
      const key = sanitizeInputKey(input);
      const grace = graceRef.current.get(key);
      if (!grace || grace.expiresAt <= Date.now()) {
        return input;
      }
      nextValues[key] = grace.value;
      return {
        ...input,
        mixed: false,
        annotation: pendingCount > 0 ? 'Syncing member state...' : '',
      };
    });
    const mixedKeys = sharedControls.mixedKeys.filter((key) => {
      const grace = graceRef.current.get(key);
      return !grace || grace.expiresAt <= Date.now();
    });
    return {
      inputs: nextInputs,
      values: nextValues,
      mixedKeys,
    };
  }, [graceVersion, pendingCount, sharedControls]);

  const handleSharedValueChange = useCallback((key, nextValue) => {
    setSharedValues((prev) => ({ ...prev, [key]: nextValue }));
  }, []);

  const handleSharedCommand = useCallback(async (input, nextValue) => {
    if (!enabled || !accessToken) {
      setSharedError('Authentication required');
      return;
    }
    const payload = buildPayloadForInput(input, nextValue);
    const inputKey = sanitizeInputKey(input);
    if (!payload || !inputKey) return;
    setSharedError('');
    setSharedValues((prev) => ({ ...prev, [inputKey]: nextValue }));
    applyGrace(inputKey, nextValue);
    setPendingCount((current) => current + 1);
    try {
      const results = await Promise.allSettled(members.map((device) => {
        const deviceId = getCommandDeviceId(device);
        if (!deviceId) return Promise.resolve({ success: false });
        return sendDeviceCommand(deviceId, payload, accessToken);
      }));
      const failed = results.filter((result) => result.status === 'rejected' || !result.value?.success).length;
      if (failed > 0) {
        clearGrace(inputKey);
        throw new Error(`${failed} device command${failed === 1 ? '' : 's'} failed`);
      }
      applyGrace(inputKey, nextValue);
    } catch (err) {
      setSharedError(err?.message || 'Unable to update shared group controls');
    } finally {
      setPendingCount((current) => Math.max(0, current - 1));
    }
  }, [accessToken, applyGrace, clearGrace, enabled, members]);

  return {
    sharedControls: resolvedControls,
    sharedValues,
    sharedPending: pendingCount > 0,
    sharedError,
    handleSharedValueChange,
    handleSharedCommand,
  };
}

function GroupSharedControlsPanel({ members, accessToken, enabled, compact = false, bootstrapping = false }) {
  const {
    sharedControls,
    sharedValues,
    sharedPending,
    sharedError,
    handleSharedValueChange,
    handleSharedCommand,
  } = useGroupSharedControls(members, { accessToken, enabled });

  return (
    <div className={`groups-shared-controls-panel${compact ? ' compact' : ''}`}>
      <div className="groups-shared-controls-head">
        <div>
          {compact ? <h4>Shared controls</h4> : <h3>Shared controls</h3>}
          <p>
            {compact
              ? 'Common actions stay available directly on the list card.'
              : 'Only capabilities available on every member are shown here. Changing a control fans the same command out to all devices.'}
          </p>
        </div>
        {sharedControls.inputs.length ? (
          <GlassPill icon={faBolt} text={`${sharedControls.inputs.length} common`} />
        ) : null}
      </div>

      {sharedError ? <div className="groups-shared-controls-error">{sharedError}</div> : null}
      {sharedControls.mixedKeys.length ? (
        <div className="groups-shared-controls-note">
          {sharedControls.mixedKeys.length} shared control{sharedControls.mixedKeys.length === 1 ? '' : 's'} currently show mixed member state.
        </div>
      ) : null}
      {sharedControls.inputs.length ? (
        <DeviceControlList
          inputs={sharedControls.inputs}
          values={sharedValues}
          pending={sharedPending || bootstrapping}
          onValueChange={handleSharedValueChange}
          onCommand={handleSharedCommand}
          layout={compact ? 'list' : 'cards'}
          collapseAfter={compact ? 3 : 8}
        />
      ) : (
        <div className="groups-shared-controls-empty">
          No common writable controls are available across all members yet.
        </div>
      )}
    </div>
  );
}

function GroupEditorModal({ open, onClose, onSubmit, devices, initialGroup, pending, error }) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [selectedIds, setSelectedIds] = useState([]);
  const [search, setSearch] = useState('');

  useEffect(() => {
    if (!open) return;
    setName(initialGroup?.name || '');
    setDescription(initialGroup?.description || '');
    setSelectedIds(arrayOrEmpty(initialGroup?.deviceIds));
    setSearch('');
  }, [initialGroup, open]);

  const filteredDevices = useMemo(() => {
    const query = stringOrEmpty(search).toLowerCase();
    const source = arrayOrEmpty(devices);
    if (!query) return source;
    return source.filter((device) => {
      const haystack = [
        device?.displayName,
        device?.name,
        device?.roomName,
        device?.manufacturer,
        device?.model,
        device?.hdpId,
      ].map((value) => stringOrEmpty(value).toLowerCase()).join(' ');
      return haystack.includes(query);
    });
  }, [devices, search]);

  const toggleDevice = useCallback((deviceId) => {
    setSelectedIds((prev) => (
      prev.includes(deviceId)
        ? prev.filter((id) => id !== deviceId)
        : [...prev, deviceId]
    ));
  }, []);

  const handleSubmit = useCallback((event) => {
    event.preventDefault();
    onSubmit({
      id: initialGroup?.id,
      name: stringOrEmpty(name),
      description: stringOrEmpty(description),
      deviceIds: selectedIds,
    });
  }, [description, initialGroup?.id, name, onSubmit, selectedIds]);

  if (!open) return null;

  return (
    <BaseModal
      open={open}
      onClose={onClose}
      backdropClassName="add-device-modal-backdrop"
      dialogClassName="add-device-modal-glass groups-editor-modal"
      closeAriaLabel="Close group dialog"
    >
      <div className="auth-modal-content add-device-shell groups-editor-shell">
        <div className="add-device-toolbar">
          <div className="add-device-toolbar-title">
            <div className="add-device-toolbar-heading">
              <h2>{initialGroup?.id ? 'Edit group' : 'Create group'}</h2>
              <p className="groups-editor-subtitle">
                Build a refined ERS collection that behaves like a first-class control target across Homenavi.
              </p>
            </div>
          </div>
        </div>

        <div className="auth-modal-content-outer add-device-scroll-region">
          <div className="auth-modal-content-inner add-device-form-shell">
            <form className="auth-modal-form add-device-form groups-editor-form" onSubmit={handleSubmit} noValidate>
              <div className="add-device-body-grid">
                <section className="add-device-card add-device-card-emphasis groups-editor-card">
                  <div className="add-device-card-head add-device-card-head-center">
                    <div>
                      <h4>Group details</h4>
                      <p>Name the collection and describe the shared controls it represents.</p>
                    </div>
                  </div>

                  <div className="groups-editor-fields">
                    <div className="auth-modal-field add-device-field groups-editor-field-wide">
                      <input
                        id="group-name"
                        className="auth-modal-input"
                        type="text"
                        placeholder=" "
                        value={name}
                        onChange={(event) => setName(event.target.value)}
                        disabled={pending}
                        required
                      />
                      <label className="auth-modal-label" htmlFor="group-name">Group name</label>
                    </div>

                    <div className="auth-modal-field add-device-field groups-editor-field-wide">
                      <textarea
                        id="group-description"
                        className="auth-modal-input groups-editor-textarea"
                        placeholder=" "
                        rows={4}
                        value={description}
                        onChange={(event) => setDescription(event.target.value)}
                        disabled={pending}
                      />
                      <label className="auth-modal-label" htmlFor="group-description">Description</label>
                    </div>
                  </div>
                </section>

                <section className="add-device-card groups-editor-card">
                  <div className="add-device-card-head add-device-card-head-center">
                    <div>
                      <h4>Group members</h4>
                      <p>Select the devices that should appear and behave together.</p>
                    </div>
                    <GlassPill icon={faUsers} text={`${selectedIds.length} selected`} />
                  </div>

                  <SearchBar
                    value={search}
                    onChange={setSearch}
                    onClear={() => setSearch('')}
                    placeholder="Search devices to add..."
                    ariaLabel="Search group devices"
                    className="groups-member-search"
                  />

                  <div className="groups-member-picker">
                    {filteredDevices.map((device) => {
                      const deviceId = stringOrEmpty(device?.ersId || device?.id);
                      if (!deviceId) return null;
                      const label = stringOrEmpty(device?.displayName || device?.name || device?.hdpId || deviceId);
                      const meta = [device?.roomName, device?.manufacturer, device?.model]
                        .map(stringOrEmpty)
                        .filter(Boolean)
                        .join(' • ');
                      const icon = resolveGroupDeviceIcon(device);
                      return (
                        <label key={deviceId} className="groups-member-option">
                          <input
                            type="checkbox"
                            checked={selectedIds.includes(deviceId)}
                            onChange={() => toggleDevice(deviceId)}
                            disabled={pending}
                          />
                          <span className="groups-member-option-icon" aria-hidden="true">
                            <FontAwesomeIcon icon={icon} />
                          </span>
                          <div className="groups-member-option-copy">
                            <span>{label}</span>
                            {meta ? <small>{meta}</small> : null}
                          </div>
                        </label>
                      );
                    })}
                    {filteredDevices.length === 0 ? (
                      <div className="groups-member-empty">No devices match the current search.</div>
                    ) : null}
                  </div>
                </section>
              </div>

              {error ? <div className="auth-modal-error groups-form-error">{error}</div> : null}

              <div className="groups-editor-actions">
                <button type="button" className="auth-modal-btn secondary groups-editor-cancel" onClick={onClose} disabled={pending}>
                  Cancel
                </button>
                <button type="submit" className="auth-modal-btn groups-editor-submit" disabled={pending || !stringOrEmpty(name)}>
                  {pending ? 'Saving...' : (initialGroup?.id ? 'Save group' : 'Create group')}
                </button>
              </div>
            </form>
          </div>
        </div>
      </div>
    </BaseModal>
  );
}

function GroupDeleteModal({ open, onClose, onConfirm, group, pending, error }) {
  if (!open || !group) return null;

  return (
    <BaseModal
      open
      onClose={onClose}
      backdropClassName="device-delete-modal-backdrop"
      dialogClassName="device-delete-modal"
      showClose={false}
      disableBackdropClose={pending}
    >
      <div className="device-delete-modal-body" role="dialog" aria-modal="true">
        <p className="device-delete-eyebrow">Group → Edit</p>
        <h3>Delete {group.name || group.slug || 'this group'}?</h3>
        <p>
          Removing this group only deletes the ERS collection. Devices stay available, but dashboards and automations
          that target this group will need to be updated.
        </p>
        {error ? <div className="device-delete-error">{error}</div> : null}
        <div className="device-delete-actions">
          <button type="button" className="device-delete-cancel" onClick={onClose} disabled={pending}>
            Cancel
          </button>
          <button type="button" className="device-delete-confirm" onClick={onConfirm} disabled={pending}>
            {pending ? 'Deleting...' : 'Delete'}
          </button>
        </div>
      </div>
    </BaseModal>
  );
}

function GroupTile({ group, onOpen, onEdit, onDelete, accessToken, canControl, bootstrapping }) {
  const stats = buildGroupStats(group);
  const members = arrayOrEmpty(group?.devices);
  const {
    sharedControls,
    sharedValues,
    sharedPending,
    sharedError,
    handleSharedValueChange,
    handleSharedCommand,
  } = useGroupSharedControls(members, { accessToken, enabled: canControl });

  const handleClick = useCallback((event) => {
    if (event.defaultPrevented) return;
    if (typeof event.target?.closest === 'function' && event.target.closest('button, a, input, select, textarea, label')) {
      return;
    }
    onOpen(group);
  }, [group, onOpen]);

  const handleKeyDown = useCallback((event) => {
    if (event.defaultPrevented) return;
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onOpen(group);
    }
  }, [group, onOpen]);

  return (
    <GlassCard
      className="group-tile-card device-tile-card device-tile-clickable"
      interactive={false}
      role="button"
      tabIndex={0}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      <div className="device-tile group-tile-surface">
        <div className="device-tile-header group-tile-header">
          <div className="device-title-container">
            <div className="device-title-row group-tile-title-row">
              <span className="device-title-icon group-tile-icon" aria-hidden="true">
                <FontAwesomeIcon icon={faLayerGroup} />
              </span>
              <span className="device-title">{group.name}</span>
              <div className="device-title-actions group-tile-actions">
              <button
                type="button"
                className="device-title-action device-title-edit"
                onClick={() => onEdit(group)}
                aria-label="Edit group"
                title="Edit group"
              >
                <FontAwesomeIcon icon={faPen} />
              </button>
              <button
                type="button"
                className="device-title-action device-title-delete"
                onClick={() => onDelete(group)}
                aria-label="Delete group"
                title="Delete group"
              >
                <FontAwesomeIcon icon={faTrash} />
              </button>
              </div>
            </div>
            <div className="device-meta group-tile-meta">
              <span>ERS group</span>
              <span className="device-meta-dot">•</span>
              <span>Updated {relativeTime(group.updatedAt)}</span>
            </div>
          </div>
        </div>

        <p className="device-description group-tile-description">
          {group.description || 'Shared devices, reusable automation targets, and one control surface for the whole collection.'}
        </p>

        <div className="device-pill-row group-tile-pill-row">
          <GlassPill icon={faUsers} text={`${stats.memberCount} member${stats.memberCount === 1 ? '' : 's'}`} />
          <GlassPill icon={faTag} text={group.slug || 'No slug'} />
          {stats.roomNames.length ? <GlassPill icon={faLayerGroup} text={`${stats.roomNames.length} room${stats.roomNames.length === 1 ? '' : 's'}`} /> : null}
        </div>

        {sharedError ? <div className="groups-shared-controls-error group-tile-controls-feedback">{sharedError}</div> : null}
        {sharedControls.mixedKeys.length ? (
          <div className="groups-shared-controls-note group-tile-controls-feedback">
            {sharedControls.mixedKeys.length} shared control{sharedControls.mixedKeys.length === 1 ? '' : 's'} currently show mixed member state.
          </div>
        ) : null}
        {sharedControls.inputs.length ? (
          <DeviceControlList
            inputs={sharedControls.inputs}
            values={sharedValues}
            pending={sharedPending || bootstrapping}
            onValueChange={handleSharedValueChange}
            onCommand={handleSharedCommand}
            layout="cards"
            collapseAfter={4}
          />
        ) : (
          <div className="groups-shared-controls-empty group-tile-controls-feedback">
            No common writable controls are available across all members yet.
          </div>
        )}
      </div>
    </GlassCard>
  );
}

function GroupDetailView({ group, onBack, onEdit, onDelete, onOpenDevice }) {
  const stats = buildGroupStats(group);
  const members = arrayOrEmpty(group?.devices);
  const { accessToken, user, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  return (
    <div className="device-detail-page groups-detail-page">
      <PageHeader
        title={group.name}
        subtitle={group.description || 'ERS-backed device group for shared controls, dashboards, and automation targets.'}
        showBack
        backText="Back to groups"
        onBack={onBack}
      >
        <div className="devices-header-pills groups-detail-header-pills">
          <GlassPill icon={faUsers} text={`${stats.memberCount} member${stats.memberCount === 1 ? '' : 's'}`} />
          <GlassPill icon={faTag} text={group.slug || 'No slug'} />
          {stats.roomNames.length ? <GlassPill icon={faLayerGroup} text={`${stats.roomNames.length} room${stats.roomNames.length === 1 ? '' : 's'}`} /> : null}
        </div>
      </PageHeader>

      <section className="device-detail-grid groups-detail-grid">
        <div className="device-detail-grid-left">
          <GlassCard className="group-detail-summary-card" interactive={false}>
            <div className="group-detail-summary-head">
              <div className="group-detail-summary-icon" aria-hidden="true">
                <FontAwesomeIcon icon={faLayerGroup} />
              </div>
              <div>
                <h2>{group.name}</h2>
                <p>{group.description || 'No description provided yet.'}</p>
              </div>
            </div>

            <div className="device-pill-row">
              <GlassPill icon={faUsers} text={`${stats.memberCount} member${stats.memberCount === 1 ? '' : 's'}`} />
              <GlassPill icon={faTag} text={group.slug || 'No slug'} />
            </div>

            <div className="group-detail-meta-grid">
              <div className="group-detail-meta-item">
                <span>Rooms</span>
                <strong>{stats.roomNames.length ? stats.roomNames.join(', ') : 'Unassigned'}</strong>
              </div>
              <div className="group-detail-meta-item">
                <span>Last updated</span>
                <strong>{relativeTime(group.updatedAt)}</strong>
              </div>
            </div>

            <div className="group-detail-actions">
              <Button onClick={() => onEdit(group)}>Edit group</Button>
              <Button variant="secondary" onClick={() => onDelete(group)}>Delete group</Button>
            </div>
          </GlassCard>
        </div>

        <div className="device-detail-grid-right groups-detail-sections">
          <GlassCard className="groups-detail-section groups-shared-controls-section" interactive={false}>
            <GroupSharedControlsPanel
              members={members}
              accessToken={accessToken}
              enabled={Boolean(isResidentOrAdmin && accessToken)}
              bootstrapping={bootstrapping}
            />
          </GlassCard>

          <GlassCard className="groups-detail-section" interactive={false}>
            <div className="groups-detail-section-head">
              <div>
                <h3>Members</h3>
                <p>Devices in this group keep their own control surfaces and device detail pages.</p>
              </div>
            </div>

            <div className="groups-detail-members-grid">
              {members.map((device) => (
                <DeviceTile
                  key={`${device.ersId || 'ers'}-${device.id || device.hdpId || device.name}`}
                  device={device}
                  protocolLabel={stringOrEmpty(device?.protocol) || 'Unknown'}
                  onOpen={onOpenDevice}
                />
              ))}
              {members.length === 0 ? (
                <GlassCard className="device-filter-empty-card groups-detail-empty" interactive={false}>
                  <div className="devices-filter-empty-text">This group does not contain any devices yet.</div>
                </GlassCard>
              ) : null}
            </div>
          </GlassCard>
        </div>
      </section>
    </div>
  );
}

export default function Groups() {
  const { user, accessToken, bootstrapping } = useAuth();
  const navigate = useNavigate();
  const { groupId: groupRouteParam } = useParams();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingGroup, setEditingGroup] = useState(null);
  const [editorError, setEditorError] = useState('');
  const [savePending, setSavePending] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [deletePending, setDeletePending] = useState(false);
  const [deleteError, setDeleteError] = useState('');
  const [searchTerm, setSearchTerm] = useState('');

  const { devices: realtimeDevices } = useDeviceHubDevices({
    enabled: Boolean(isResidentOrAdmin),
    metadataMode: 'ws',
    accessToken,
    authReady: Boolean(accessToken) && !bootstrapping,
  });

  const { devices, groups, loading, error, refresh } = useErsInventory({
    enabled: Boolean(isResidentOrAdmin && accessToken),
    accessToken,
    realtimeDevices,
  });

  const ersDevices = useMemo(() => {
    return arrayOrEmpty(devices)
      .filter((device) => stringOrEmpty(device?.ersId || device?.id))
      .sort((left, right) => {
        const leftName = stringOrEmpty(left?.displayName || left?.name);
        const rightName = stringOrEmpty(right?.displayName || right?.name);
        return leftName.localeCompare(rightName);
      });
  }, [devices]);

  const normalizedGroups = useMemo(() => arrayOrEmpty(groups).map(normalizeGroup), [groups]);
  const decodedGroupParam = useMemo(() => {
    if (!groupRouteParam) return '';
    try {
      return decodeURIComponent(groupRouteParam);
    } catch {
      return groupRouteParam;
    }
  }, [groupRouteParam]);

  const selectedGroup = useMemo(() => {
    if (!decodedGroupParam) return null;
    return normalizedGroups.find((group) => group.id === decodedGroupParam || group.slug === decodedGroupParam) || null;
  }, [decodedGroupParam, normalizedGroups]);

  const filteredGroups = useMemo(() => {
    const query = stringOrEmpty(searchTerm).toLowerCase();
    if (!query) return normalizedGroups;
    return normalizedGroups.filter((group) => {
      const memberText = group.devices
        .map((device) => stringOrEmpty(device?.displayName || device?.name || device?.hdpId))
        .join(' ')
        .toLowerCase();
      const haystack = [group.name, group.slug, group.description, memberText]
        .map((value) => stringOrEmpty(value).toLowerCase())
        .join(' ');
      return haystack.includes(query);
    });
  }, [normalizedGroups, searchTerm]);

  const openCreate = useCallback(() => {
    setEditingGroup(null);
    setEditorError('');
    setEditorOpen(true);
  }, []);

  const openEdit = useCallback((group) => {
    setEditingGroup(group);
    setEditorError('');
    setEditorOpen(true);
  }, []);

  const closeEditor = useCallback(() => {
    if (savePending) return;
    setEditorOpen(false);
    setEditingGroup(null);
    setEditorError('');
  }, [savePending]);

  const openDelete = useCallback((group) => {
    setDeleteTarget(group);
    setDeleteError('');
  }, []);

  const closeDelete = useCallback(() => {
    if (deletePending) return;
    setDeleteTarget(null);
    setDeleteError('');
  }, [deletePending]);

  const handleSubmit = useCallback(async ({ id, name, description, deviceIds }) => {
    if (!accessToken) return;
    setSavePending(true);
    setEditorError('');
    try {
      if (id) {
        const patchResult = await patchErsGroup(id, { name, description }, accessToken);
        if (!patchResult.success) throw new Error(patchResult.error || 'Failed to update group');
        const membersResult = await setErsGroupMembers(id, deviceIds, accessToken);
        if (!membersResult.success) throw new Error(membersResult.error || 'Failed to update group members');
      } else {
        const createResult = await createErsGroup({ name, description, device_ids: deviceIds }, accessToken);
        if (!createResult.success) throw new Error(createResult.error || 'Failed to create group');
      }
      closeEditor();
      await refresh();
    } catch (err) {
      setEditorError(err?.message || 'Unable to save group');
    } finally {
      setSavePending(false);
    }
  }, [accessToken, closeEditor, refresh]);

  const confirmDelete = useCallback(async () => {
    if (!accessToken || !deleteTarget?.id) return;
    setDeletePending(true);
    setDeleteError('');
    try {
      const result = await deleteErsGroup(deleteTarget.id, accessToken);
      if (!result.success) throw new Error(result.error || 'Failed to delete group');
      const deletedId = deleteTarget.id;
      const deletedSlug = deleteTarget.slug;
      closeDelete();
      if (decodedGroupParam && (decodedGroupParam === deletedId || decodedGroupParam === deletedSlug)) {
        navigate('/groups');
      }
      await refresh();
    } catch (err) {
      setDeleteError(err?.message || 'Unable to delete group');
    } finally {
      setDeletePending(false);
    }
  }, [accessToken, closeDelete, decodedGroupParam, deleteTarget, navigate, refresh]);

  const openGroupDetail = useCallback((group) => {
    const id = group?.slug || group?.id;
    if (!id) return;
    navigate(`/groups/${encodeURIComponent(id)}`);
  }, [navigate]);

  const openDeviceDetail = useCallback((device) => {
    const id = device?.hdpId || device?.id;
    if (!id) return;
    navigate(`/devices/${encodeURIComponent(id)}`);
  }, [navigate]);

  if (!isResidentOrAdmin) {
    return <UnauthorizedView title="Groups" message="Resident or admin access is required to manage device groups." />;
  }

  if (loading && normalizedGroups.length === 0) {
    return <LoadingView title="Loading groups" message="Fetching ERS groups and members..." />;
  }

  if (decodedGroupParam) {
    if (!selectedGroup && loading) {
      return <LoadingView title="Loading group" message="Fetching group details..." />;
    }
    if (!selectedGroup) {
      return (
        <div className="device-detail-page groups-detail-page">
          <PageHeader title="Group not found" subtitle="The requested group could not be found." showBack backText="Back to groups" onBack={() => navigate('/groups')} />
          <GlassCard className="devices-empty-card" interactive={false}>
            <div className="device-detail-missing-text">This group may have been deleted or renamed.</div>
          </GlassCard>
        </div>
      );
    }
    return (
      <>
        {(error || deleteError) ? (
          <div className="device-detail-page groups-detail-page groups-detail-alert-wrap">
            <GlassCard className="devices-empty-card groups-status-card groups-status-card-error" interactive={false}>
              <div className="groups-status-copy">{deleteError || error}</div>
            </GlassCard>
          </div>
        ) : null}
        <GroupDetailView
          group={selectedGroup}
          onBack={() => navigate('/groups')}
          onEdit={openEdit}
          onDelete={openDelete}
          onOpenDevice={openDeviceDetail}
        />
        <GroupEditorModal
          open={editorOpen}
          onClose={closeEditor}
          onSubmit={handleSubmit}
          devices={ersDevices}
          initialGroup={editingGroup}
          pending={savePending}
          error={editorError}
        />
        <GroupDeleteModal
          open={Boolean(deleteTarget)}
          onClose={closeDelete}
          onConfirm={confirmDelete}
          group={deleteTarget}
          pending={deletePending}
          error={deleteError}
        />
      </>
    );
  }

  return (
    <div className="devices-page groups-page">
      <PageHeader
        title="Groups"
        subtitle="ERS-backed device collections for shared controls, automation targets, and dashboard widgets."
      />

      {(error || deleteError) ? (
        <GlassCard className="devices-empty-card groups-status-card groups-status-card-error" interactive={false}>
          <div className="groups-status-copy">{deleteError || error}</div>
        </GlassCard>
      ) : null}

      <div className="devices-toolbar groups-toolbar">
        <SearchBar
          value={searchTerm}
          onChange={setSearchTerm}
          onClear={() => setSearchTerm('')}
          placeholder="Search by group, slug, or member..."
          ariaLabel="Search groups"
          className="devices-search"
        />
        <div className="devices-toolbar-meta">{filteredGroups.length} visible</div>
      </div>

      <section className="devices-grid groups-grid">
        {filteredGroups.map((group) => (
          <GroupTile
            key={group.id}
            group={group}
            onOpen={openGroupDetail}
            onEdit={openEdit}
            onDelete={openDelete}
            accessToken={accessToken}
            canControl={Boolean(isResidentOrAdmin && accessToken)}
            bootstrapping={bootstrapping}
          />
        ))}

        <GlassCard className="device-add-card" interactive={false}>
          <button type="button" className="device-add-card-btn" onClick={openCreate}>
            <span className="device-add-card-icon">
              <FontAwesomeIcon icon={faPlus} />
            </span>
            <span className="device-add-card-title">Create group</span>
            <span className="device-add-card-subtitle">Combine devices with shared controls into one reusable ERS group.</span>
            <span className="device-add-card-cta">Open form</span>
          </button>
        </GlassCard>

        {!loading && filteredGroups.length === 0 && normalizedGroups.length > 0 ? (
          <GlassCard className="device-filter-empty-card groups-filter-empty-card" interactive={false}>
            <div className="devices-filter-empty-text">No groups match the current search.</div>
          </GlassCard>
        ) : null}
      </section>

      <button type="button" className="devices-fab" onClick={openCreate} title="Create group" aria-label="Create group">
        <span className="devices-fab-icon">
          <FontAwesomeIcon icon={faPlus} />
        </span>
        <span className="devices-fab-label">Create group</span>
      </button>

      <GroupEditorModal
        open={editorOpen}
        onClose={closeEditor}
        onSubmit={handleSubmit}
        devices={ersDevices}
        initialGroup={editingGroup}
        pending={savePending}
        error={editorError}
      />

      <GroupDeleteModal
        open={Boolean(deleteTarget)}
        onClose={closeDelete}
        onConfirm={confirmDelete}
        group={deleteTarget}
        pending={deletePending}
        error={deleteError}
      />
    </div>
  );
}
