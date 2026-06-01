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
import { toControlBoolean } from '../../../common/DeviceControlRenderer/deviceControlUtils';
import {
  applyPendingStateToDevice,
  clearPendingTimeout,
  createPendingCommand,
  shouldClearPendingFromDevice,
} from '../../../Devices/commandPending';
import { DEVICE_ICON_MAP } from '../../../Devices/deviceIconChoices';
import { buildPayloadForInput, canToggleDevice, findToggleInput } from '../../../../utils/groupControls';
import { resolveQuickControlDevices } from '../../../../utils/quickControls';
import { resolveCommandDeviceId } from '../../../../utils/deviceIdentity';
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

function getDeviceCommandId(device) {
  return resolveCommandDeviceId(device);
}

function getDeviceDisplayName(device) {
  return device?.displayName || device?.name || device?.hdpId || device?.ersId || device?.id || 'Device';
}

function buildTogglePayload(device, value) {
  const input = findToggleInput(device);
  if (!input) return null;
  return buildPayloadForInput(input, toControlBoolean(value));
}

export default function MultiDeviceWidget({
  settings = {},
  editMode,
  onSettings,
  onRemove,
}) {
  const { accessToken, user, bootstrapping } = useAuth();
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

  const { devices: realtimeDevices, loading: hdpLoading, connectionInfo } = useDeviceHubDevices({
    enabled: Boolean(isResidentOrAdmin),
    metadataMode: 'ws',
    accessToken,
    authReady: Boolean(accessToken) && !bootstrapping,
  });

  const { devices: ersDevices, groups: ersGroups, loading: ersLoading } = useErsInventory({
    enabled: Boolean(isResidentOrAdmin && accessToken),
    accessToken,
    realtimeDevices,
  });

  const loading = hdpLoading || ersLoading;
  const selectedIds = Array.isArray(settings.device_ids) ? settings.device_ids : [];
  const selectedGroupIds = Array.isArray(settings.group_ids) ? settings.group_ids : [];

  const devices = useMemo(() => {
    return resolveQuickControlDevices({
      selectedIds,
      selectedGroupIds,
      ersDevices,
      ersGroups,
      realtimeDevices,
    });
  }, [selectedGroupIds, selectedIds, ersDevices, ersGroups, realtimeDevices]);

  useEffect(() => {
    if (!devices.length) return;
    setPendingMap((prev) => {
      let changed = false;
      const next = { ...prev };
      devices.forEach((device) => {
        const commandId = getDeviceCommandId(device);
        if (!commandId) return;
        const pending = next[commandId];
        if (!pending) return;
        if (!shouldClearPendingFromDevice(pending, device)) return;
        clearPendingTimeout(pending);
        delete next[commandId];
        changed = true;
      });
      return changed ? next : prev;
    });
  }, [devices]);

  const commandsReady = Boolean(connectionInfo?.commandsReady);
  const commandLockReason = connectionInfo?.commandLockReason || 'Preparing live controls…';

  const handleToggle = useCallback((device) => {
    const deviceId = getDeviceCommandId(device);
    if (!deviceId) return;
    if (!canToggleDevice(device)) return;
    if (!commandsReady) {
      setCommandError(commandLockReason);
      return;
    }
    if (!accessToken) {
      setCommandError('Authentication required');
      return;
    }

    const nextValue = !resolveToggleState(device);
    const payload = buildTogglePayload(device, nextValue);
    if (!payload) return;

    setCommandError('');
    const { corr, enrichedPayload, pending } = createPendingCommand(device, payload, {
      onTimeout: ({ corr: expiredCorr }) => {
        setCommandError('Device did not confirm the command in time');
        setPendingMap((prev) => {
          const next = { ...prev };
          const current = next[deviceId];
          if (!current || current.corr !== expiredCorr) return prev;
          clearPendingTimeout(current);
          delete next[deviceId];
          return next;
        });
      },
    });

    setPendingMap((prev) => ({
      ...prev,
      [deviceId]: pending,
    }));

    sendDeviceCommand(deviceId, enrichedPayload, accessToken)
      .then((res) => {
        if (!res.success) throw new Error(res.error || 'Unable to send command');
      })
      .catch((err) => {
        setCommandError(err?.message || 'Unable to send device command');
        setPendingMap((prev) => {
          const next = { ...prev };
          const current = next[deviceId];
          if (!current || current.corr !== corr) return prev;
          clearPendingTimeout(current);
          delete next[deviceId];
          return next;
        });
      })
      .finally(() => {});
  }, [accessToken, commandLockReason, commandsReady]);

  if (!selectedIds.length && !selectedGroupIds.length) {
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
        {!commandError && !commandsReady ? <div className="multi-device-widget__error">{commandLockReason}</div> : null}
        <div className="multi-device-widget__grid">
          {devices.map((device) => {
            const commandId = getDeviceCommandId(device);
            const pending = Boolean(commandId && pendingMap[commandId]);
            const displayDevice = applyPendingStateToDevice(device, commandId ? pendingMap[commandId] : null);
            const isOn = resolveToggleState(displayDevice);
            const capabilities = collectCapabilities(displayDevice);
            const icon = resolveDeviceIcon(displayDevice, capabilities);
            const canToggle = canToggleDevice(device);

            if (!canToggle) return null;

            return (
              <button
                key={device.id || device.hdpId || device.ersId}
                type="button"
                className={`multi-device-widget__tile${isOn ? ' on' : ''}`}
                onClick={() => handleToggle(device)}
                disabled={pending || !canToggle || !commandsReady}
              >
                <div className="multi-device-widget__tile-header">
                  <div className="multi-device-widget__tile-icon">
                    <FontAwesomeIcon icon={icon} />
                  </div>
                  <div className="multi-device-widget__tile-info">
                    <div className="multi-device-widget__tile-name">
                      {getDeviceDisplayName(displayDevice)}
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
