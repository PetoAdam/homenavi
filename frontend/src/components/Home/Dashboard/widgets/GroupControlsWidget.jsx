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

  const selectedGroupId = settings.group_id || (Array.isArray(settings.group_ids) ? settings.group_ids[0] : '');
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

  const group = useMemo(() => {
    const groupMap = new Map();
    (Array.isArray(ersGroups) ? ersGroups : []).forEach((group) => {
      if (group?.id) groupMap.set(group.id, group);
    });
    const selectedGroup = groupMap.get(selectedGroupId) || null;
    if (!selectedGroup) return null;
    const devices = Array.isArray(selectedGroup.devices) ? selectedGroup.devices : [];
    const sharedControls = intersectSharedInputs(devices);
    const commonFieldState = buildSharedFieldState(devices);
    const configuredControls = Array.isArray(settings.controls)
      ? settings.controls.map((item) => String(item || '').trim().toLowerCase()).filter(Boolean)
      : null;
    const configuredFields = Array.isArray(settings.fields)
      ? settings.fields.map((field) => String(field || '').trim()).filter(Boolean)
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
      ...selectedGroup,
      devices,
      sharedControls,
      visibleInputs,
      visibleValues,
      visibleFieldState,
      visibleFields: Object.keys(visibleFieldState),
      primaryToggle,
    };
  }, [ersGroups, selectedGroupId, settings.controls, settings.fields]);

  useEffect(() => {
    setGroupValues((prev) => {
      const next = {};
      if (!group) return prev;
      const baseValues = { ...(group.visibleValues || {}) };
      group.visibleInputs.forEach((input) => {
        const inputKey = sanitizeInputKey(input);
        const grace = graceRef.current.get(buildGraceKey(group.id, inputKey));
        if (grace && grace.expiresAt > Date.now()) {
          baseValues[inputKey] = grace.value;
        }
      });
      next[group.id] = baseValues;
      return next;
    });
  }, [buildGraceKey, graceVersion, group]);

  useEffect(() => () => {
    graceTimersRef.current.forEach((timer) => window.clearTimeout(timer));
    graceTimersRef.current.clear();
    graceRef.current.clear();
  }, []);

  const resolvedGroup = useMemo(() => {
    if (!group) return null;
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
  }, [buildGraceKey, graceVersion, group, pendingGroupCounts]);

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

  if (!selectedGroupId || !group) {
    return (
      <WidgetShell
        title={settings.title || 'Group Controls'}
        subtitle={selectedGroupId ? 'Selected group unavailable' : 'No group selected'}
        editMode={editMode}
        onSettings={onSettings}
        onRemove={onRemove}
        className="group-controls-widget group-controls-widget--empty"
      >
        <div className="group-controls-widget__empty">
          <FontAwesomeIcon icon={faQuestionCircle} className="group-controls-widget__empty-icon" />
          <span>{selectedGroupId ? 'Re-select a group in widget settings' : 'Configure this widget to select a group'}</span>
        </div>
      </WidgetShell>
    );
  }

  return (
    <WidgetShell
      editMode={editMode}
      onSettings={onSettings}
      onRemove={onRemove}
      loading={loading}
      className="group-controls-widget"
    >
      <div className="group-controls-widget__content">
        {commandError ? <div className="group-controls-widget__error">{commandError}</div> : null}
        {!commandError && !commandsReady ? <div className="group-controls-widget__error">{commandLockReason}</div> : null}
        <div className="group-controls-widget__header">
          <div className="group-controls-widget__title-group">
            <FontAwesomeIcon icon={faLayerGroup} className="group-controls-widget__icon" />
            <div className="group-controls-widget__title-info">
              <span className="group-controls-widget__name">{group.name || group.slug || settings.title || 'Group'}</span>
              <span className="group-controls-widget__status">
                {group.devices.length} member{group.devices.length === 1 ? '' : 's'}
              </span>
            </div>
          </div>
        </div>
        <div className="group-controls-widget__controls-area">
        {resolvedGroup ? (() => {
          const pending = resolvedGroup.pending;
          const values = groupValues[resolvedGroup.id] || resolvedGroup.visibleValues || {};
          return (
            <>
              {resolvedGroup.visibleInputs.length ? (
                <DeviceControlList
                  inputs={resolvedGroup.visibleInputs}
                  values={values}
                  pending={pending || bootstrapping}
                  onValueChange={(key, nextValue) => handleGroupValueChange(resolvedGroup.id, key, nextValue)}
                  onCommand={(input, nextValue) => handleGroupCommand(resolvedGroup, input, nextValue)}
                  layout="cards"
                  collapseAfter={4}
                />
              ) : null}

              {resolvedGroup.visibleFields.length ? (
                <DeviceMetricsRenderer
                  device={{ state: resolvedGroup.visibleFieldState }}
                  selectedFields={resolvedGroup.visibleFields}
                  layout="cards"
                  collapseAfter={5}
                />
              ) : null}
            </>
          );
        })() : null}
        </div>
      </div>
    </WidgetShell>
  );
}

GroupControlsWidget.defaultHeight = 4;