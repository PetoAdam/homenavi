import React from 'react';
import PageHeader from '../common/PageHeader/PageHeader';
import GlassCard from '../common/GlassCard/GlassCard';
import LoadingView from '../common/LoadingView/LoadingView';
import UnauthorizedView from '../common/UnauthorizedView/UnauthorizedView';
import useMapController from './services/useMapController.jsx';

import '../Automation/Automation.css';
import './Map.css';
import MapEditorTopbar from './MapEditorTopbar';
import MapDevicePalette from './MapDevicePalette';
import MapEditorPanel from './MapEditorPanel';
import MapDevicePopover from './MapDevicePopover';
import MapCanvasControls from './MapCanvasControls';
import MapSvg from './MapSvg';

export default function Map() {
  const c = useMapController();

  if (!c.isResidentOrAdmin) {
    if (c.bootstrapping) {
      return <LoadingView title="Map" message="Loading map…" />;
    }
    return (
      <UnauthorizedView
        title="Map"
        message="You do not have permission to view this page."
      />
    );
  }

  return (
    <div className="automation-page map-page">
      <PageHeader title="Map" subtitle="Draw rooms and place devices" />

      {c.editEnabled ? (
        <MapEditorTopbar
          canUndo={c.canUndo}
          canRedo={c.canRedo}
          undo={c.handleUndo}
          redo={c.handleRedo}
          done={() => c.setEditEnabled(false)}
        />
      ) : null}

      <div className={`automation-layout map-layout${c.editEnabled ? ' edit' : ' view'}`}>
        {c.editEnabled ? (
          <MapDevicePalette
            ersError={c.ersError}
            busy={c.busy}
            devices={c.devicesForPalette}
            selectedDeviceId={c.selectedDeviceId}
            setSelectedDeviceId={c.setSelectedDeviceId}
          />
        ) : null}

        <div className="automation-center map-center">
          <div
            className="automation-canvas map-canvas"
            ref={c.canvasRef}
            onPointerMove={c.handleCanvasPointerMoveCombined}
            onClick={c.handleCanvasClick}
            onPointerDown={c.handlePointerDown}
            onPointerUp={c.handlePointerUp}
            onPointerCancel={c.handlePointerUp}
            onContextMenu={c.handleContextMenu}
            onDrop={c.handleDrop}
            onDragOver={c.preventDefault}
            style={{
              backgroundSize: `${c.gridSize * (c.view.scale || 1)}px ${c.gridSize * (c.view.scale || 1)}px`,
              backgroundPosition: `${c.view.tx}px ${c.view.ty}px`,
            }}
          >
            <MapEditorPanel
              editEnabled={c.editEnabled}
              mode={c.mode}
              mapError={c.mapError}
              activeRoom={c.activeRoom}
              roomNameEdit={c.roomNameEdit}
              setRoomNameEdit={c.setRoomNameEdit}
              updateRoomName={c.updateRoomName}
              opPending={c.opPending}
              deleteRoom={c.deleteRoom}
              activeVertexIndex={c.activeVertexIndex}
              deleteCornerOnRoom={c.deleteCornerOnRoom}
              cancelDraft={c.cancelDraft}
              startRoom={c.startRoom}
              draft={c.draft}
              setDraft={c.setDraft}
              snapSettings={c.snapSettings}
              setSnapSettings={c.setSnapSettings}
              activeWallDisplay={c.activeWallDisplay}
              setDraftWallLength={c.setDraftWallLength}
              setWallLength={c.setWallLength}
              persistRoomGeometry={c.persistRoomGeometry}
              selectedDeviceId={c.selectedDeviceId}
              normalizeNumber={c.normalizeNumber}
            />

            <MapDevicePopover
              expandedDevice={c.expandedDevice}
              editEnabled={c.editEnabled}
              favoritesEditorKey={c.favoritesEditorKey}
              setFavoritesEditorKey={c.setFavoritesEditorKey}
              removeDeviceFromMap={c.removeDeviceFromMap}
              persistDeviceFavoriteFields={c.persistDeviceFavoriteFields}
              setExpandedDeviceKey={c.setExpandedDeviceKey}
              navigate={c.navigate}
              iconForFactLabel={c.iconForFactLabel}
            />

            <MapSvg
              svgRef={c.svgRef}
              view={c.view}
              mode={c.mode}
              snapGuide={c.snapGuide}
              roomPaths={c.roomPaths}
              activeRoomId={c.activeRoomId}
              editEnabled={c.editEnabled}
              draftPath={c.draftPath}
              draft={c.draft}
              finalizeDraft={c.finalizeDraft}
              insertCornerPreview={c.insertCornerPreview}
              beginRoomDrag={c.beginRoomDrag}
              onRoomClick={c.onRoomClick}
              beginRoomVertexDrag={c.beginRoomVertexDrag}
              beginInsertCornerDrag={c.beginInsertCornerDrag}
              activeVertexIndex={c.activeVertexIndex}
              renderPlacedDevices={c.renderPlacedDevices}
            />

            {c.editEnabled && c.rooms.length === 0 && c.mode !== 'draw' ? (
              <div className="automation-canvas-empty muted">
                Click “Draw room” to start.
              </div>
            ) : null}

            <MapCanvasControls
              editEnabled={c.editEnabled}
              setEditEnabled={c.setEditEnabled}
              zoomBy={c.zoomBy}
              resetView={c.resetView}
            />
          </div>
        </div>
      </div>

      {c.editEnabled ? (
        <GlassCard className="map-footer" interactive={false}>
          <div className="muted">
            Desktop: drag a device onto a room. Mobile: tap a device, then tap a room.
          </div>
        </GlassCard>
      ) : null}
    </div>
  );
}
