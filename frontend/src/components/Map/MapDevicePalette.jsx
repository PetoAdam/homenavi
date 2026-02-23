import React from 'react';

export default function MapDevicePalette({
  ersError,
  busy,
  devices,
  selectedDeviceId,
  setSelectedDeviceId,
}) {
  return (
    <div className="map-sidebar map-left">
      <div className="map-sidebar-section">
        <div className="map-panel-title">Devices</div>
        <div className="map-palette">
          {ersError ? <div className="muted" style={{ padding: '8px 0' }}>{ersError}</div> : null}
          {busy ? <div className="muted" style={{ padding: '8px 0' }}>Loading devices…</div> : null}
          {devices.length === 0 && !busy ? (
            <div className="muted" style={{ padding: '8px 0' }}>No devices yet.</div>
          ) : null}
          <div className="map-palette-group">
            <div className="map-palette-grid map-device-grid">
              {devices.map(d => {
                const id = String(d?.ersId || d?.id || d?.hdpId || '');
                const label = String(d?.displayName || d?.name || id);
                const selected = selectedDeviceId && selectedDeviceId === id;
                return (
                  <div
                    key={id}
                    className={`map-palette-item${selected ? ' active' : ''}`}
                    draggable
                    onDragStart={(e) => {
                      e.dataTransfer.setData('text/homenavi-device-key', id);
                      e.dataTransfer.effectAllowed = 'move';
                    }}
                    onClick={() => {
                      // On mobile, use click-to-place.
                      setSelectedDeviceId(prev => (prev === id ? '' : id));
                    }}
                    title="Drag to a room (desktop) or tap then tap a room (mobile)"
                  >
                    <div className="label">
                      <span>{label}</span>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
