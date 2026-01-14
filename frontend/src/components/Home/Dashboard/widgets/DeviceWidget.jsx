/**
 * DeviceWidget - Dashboard widget for device control and display.
 * Uses shared DeviceControlRenderer and DeviceMetricsRenderer components.
 * Does NOT import DeviceTile - designed specifically for dashboard use.
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBatteryThreeQuarters,
  faBolt,
  faDoorOpen,
  faDroplet,
  faLightbulb,
  faMicrochip,
  faPlug,
  faQuestionCircle,
  faThermometerHalf,
} from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../../context/AuthContext';
import useDeviceHubDevices from '../../../../hooks/useDeviceHubDevices';
import useErsInventory from '../../../../hooks/useErsInventory';
import { sendDeviceCommand } from '../../../../services/deviceHubService';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';
import GlassSwitch from '../../../common/GlassSwitch/GlassSwitch';
import DeviceControlList from '../../../common/DeviceControlRenderer/DeviceControlRenderer';
import DeviceMetricsRenderer from '../../../common/DeviceMetricsRenderer/DeviceMetricsRenderer';
import { sanitizeInputKey, toControlBoolean } from '../../../common/DeviceControlRenderer/deviceControlUtils';
import { normalizeColorHex } from '../../../../utils/colorHex';
import './DeviceWidget.css';

// ────────────────────────────────────────────────────────────────────
// Helper utilities
// ────────────────────────────────────────────────────────────────────

function getCapabilityKeyParts(cap) {
  if (!cap || typeof cap !== 'object') return [];
  return [cap.id, cap.property, cap.name]
    .filter(part => part !== undefined && part !== null)
    .map(part => part.toString().toLowerCase());
}

function resolveDeviceIcon(device, capabilities = []) {
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

function extractStateColorHex(raw) {
  const normalized = normalizeColorHex(raw, '');
  return normalized || null;
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

function formatDisplayLabel(value) {
  if (!value) return '';
  return value
    .toString()
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, ch => ch.toUpperCase());
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

function normalizeToggleProperty(property) {
  const key = (property ?? '').toString().trim();
  const lower = key.toLowerCase();
  if (lower === 'state' || lower === 'power' || lower === 'on') return 'on';
  return key;
}

function buildPayloadForInput(input, value) {
  const key = sanitizeInputKey(input);
  if (!key) {
    return null;
  }
  const stateKeyRaw = (input.property || key || '').toString().trim();
  if (!stateKeyRaw) return null;

  const state = {};
  const propertyLower = stateKeyRaw.toLowerCase();

  switch (input.type) {
  case 'toggle':
    state[normalizeToggleProperty(stateKeyRaw)] = toControlBoolean(value);
    break;
  case 'slider':
  case 'number': {
    const numeric = Number(value);
    if (Number.isNaN(numeric)) return null;
    state[stateKeyRaw] = numeric;
    if (propertyLower.startsWith('brightness')) {
      state.on = numeric > 0;
    }
    break;
  }
  case 'color': {
    state[stateKeyRaw] = value;
    state.on = true;
    break;
  }
  case 'select':
    state[stateKeyRaw] = value;
    if (propertyLower.includes('effect')) {
      state.on = true;
    }
    break;
  default:
    state[stateKeyRaw] = value;
  }

  return { state };
}

// ────────────────────────────────────────────────────────────────────
// DeviceWidget Component
// ────────────────────────────────────────────────────────────────────

export default function DeviceWidget({
  settings = {},
  editMode,
  onSettings,
  onRemove,
}) {
  const navigate = useNavigate();
  const { accessToken, user } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const [commandError, setCommandError] = useState('');
  const [pendingCommands, setPendingCommands] = useState({});
  const pendingCommandsRef = useRef({});

  useEffect(() => {
    pendingCommandsRef.current = pendingCommands;
  }, [pendingCommands]);

  useEffect(() => {
    return () => {
      Object.values(pendingCommandsRef.current || {}).forEach((entry) => {
        if (entry?.timeoutId) {
          clearTimeout(entry.timeoutId);
        }
      });
    };
  }, []);

  const { devices: realtimeDevices, loading: hdpLoading } = useDeviceHubDevices({
    enabled: Boolean(isResidentOrAdmin),
    metadataMode: 'ws',
  });

  const { devices: ersDevices, loading: ersLoading } = useErsInventory({
    enabled: Boolean(isResidentOrAdmin && accessToken),
    accessToken,
    realtimeDevices,
  });

  const deviceId = settings.device_id || settings.ers_device_id || settings.hdp_device_id || '';
  const selectedControls = Array.isArray(settings.controls) ? settings.controls : null;
  const selectedFields = Array.isArray(settings.fields) ? settings.fields : null;
  const fieldsLayout = settings.fields_layout || 'cards';

  const device = useMemo(() => {
    if (!deviceId) return null;

    const ersDevice = (ersDevices || []).find((d) =>
      d.ersId === deviceId || d.id === deviceId || d.hdpId === deviceId
    );
    if (ersDevice) return ersDevice;

    return (realtimeDevices || []).find((d) =>
      d.id === deviceId || d.hdpId === deviceId
    );
  }, [deviceId, ersDevices, realtimeDevices]);

  const loading = hdpLoading || ersLoading;

  const capabilities = useMemo(() => collectCapabilities(device), [device]);
  const capabilityLookup = useMemo(() => buildCapabilityLookup(capabilities), [capabilities]);
  const deviceIcon = useMemo(() => resolveDeviceIcon(device, capabilities), [device, capabilities]);

  // Build normalized inputs (controls)
  const normalizedInputs = useMemo(() => {
    if (!device) return [];
    
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
      normalized.push(fallback);
    });
    
    return normalized;
  }, [device, capabilityLookup, capabilities]);

  // Filter inputs based on selected controls
  const filteredInputs = useMemo(() => {
    if (!selectedControls) return normalizedInputs;
    const selectedSet = new Set(selectedControls.map(k => String(k).toLowerCase()));
    return normalizedInputs.filter(input => {
      const key = sanitizeInputKey(input).toLowerCase();
      return key ? selectedSet.has(key) : false;
    });
  }, [normalizedInputs, selectedControls]);

  // Find primary toggle for header switch
  const primaryToggleInput = useMemo(() => {
    return filteredInputs.find(input => {
      if (input.type !== 'toggle') return false;
      const key = sanitizeInputKey(input).toLowerCase();
      return key === 'state' || key === 'on' || key === 'power';
    }) || null;
  }, [filteredInputs]);

  const primaryToggleKey = primaryToggleInput ? sanitizeInputKey(primaryToggleInput) : null;

  // Interactive inputs (excluding primary toggle since it's in header)
  const interactiveInputs = useMemo(() => {
    if (!primaryToggleInput) return filteredInputs;
    return filteredInputs.filter(input => input !== primaryToggleInput);
  }, [filteredInputs, primaryToggleInput]);

  const [controlValues, setControlValues] = useState(() => buildInitialControlValues(device, normalizedInputs));

  // Update control values when device state changes
  useEffect(() => {
    if (!device) return;
    const isPending = Boolean(device?.id && pendingCommands[device.id]);
    if (isPending) return; // Don't update while command pending

    setControlValues((prev) => {
      const baseline = buildInitialControlValues(device, normalizedInputs);
      if (!prev || typeof prev !== 'object') {
        return baseline;
      }

      // If the device state doesn't provide a color value, keep previous to avoid
      // the UI snapping back to white.
      const merged = { ...baseline };
      normalizedInputs.forEach((input) => {
        if (input.type !== 'color') return;
        const key = sanitizeInputKey(input);
        if (!key) return;
        const stateRaw = getStateValueForInput(device, input);
        const stateHex = extractStateColorHex(stateRaw);
        if (!stateHex && prev[key]) {
          merged[key] = prev[key];
        }
      });
      return merged;
    });
  }, [device, normalizedInputs, pendingCommands]);

  const toggleValue = primaryToggleKey ? toControlBoolean(controlValues[primaryToggleKey]) : null;
  const isPending = Boolean(device?.id && pendingCommands[device.id]);

  // Clear pending when we observe a matching command_result and state has advanced.
  useEffect(() => {
    if (!device?.id) return;
    const pending = pendingCommands[device.id];
    if (!pending) return;
    const result = device.lastCommandResult;
    if (!result) return;

    const resultCorr = result.corr || result.correlation_id || result.correlationId;
    if (!pending.corr || !resultCorr || pending.corr !== resultCorr) return;

    const stateTs = device.stateUpdatedAt instanceof Date
      ? device.stateUpdatedAt.getTime()
      : (device.stateUpdatedAt || 0);
    const baselineTs = pending.stateVersion || 0;
    const resultTs = Number(result.ts || 0);
    const hasStateAdvanced = stateTs && stateTs > baselineTs;
    const stateCoversResult = stateTs && resultTs && stateTs >= resultTs;

    const shouldClear = !result.success || hasStateAdvanced || stateCoversResult;
    if (!shouldClear) return;

    setPendingCommands((prev) => {
      const next = { ...prev };
      const current = next[device.id];
      if (!current) return prev;
      if (current.timeoutId) {
        clearTimeout(current.timeoutId);
      }
      delete next[device.id];
      return next;
    });
  }, [device, device?.id, device?.lastCommandResult, device?.stateUpdatedAt, pendingCommands]);

  const handleCommand = useCallback((dev, payload) => {
    if (!dev?.id) return Promise.resolve();
    if (!accessToken) {
      setCommandError('Authentication required');
      return Promise.reject(new Error('Authentication required'));
    }
    setCommandError('');

    const stateVersionAtSend = dev?.stateUpdatedAt instanceof Date
      ? dev.stateUpdatedAt.getTime()
      : (dev?.stateUpdatedAt || 0);
    const startedAt = Date.now();

    const corr = (payload && payload.correlation_id)
      || (typeof crypto !== 'undefined' && crypto.randomUUID
        ? crypto.randomUUID()
        : `${Date.now()}-${Math.random().toString(16).slice(2)}`);
    const enrichedPayload = payload && typeof payload === 'object'
      ? { ...payload, correlation_id: corr }
      : { correlation_id: corr };

    setPendingCommands((prev) => {
      const next = { ...prev };
      const existing = next[dev.id];
      if (existing?.timeoutId) {
        clearTimeout(existing.timeoutId);
      }
      next[dev.id] = { corr, startedAt, stateVersion: stateVersionAtSend, timeoutId: null };
      return next;
    });

    return sendDeviceCommand(dev.id, enrichedPayload, accessToken)
      .then((res) => {
        if (!res.success) {
          const message = res.error || 'Unable to send command';
          throw new Error(message);
        }
        return res.data;
      })
      .catch((err) => {
        const message = err?.message || 'Unable to send device command';
        setCommandError(message);
        throw err;
      })
      .finally(() => {
        // Leave pending entry until cleared by command_result or timeout fallback.
        setPendingCommands((prev) => {
          const next = { ...prev };
          const current = next[dev.id];
          if (!current) return prev;
          if (current.timeoutId) {
            clearTimeout(current.timeoutId);
          }
          next[dev.id] = {
            ...current,
            timeoutId: setTimeout(() => {
              setPendingCommands((latePrev) => {
                const clone = { ...latePrev };
                delete clone[dev.id];
                return clone;
              });
            }, 5000),
          };
          return next;
        });
      });
  }, [accessToken]);

  const handleInputCommand = useCallback((input, value) => {
    if (!device) return;
    const payload = buildPayloadForInput(input, value);
    if (!payload) return;
    handleCommand(device, payload);
  }, [device, handleCommand]);

  const handleValueChange = useCallback((key, value) => {
    setControlValues(prev => ({ ...prev, [key]: value }));
  }, []);

  const handlePrimaryToggle = useCallback((next) => {
    if (!primaryToggleInput || !device) return;
    const value = toControlBoolean(next);
    setControlValues(prev => ({ ...prev, [primaryToggleKey]: value }));
    handleInputCommand(primaryToggleInput, value);
  }, [primaryToggleInput, primaryToggleKey, device, handleInputCommand]);

  const handleOpenDevice = useCallback(() => {
    if (editMode || !device) return;
    const id = deviceId || device.hdpId || device.id || device.ersId;
    if (!id) return;
    navigate(`/devices/${encodeURIComponent(id)}`);
  }, [device, deviceId, editMode, navigate]);

  // Note: We intentionally do not auto-resize DeviceWidget height.
  // The widget already contains its content via internal scrolling, and
  // auto-height can cause feedback loops where the widget grows over time.

  if (!deviceId) {
    return (
      <WidgetShell
        title={settings.title || 'Device'}
        subtitle="No device selected"
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
        className="device-widget device-widget--empty"
      >
        <div className="device-widget__empty">
          <FontAwesomeIcon icon={faQuestionCircle} className="device-widget__empty-icon" />
          <span>Configure this widget to select a device</span>
        </div>
      </WidgetShell>
    );
  }

  if (!loading && !device) {
    return (
      <WidgetShell
        title={settings.title || 'Device'}
        subtitle="Device not found"
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
        className="device-widget device-widget--error"
        error="The selected device could not be found"
      />
    );
  }

  return (
    <WidgetShell
      className="device-widget"
      loading={loading}
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      interactive={!editMode}
      onClick={handleOpenDevice}
    >
        <div className="device-widget__content">
        {commandError && <div className="device-widget__command-error">{commandError}</div>}
        
        {/* Header - Prominent icon and name */}
        <div className="device-widget__header">
          <div className="device-widget__title-group">
            <FontAwesomeIcon icon={deviceIcon} className="device-widget__icon" />
            <div className="device-widget__title-info">
              <span className="device-widget__name">{device?.displayName || device?.name || 'Device'}</span>
              <span className={`device-widget__status ${device?.online ? '' : 'offline'}`}>
                {device?.online ? 'Online' : 'Offline'}
              </span>
            </div>
          </div>
          {primaryToggleInput && (
            <div
              className="device-widget__toggle"
              onClick={(e) => e.stopPropagation()}
              onMouseDown={(e) => e.stopPropagation()}
              onPointerDown={(e) => e.stopPropagation()}
              onTouchStart={(e) => e.stopPropagation()}
              onKeyDown={(e) => e.stopPropagation()}
            >
              <GlassSwitch
                checked={Boolean(toggleValue)}
                disabled={isPending}
                onChange={handlePrimaryToggle}
              />
            </div>
          )}
        </div>

        {/* Controls and Metrics - Centered in remaining space */}
        <div className="device-widget__controls-area">
          {/* Controls */}
          {interactiveInputs.length > 0 && (
            <DeviceControlList
              inputs={interactiveInputs}
              values={controlValues}
              pending={isPending}
              onValueChange={handleValueChange}
              onCommand={handleInputCommand}
              layout={fieldsLayout}
              collapseAfter={4}
            />
          )}

          {/* Metrics/State fields */}
          {selectedFields && selectedFields.length > 0 && (
            <DeviceMetricsRenderer
              device={device}
              selectedFields={selectedFields}
              layout={fieldsLayout}
              collapseAfter={5}
              showBinaryPills={true}
            />
          )}
        </div>
      </div>
    </WidgetShell>
  );
}

DeviceWidget.defaultHeight = 4;
