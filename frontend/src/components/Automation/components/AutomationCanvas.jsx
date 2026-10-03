import React from 'react';
import Button from '../../common/Button/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faPlay,
  faCrosshairs,
  faMagnifyingGlassMinus,
  faMagnifyingGlassPlus,
} from '@fortawesome/free-solid-svg-icons';
import '../../common/CanvasPrimitives/CanvasPrimitives.css';

function displaySelector(selector) {
  const value = String(selector || '').trim();
  if (!value) return 'selected devices';
  const separator = value.indexOf(':');
  if (separator < 1) return value;
  const type = value.slice(0, separator).trim().toLowerCase();
  const name = value.slice(separator + 1).trim();
  if (!name) return value;
  if (type === 'group') return `the ${name} group`;
  if (type === 'tag') return `devices tagged ${name}`;
  return `devices matching ${value}`;
}

function describeTarget(targets, deviceNameById) {
  const type = String(targets?.type || 'device').toLowerCase();
  if (type === 'selector') return displaySelector(targets?.selector);
  const ids = Array.isArray(targets?.ids) ? targets.ids.map(id => String(id || '').trim()).filter(Boolean) : [];
  const names = ids.map(id => deviceNameById.get(id) || id);
  if (names.length === 2) return `${names[0]} and ${names[1]}`;
  if (names.length === 1) return names[0];
  if (names.length > 2) return `${names.slice(0, 2).join(', ')}, and ${names.length - 2} more`;
  return 'a selected device';
}

