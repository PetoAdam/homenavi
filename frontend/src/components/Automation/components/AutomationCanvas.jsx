import React from 'react';
import Button from '../../common/Button/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faPlay,
  faUpRightAndDownLeftFromCenter,
} from '@fortawesome/free-solid-svg-icons';

export default function AutomationCanvas({
  canvasRef,
  onCanvasDragOver,
  onCanvasDrop,
  onCanvasPointerDown,
  GRID_SIZE,
  viewport,
  setViewport,
  svgWorldSize,
  edgesToRender,
  connectMode,
  connectHoverId,
  setConnectHoverId,
  connectModeRef,
  setConnectMode,
  cancelConnect,
  deleteEdge,
  editorNodes,
  selectedNodeId,
  setSelectedNodeId,
  NODE_WIDTH,
  NODE_HEADER_HEIGHT,
  isTriggerNode,
  nodeTitle,
  nodeSubtitle,
  nodeBodyText,
  iconForNodeKind,
  deviceNameById,
  liveRunNodeStates,
  commitConnection,
  startConnectFromNode,
  onNodePointerDown,
  executeFromNodeTitle,
  canExecuteFromNode,
  runNow,
  canvasSize,
  zoomAroundPoint,
  workflowName,
  onWorkflowNameChange,
}) {
  return (
    <div className="automation-center">
      <div
        className="automation-canvas"
        ref={canvasRef}
        onDragOver={onCanvasDragOver}
        onDrop={onCanvasDrop}
        onPointerDown={onCanvasPointerDown}
        style={{
          backgroundSize: `${GRID_SIZE * viewport.scale}px ${GRID_SIZE * viewport.scale}px`,
          backgroundPosition: `${viewport.x}px ${viewport.y}px`,
        }}
        role="application"
        aria-label="Workflow canvas"
      >
        <div className="automation-canvas-name" onPointerDown={(e) => e.stopPropagation()}>
          <input
            className="input automation-canvas-name-input"
            value={workflowName}
            onChange={(e) => onWorkflowNameChange(e.target.value)}
            placeholder="Untitled workflow"
            aria-label="Workflow name"
          />
        </div>

        <div
          className="automation-canvas-layer"
          style={{
            transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.scale})`,
            transformOrigin: '0 0',
          }}
        >
          <svg
            className="automation-edges"
            aria-hidden="true"
            viewBox={`${svgWorldSize.x} ${svgWorldSize.y} ${svgWorldSize.w} ${svgWorldSize.h}`}
            preserveAspectRatio="xMinYMin meet"
            style={{
              width: `${svgWorldSize.w}px`,
              height: `${svgWorldSize.h}px`,
              left: `${svgWorldSize.x}px`,
              top: `${svgWorldSize.y}px`,
            }}
          >
            {connectMode && (() => {
              const dx = connectMode.x2 - connectMode.x1;
              const dir = dx >= 0 ? 1 : -1;
              const handle = Math.min(120, Math.abs(dx) / 2);
              const c1x = connectMode.x1 + dir * handle;
              const c2x = connectMode.x2 - dir * handle;
              return (
                <path
                  className="automation-edge automation-edge-preview"
                  d={`M ${connectMode.x1} ${connectMode.y1} C ${c1x} ${connectMode.y1}, ${c2x} ${connectMode.y2}, ${connectMode.x2} ${connectMode.y2}`}
                />
              );
            })()}
            {edgesToRender.map(ed => (
              <g key={ed.key}>
                <path
                  className="automation-edge-hit"
                  d={`M ${ed.x1} ${ed.y1} C ${ed.c1x} ${ed.y1}, ${ed.c2x} ${ed.y2}, ${ed.x2} ${ed.y2}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    deleteEdge(ed.from, ed.to);
                  }}
                />
                <path
                  className="automation-edge"
                  d={`M ${ed.x1} ${ed.y1} C ${ed.c1x} ${ed.y1}, ${ed.c2x} ${ed.y2}, ${ed.x2} ${ed.y2}`}
                />
              </g>
            ))}
          </svg>

          {(!Array.isArray(editorNodes) || editorNodes.length === 0) && (
            <div className="automation-canvas-empty muted">
              Drag nodes from the palette to the canvas.
            </div>
          )}

          {(Array.isArray(editorNodes) ? editorNodes : []).map((node) => {
            if (!node) return null;
            const id = String(node.id);
            const isTrigger = isTriggerNode(node);
            const title = nodeTitle(node.kind);
            const kind = String(node.kind || '').toLowerCase();
            const nodeIcon = iconForNodeKind(node.kind);

            let subtitle = nodeSubtitle(node);
            let bodyText = nodeBodyText(node);

            if (kind === 'trigger.device_state' || kind === 'action.send_command') {
              const deviceId = String(node?.data?.device_id || '').trim();
              const deviceName = deviceId ? (deviceNameById.get(deviceId) || deviceId) : '';
              if (kind === 'trigger.device_state') {
                subtitle = deviceName ? `Device: ${deviceName}` : 'Device state';
                const key = String(node?.data?.key || '').trim();
                const op = String(node?.data?.op || 'exists').trim() || 'exists';
                bodyText = `${deviceName ? `device: ${deviceName}` : 'device: —'}${key ? ` • ${key} ${op}` : ''}`;
              }
              if (kind === 'action.send_command') {
                const cmd = String(node?.data?.command || '').trim() || 'set_state';
                subtitle = `${deviceName ? `Device: ${deviceName}` : 'Device'}${cmd ? ` • ${cmd}` : ''}`;
                bodyText = `${deviceName ? `device: ${deviceName}` : 'device: —'} • cmd: ${cmd}`;
              }
            }

            const connectEligible = !!connectMode && String(connectMode.fromId) !== id && !isTrigger;
            const connectHover = !!connectHoverId && String(connectHoverId) === id;

            const liveState = liveRunNodeStates[id];
            const liveClass = liveState === 'active'
              ? ' run-live-active'
              : liveState === 'done'
                ? ' run-live-done'
                : liveState === 'failed'
                  ? ' run-live-failed'
                  : '';

            return (
              <div
                key={id}
                className={`automation-node${selectedNodeId === id ? ' selected' : ''}${liveClass}${connectEligible ? ' connect-eligible' : ''}${connectHover ? ' connect-hover' : ''}`}
                style={{ left: node.x, top: node.y, width: NODE_WIDTH }}
                onClick={(e) => {
                  e.stopPropagation();
                  if (connectMode && connectMode.mode === 'click') {
                    const fromId = String(connectMode.fromId || '');
                    if (fromId && fromId !== id && !isTrigger) {
                      commitConnection(fromId, id);
                      cancelConnect();
                      return;
                    }
                  }
                  setSelectedNodeId(id);
                }}
                onPointerEnter={() => {
                  if (!connectMode) return;
                  if (connectMode.mode !== 'click') return;
                  const fromId = String(connectMode.fromId || '');
                  if (!fromId || fromId === id || isTrigger) return;
                  setConnectHoverId(id);
                  const next = { ...connectMode, x2: node.x, y2: node.y + NODE_HEADER_HEIGHT / 2 };
                  connectModeRef.current = next;
                  setConnectMode(next);
                }}
                onPointerLeave={() => {
                  if (!connectMode) return;
                  if (connectMode.mode !== 'click') return;
                  setConnectHoverId(null);
                  const next = { ...connectMode, x2: connectMode.x1, y2: connectMode.y1 };
                  connectModeRef.current = next;
                  setConnectMode(next);
                }}
              >
                {!isTrigger && (
                  <button
                    type="button"
                    className="automation-node-port in"
                    data-connect-target={connectMode && String(connectHoverId || '') === id ? 'true' : undefined}
                    title="Input"
                    onPointerDown={(e) => { e.stopPropagation(); }}
                    onClick={(e) => {
                      e.stopPropagation();
                      if (connectMode && connectMode.mode === 'click') {
                        const fromId = String(connectMode.fromId || '');
                        if (fromId && fromId !== id) {
                          commitConnection(fromId, id);
                          cancelConnect();
                          return;
                        }
                      }
                      setSelectedNodeId(id);
                    }}
                  />
                )}
                <button
                  type="button"
                  className="automation-node-port out"
                  title="Connect"
                  onPointerDown={(e) => startConnectFromNode(e, id)}
                />

                <div
                  className="automation-node-header"
                  onPointerDown={(e) => onNodePointerDown(e, id)}
                >
                  <div className="title">
                    {nodeIcon && (
                      <span className="node-kind-icon" aria-hidden="true">
                        <FontAwesomeIcon icon={nodeIcon} />
                      </span>
                    )}
                    <span>{title}</span>
                  </div>
                  <div className="automation-node-header-right">
                    {subtitle && <div className="muted">{subtitle}</div>}
                    {isTrigger && (
                      <button
                        type="button"
                        className="automation-node-icon-btn"
                        title={executeFromNodeTitle}
                        disabled={!canExecuteFromNode}
                        onClick={(e) => {
                          e.stopPropagation();
                          runNow();
                        }}
                        onPointerDown={(e) => { e.stopPropagation(); }}
                      >
                        <FontAwesomeIcon icon={faPlay} />
                      </button>
                    )}
                  </div>
                </div>
                <div className="automation-node-body muted">{bodyText}</div>
              </div>
            );
          })}
        </div>

        <div className="automation-canvas-controls" onPointerDown={(e) => e.stopPropagation()}>
          <div className="actions compact">
            <Button
              variant="secondary"
              type="button"
              onClick={() => {
                const pt = { x: canvasSize.width / 2, y: canvasSize.height / 2 };
                setViewport(prev => zoomAroundPoint(prev, pt, prev.scale * 1.12));
              }}
              title="Zoom in"
            >
              Zoom +
            </Button>
            <Button
              variant="secondary"
              type="button"
              onClick={() => {
                const pt = { x: canvasSize.width / 2, y: canvasSize.height / 2 };
                setViewport(prev => zoomAroundPoint(prev, pt, prev.scale / 1.12));
              }}
              title="Zoom out"
            >
              Zoom -
            </Button>
            <Button
              variant="secondary"
              type="button"
              onClick={() => setViewport({ x: 24, y: 24, scale: 1 })}
              title="Reset canvas view"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faUpRightAndDownLeftFromCenter} /></span>
              View
            </Button>
          </div>
        </div>

        {connectMode && (
          <div className="automation-connect-hint muted" onPointerDown={(e) => e.stopPropagation()}>
            {connectMode.mode === 'drag'
              ? 'Connecting: drag to a node’s left dot (Esc cancels)'
              : 'Connecting: tap a node to connect (tap empty canvas cancels)'}
            <button
              type="button"
              className="link-btn"
              onClick={(e) => { e.stopPropagation(); cancelConnect(); }}
              style={{ marginLeft: 10 }}
            >
              Cancel
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
