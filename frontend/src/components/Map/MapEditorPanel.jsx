import React from 'react';
import GlassPill from '../common/GlassPill/GlassPill';
import Button from '../common/Button/Button';
import HoverDescription from '../common/HoverDescription/HoverDescription';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faArrowsUpDownLeftRight,
  faBullseye,
  faGripLines,
  faMinus,
  faPlus,
  faTableCells,
  faUpRightAndDownLeftFromCenter,
} from '@fortawesome/free-solid-svg-icons';

export default function MapEditorPanel({
  editEnabled,
  mode,
  mapError,

  activeRoom,
  roomNameEdit,
  setRoomNameEdit,
  updateRoomName,

  opPending,
  deleteRoom,

  activeVertexIndex,
  deleteCornerOnRoom,

  cancelDraft,
  startRoom,
  draft,
  setDraft,

  snapSettings,
  setSnapSettings,

  activeWallDisplay,
  setDraftWallLength,
  setWallLength,
  persistRoomGeometry,

  selectedDeviceId,

  normalizeNumber,
}) {
  return (
    <div
      className="automation-canvas-name map-panel"
      onPointerDown={(e) => e.stopPropagation()}
      onPointerUp={(e) => e.stopPropagation()}
      onClick={(e) => e.stopPropagation()}
    >
      <div className="map-panel-header">
        <div className="muted" style={{ fontWeight: 800 }}>Map</div>
        <div className="muted" style={{ opacity: 0.8 }}>{editEnabled ? 'Edit mode' : 'View mode'}</div>
      </div>

      {mapError ? <div className="muted" style={{ paddingTop: 6 }}>{mapError}</div> : null}

      {editEnabled ? (
        <div className="map-panel-sections">
          <div className="map-panel-section">
            <div className="muted" style={{ fontWeight: 700 }}>Rooms</div>
            {activeRoom && mode !== 'draw' ? (
              <>
                <div className="map-room-row">
                  <input
                    type="text"
                    className="input automation-canvas-name-input map-room-input"
                    value={roomNameEdit}
                    onChange={(e) => setRoomNameEdit(e.target.value)}
                    onClick={(e) => e.stopPropagation()}
                    onPointerDown={(e) => e.stopPropagation()}
                    onBlur={() => {
                      if (!activeRoom?.id) return;
                      void updateRoomName(activeRoom.id, roomNameEdit);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') e.currentTarget.blur();
                    }}
                    placeholder="Room name"
                  />
                  <GlassPill
                    text={opPending ? 'Working…' : 'Delete'}
                    tone="danger"
                    onClick={() => {
                      if (opPending) return;
                      deleteRoom(activeRoom.id);
                    }}
                    title="Delete this room"
                  />
                </div>
                <div className="muted" style={{ marginTop: 4 }}>
                  Tip: click a wall segment to edit its length.
                </div>

                {Number.isFinite(activeVertexIndex) ? (
                  <div className="map-room-row" style={{ marginTop: 8 }}>
                    <GlassPill
                      text="Delete corner"
                      tone="danger"
                      onClick={() => deleteCornerOnRoom(activeRoom.id, activeVertexIndex)}
                      title="Delete selected corner (Delete/Backspace)"
                    />
                    <span className="muted">corner #{activeVertexIndex + 1}</span>
                  </div>
                ) : null}
              </>
            ) : (
              <div className="muted">Select a room on the map to edit it.</div>
            )}
          </div>

          <div className="map-panel-section">
            <div className="muted" style={{ fontWeight: 700 }}>Drawing</div>
            <div className="actions compact" style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              <Button
                variant={mode === 'draw' ? 'secondary' : 'primary'}
                type="button"
                className="map-draw-room-btn"
                onClick={mode === 'draw' ? cancelDraft : startRoom}
                title={mode === 'draw' ? 'Cancel drawing' : 'Start drawing a room'}
              >
                <span className="btn-icon"><FontAwesomeIcon icon={mode === 'draw' ? faMinus : faPlus} /></span>
                <span className="btn-label">{mode === 'draw' ? 'Cancel' : 'Draw room'}</span>
              </Button>
            </div>

            {mode === 'draw' && draft ? (
              <>
                <input
                  type="text"
                  className="input automation-canvas-name-input"
                  value={String(draft.name || '')}
                  onChange={(e) => setDraft(prev => (prev ? { ...prev, name: e.target.value } : prev))}
                  placeholder="New room name"
                  style={{ marginTop: 8 }}
                />
                <div className="muted" style={{ marginTop: 6 }}>
                  Click to add corners. Click the first point to close. Right click to undo.
                </div>
              </>
            ) : null}
          </div>

          <div className="map-panel-section">
            <div className="muted" style={{ fontWeight: 700 }}>Snapping</div>
            <div className="automation-chip-row" style={{ gap: 8 }}>
              <HoverDescription title="Vertices" description="Snap to existing corners.">
                <button
                  type="button"
                  className={`automation-chip automation-chip-btn${snapSettings.vertex ? ' active' : ''}`}
                  onClick={() => setSnapSettings(prev => ({ ...prev, vertex: !prev.vertex }))}
                >
                  <span className="map-snap-chip">
                    <FontAwesomeIcon icon={faBullseye} />
                    <span>Vertices</span>
                  </span>
                </button>
              </HoverDescription>

              <HoverDescription title="Edges" description="Snap to existing walls.">
                <button
                  type="button"
                  className={`automation-chip automation-chip-btn${snapSettings.edge ? ' active' : ''}`}
                  onClick={() => setSnapSettings(prev => ({ ...prev, edge: !prev.edge }))}
                >
                  <span className="map-snap-chip">
                    <FontAwesomeIcon icon={faGripLines} />
                    <span>Edges</span>
                  </span>
                </button>
              </HoverDescription>

              <HoverDescription title="Align" description="Align to nearby corners on X/Y. Helps keep walls parallel.">
                <button
                  type="button"
                  className={`automation-chip automation-chip-btn${snapSettings.align ? ' active' : ''}`}
                  onClick={() => setSnapSettings(prev => ({ ...prev, align: !prev.align }))}
                >
                  <span className="map-snap-chip">
                    <FontAwesomeIcon icon={faUpRightAndDownLeftFromCenter} />
                    <span>Align</span>
                  </span>
                </button>
              </HoverDescription>

              <HoverDescription title="Ortho" description="Snap to near-horizontal/vertical walls (~5°).">
                <button
                  type="button"
                  className={`automation-chip automation-chip-btn${snapSettings.ortho ? ' active' : ''}`}
                  onClick={() => setSnapSettings(prev => ({ ...prev, ortho: !prev.ortho }))}
                >
                  <span className="map-snap-chip">
                    <FontAwesomeIcon icon={faArrowsUpDownLeftRight} />
                    <span>Ortho</span>
                  </span>
                </button>
              </HoverDescription>

              <HoverDescription title="Grid" description="Snap to grid intersections. Useful for consistent spacing.">
                <button
                  type="button"
                  className={`automation-chip automation-chip-btn${snapSettings.grid ? ' active' : ''}`}
                  onClick={() => setSnapSettings(prev => ({ ...prev, grid: !prev.grid }))}
                >
                  <span className="map-snap-chip">
                    <FontAwesomeIcon icon={faTableCells} />
                    <span>Grid</span>
                  </span>
                </button>
              </HoverDescription>
            </div>
          </div>

          {activeWallDisplay && activeRoom ? (
            <div className="map-panel-section">
              <div className="muted" style={{ fontWeight: 700 }}>Wall length</div>
              <div className="map-wall-row">
                <input
                  type="number"
                  className="input automation-canvas-name-input"
                  value={Number.isFinite(activeWallDisplay.value) ? String(Math.round(activeWallDisplay.value * 100) / 100) : ''}
                  onChange={(e) => {
                    const val = normalizeNumber(e.target.value);
                    if (mode === 'draw') {
                      setDraftWallLength(activeWallDisplay.wallIndex, val);
                    } else {
                      setWallLength(activeRoom.id, activeWallDisplay.wallIndex, val);
                    }
                  }}
                  onBlur={() => {
                    if (mode === 'draw') return;
                    if (!activeRoom?.id) return;
                    void persistRoomGeometry(activeRoom.id, activeRoom.points, activeRoom.wallLengths);
                  }}
                  placeholder="Length"
                  title="Set wall length (your units)"
                />
                <span className="muted">wall #{activeWallDisplay.wallIndex + 1}</span>
              </div>
            </div>
          ) : null}

          {selectedDeviceId ? (
            <div className="automation-connect-hint" style={{ position: 'relative', left: 0, bottom: 0, marginTop: 6 }}>
              Tap a room to place the selected device.
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
