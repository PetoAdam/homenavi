import React, { useCallback, useMemo, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faLayerGroup, faQuestionCircle } from '@fortawesome/free-solid-svg-icons';
import { useAuth } from '../../../../context/AuthContext';
import useDeviceHubDevices from '../../../../hooks/useDeviceHubDevices';
import useErsInventory from '../../../../hooks/useErsInventory';
import { sendDeviceCommand } from '../../../../services/deviceHubService';
import WidgetShell from '../../../common/WidgetShell/WidgetShell';
import './GroupControlsWidget.css';

function normalizeToggleProperty(property) {
  const key = (property ?? '').toString().trim();
  const lower = key.toLowerCase();
  if (lower === 'state' || lower === 'power' || lower === 'on') return 'on';
  return key;
}

function toBool(value) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  const normalized = String(value ?? '').trim().toLowerCase();
  return normalized === 'on' || normalized === 'true' || normalized === '1';
}

function isOnOffSelectInput(input) {
  const type = (input?.type || input?.kind || '').toString().toLowerCase();
  if (type !== 'select') return false;
  const property = (input?.property || input?.id || '').toString().toLowerCase();
  if (property !== 'power') return false;
  const options = Array.isArray(input?.options) ? input.options : [];
  const hasOn = options.some((opt) => String(opt?.value ?? '').trim().toLowerCase() === 'on');
  const hasOff = options.some((opt) => String(opt?.value ?? '').trim().toLowerCase() === 'off');
  return hasOn && hasOff;
}

function getToggleInput(device) {
  const inputs = Array.isArray(device?.inputs) ? device.inputs : [];
  return inputs.find((input) => {
    const type = (input?.type || input?.kind || '').toString().toLowerCase();
    const valueType = (input?.value_type || input?.valueType || '').toString().toLowerCase();
    return type === 'toggle' || type === 'binary' || valueType === 'boolean' || isOnOffSelectInput(input);
  }) || null;
}

function isToggleWritable(input) {
  if (!input || typeof input !== 'object') return true;
  const access = input.access || input.metadata?.access || {};
  if (access.readOnly === true) return false;
  const negativeFlags = [access.write, access.set, access.command, access.toggle].filter((flag) => flag === false);
  return negativeFlags.length === 0;
}

function getToggleProperty(device) {
  const input = getToggleInput(device);
  if (input?.property) return input.property;
  if (input?.id) return input.id;
  const state = device?.state || {};
  if ('on' in state) return 'on';
  if ('state' in state) return 'state';
  if ('power' in state) return 'power';
  return '';
}

function canToggleDevice(device) {
  const input = getToggleInput(device);
  if (input && !isToggleWritable(input)) return false;
  return Boolean(getToggleProperty(device));
}

function getCommandDeviceId(device) {
  const hdpIds = Array.isArray(device?.hdpIds) ? device.hdpIds : [];
  return device?.id || device?.hdpId || hdpIds[0] || '';
}

function resolveToggleState(device) {
  const state = device?.state || {};
  const candidates = [state.on, state.state, state.power, device?.toggleState];
  for (const candidate of candidates) {
    if (candidate !== undefined) return toBool(candidate);
  }
  return false;
}

function buildTogglePayload(device, value) {
  const property = getToggleProperty(device);
  if (!property) return null;
  if (property.toLowerCase() === 'power') {
    return { state: { power: value ? 'on' : 'off' } };
  }
  return { state: { [normalizeToggleProperty(property)]: value } };
}

function getGroupStatus(devices) {
  if (!devices.length) return { label: 'Unavailable', nextValue: true, canToggle: false };
  const onCount = devices.filter(resolveToggleState).length;
  if (onCount === devices.length) return { label: 'On', nextValue: false, canToggle: true };
  if (onCount === 0) return { label: 'Off', nextValue: true, canToggle: true };
  return { label: 'Mixed', nextValue: true, canToggle: true };
}

export default function GroupControlsWidget({ settings = {}, editMode, onSettings, onRemove }) {
  const { accessToken, user, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');
  const [commandError, setCommandError] = useState('');
  const [pendingGroupIds, setPendingGroupIds] = useState([]);

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
  const commandsReady = Boolean(connectionInfo?.commandsReady);
  const commandLockReason = connectionInfo?.commandLockReason || 'Preparing live controls…';
  const loading = hdpLoading || ersLoading;

  const groups = useMemo(() => {
    const groupMap = new Map();
    (Array.isArray(ersGroups) ? ersGroups : []).forEach((group) => {
      if (group?.id) groupMap.set(group.id, group);
    });
    return selectedGroupIds
      .map((groupId) => groupMap.get(groupId) || null)
      .filter(Boolean)
      .map((group) => {
        const devices = (Array.isArray(group.devices) ? group.devices : []).filter(canToggleDevice);
        return {
          ...group,
          devices,
          status: getGroupStatus(devices),
        };
      });
  }, [ersGroups, selectedGroupIds]);

  const handleToggleGroup = useCallback(async (group) => {
    if (!commandsReady) {
      setCommandError(commandLockReason);
      return;
    }
    if (!accessToken) {
      setCommandError('Authentication required');
      return;
    }
    if (!group?.status?.canToggle) return;

    setCommandError('');
    setPendingGroupIds((prev) => [...prev, group.id]);
    try {
      const results = await Promise.allSettled(group.devices.map((device) => {
        const deviceId = getCommandDeviceId(device);
        const payload = buildTogglePayload(device, group.status.nextValue);
        if (!deviceId || !payload) return Promise.resolve({ success: false });
        return sendDeviceCommand(deviceId, payload, accessToken);
      }));
      const failed = results.filter((result) => result.status === 'rejected' || !result.value?.success).length;
      if (failed > 0) {
        throw new Error(`${failed} device command${failed === 1 ? '' : 's'} failed`);
      }
    } catch (err) {
      setCommandError(err?.message || 'Unable to toggle group');
    } finally {
      setPendingGroupIds((prev) => prev.filter((id) => id !== group.id));
    }
  }, [accessToken, commandLockReason, commandsReady]);

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
          {groups.map((group) => {
            const pending = pendingGroupIds.includes(group.id);
            return (
              <button
                key={group.id}
                type="button"
                className={`group-controls-widget__tile${group.status.label === 'On' ? ' on' : ''}`}
                onClick={() => handleToggleGroup(group)}
                disabled={pending || !group.status.canToggle || !commandsReady}
              >
                <div className="group-controls-widget__tile-header">
                  <div className="group-controls-widget__tile-icon"><FontAwesomeIcon icon={faLayerGroup} /></div>
                  <div>
                    <div className="group-controls-widget__tile-name">{group.name || group.slug || group.id}</div>
                    <div className="group-controls-widget__tile-status">{pending ? 'Sending…' : group.status.label}</div>
                  </div>
                </div>
                <div className="group-controls-widget__tile-meta">{group.devices.length} controllable member{group.devices.length === 1 ? '' : 's'}</div>
              </button>
            );
          })}
        </div>
      </div>
    </WidgetShell>
  );
}

GroupControlsWidget.defaultHeight = 4;