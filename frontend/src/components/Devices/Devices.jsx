import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faBolt, faGaugeHigh, faSatelliteDish, faSignal, faPlus, faMagnifyingGlass, faLayerGroup } from '@fortawesome/free-solid-svg-icons';
import { useNavigate } from 'react-router-dom';
import GlassCard from '../common/GlassCard/GlassCard';
import GlassMetric from '../common/GlassMetric/GlassMetric';
import GlassPill from '../common/GlassPill/GlassPill';
import PageHeader from '../common/PageHeader/PageHeader';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import LoadingView from '../common/LoadingView/LoadingView';
import useDeviceHubDevices from '../../hooks/useDeviceHubDevices';
import useErsInventory from '../../hooks/useErsInventory';
import DeviceTile from './DeviceTile';
import { useAuth } from '../../context/AuthContext';
import {
  sendDeviceCommand as sendDeviceCommandApi,
  createDevice as createDeviceApi,
  listIntegrations as listIntegrationsApi,
  setDeviceIcon as setDeviceIconApi,
  deleteDevice as deleteDeviceApi,
  startPairing as startPairingApi,
  stopPairing as stopPairingApi,
} from '../../services/deviceHubService';
import {
  createErsDevice as createErsDeviceApi,
  patchErsDevice as patchErsDeviceApi,
  setErsDeviceHdpBindings as setErsDeviceHdpBindingsApi,
} from '../../services/entityRegistryService';
import AddDeviceModal from './AddDeviceModal';
import { loadDevicesListPrefs, normalizeDevicesListPrefs, saveDevicesListPrefs } from './devicesListPrefs';
import './Devices.css';

const FALLBACK_INTEGRATIONS = [
  { protocol: 'zigbee', label: 'Zigbee', status: 'active' },
  { protocol: 'matter', label: 'Matter', status: 'planned' },
  { protocol: 'thread', label: 'Thread', status: 'planned' },
  { protocol: 'lan', label: 'LAN Bridge', status: 'active' },
];

