import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faBatteryThreeQuarters,
  faBolt,
  faDoorOpen,
  faDroplet,
  faLayerGroup,
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
import {
  applyPendingStateToDevice,
  clearPendingTimeout,
  createPendingCommand,
  shouldClearPendingFromDevice,
} from '../../../Devices/commandPending';
import { DEVICE_ICON_MAP } from '../../../Devices/deviceIconChoices';
import { buildPayloadForInput, canToggleDevice, findToggleInput } from '../../../../utils/groupControls';
import { resolveQuickControlItems } from '../../../../utils/quickControls';
import { resolveCommandDeviceId } from '../../../../utils/deviceIdentity';
import './MultiDeviceWidget.css';

const FALLBACK_ICON = faMicrochip;
const GROUP_SHARED_MIXED_GRACE_MS = 2800;

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
  const hasCapability = (key) => capabilities.some((cap) => getCapabilityKeyParts(cap).includes(key));
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

export default function MultiDeviceWidget({ settings = {}, editMode, onSettings, onRemove }) {
  const { accessToken, user, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const [commandError, setCommandError] = useState('');
  const [pendingMap, setPendingMap] = useState({});
  const [pendingGroupCounts, setPendingGroupCounts] = useState({});
  const pendingRef = useRef({});
  const graceRef = useRef(new Map());
  const graceTimersRef = useRef(new Map());
  const [graceVersion, setGraceVersion] = useState(0);

  useEffect(() => {
    pendingRef.current = pendingMap;
  }, [pendingMap]);

  useEffect(() => () => {
    Object.values(pendingRef.current || {}).forEach((entry) => {
      if (entry?.timeoutId) clearTimeout(entry.timeoutId);
    });
    graceTimersRef.current.forEach((timer) => clearTimeout(timer));
    graceTimersRef.current.clear();
    graceRef.current.clear();
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

  const items = useMemo(() => resolveQuickControlItems({
    selectedIds,
    selectedGroupIds,
    ersDevices,
    ersGroups,
    realtimeDevices,
  }), [selectedGroupIds, selectedIds, ersDevices, ersGroups, realtimeDevices]);

  const buildGraceKey = useCallback((groupId, inputKey) => `${groupId}::${inputKey}`, []);

  const clearGrace = useCallback((groupId, inputKey) => {
    const graceKey = buildGraceKey(groupId, inputKey);
    const timer = graceTimersRef.current.get(graceKey);
    if (timer) {
      clearTimeout(timer);
      graceTimersRef.current.delete(graceKey);
    }
    if (graceRef.current.delete(graceKey)) {
      setGraceVersion((value) => value + 1);
    }
  }, [buildGraceKey]);

  const applyGrace = useCallback((groupId, inputKey, value) => {
    if (!groupId || !inputKey) return;
    const graceKey = buildGraceKey(groupId, inputKey);
    clearGrace(groupId, inputKey);
    graceRef.current.set(graceKey, {
      value,
      expiresAt: Date.now() + GROUP_SHARED_MIXED_GRACE_MS,
    });
    const timer = window.setTimeout(() => {
      graceTimersRef.current.delete(graceKey);
      if (graceRef.current.delete(graceKey)) {
        setGraceVersion((current) => current + 1);
      }
    }, GROUP_SHARED_MIXED_GRACE_MS + 25);
    graceTimersRef.current.set(graceKey, timer);
    setGraceVersion((current) => current + 1);
  }, [buildGraceKey, clearGrace]);

  useEffect(() => {
    const devices = items.filter((item) => item.kind === 'device').map((item) => item.device);
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
  }, [items]);

  const commandsReady = Boolean(connectionInfo?.commandsReady);
  const commandLockReason = connectionInfo?.commandLockReason || 'Preparing live controls…';

  const handleDeviceToggle = useCallback((device) => {
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
      });
  }, [accessToken, commandLockReason, commandsReady]);

  const handleGroupToggle = useCallback((item, displayState) => {
    if (!item?.group?.id || !item?.toggleInput) return;
    if (!commandsReady) {
      setCommandError(commandLockReason);
      return;
    }
    if (!accessToken) {
      setCommandError('Authentication required');
      return;
    }

    const nextValue = !displayState;
    const payload = buildPayloadForInput(item.toggleInput, nextValue);
    if (!payload || !item.group.devices.length) return;

    const inputKey = sanitizeInputKey(item.toggleInput);
    if (!inputKey) return;

    setCommandError('');
    applyGrace(item.group.id, inputKey, nextValue);
    setPendingGroupCounts((prev) => ({
      ...prev,
      [item.group.id]: (prev[item.group.id] || 0) + 1,
    }));

    Promise.allSettled(item.group.devices.map((device) => {
      const deviceId = getDeviceCommandId(device);
      if (!deviceId) return Promise.resolve({ success: false });
      return sendDeviceCommand(deviceId, payload, accessToken);
    }))
      .then((results) => {
        const failed = results.filter((result) => result.status === 'rejected' || !result.value?.success).length;
        if (failed > 0) {
          clearGrace(item.group.id, inputKey);
          throw new Error(`${failed} device command${failed === 1 ? '' : 's'} failed`);
        }
        applyGrace(item.group.id, inputKey, nextValue);
      })
      .catch((err) => {
        setCommandError(err?.message || 'Unable to send group command');
      })
      .finally(() => {
        setPendingGroupCounts((prev) => {
          const nextCount = Math.max(0, (prev[item.group.id] || 0) - 1);
          if (nextCount === 0) {
            const { [item.group.id]: _unused, ...rest } = prev;
            return rest;
          }
          return {
            ...prev,
            [item.group.id]: nextCount,
          };
        });
      });
  }, [accessToken, applyGrace, clearGrace, commandLockReason, commandsReady]);

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
          <span>Configure this widget to select devices or groups</span>
        </div>
      </WidgetShell>
    );
  }

  if (!loading && items.length === 0) {
    return (
      <WidgetShell
        title={settings.title || 'Quick Controls'}
        subtitle="No toggle targets"
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
        className="multi-device-widget multi-device-widget--empty"
      >
        <div className="multi-device-widget__empty">
          <FontAwesomeIcon icon={faQuestionCircle} className="multi-device-widget__empty-icon" />
          <span>Select devices or groups with shared on/off controls</span>
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
          {items.map((item) => {
            if (item.kind === 'group') {
              const grace = graceRef.current.get(buildGraceKey(item.id, item.toggleKey));
              const graceActive = Boolean(grace && grace.expiresAt > Date.now());
              const pending = (pendingGroupCounts[item.id] || 0) > 0;
              const isOn = graceActive ? toControlBoolean(grace.value) : toControlBoolean(item.toggleValue);
              const isMixed = graceActive ? false : Boolean(item.mixed);
              const status = pending ? 'Syncing' : isMixed ? 'Mixed' : isOn ? 'On' : 'Off';

              return (
                <button
                  key={item.key}
                  type="button"
                  className={`multi-device-widget__tile multi-device-widget__tile--group${isOn ? ' on' : ''}${isMixed ? ' mixed' : ''}`}
                  onClick={() => handleGroupToggle(item, isOn)}
                  disabled={pending || !commandsReady}
                  title={item.annotation || undefined}
                >
                  <div className="multi-device-widget__tile-header">
                    <div className="multi-device-widget__tile-icon">
                      <FontAwesomeIcon icon={faLayerGroup} />
                    </div>
                    <div className="multi-device-widget__tile-info">
                      <div className="multi-device-widget__tile-name">
                        {item.group?.name || item.group?.slug || item.id || 'Group'}
                      </div>
                      <div className="multi-device-widget__tile-status">{status}</div>
                    </div>
                  </div>
                </button>
              );
            }

            const { device } = item;
            const commandId = item.commandId;
            const pending = Boolean(commandId && pendingMap[commandId]);
            const displayDevice = applyPendingStateToDevice(device, commandId ? pendingMap[commandId] : null);
            const isOn = resolveToggleState(displayDevice);
            const capabilities = collectCapabilities(displayDevice);
            const icon = resolveDeviceIcon(displayDevice, capabilities);
            const canToggle = canToggleDevice(device);

            if (!canToggle) return null;

            return (
              <button
                key={item.key}
                type="button"
                className={`multi-device-widget__tile${isOn ? ' on' : ''}`}
                onClick={() => handleDeviceToggle(device)}
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
