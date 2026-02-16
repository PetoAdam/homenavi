import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBolt,
  faBatteryThreeQuarters,
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
import { sanitizeInputKey, toControlBoolean } from '../../../common/DeviceControlRenderer/deviceControlUtils';
import { DEVICE_ICON_MAP } from '../../../Devices/deviceIconChoices';
import './MultiDeviceWidget.css';

const FALLBACK_ICON = faMicrochip;

function getCapabilityKeyParts(cap) {
  if (!cap || typeof cap !== 'object') return [];
  return [cap.id, cap.property, cap.name]
    .filter((part) => part !== undefined && part !== null)
    .map((part) => part.toString().toLowerCase());
}

function collectCapabilities(device) {
  const lists = [
    Array.isArray(device?.capabilities) ? device.capabilities : [],
    Array.isArray(device?.state?.capabilities) ? device.state.capabilities : [],
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

function resolveDeviceIcon(device, capabilities = []) {
  const manualKey = typeof device?.icon === 'string' ? device.icon.toLowerCase() : '';
  if (manualKey && manualKey !== 'auto' && DEVICE_ICON_MAP[manualKey]) {
    return DEVICE_ICON_MAP[manualKey];
  }
  const keywords = [device?.type, device?.description, device?.model, device?.displayName, device?.manufacturer]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  const hasCapability = (key) => capabilities.some((cap) => {
    const parts = getCapabilityKeyParts(cap);
    return parts.includes(key);
  });
  if (hasCapability('contact') || keywords.includes('door')) return faDoorOpen;
  if (hasCapability('brightness') || hasCapability('color') || keywords.includes('light') || keywords.includes('lamp')) {
    return faLightbulb;
  }
  if (hasCapability('power') || keywords.includes('plug') || keywords.includes('socket') || keywords.includes('outlet')) {
    return faPlug;
  }
  if (hasCapability('temperature') || keywords.includes('thermo') || keywords.includes('heating')) {
    return faThermometerHalf;
  }
  if (hasCapability('humidity')) return faDroplet;
  if (hasCapability('battery')) return faBatteryThreeQuarters;
  if (hasCapability('voltage') || hasCapability('power')) return faBolt;
  return FALLBACK_ICON;
}

function resolveToggleState(device) {
  const state = device?.state || {};
  const candidates = [state.on, state.state, state.power, device?.toggleState];
  for (const candidate of candidates) {
    if (candidate !== undefined) return toControlBoolean(candidate);
  }
  return false;
}

function normalizeToggleProperty(property) {
  const key = (property ?? '').toString().trim();
  const lower = key.toLowerCase();
  if (lower === 'state' || lower === 'power' || lower === 'on') return 'on';
  return key;
}

function getDeviceCommandId(device) {
  const hdpIds = Array.isArray(device?.hdpIds) ? device.hdpIds : [];
  return device?.id || device?.hdpId || hdpIds[0] || device?.device_id || device?.externalId || '';
}

function getDeviceDisplayName(device) {
  return device?.displayName || device?.name || device?.hdpId || device?.ersId || device?.id || 'Device';
}

function getToggleInput(device) {
  const inputs = Array.isArray(device?.inputs) ? device.inputs : [];
  return inputs.find((input) => {
    const type = (input?.type || input?.kind || '').toString().toLowerCase();
    const valueType = (input?.value_type || input?.valueType || '').toString().toLowerCase();
    return type === 'toggle' || type === 'binary' || valueType === 'boolean';
  }) || null;
}

function isToggleWritable(input) {
  if (!input || typeof input !== 'object') return true;
  const access = input.access || input.metadata?.access || {};
  if (access.readOnly === true) return false;
  const negativeFlags = [access.write, access.set, access.command, access.toggle].filter((flag) => flag === false);
  if (negativeFlags.length > 0) return false;
  return true;
}

function getToggleProperty(device) {
  const input = getToggleInput(device);
  const inputKey = sanitizeInputKey(input);
  if (inputKey) return inputKey;
  const state = device?.state || {};
  if ('on' in state) return 'on';
  if ('state' in state) return 'state';
  if ('power' in state) return 'power';
  return '';
}

function canToggleDevice(device) {
  if (!device) return false;
  const input = getToggleInput(device);
  if (input && !isToggleWritable(input)) return false;
  return Boolean(getToggleProperty(device));
}

function buildTogglePayload(device, value) {
  const property = getToggleProperty(device);
  if (!property) return null;
  return { state: { [normalizeToggleProperty(property)]: toControlBoolean(value) } };
}

export default function MultiDeviceWidget({
  settings = {},
  editMode,
  onSettings,
  onRemove,
}) {
  const { accessToken, user } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const [commandError, setCommandError] = useState('');
  const [pendingMap, setPendingMap] = useState({});
  const pendingRef = useRef({});

  useEffect(() => {
    pendingRef.current = pendingMap;
  }, [pendingMap]);

  useEffect(() => () => {
    Object.values(pendingRef.current || {}).forEach((entry) => {
      if (entry?.timeoutId) clearTimeout(entry.timeoutId);
    });
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

  const loading = hdpLoading || ersLoading;
  const selectedIds = Array.isArray(settings.device_ids) ? settings.device_ids : [];

  const devices = useMemo(() => {
    if (!selectedIds.length) return [];
    const ersMap = new Map();
    (Array.isArray(ersDevices) ? ersDevices : []).forEach((device) => {
      const key = device?.id || device?.ersId || device?.hdpId;
      if (key) ersMap.set(key, device);
    });
    const rtMap = new Map();
    (Array.isArray(realtimeDevices) ? realtimeDevices : []).forEach((device) => {
      const key = device?.id || device?.hdpId;
      if (key) rtMap.set(key, device);
    });

    return selectedIds
      .map((id) => {
        const ers = ersMap.get(id) || null;
        const ersHdpId = ers?.hdpId || (Array.isArray(ers?.hdpIds) ? ers.hdpIds[0] : null);
        const rt = rtMap.get(id) || (ersHdpId ? rtMap.get(ersHdpId) : null) || null;
        if (!ers && !rt) return null;
        if (!ers) return rt;
        if (!rt) return ers;
        return {
          ...rt,
          ...ers,
          state: rt.state || ers.state,
          inputs: rt.inputs || ers.inputs,
          capabilities: rt.capabilities || ers.capabilities,
        };
      })
      .filter((device) => {
        if (!device) return false;
        const commandId = getDeviceCommandId(device);
        if (!commandId) return false;
        return canToggleDevice(device);
      });
  }, [selectedIds, ersDevices, realtimeDevices]);

  const handleToggle = useCallback((device) => {
    const deviceId = getDeviceCommandId(device);
    if (!deviceId) return;
    if (!canToggleDevice(device)) return;
    if (!accessToken) {
      setCommandError('Authentication required');
      return;
    }

    const nextValue = !resolveToggleState(device);
    const payload = buildTogglePayload(device, nextValue);
    if (!payload) return;

    setCommandError('');
    const corr = (typeof crypto !== 'undefined' && crypto.randomUUID)
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const enrichedPayload = { ...payload, correlation_id: corr };

    setPendingMap((prev) => ({
      ...prev,
      [deviceId]: { corr, timeoutId: null },
    }));

    sendDeviceCommand(deviceId, enrichedPayload, accessToken)
      .then((res) => {
        if (!res.success) throw new Error(res.error || 'Unable to send command');
      })
      .catch((err) => {
        setCommandError(err?.message || 'Unable to send device command');
      })
      .finally(() => {
        setPendingMap((prev) => {
          const next = { ...prev };
          const current = next[deviceId];
          if (!current) return prev;
          if (current.timeoutId) clearTimeout(current.timeoutId);
          next[deviceId] = {
            ...current,
            timeoutId: setTimeout(() => {
              setPendingMap((latePrev) => {
                const clone = { ...latePrev };
                delete clone[deviceId];
                return clone;
              });
            }, 4000),
          };
          return next;
        });
      });
  }, [accessToken]);

  if (!selectedIds.length) {
    return (
      <WidgetShell
        title={settings.title || 'Quick Controls'}
        subtitle="No devices selected"
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
        className="multi-device-widget multi-device-widget--empty"
      >
        <div className="multi-device-widget__empty">
          <FontAwesomeIcon icon={faQuestionCircle} className="multi-device-widget__empty-icon" />
          <span>Configure this widget to select devices</span>
        </div>
      </WidgetShell>
    );
  }

  if (!loading && devices.length === 0) {
    return (
      <WidgetShell
        title={settings.title || 'Quick Controls'}
        subtitle="No toggle devices"
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
        className="multi-device-widget multi-device-widget--empty"
      >
        <div className="multi-device-widget__empty">
          <FontAwesomeIcon icon={faQuestionCircle} className="multi-device-widget__empty-icon" />
          <span>Select devices with on/off controls</span>
        </div>
      </WidgetShell>
    );
  }

  return (
    <WidgetShell
      title={settings.title || 'Quick Controls'}
      subtitle={settings.subtitle}
      icon={faBolt}
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      loading={loading}
      className="multi-device-widget"
    >
      <div className="multi-device-widget__content">
        {commandError && <div className="multi-device-widget__error">{commandError}</div>}
        <div className="multi-device-widget__grid">
          {devices.map((device) => {
            const isOn = resolveToggleState(device);
            const capabilities = collectCapabilities(device);
            const icon = resolveDeviceIcon(device, capabilities);
            const commandId = getDeviceCommandId(device);
            const pending = Boolean(commandId && pendingMap[commandId]);
            const canToggle = canToggleDevice(device);

            if (!canToggle) return null;

            return (
              <button
                key={device.id || device.hdpId || device.ersId}
                type="button"
                className={`multi-device-widget__tile${isOn ? ' on' : ''}`}
                onClick={() => handleToggle(device)}
                disabled={pending || !canToggle}
              >
                <div className="multi-device-widget__tile-header">
                  <div className="multi-device-widget__tile-icon">
                    <FontAwesomeIcon icon={icon} />
                  </div>
                  <div className="multi-device-widget__tile-info">
                    <div className="multi-device-widget__tile-name">
                      {getDeviceDisplayName(device)}
                    </div>
                    <div className="multi-device-widget__tile-status">
                      {isOn ? 'On' : 'Off'}
                    </div>
                  </div>
                </div>
              </button>
            );
          })}
        </div>
      </div>
    </WidgetShell>
  );
}

MultiDeviceWidget.defaultHeight = 4;
