import React from 'react';

function computeRoomCenter(points) {
  if (!Array.isArray(points) || points.length === 0) return null;
  let sumX = 0;
  let sumY = 0;
  let count = 0;
  points.forEach((p) => {
    const x = Number(p?.x);
    const y = Number(p?.y);
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;
    sumX += x;
    sumY += y;
    count += 1;
  });
  if (count === 0) return null;
  return { x: sumX / count, y: sumY / count };
}

export default function MapSvg({
  svgRef,
  view,
  mode,
  snapGuide,
  roomPaths,
  activeRoomId,
  editEnabled,
  draftPath,
  draft,
  finalizeDraft,
  insertCornerPreview,
  beginRoomDrag,
  onRoomClick,
  beginRoomVertexDrag,
  beginInsertCornerDrag,
  activeVertexIndex,
  roomLabelFontPx,
  renderPlacedDevices,
}) {
  return (
    <div className="hn-canvas-layer map-canvas-layer">
      <svg
        ref={svgRef}
        className="map-svg"
        width="100%"
        height="100%"
        role="img"
        aria-label="House map editor"
      >
        <defs>
          <style>
            {`
              .map-room-fill { fill: rgba(255,255,255,0.06); }
              .map-room-stroke { stroke: rgba(255,255,255,0.55); stroke-width: 2; }
              .map-room-selected { stroke: rgba(255,255,255,0.85); }
              .map-draft { stroke: rgba(255,255,255,0.7); stroke-dasharray: 6 6; stroke-width: 2; fill: none; }
              .map-vertex { fill: rgba(255,255,255,0.75); }
              .map-room-vertex { fill: var(--color-primary); stroke: rgba(255,255,255,0.30); stroke-width: 1.2; }
              .map-room-vertex.active { stroke: rgba(255,255,255,0.75); stroke-width: 2.2; }
              .map-room-midpoint { fill: rgba(255,255,255,0.18); stroke: rgba(255,255,255,0.35); stroke-width: 1; cursor: grab; }
              .map-room-midpoint:active { cursor: grabbing; }
              .map-room-midpoint:hover { fill: rgba(255,255,255,0.30); }
              .map-room-insert-preview { fill: var(--color-primary); opacity: 0.9; pointer-events: none; }
              .map-device { fill: rgba(255,255,255,0.75); }
              .map-label { fill: rgba(255,255,255,0.75); font-size: 12px; }
              .map-room-label {
                fill: rgba(255,255,255,0.42);
                font-size: 12px;
                font-family: var(--font-family-primary, inherit);
                letter-spacing: 0.16em;
                font-weight: 700;
              }
              .map-fav-icon { fill: rgba(255,255,255,0.75); }
              .map-guide { stroke: var(--color-glass-border-light); stroke-width: 1.5; stroke-dasharray: 6 6; opacity: 0.85; pointer-events: none; }
              .map-guide-point { fill: var(--color-primary); opacity: 0.85; pointer-events: none; }
            `}
          </style>
        </defs>

        <g transform={`translate(${view.tx} ${view.ty}) scale(${view.scale})`}>
          {mode === 'draw' && snapGuide?.x1 !== undefined ? (
            <line x1={snapGuide.x1} y1={snapGuide.y1} x2={snapGuide.x2} y2={snapGuide.y2} className="map-guide" />
          ) : null}
          {mode === 'draw' && snapGuide?.px !== undefined && snapGuide?.py !== undefined ? (
            <circle cx={snapGuide.px} cy={snapGuide.py} r={4.2} className="map-guide-point" />
          ) : null}

          {roomPaths.map(r => (
            <g key={r.id}>
              <path
                d={r.path}
                className={`map-room-fill map-room-stroke${activeRoomId === r.id ? ' map-room-selected' : ''}${editEnabled && mode !== 'draw' ? ' map-room-draggable' : ''}`}
                onPointerDown={(e) => {
                  if (!editEnabled) return;
                  if (mode === 'draw') return;
                  beginRoomDrag(e, r.id);
                }}
                onClick={(e) => {
                  onRoomClick(r.id, e);
                }}
              />

              {editEnabled && mode !== 'draw' && activeRoomId === r.id && Array.isArray(r.points) ? (
                <g aria-label="Room vertices">
                  {r.points.map((p, idx) => (
                    <circle
                      key={`${r.id}-v-${idx}`}
                      cx={p.x}
                      cy={p.y}
                      r={5.2}
                      className={`map-room-vertex${Number.isFinite(activeVertexIndex) && activeVertexIndex === idx ? ' active' : ''}`}
                      onPointerDown={(e) => beginRoomVertexDrag(e, r.id, idx)}
                      onClick={(e) => e.stopPropagation()}
                      title="Drag to move corner"
                    />
                  ))}

                  {r.points.length >= 3 ? r.points.map((p, idx) => {
                    const next = r.points[(idx + 1) % r.points.length];
                    if (!next) return null;
                    const mx = (p.x + next.x) / 2;
                    const my = (p.y + next.y) / 2;
                    return (
                      <circle
                        key={`${r.id}-m-${idx}`}
                        cx={mx}
                        cy={my}
                        r={4.3}
                        className="map-room-midpoint"
                        onPointerDown={(e) => beginInsertCornerDrag(e, r.id, idx)}
                        onClick={(e) => e.stopPropagation()}
                        title="Drag to add a new corner"
                      />
                    );
                  }) : null}
                </g>
              ) : null}

              {(() => {
                const center = computeRoomCenter(r.points);
                if (!center) return null;
                const text = String(r.name || '').trim();
                if (!text) return null;
                const labelText = text.toUpperCase();
                const base = Number(roomLabelFontPx) || 12;
                const fontPx = Math.max(14, Math.min(42, Math.round(base * 1.55)));
                return (
                  <g transform={`translate(${center.x} ${center.y})`}>
                    <text
                      x={0}
                      y={fontPx * 0.34}
                      className="map-room-label"
                      textAnchor="middle"
                      style={{
                        fontSize: `${fontPx}px`,
                        fill: 'rgba(255,255,255,0.42)',
                        letterSpacing: `${Math.max(1.8, fontPx * 0.12)}px`,
                        filter: 'drop-shadow(0 1px 1px rgba(0,0,0,0.18))',
                        pointerEvents: 'none',
                      }}
                    >
                      {labelText}
                    </text>
                  </g>
                );
              })()}
            </g>
          ))}

          {draftPath ? <path d={draftPath} className="map-draft" /> : null}

          {editEnabled && mode !== 'draw' && insertCornerPreview?.point ? (
            <circle
              cx={insertCornerPreview.point.x}
              cy={insertCornerPreview.point.y}
              r={5.4}
              className="map-room-insert-preview"
            />
          ) : null}

          {draft?.points?.map((p, idx) => (
            <circle
              key={`draft-v-${idx}`}
              cx={p.x}
              cy={p.y}
              r={4}
              className="map-vertex"
              onClick={(e) => {
                e.stopPropagation();
                if (idx === 0 && draft.points.length >= 3) finalizeDraft();
              }}
            />
          ))}

          {typeof renderPlacedDevices === 'function' ? renderPlacedDevices() : null}
        </g>
      </svg>
    </div>
  );
}