export default function Devices() {
  const navigate = useNavigate();
  const { user, accessToken, bootstrapping } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

  const initialPrefsRef = React.useRef(null);
  if (initialPrefsRef.current === null) {
    initialPrefsRef.current = normalizeDevicesListPrefs(loadDevicesListPrefs());
  }
  const initialPrefs = initialPrefsRef.current;

  const [metadataMode, setMetadataMode] = useState(initialPrefs.metadataMode);
  const [groupByRoom, setGroupByRoom] = useState(initialPrefs.groupByRoom);
  const {
    devices: realtimeDevices,
    loading: realtimeLoading,
    error: realtimeError,
    connectionInfo,
    pairingSessions,
    pairingConfig,
    refreshPairings,
  } = useDeviceHubDevices({ enabled: isResidentOrAdmin, metadataMode });

  const {
    devices,
    rooms,
    tags,
    loading: ersLoading,
    error: ersError,
    refresh: refreshErs,
  } = useErsInventory({ enabled: isResidentOrAdmin, accessToken, realtimeDevices });

  const stats = useMemo(() => {
    const total = devices.length;
    if (total === 0) {
      return { total: 0, online: 0, withState: 0, sensors: 0 };
    }
    let online = 0;
    let withState = 0;
    let sensors = 0;
    devices.forEach(dev => {
      if (dev.online) online += 1;
      if (dev.stateHasValues) withState += 1;
      if (Array.isArray(dev.capabilities) && dev.capabilities.some(cap => (cap.kind || '').toLowerCase() === 'numeric')) {
        sensors += 1;
      }
    });
    return { total, online, withState, sensors };
  }, [devices]);

  const [pendingCommands, setPendingCommands] = useState({}); // { [hdpId]: { corr, timerId } }
  const [commandError, setCommandError] = useState(null);
  const [integrations, setIntegrations] = useState([]);
  const [integrationsLoading, setIntegrationsLoading] = useState(false);
  const [integrationsError, setIntegrationsError] = useState(null);
  const [showAddModal, setShowAddModal] = useState(false);
  const [iconOverrides, setIconOverrides] = useState({});
  const [protocolFilter, setProtocolFilter] = useState(initialPrefs.protocolFilter);
  const [roomFilter, setRoomFilter] = useState(initialPrefs.roomFilter);
  const [tagFilter, setTagFilter] = useState(initialPrefs.tagFilter);
  const [searchTerm, setSearchTerm] = useState(initialPrefs.searchTerm);
  const openAddModal = useCallback(() => setShowAddModal(true), []);

  useEffect(() => {
    saveDevicesListPrefs({
      metadataMode,
      groupByRoom,
      protocolFilter,
      roomFilter,
      tagFilter,
      searchTerm,
    });
  }, [groupByRoom, metadataMode, protocolFilter, roomFilter, searchTerm, tagFilter]);

  const integrationsErrorDisplay = useMemo(() => {
    if (!integrationsError) return null;
    if (/not\s+found/i.test(integrationsError)) {
      return null;
    }
    return integrationsError;
  }, [integrationsError]);

  const toggleMetadataMode = useCallback(() => {
    setMetadataMode(prev => (prev === 'rest' ? 'ws' : 'rest'));
  }, []);

  const connectionPills = useMemo(() => {
    const pills = [];
    const metadataStatus = connectionInfo?.metadata || { connected: false };
    const metadataLabel = metadataMode === 'rest'
      ? 'HDP metadata: REST'
      : 'HDP metadata: WebSocket';
    pills.push({
      key: 'metadata',
      text: metadataLabel,
      tone: metadataStatus.connected ? 'success' : 'warning',
      icon: faSatelliteDish,
      title: metadataMode === 'rest'
        ? 'Device Hub metadata uses REST bootstrap (state is still WebSocket). Click to switch to WebSocket-only metadata.'
        : 'Device Hub metadata uses WebSocket-only metadata (state is still WebSocket). Click to switch back to REST bootstrap.',
      onClick: toggleMetadataMode,
    });
    if (connectionInfo?.state) {
      const tone = connectionInfo.state.connected ? 'success' : 'warning';
      const label = connectionInfo.state.connected ? 'State: connected' : 'State stream offline';
      pills.push({ key: 'state', text: label, tone, icon: faSignal });
    }
    return pills;
  }, [connectionInfo, metadataMode, toggleMetadataMode]);

  const subtitleText = useMemo(() => {
    const segments = ['Live inventory sourced from the Device Hub over MQTT/WebSocket'];
    if (typeof stats.total === 'number') {
      segments.push(`· ${stats.total} discovered`);
    }
    if (typeof stats.online === 'number' && stats.total > 0) {
      segments.push(`· ${stats.online} online`);
    }
    return segments.join(' ');
  }, [stats.total, stats.online]);

  const handleCommand = (device, payload) => {
    if (!device?.id) return Promise.resolve();
    setCommandError(null);

    const stateVersionAtSend = device?.stateUpdatedAt instanceof Date
      ? device.stateUpdatedAt.getTime()
      : (device?.stateUpdatedAt || 0);
    const startedAt = Date.now();

    const corr = (payload && payload.correlation_id)
      || (typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`);
    const enrichedPayload = payload && typeof payload === 'object'
      ? { ...payload, correlation_id: corr }
      : { correlation_id: corr };

    // Keep UI in "pending" until we see a matching command_result or a timeout.
    setPendingCommands(prev => ({ ...prev, [device.id]: { corr, startedAt, stateVersion: stateVersionAtSend } }));

    return sendDeviceCommandApi(device.id, enrichedPayload, accessToken)
      .then(res => {
        if (!res.success) {
          const message = res.error || 'Unable to send command';
          throw new Error(message);
        }
        return res.data;
      })
      .catch(err => {
        console.error('Failed to send device command', err);
        const message = err?.message || 'Unable to send device command';
        setCommandError(message);
        throw err;
      })
      .finally(() => {
        // Leave pending entry until cleared by command_result or timeout fallback.
        // Set a short timeout so the UI doesn't get stuck if no result arrives.
        setPendingCommands(prev => {
          const next = { ...prev };
          const current = next[device.id];
          if (!current) return next;
          if (current.timeoutId) {
            clearTimeout(current.timeoutId);
          }
          next[device.id] = {
            ...current,
            timeoutId: setTimeout(() => {
              setPendingCommands(latePrev => {
                const clone = { ...latePrev };
                delete clone[device.id];
                return clone;
              });
            }, 3000),
          };
          return next;
        });
      });
  };

  // Clear pending when command_result with matching corr is observed.
  useEffect(() => {
    if (!devices || !devices.length) return;
    setPendingCommands(prev => {
      let changed = false;
      const next = { ...prev };
      devices.forEach(dev => {
        const pending = next[dev.id];
        const result = dev.lastCommandResult;
        if (!pending || !result) return;
        if (!pending.corr || !result.corr || pending.corr !== result.corr) return;

        // Avoid clearing too early; wait for a newer state than when we sent the command
        // so the optimistic UI does not flicker back to the previous value.
        const stateTs = dev.stateUpdatedAt instanceof Date
          ? dev.stateUpdatedAt.getTime()
          : (dev.stateUpdatedAt || 0);
        const baselineTs = pending.stateVersion || 0;
        const resultTs = Number(result.ts || 0);
        const hasStateAdvanced = stateTs && stateTs > baselineTs;
        const stateCoversResult = stateTs && resultTs && stateTs >= resultTs;

        const shouldClear = !result.success || hasStateAdvanced || stateCoversResult;
        if (!shouldClear) return;

        if (pending.timeoutId) {
          clearTimeout(pending.timeoutId);
        }
        delete next[dev.id];
        changed = true;
      });
      return changed ? next : prev;
    });
  }, [devices]);

  const handleRename = async (device, name) => {
    if (!device?.ersId) {
      throw new Error('Device not ready for rename');
    }
    const trimmed = typeof name === 'string' ? name.trim() : '';
    if (!accessToken) {
      throw new Error('Authentication required to rename device');
    }
    const res = await patchErsDeviceApi(device.ersId, { name: trimmed }, accessToken);
    if (!res.success) {
      throw new Error(res.error || 'Unable to rename device');
    }
    refreshErs?.();
    return res.data;
  };

  const handleCreateDevice = useCallback(async payload => {
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    const { name: ersName, ...hdpPayload } = (payload && typeof payload === 'object') ? payload : {};
    const res = await createDeviceApi(hdpPayload, accessToken);
    if (!res.success) {
      throw new Error(res.error || 'Failed to create device');
    }

    const created = res.data || {};
    const hdpId = created.device_id || created.deviceId || created.id || payload?.identifier || '';
    const name = (typeof ersName === 'string' && ersName.trim()) ? ersName.trim() : hdpId;
    const description = payload?.description || created.description || '';

    // Best effort: create the canonical logical device in ERS and bind it to the newly-created HDP ID.
    const ersRes = await createErsDeviceApi({ name, description }, accessToken);
    if (!ersRes.success) {
      throw new Error(ersRes.error || 'Created device, but failed to register it in Entity Registry');
    }
    const ersId = ersRes.data?.id;
    if (ersId && hdpId) {
      const bindRes = await setErsDeviceHdpBindingsApi(ersId, [hdpId], accessToken);
      if (!bindRes.success) {
        throw new Error(bindRes.error || 'Created device, but failed to bind it in Entity Registry');
      }
    }

    refreshErs?.();
    return res.data;
  }, [accessToken, refreshErs]);

  const handleUpdateIcon = useCallback(async (device, iconKey) => {
    if (!device?.id) {
      throw new Error('Device not ready for icon update');
    }
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    const normalized = typeof iconKey === 'string' && iconKey.trim() ? iconKey.trim() : null;
    const res = await setDeviceIconApi(device.id, normalized, accessToken);
    if (!res.success) {
      throw new Error(res.error || 'Unable to update icon');
    }
    setIconOverrides(prev => {
      const next = { ...prev };
      if (!normalized) {
        delete next[device.id];
      } else {
        next[device.id] = normalized;
      }
      return next;
    });
    return res.data;
  }, [accessToken]);

  const handleDeleteDevice = useCallback(async (device, options = {}) => {
    if (!accessToken) {
      throw new Error('Authentication required');
    }

    if (!device?.id) {
      throw new Error('Device not ready for deletion');
    }

    // Always delete via HDP so the owning adapter can perform protocol-specific removal.
    const hdpRes = await deleteDeviceApi(device.id, accessToken, options);
    if (!hdpRes.success) {
      throw new Error(hdpRes.error || 'Unable to delete device');
    }

    // ERS is auto-managed from HDP device_removed events.
    refreshErs?.();
    return hdpRes.data;
  }, [accessToken, refreshErs]);

  const handleStartPairing = useCallback(async payload => {
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    const res = await startPairingApi(payload, accessToken);
    if (!res.success) {
      throw new Error(res.error || 'Unable to start pairing');
    }
    refreshPairings?.();
    return res.data;
  }, [accessToken, refreshPairings]);

  const handleStopPairing = useCallback(async protocol => {
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    const res = await stopPairingApi(protocol, accessToken);
    if (!res.success) {
      throw new Error(res.error || 'Unable to stop pairing');
    }
    refreshPairings?.();
    return res.data;
  }, [accessToken, refreshPairings]);

  const loadIntegrations = useCallback(async () => {
    if (!isResidentOrAdmin) return;
    setIntegrationsLoading(true);
    setIntegrationsError(null);
    try {
      const res = await listIntegrationsApi(accessToken);
      if (!res.success) {
        throw new Error(res.error || 'Unable to load integrations');
      }
      setIntegrations(Array.isArray(res.data) ? res.data : []);
    } catch (err) {
      setIntegrationsError(err?.message || 'Unable to load integrations');
    } finally {
      setIntegrationsLoading(false);
    }
  }, [accessToken, isResidentOrAdmin]);

  useEffect(() => {
    if (isResidentOrAdmin) {
      loadIntegrations();
    }
  }, [isResidentOrAdmin, loadIntegrations]);

  const devicesWithOverrides = useMemo(() => {
    if (!devices.length) return devices;
    return devices.map(device => {
      const override = device?.id ? iconOverrides[device.id] : null;
      if (!override) {
        return device;
      }
      return { ...device, icon: override };
    });
  }, [devices, iconOverrides]);

  const groupedDevices = useMemo(() => {
    const groups = new Map();
    devicesWithOverrides.forEach(device => {
      const key = (device?.roomName || '').trim() || 'Unassigned';
      if (!groups.has(key)) {
        groups.set(key, []);
      }
      groups.get(key).push(device);
    });

    const keys = Array.from(groups.keys()).sort((a, b) => {
      if (a === 'Unassigned') return 1;
      if (b === 'Unassigned') return -1;
      return a.localeCompare(b, undefined, { sensitivity: 'base' });
    });

    return keys.map(key => ({
      key,
      title: key,
      devices: groups.get(key) || [],
    }));
  }, [devicesWithOverrides]);

  const protocolCounts = useMemo(() => {
    return devicesWithOverrides.reduce((acc, device) => {
      const key = (device.protocol || '').toLowerCase() || 'unknown';
      acc[key] = (acc[key] || 0) + 1;
      return acc;
    }, {});
  }, [devicesWithOverrides]);

  const integrationDisplay = useMemo(
    () => (integrations.length ? integrations : FALLBACK_INTEGRATIONS),
    [integrations],
  );

  const filterChips = useMemo(() => {
    const map = new Map();
    integrationDisplay.forEach(item => {
      if (!item || !item.protocol) return;
      const key = item.protocol.toLowerCase();
      map.set(key, {
        key,
        label: item.label || item.protocol,
        status: item.status || 'unknown',
        count: protocolCounts[key] || 0,
      });
    });
    devicesWithOverrides.forEach(device => {
      const raw = (device.protocol || '').toLowerCase();
      if (!raw) return;
      if (map.has(raw)) {
        map.set(raw, {
          ...map.get(raw),
          count: protocolCounts[raw] || 0,
        });
        return;
      }
      map.set(raw, {
        key: raw,
        label: device.protocol || raw.toUpperCase(),
        status: 'detected',
        count: protocolCounts[raw] || 0,
      });
    });
    return Array.from(map.values()).sort((a, b) => a.label.localeCompare(b.label));
  }, [devicesWithOverrides, integrationDisplay, protocolCounts]);

  const filterOptions = useMemo(() => {
    const base = {
      key: 'all',
      label: 'All',
      status: 'all',
      count: devicesWithOverrides.length,
    };
    return [base, ...filterChips];
  }, [devicesWithOverrides.length, filterChips]);

  const filteredByProtocol = useMemo(() => {
    if (protocolFilter === 'all') {
      return devicesWithOverrides;
    }
    return devicesWithOverrides.filter(device => (device.protocol || '').toLowerCase() === protocolFilter);
  }, [devicesWithOverrides, protocolFilter]);

  const filteredDevices = useMemo(() => {
    const query = searchTerm.trim().toLowerCase();
    let base = filteredByProtocol;

    if (roomFilter !== 'all') {
      base = base.filter(device => {
        const roomId = (device?.room?.id || device?.room_id || '').toString();
        if (roomFilter === 'none') {
          return !roomId;
        }
        return roomId === roomFilter;
      });
    }

    if (tagFilter !== 'all') {
      base = base.filter(device => {
        const tags = Array.isArray(device?.tags) ? device.tags : [];
        if (tagFilter === 'none') {
          return tags.length === 0;
        }
        return tags.some(t => (t?.id || '').toString() === tagFilter);
      });
    }

    if (!query) {
      return base;
    }
    return base.filter(device => {
      const haystack = [
        device.displayName,
        device.name,
        device.manufacturer,
        device.model,
        device.protocol,
        device.type,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();
      return haystack.includes(query);
    });
  }, [filteredByProtocol, roomFilter, searchTerm, tagFilter]);

  const flatDevices = useMemo(() => {
    return [...filteredDevices].sort((a, b) => {
      const aName = (a?.displayName || a?.name || '').trim();
      const bName = (b?.displayName || b?.name || '').trim();
      return aName.localeCompare(bName, undefined, { sensitivity: 'base' });
    });
  }, [filteredDevices]);

  const activeFilterChip = useMemo(() => {
    if (protocolFilter === 'all') {
      return { key: 'all', label: 'All' };
    }
    return filterChips.find(chip => chip.key === protocolFilter) || null;
  }, [filterChips, protocolFilter]);

  const filterSummary = useMemo(() => {
    const clauses = [];
    if (protocolFilter !== 'all') {
      clauses.push(`protocol ${activeFilterChip?.label || protocolFilter}`);
    }
    if (roomFilter !== 'all') {
      const room = (Array.isArray(rooms) ? rooms : []).find(r => (r?.id || '').toString() === roomFilter);
      clauses.push(roomFilter === 'none' ? 'no room' : `room ${(room?.name || '').trim() || roomFilter}`);
    }
    if (tagFilter !== 'all') {
      const tag = (Array.isArray(tags) ? tags : []).find(t => (t?.id || '').toString() === tagFilter);
      clauses.push(tagFilter === 'none' ? 'no tags' : `tag ${(tag?.name || '').trim() || tagFilter}`);
    }
    const trimmed = searchTerm.trim();
    if (trimmed) {
      clauses.push(`matching "${trimmed}"`);
    }
    if (!clauses.length) {
      return null;
    }
    return `Showing ${filteredDevices.length} ${filteredDevices.length === 1 ? 'device' : 'devices'} for ${clauses.join(' and ')}.`;
  }, [activeFilterChip, filteredDevices.length, protocolFilter, roomFilter, rooms, searchTerm, tagFilter, tags]);

  if (!isResidentOrAdmin) {
    if (bootstrapping) {
      return <LoadingView title="Devices" message="Loading devices…" />;
    }
    return (
      <UnauthorizedView
        title="Devices"
        message="You do not have permission to view this page."
      />
    );
  }

  return (
    <div className="devices-page">
      <PageHeader title="Devices" subtitle={subtitleText}>
        {connectionPills.length > 0 && (
          <div className="devices-header-pills">
            {connectionPills.map(pill => (
              <GlassPill
                key={pill.key || pill.text}
                icon={pill.icon}
                tone={pill.tone}
                text={pill.text}
                title={pill.title}
                onClick={pill.onClick}
              />
            ))}
          </div>
        )}
      </PageHeader>

      <section className="devices-summary">
        <GlassCard className="devices-summary-card">
          <div className="devices-summary-content">
            <GlassMetric icon={faGaugeHigh} label="Total devices" value={stats.total} />
            <GlassMetric icon={faBolt} label="Online" value={stats.online} />
            <GlassMetric icon={faSignal} label="With state" value={stats.withState} />
            <GlassMetric icon={faSatelliteDish} label="Sensors" value={stats.sensors} />
          </div>
          <div className="devices-integrations-row">
            <span className="devices-integrations-label">Filter by protocol</span>
            <div className="devices-integrations-list devices-filter-chip-list">
              {filterOptions.map(chip => {
                const className = [
                  'devices-integrations-chip',
                  'filter-chip',
                  `status-${chip.status || 'unknown'}`,
                  protocolFilter === chip.key ? 'active' : '',
                  chip.count ? '' : 'muted',
                ].filter(Boolean).join(' ');
                return (
                  <button
                    key={chip.key}
                    type="button"
                    className={className}
                    onClick={() => setProtocolFilter(chip.key)}
                  >
                    <span className="devices-integrations-dot" />
                    <span>{chip.label}</span>
                    <span className="devices-filter-chip-count">{chip.count ?? 0}</span>
                  </button>
                );
              })}
            </div>
            <div className="devices-integrations-meta">
              {protocolFilter !== 'all' ? (
                <button type="button" className="devices-filter-clear" onClick={() => setProtocolFilter('all')}>
                  Clear filter
                </button>
              ) : null}
              {integrationsLoading ? <span>Refreshing…</span> : null}
              {integrationsErrorDisplay ? (
                <span className="devices-integrations-error">{integrationsErrorDisplay}</span>
              ) : null}
              <button
                type="button"
                className="devices-integrations-refresh"
                onClick={loadIntegrations}
                disabled={integrationsLoading}
              >
                Reload
              </button>
            </div>
          </div>
        </GlassCard>
      </section>

      {(realtimeError || ersError || commandError) && (
        <GlassCard className="devices-error-card" interactive={false}>
          <div className="devices-error-text">{commandError || ersError || realtimeError}</div>
        </GlassCard>
      )}

      <div className="devices-toolbar">
        <div className="devices-search">
          <FontAwesomeIcon icon={faMagnifyingGlass} className="devices-search-icon" />
          <input
            type="search"
            placeholder="Search by name, model, protocol…"
            value={searchTerm}
            onChange={event => setSearchTerm(event.target.value)}
          />
          {searchTerm ? (
            <button type="button" className="devices-search-clear" onClick={() => setSearchTerm('')}>
              Clear
            </button>
          ) : null}
        </div>
        <div className="devices-toolbar-meta">
          <div className="devices-toolbar-filters">
            <label className="devices-toolbar-filter">
              <span>Room</span>
              <select value={roomFilter} onChange={e => setRoomFilter(e.target.value)}>
                <option value="all">All</option>
                <option value="none">None</option>
                {(Array.isArray(rooms) ? rooms : []).map(r => (
                  <option key={r.id} value={r.id}>{r.name}</option>
                ))}
              </select>
            </label>
            <label className="devices-toolbar-filter">
              <span>Tag</span>
              <select value={tagFilter} onChange={e => setTagFilter(e.target.value)}>
                <option value="all">All</option>
                <option value="none">None</option>
                {(Array.isArray(tags) ? tags : []).map(t => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
            </label>
            <button
              type="button"
              className={[
                'devices-group-toggle',
                groupByRoom ? 'active' : '',
              ].filter(Boolean).join(' ')}
              onClick={() => setGroupByRoom(prev => !prev)}
              title={groupByRoom ? 'Click to show a flat list' : 'Click to group devices by room'}
            >
              <FontAwesomeIcon icon={faLayerGroup} />
              <span>Group by room</span>
            </button>
          </div>
          <span>{filteredDevices.length} visible</span>
        </div>
      </div>

      {(ersLoading || realtimeLoading) && !devices.length ? (
        <GlassCard className="devices-loading-card" interactive={false}>
          <div className="devices-loading">
            <div className="devices-spinner" />
            <span>Loading devices…</span>
          </div>
        </GlassCard>
      ) : null}

      {!ersLoading && !realtimeLoading && devices.length === 0 ? (
        <GlassCard className="devices-empty-card" interactive={false}>
          <div className="devices-empty">No devices discovered yet. Start pairing to see devices show up here.</div>
        </GlassCard>
      ) : null}

      {filterSummary ? (
        <div className="devices-filter-summary">
          {filterSummary}
        </div>
      ) : null}

      <section className="devices-grid">
        {groupByRoom
          ? groupedDevices
            .filter(group => group.devices.some(dev => filteredDevices.includes(dev)))
            .map(group => {
              const groupFiltered = group.devices.filter(dev => filteredDevices.includes(dev));
              if (groupFiltered.length === 0) return null;
              return (
                <React.Fragment key={`group-${group.key}`}>
                  <div className="devices-group-header">
                    <span className="devices-group-title">{group.title}</span>
                    <span className="devices-group-count">{groupFiltered.length}</span>
                  </div>
                  {groupFiltered.map(device => (
                    <DeviceTile
                      key={`${device.ersId || 'ers'}-${device.id || device.key || device.externalId || device.name}`}
                      device={device}
                      pending={Boolean(device.id && pendingCommands[device.id])}
                      onCommand={handleCommand}
                      onRename={handleRename}
                      onUpdateIcon={handleUpdateIcon}
                      onDelete={handleDeleteDevice}
                      onOpen={(dev) => {
                        const id = dev?.hdpId || dev?.id;
                        if (!id) return;
                        navigate(`/devices/${encodeURIComponent(id)}`);
                      }}
                    />
                  ))}
                </React.Fragment>
              );
            })
          : flatDevices.map(device => (
            <DeviceTile
              key={`${device.ersId || 'ers'}-${device.id || device.key || device.externalId || device.name}`}
              device={device}
              pending={Boolean(device.id && pendingCommands[device.id])}
              onCommand={handleCommand}
              onRename={handleRename}
              onUpdateIcon={handleUpdateIcon}
              onDelete={handleDeleteDevice}
              onOpen={(dev) => {
                const id = dev?.hdpId || dev?.id;
                if (!id) return;
                navigate(`/devices/${encodeURIComponent(id)}`);
              }}
            />
          ))}
        {!ersLoading && !realtimeLoading && filteredDevices.length === 0 && devicesWithOverrides.length > 0 ? (
          <GlassCard className="device-filter-empty-card" interactive={false}>
            <div className="devices-filter-empty-text">
              No devices found for the current filters{searchTerm.trim() ? ` and "${searchTerm.trim()}" search` : ''}.
            </div>
          </GlassCard>
        ) : null}
        <GlassCard className="device-add-card">
          <button type="button" className="device-add-card-btn" onClick={openAddModal}>
            <span className="device-add-card-icon">
              <FontAwesomeIcon icon={faPlus} />
            </span>
            <span className="device-add-card-title">Add device</span>
            <span className="device-add-card-subtitle">Register hardware manually or start the pairing flow.</span>
            <span className="device-add-card-cta">Open form</span>
          </button>
        </GlassCard>
      </section>

      <button type="button" className="devices-fab" onClick={openAddModal}>
        <span className="devices-fab-icon">+</span>
        <span>Add device</span>
      </button>

      <AddDeviceModal
        open={showAddModal}
        onClose={() => setShowAddModal(false)}
        onCreate={async payload => {
          await handleCreateDevice(payload);
          setShowAddModal(false);
        }}
        integrations={integrations}
        pairingSessions={pairingSessions}
        pairingConfig={pairingConfig}
        onStartPairing={handleStartPairing}
        onStopPairing={handleStopPairing}
      />
    </div>
  );
}
