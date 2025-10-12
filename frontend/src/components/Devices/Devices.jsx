import React, { useMemo, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faBolt, faGaugeHigh, faSatelliteDish, faSignal } from '@fortawesome/free-solid-svg-icons';
import GlassCard from '../common/GlassCard/GlassCard';
import GlassMetric from '../common/GlassMetric/GlassMetric';
import GlassPill from '../common/GlassPill/GlassPill';
import useDeviceHubDevices from '../../hooks/useDeviceHubDevices';
import DeviceTile from './DeviceTile';
import { useAuth } from '../../context/AuthContext';
import { renameDevice as renameDeviceApi, sendDeviceCommand as sendDeviceCommandApi } from '../../services/deviceHubService';
import './Devices.css';

export default function Devices() {
  const { user, accessToken } = useAuth();
  const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');
  const {
    devices,
    stats,
    loading,
    error,
    connectionInfo,
    renameDevice: renameDeviceWs,
  } = useDeviceHubDevices({ enabled: isResidentOrAdmin });
  const [pendingCommands, setPendingCommands] = useState({});
  const [commandError, setCommandError] = useState(null);

  const connectionPills = useMemo(() => {
    const pills = [];
    if (connectionInfo?.metadata) {
      const tone = connectionInfo.metadata.connected ? 'success' : 'warning';
      const label = connectionInfo.metadata.connected
        ? `Metadata: ${connectionInfo.metadata.source || 'connected'}`
        : 'Metadata stream offline';
      pills.push({ text: label, tone, icon: faSatelliteDish });
    }
    if (connectionInfo?.state) {
      const tone = connectionInfo.state.connected ? 'success' : 'warning';
      const label = connectionInfo.state.connected ? 'State: connected' : 'State stream offline';
      pills.push({ text: label, tone, icon: faSignal });
    }
    return pills;
  }, [connectionInfo]);

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
              <GlassPill key={pill.text} icon={pill.icon} tone={pill.tone} text={pill.text} />
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
        </GlassCard>
      </section>

      {(error || commandError) && (
        <GlassCard className="devices-error-card" interactive={false}>
          <div className="devices-error-text">{commandError || error}</div>
        </GlassCard>
      )}

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

      <section className="devices-grid">
        {devices.map(device => (
          <DeviceTile
            key={device.key || device.externalId}
            device={device}
            pending={Boolean(device.id && pendingCommands[device.id])}
            onCommand={handleCommand}
            onRename={handleRename}
          />
        ))}
      </section>
    </div>
  );
}
