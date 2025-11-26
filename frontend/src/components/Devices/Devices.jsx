import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faBolt, faGaugeHigh, faSatelliteDish, faSignal, faPlus, faMagnifyingGlass } from '@fortawesome/free-solid-svg-icons';
import GlassCard from '../common/GlassCard/GlassCard';
import GlassMetric from '../common/GlassMetric/GlassMetric';
import GlassPill from '../common/GlassPill/GlassPill';
import useDeviceHubDevices from '../../hooks/useDeviceHubDevices';
import DeviceTile from './DeviceTile';
import { useAuth } from '../../context/AuthContext';
import {
  renameDevice as renameDeviceApi,
  sendDeviceCommand as sendDeviceCommandApi,
  createDevice as createDeviceApi,
  listIntegrations as listIntegrationsApi,
  setDeviceIcon as setDeviceIconApi,
  deleteDevice as deleteDeviceApi,
} from '../../services/deviceHubService';
import AddDeviceModal from './AddDeviceModal';
import './Devices.css';

const FALLBACK_INTEGRATIONS = [
  { protocol: 'zigbee', label: 'Zigbee', status: 'active' },
  { protocol: 'matter', label: 'Matter', status: 'planned' },
  { protocol: 'thread', label: 'Thread', status: 'planned' },
  { protocol: 'lan', label: 'LAN Bridge', status: 'active' },
];

export default function Devices() {
  const { user, accessToken } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');
  const [metadataMode, setMetadataMode] = useState('rest');
  const {
    devices,
    stats,
    loading,
    error,
    connectionInfo,
    renameDevice: renameDeviceWs,
  } = useDeviceHubDevices({ enabled: isResidentOrAdmin, metadataMode });
  const [pendingCommands, setPendingCommands] = useState({});
  const [commandError, setCommandError] = useState(null);
  const [integrations, setIntegrations] = useState([]);
  const [integrationsLoading, setIntegrationsLoading] = useState(false);
  const [integrationsError, setIntegrationsError] = useState(null);
  const [showAddModal, setShowAddModal] = useState(false);
  const [iconOverrides, setIconOverrides] = useState({});
  const [protocolFilter, setProtocolFilter] = useState('all');
  const [searchTerm, setSearchTerm] = useState('');
  const openAddModal = useCallback(() => setShowAddModal(true), []);

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
      ? 'Metadata: REST'
      : 'Metadata: WebSocket';
    pills.push({
      key: 'metadata',
      text: metadataLabel,
      tone: metadataStatus.connected ? 'success' : 'warning',
      icon: faSatelliteDish,
      title: metadataMode === 'rest'
        ? 'Click to switch to WebSocket-only metadata'
        : 'Click to switch back to REST bootstrap',
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
    setPendingCommands(prev => ({ ...prev, [device.id]: true }));
    return sendDeviceCommandApi(device.id, payload, accessToken)
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
        setPendingCommands(prev => {
          const clone = { ...prev };
          delete clone[device.id];
          return clone;
        });
      });
  };

  const handleRename = async (device, name) => {
    if (!device?.id) {
      throw new Error('Device not ready for rename');
    }
    const trimmed = typeof name === 'string' ? name.trim() : '';
    let wsError = null;
    if (typeof renameDeviceWs === 'function') {
      try {
        await renameDeviceWs(device.id, trimmed);
        return { status: 'queued', device_id: device.id, name: trimmed };
      } catch (err) {
        wsError = err;
        console.warn('WebSocket rename failed, falling back to HTTP', err);
      }
    }
    if (!accessToken) {
      throw wsError || new Error('Authentication required to rename device');
    }
    const res = await renameDeviceApi(device.id, trimmed, accessToken);
    if (!res.success) {
      const message = res.error || wsError?.message || 'Unable to rename device';
      throw new Error(message);
    }
    return res.data;
  };

  const handleCreateDevice = useCallback(async payload => {
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    const res = await createDeviceApi(payload, accessToken);
    if (!res.success) {
      throw new Error(res.error || 'Failed to create device');
    }
    return res.data;
  }, [accessToken]);

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

  const handleDeleteDevice = useCallback(async (device) => {
    if (!device?.id) {
      throw new Error('Device not ready for deletion');
    }
    if (!accessToken) {
      throw new Error('Authentication required');
    }
    const res = await deleteDeviceApi(device.id, accessToken);
    if (!res.success) {
      throw new Error(res.error || 'Unable to delete device');
    }
    return res.data;
  }, [accessToken]);

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
    if (!query) {
      return filteredByProtocol;
    }
    return filteredByProtocol.filter(device => {
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
  }, [filteredByProtocol, searchTerm]);

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
    const trimmed = searchTerm.trim();
    if (trimmed) {
      clauses.push(`matching "${trimmed}"`);
    }
    if (!clauses.length) {
      return null;
    }
    return `Showing ${filteredDevices.length} ${filteredDevices.length === 1 ? 'device' : 'devices'} for ${clauses.join(' and ')}.`;
  }, [activeFilterChip, filteredDevices.length, protocolFilter, searchTerm]);

  if (!isResidentOrAdmin) {
    return (
      <div className="devices-page">
        <div className="card">
          <div className="card-header">Devices</div>
          <div className="card-body">You do not have permission to view this page.</div>
        </div>
      </div>
    );
  }

  return (
    <div className="devices-page">
      <div className="page-header-flat">
        <h1 className="page-title">Devices</h1>
        <div className="page-subtitle">{subtitleText}</div>
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
      </div>

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

      {(error || commandError) && (
        <GlassCard className="devices-error-card" interactive={false}>
          <div className="devices-error-text">{commandError || error}</div>
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
          <span>{filteredDevices.length} visible</span>
        </div>
      </div>

      {loading && !devices.length ? (
        <GlassCard className="devices-loading-card" interactive={false}>
          <div className="devices-loading">
            <div className="devices-spinner" />
            <span>Connecting to Device Hub…</span>
          </div>
        </GlassCard>
      ) : null}

      {!loading && devices.length === 0 ? (
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
        {filteredDevices.map(device => (
          <DeviceTile
            key={device.id || device.key || device.externalId || device.name}
            device={device}
            pending={Boolean(device.id && pendingCommands[device.id])}
            onCommand={handleCommand}
            onRename={handleRename}
            onUpdateIcon={handleUpdateIcon}
            onDelete={handleDeleteDevice}
          />
        ))}
        {!loading && filteredDevices.length === 0 && devicesWithOverrides.length > 0 ? (
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
      />
    </div>
  );
}