function displayValue(value) {
  if (value === undefined || value === null || value === '') return 'value';
  if (typeof value === 'boolean') return value ? 'on' : 'off';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function describeCommandUpdates(data) {
  const commands = Array.isArray(data?.capability_commands) ? data.capability_commands : [];
  const updates = commands.length > 0
    ? commands
    : [{ id: data?.capability_id, property: data?.capability_property, value: data?.capability_value }];
  return updates
    .map(command => {
      const property = String(command?.property || command?.id || '').trim();
      return property ? `${property.replace(/[_-]+/g, ' ')} to ${displayValue(command?.value)}` : '';
    })
    .filter(Boolean)
    .join(' and ');
}

function conditionValue(rawValue) {
  if (typeof rawValue !== 'string') return rawValue;
  try {
    return JSON.parse(rawValue);
  } catch {
    return rawValue;
  }
}

function describeCondition(condition) {
  const key = String(condition?.key || '').trim();
  if (!key) return '';
  const op = String(condition?.op || 'exists').trim() || 'exists';
  const phrase = { eq: 'is', neq: 'is not', gt: 'is above', gte: 'is at least', lt: 'is below', lte: 'is at most', exists: 'has a value', changed: 'changes' }[op] || op;
  const needsValue = !['exists', 'changed'].includes(op);
  return `${key.replace(/[_-]+/g, ' ')} ${phrase}${needsValue ? ` ${displayValue(condition?.value)}` : ''}`;
}

function storedConditionValue(condition) {
  if (String(condition?.value_mode || 'json') !== 'builder') return conditionValue(condition?.value_text);
  const type = String(condition?.value_type || 'boolean').toLowerCase();
  if (type === 'boolean') return Boolean(condition?.value_bool);
  if (type === 'number') return condition?.value_number;
  return condition?.value_string;
}

function describeTriggerConditions(data) {
  const persisted = Array.isArray(data?.conditions) ? data.conditions : [];
  if (persisted.length > 0) return persisted.map(describeCondition).filter(Boolean).join(' and ');

  const ui = data?.ui || {};
  const primaryValue = ui.value_type === 'number' ? ui.value_number
    : ui.value_type === 'text' ? ui.value_string
      : ui.value_bool === false ? false : true;
  const primary = { key: data?.key, op: data?.op, value: primaryValue };
  const additional = Array.isArray(data?.additional_conditions) ? data.additional_conditions.map(condition => ({
    key: condition?.key,
    op: condition?.op,
    value: storedConditionValue(condition),
  })) : [];
  return [primary, ...additional].map(describeCondition).filter(Boolean).join(' and ');
}

export default function AutomationCanvas({
  canvasRef,
  onCanvasDragOver,
  onCanvasDrop,
  onCanvasPointerDown,
  onCanvasPointerMove,
  onCanvasPointerUp,
  onCanvasPointerCancel,
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
  autoFitKey,
  autoFitDataReady = true,
  onAutoFitComplete,
  renderDelayMs = 100,
  readOnly = false,
}) {
  const lastAutoFitKeyRef = React.useRef('');
  const onAutoFitCompleteRef = React.useRef(onAutoFitComplete);

  React.useEffect(() => {
    onAutoFitCompleteRef.current = onAutoFitComplete;
  }, [onAutoFitComplete]);

  const fitWorkflowToView = React.useCallback(() => {
    const canvasEl = canvasRef?.current;
    const canvasRect = canvasEl?.getBoundingClientRect?.();
    const width = Math.max(1, Number(canvasRect?.width) || Number(canvasSize?.width) || 0);
    const height = Math.max(1, Number(canvasRect?.height) || Number(canvasSize?.height) || 0);
    const nodes = Array.isArray(editorNodes) ? editorNodes : [];
    const edges = Array.isArray(edgesToRender) ? edgesToRender : [];

    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;

    if (canvasEl) {
      const domNodes = canvasEl.querySelectorAll('.automation-node');
      domNodes.forEach((nodeEl) => {
        const x = Number(nodeEl?.offsetLeft);
        const y = Number(nodeEl?.offsetTop);
        const w = Number(nodeEl?.offsetWidth);
        const h = Number(nodeEl?.offsetHeight);
        if (!Number.isFinite(x) || !Number.isFinite(y) || !Number.isFinite(w) || !Number.isFinite(h)) return;
        minX = Math.min(minX, x);
        minY = Math.min(minY, y);
        maxX = Math.max(maxX, x + w);
        maxY = Math.max(maxY, y + h);
      });
    }

    nodes.forEach((node) => {
      const x = Number(node?.x);
      const y = Number(node?.y);
      if (!Number.isFinite(x) || !Number.isFinite(y)) return;
      minX = Math.min(minX, x);
      minY = Math.min(minY, y);
      maxX = Math.max(maxX, x + NODE_WIDTH);
      maxY = Math.max(maxY, y + NODE_HEADER_HEIGHT + 78);
    });

    edges.forEach((edge) => {
      const points = [
        [Number(edge?.x1), Number(edge?.y1)],
        [Number(edge?.c1x), Number(edge?.y1)],
        [Number(edge?.c2x), Number(edge?.y2)],
        [Number(edge?.x2), Number(edge?.y2)],
      ];
      points.forEach(([x, y]) => {
        if (!Number.isFinite(x) || !Number.isFinite(y)) return;
        minX = Math.min(minX, x);
        minY = Math.min(minY, y);
        maxX = Math.max(maxX, x);
        maxY = Math.max(maxY, y);
      });
    });

    if (!Number.isFinite(minX) || !Number.isFinite(minY) || !Number.isFinite(maxX) || !Number.isFinite(maxY)) {
      setViewport({ x: 24, y: 24, scale: 1 });
      return true;
    }

    const pad = 64;
    const boundsW = Math.max(1, (maxX - minX) + pad * 2);
    const boundsH = Math.max(1, (maxY - minY) + pad * 2);
    const fitScaleX = Math.max(0.1, width / boundsW);
    const fitScaleY = Math.max(0.1, height / boundsH);
    const scale = Math.max(0.35, Math.min(2.5, Math.min(fitScaleX, fitScaleY)));

    const worldCenterX = (minX + maxX) / 2;
    const worldCenterY = (minY + maxY) / 2;
    setViewport({
      x: (width / 2) - worldCenterX * scale,
      y: (height / 2) - worldCenterY * scale,
      scale,
    });
    return true;
  }, [canvasRef, canvasSize, editorNodes, edgesToRender, NODE_HEADER_HEIGHT, NODE_WIDTH, setViewport]);

  React.useEffect(() => {
    const key = String(autoFitKey || '').trim();
    if (!key) return;
    if (!autoFitDataReady) return;
    if (lastAutoFitKeyRef.current === key) {
      if (typeof onAutoFitCompleteRef.current === 'function') {
        const doneTimer = window.setTimeout(() => {
          const cb = onAutoFitCompleteRef.current;
          if (typeof cb === 'function') cb(key);
        }, Math.max(0, Number(renderDelayMs) || 0));
        return () => window.clearTimeout(doneTimer);
      }
      return;
    }

    let raf1 = 0;
    let raf2 = 0;
    let timeout = 0;

    const tryFit = () => {
      const ok = fitWorkflowToView();
      if (!ok) return false;
      lastAutoFitKeyRef.current = key;
      if (typeof onAutoFitCompleteRef.current === 'function') {
        timeout = window.setTimeout(() => {
          const cb = onAutoFitCompleteRef.current;
          if (typeof cb === 'function') cb(key);
        }, Math.max(0, Number(renderDelayMs) || 0));
      }
      return true;
    };

    raf1 = window.requestAnimationFrame(() => {
      raf2 = window.requestAnimationFrame(() => {
        if (tryFit()) return;
        timeout = window.setTimeout(() => {
          tryFit();
        }, 100);
      });
    });

    return () => {
      if (timeout) window.clearTimeout(timeout);
      if (raf1) window.cancelAnimationFrame(raf1);
      if (raf2) window.cancelAnimationFrame(raf2);
    };
  }, [autoFitDataReady, autoFitKey, fitWorkflowToView, renderDelayMs]);

  return (
    <div className="hn-canvas-center automation-center">
      <div
        className="hn-canvas-surface automation-canvas"
        ref={canvasRef}
        onDragOver={readOnly ? undefined : onCanvasDragOver}
        onDrop={readOnly ? undefined : onCanvasDrop}
        onPointerDown={onCanvasPointerDown}
        onPointerMove={onCanvasPointerMove}
        onPointerUp={onCanvasPointerUp}
        onPointerCancel={onCanvasPointerCancel}
        style={{
          backgroundSize: `${GRID_SIZE * viewport.scale}px ${GRID_SIZE * viewport.scale}px`,
          backgroundPosition: `${viewport.x}px ${viewport.y}px`,
        }}
        role="application"
        aria-label="Workflow canvas"
      >
        <div
          className="hn-canvas-layer automation-canvas-layer"
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
            <div className="hn-canvas-empty automation-canvas-empty muted">
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
              const target = describeTarget(node?.data?.targets, deviceNameById);
              if (kind === 'trigger.device_state') {
                subtitle = 'When device state changes';
                const conditions = describeTriggerConditions(node?.data);
                bodyText = conditions ? `For ${target}, ${conditions}` : `For ${target}, choose a capability`;
              }
              if (kind === 'action.send_command') {
                const cmd = String(node?.data?.command || '').trim() || 'set_state';
                const updates = describeCommandUpdates(node?.data);
                subtitle = 'Set device state';
                bodyText = updates ? `For ${target}, set ${updates}` : (cmd === 'set_state' ? `For ${target}, choose a capability to update` : `For ${target}, run ${cmd}`);
              }
            }

            const connectEligible = !readOnly && !!connectMode && String(connectMode.fromId) !== id && !isTrigger;
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
                  if (readOnly) return;
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
                  if (readOnly) return;
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
                  if (readOnly) return;
                  if (!connectMode) return;
                  if (connectMode.mode !== 'click') return;
                  setConnectHoverId(null);
                  const next = { ...connectMode, x2: connectMode.x1, y2: connectMode.y1 };
                  connectModeRef.current = next;
                  setConnectMode(next);
                }}
              >
                {!readOnly && !isTrigger && (
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
                {!readOnly && (
                  <button
                    type="button"
                    className="automation-node-port out"
                    title="Connect"
                    onPointerDown={(e) => startConnectFromNode(e, id)}
                  />
                )}

                <div
                  className="automation-node-header"
                  onPointerDown={(e) => {
                    if (readOnly) return;
                    onNodePointerDown(e, id);
                  }}
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
                        disabled={!canExecuteFromNode || readOnly}
                        onClick={(e) => {
                          e.stopPropagation();
                          if (readOnly) return;
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

        <div className={`hn-canvas-overlay-controls automation-canvas-controls ${readOnly ? 'read-only' : 'editable'}`} onPointerDown={(e) => e.stopPropagation()}>
          <div className="automation-control-groups" role="toolbar" aria-label="Automation canvas controls">
            <div className="automation-control-group">
            <Button
              variant="secondary"
              type="button"
              className="automation-control-btn"
              onClick={() => {
                const pt = { x: canvasSize.width / 2, y: canvasSize.height / 2 };
                setViewport(prev => zoomAroundPoint(prev, pt, prev.scale * 1.12));
              }}
              title="Zoom in"
              aria-label="Zoom in"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faMagnifyingGlassPlus} /></span>
            </Button>
            <Button
              variant="secondary"
              type="button"
              className="automation-control-btn"
              onClick={fitWorkflowToView}
              title="Fit and center workflow"
              aria-label="Fit and center workflow"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faCrosshairs} /></span>
            </Button>
            <Button
              variant="secondary"
              type="button"
              className="automation-control-btn"
              onClick={() => {
                const pt = { x: canvasSize.width / 2, y: canvasSize.height / 2 };
                setViewport(prev => zoomAroundPoint(prev, pt, prev.scale / 1.12));
              }}
              title="Zoom out"
              aria-label="Zoom out"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faMagnifyingGlassMinus} /></span>
            </Button>
            </div>
          </div>
        </div>

        {!readOnly && connectMode && (
          <div className="hn-canvas-hint automation-connect-hint muted" onPointerDown={(e) => e.stopPropagation()}>
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
