import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faLayerGroup, faQuestionCircle } from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../../context/AuthContext';
import useDeviceHubDevices from '../../../../hooks/useDeviceHubDevices';
import useErsInventory from '../../../../hooks/useErsInventory';
import { sendDeviceCommand } from '../../../../services/deviceHubService';
import { resolveCommandDeviceId } from '../../../../utils/deviceIdentity';
import {
  buildPayloadForInput,
  buildSharedFieldState,
  intersectSharedInputs,
} from '../../../../utils/groupControls';
import DeviceControlList from '../../../common/DeviceControlRenderer/DeviceControlRenderer';
import { sanitizeInputKey } from '../../../common/DeviceControlRenderer/deviceControlUtils';
import DeviceMetricsRenderer from '../../../common/DeviceMetricsRenderer/DeviceMetricsRenderer';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';
import './GroupControlsWidget.css';

const GROUP_SHARED_MIXED_GRACE_MS = 2800;

function getCommandDeviceId(device) {
  return resolveCommandDeviceId(device);
}

function normalizeConfiguredKeys(value) {
  return Array.isArray(value)
    ? value.map((item) => String(item || '').trim().toLowerCase()).filter(Boolean)
    : null;
}

export default function GroupControlsWidget({ settings = {}, editMode, onSettings, onRemove }) {
  const { accessToken, user, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');
  const [commandError, setCommandError] = useState('');
  const [pendingGroupCounts, setPendingGroupCounts] = useState({});
  const [groupValues, setGroupValues] = useState({});
  const graceRef = useRef(new Map());
  const graceTimersRef = useRef(new Map());
  const [graceVersion, setGraceVersion] = useState(0);

  const { devices: realtimeDevices, loading: hdpLoading, connectionInfo } = useDeviceHubDevices({
    enabled: Boolean(isResidentOrAdmin),
    metadataMode: 'ws',
    accessToken,
    authReady: Boolean(accessToken) && !bootstrapping,
  });

  const { groups: ersGroups, loading: ersLoading } = useErsInventory({
    enabled: Boolean(isResidentOrAdmin && accessToken),
    accessToken,
    realtimeDevices,
  });

  const selectedGroupIds = Array.isArray(settings.group_ids) ? settings.group_ids : [];
  const groupConfigs = settings.group_configs && typeof settings.group_configs === 'object' ? settings.group_configs : {};
  const commandsReady = Boolean(connectionInfo?.commandsReady);
  const commandLockReason = connectionInfo?.commandLockReason || 'Preparing live controls…';
  const loading = hdpLoading || ersLoading;

  const buildGraceKey = useCallback((groupId, inputKey) => `${groupId}::${inputKey}`, []);

  const clearGrace = useCallback((groupId, inputKey) => {
    const graceKey = buildGraceKey(groupId, inputKey);
    const timer = graceTimersRef.current.get(graceKey);
    if (timer) {
      window.clearTimeout(timer);
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

  const groups = useMemo(() => {
    const groupMap = new Map();
    (Array.isArray(ersGroups) ? ersGroups : []).forEach((group) => {
      if (group?.id) groupMap.set(group.id, group);
    });
    return selectedGroupIds
      .map((groupId) => groupMap.get(groupId) || null)
      .filter(Boolean)
      .map((group) => {
        const devices = Array.isArray(group.devices) ? group.devices : [];
        const sharedControls = intersectSharedInputs(devices);
        const commonFieldState = buildSharedFieldState(devices);
        const config = groupConfigs[group.id] && typeof groupConfigs[group.id] === 'object' ? groupConfigs[group.id] : {};
        const configuredControls = normalizeConfiguredKeys(config.controls);
        const configuredFields = Array.isArray(config.fields)
          ? config.fields.map((field) => String(field || '').trim()).filter(Boolean)
          : [];
        const visibleInputs = configuredControls
          ? sharedControls.inputs.filter((input) => configuredControls.includes(String(input?.id || input?.property || '').trim().toLowerCase()))
          : sharedControls.inputs;
        const visibleValues = visibleInputs.reduce((acc, input) => {
          const key = sanitizeInputKey(input);
          if (!key) return acc;
          if (Object.prototype.hasOwnProperty.call(sharedControls.values, key)) {
            acc[key] = sharedControls.values[key];
          }
          return acc;
        }, {});
        const visibleFieldState = Object.fromEntries(
          configuredFields
            .filter((field) => Object.prototype.hasOwnProperty.call(commonFieldState, field))
            .map((field) => [field, commonFieldState[field]]),
        );
        const primaryToggle = visibleInputs.find((input) => {
          const key = String(input?.id || input?.property || '').trim().toLowerCase();
          return input?.type === 'toggle' && (key === 'on' || key === 'state' || key === 'power');
        }) || visibleInputs.find((input) => input?.type === 'toggle') || null;
        return {
          ...group,
          devices,
          sharedControls,
          visibleInputs,
          visibleValues,
          visibleFieldState,
          visibleFields: Object.keys(visibleFieldState),
          primaryToggle,
        };
      });
  }, [ersGroups, groupConfigs, selectedGroupIds]);

  useEffect(() => {
    setGroupValues((prev) => {
      const next = {};
      groups.forEach((group) => {
        const baseValues = { ...(group.visibleValues || {}) };
        group.visibleInputs.forEach((input) => {
          const inputKey = sanitizeInputKey(input);
          const grace = graceRef.current.get(buildGraceKey(group.id, inputKey));
          if (grace && grace.expiresAt > Date.now()) {
            baseValues[inputKey] = grace.value;
          }
        });
        next[group.id] = baseValues;
      });
      return next;
    });
  }, [buildGraceKey, graceVersion, groups]);

  useEffect(() => () => {
    graceTimersRef.current.forEach((timer) => window.clearTimeout(timer));
    graceTimersRef.current.clear();
    graceRef.current.clear();
  }, []);

  const resolvedGroups = useMemo(() => {
    return groups.map((group) => {
      const nextValues = { ...(group.visibleValues || {}) };
      const nextInputs = group.visibleInputs.map((input) => {
        const inputKey = sanitizeInputKey(input);
        const grace = graceRef.current.get(buildGraceKey(group.id, inputKey));
        if (!grace || grace.expiresAt <= Date.now()) {
          return input;
        }
        nextValues[inputKey] = grace.value;
        return {
          ...input,
          mixed: false,
          annotation: (pendingGroupCounts[group.id] || 0) > 0 ? 'Syncing member state...' : '',
        };
      });
      return {
        ...group,
        visibleInputs: nextInputs,
        visibleValues: nextValues,
        pending: (pendingGroupCounts[group.id] || 0) > 0,
      };
    });
  }, [buildGraceKey, graceVersion, groups, pendingGroupCounts]);

  const handleGroupValueChange = useCallback((groupId, key, nextValue) => {
    setGroupValues((prev) => ({
      ...prev,
      [groupId]: {
        ...(prev[groupId] || {}),
        [key]: nextValue,
      },
    }));
  }, []);

  const handleGroupCommand = useCallback(async (group, input, nextValue) => {
    if (!commandsReady) {
      setCommandError(commandLockReason);
      return;
    }
    if (!accessToken) {
      setCommandError('Authentication required');
      return;
    }

    const payload = buildPayloadForInput(input, nextValue);
    if (!payload || !group?.devices?.length) return;

    const inputKey = sanitizeInputKey(input);
    if (!inputKey) return;

    setCommandError('');
    setGroupValues((prev) => ({
      ...prev,
      [group.id]: {
        ...(prev[group.id] || {}),
        [inputKey]: nextValue,
      },
    }));
    applyGrace(group.id, inputKey, nextValue);
    setPendingGroupCounts((prev) => ({
      ...prev,
      [group.id]: (prev[group.id] || 0) + 1,
    }));
    try {
      const results = await Promise.allSettled(group.devices.map((device) => {
        const deviceId = getCommandDeviceId(device);
        if (!deviceId || !payload) return Promise.resolve({ success: false });
        return sendDeviceCommand(deviceId, payload, accessToken);
      }));
      const failed = results.filter((result) => result.status === 'rejected' || !result.value?.success).length;
      if (failed > 0) {
        clearGrace(group.id, inputKey);
        throw new Error(`${failed} device command${failed === 1 ? '' : 's'} failed`);
      }
      applyGrace(group.id, inputKey, nextValue);
    } catch (err) {
      setCommandError(err?.message || 'Unable to toggle group');
    } finally {
      setPendingGroupCounts((prev) => {
        const nextCount = Math.max(0, (prev[group.id] || 0) - 1);
        if (nextCount === 0) {
          const { [group.id]: _unused, ...rest } = prev;
          return rest;
        }
        return {
          ...prev,
          [group.id]: nextCount,
        };
      });
    }
  }, [accessToken, applyGrace, clearGrace, commandLockReason, commandsReady]);

  if (!selectedGroupIds.length) {
    return (
      <WidgetShell
        title={settings.title || 'Group Controls'}
        subtitle="No groups selected"
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
        className="group-controls-widget group-controls-widget--empty"
      >
        <div className="group-controls-widget__empty">
          <FontAwesomeIcon icon={faQuestionCircle} className="group-controls-widget__empty-icon" />
          <span>Configure this widget to select groups</span>
        </div>
      </WidgetShell>
    );
  }

  return (
    <WidgetShell
      title={settings.title || 'Group Controls'}
      subtitle={settings.subtitle}
      icon={faLayerGroup}
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      loading={loading}
      className="group-controls-widget"
    >
      <div className="group-controls-widget__content">
        {commandError ? <div className="group-controls-widget__error">{commandError}</div> : null}
        {!commandError && !commandsReady ? <div className="group-controls-widget__error">{commandLockReason}</div> : null}
        <div className="group-controls-widget__grid">
          {resolvedGroups.map((group) => {
            const pending = group.pending;
            const values = groupValues[group.id] || group.visibleValues || {};
            const primaryToggleKey = group.primaryToggle
              ? sanitizeInputKey(group.primaryToggle)
              : null;
            const statusLabel = pending
              ? 'Sending…'
              : primaryToggleKey && values[primaryToggleKey] !== undefined
                ? (values[primaryToggleKey] ? 'On' : 'Off')
                : null;
            return (
              <div
                key={group.id}
                className={`group-controls-widget__tile${statusLabel === 'On' ? ' on' : ''}`}
              >
                <div className="group-controls-widget__tile-header">
                  <div className="group-controls-widget__tile-icon"><FontAwesomeIcon icon={faLayerGroup} /></div>
                  <div>
                    <div className="group-controls-widget__tile-name">{group.name || group.slug || group.id}</div>
                    {statusLabel ? <div className="group-controls-widget__tile-status">{statusLabel}</div> : null}
                  </div>
                </div>

                {group.visibleInputs.length ? (
                  <DeviceControlList
                    inputs={group.visibleInputs}
                    values={values}
                    pending={pending || bootstrapping}
                    onValueChange={(key, nextValue) => handleGroupValueChange(group.id, key, nextValue)}
                    onCommand={(input, nextValue) => handleGroupCommand(group, input, nextValue)}
                    layout="list"
                    collapseAfter={4}
                  />
                ) : null}

                {group.visibleFields.length ? (
                  <DeviceMetricsRenderer
                    device={{ state: group.visibleFieldState }}
                    selectedFields={group.visibleFields}
                    layout="list"
                    collapseAfter={4}
                  />
                ) : null}

                {!group.visibleInputs.length && !group.visibleFields.length ? (
                  <div className="group-controls-widget__empty-state">
                    Configure controls or state fields for this group in widget settings.
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      </div>
    </WidgetShell>
  );
}

GroupControlsWidget.defaultHeight = 4;