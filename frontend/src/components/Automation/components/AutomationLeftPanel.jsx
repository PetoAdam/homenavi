import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

export default function AutomationLeftPanel({
  paletteGroups,
  iconForNodeKind,
  onPaletteDragStart,
  addNodeAtCenter,
}) {
  return (
    <div className="automation-left">
      <div className="automation-left-section">
        <div className="automation-panel-title">Palette</div>
        <div className="automation-palette">
          {paletteGroups.map(group => (
            <div className="automation-palette-group" key={group.title}>
              <div className="muted" style={{ fontSize: '0.9rem' }}>{group.title}</div>
              <div className="automation-palette-grid">
                {group.items.map(item => {
                  const icon = iconForNodeKind(item.kind);
                  return (
                    <div
                      key={item.kind}
                      className="automation-palette-item"
                      draggable
                      onDragStart={(e) => onPaletteDragStart(e, item.kind)}
                      onClick={() => addNodeAtCenter(item.kind)}
                    >
                      <div className="label">
                        {icon && (
                          <span className="palette-icon" aria-hidden="true">
                            <FontAwesomeIcon icon={icon} />
                          </span>
                        )}
                        <span>{item.label}</span>
                      </div>
                      <div className="muted" style={{ fontSize: '0.8rem' }}>Drag or click to add</div>
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
