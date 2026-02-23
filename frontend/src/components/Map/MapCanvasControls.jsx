import React from 'react';
import Button from '../common/Button/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faFont,
  faMagnifyingGlass,
  faMagnifyingGlassMinus,
  faMagnifyingGlassPlus,
  faPenToSquare,
  faTextHeight,
  faUpRightAndDownLeftFromCenter,
} from '@fortawesome/free-solid-svg-icons';

export default function MapCanvasControls({
  editEnabled,
  setEditEnabled,
  zoomBy,
  resetView,
  decreaseLabelScale,
  resetLabelScale,
  increaseLabelScale,
}) {
  return (
    <div className="hn-canvas-overlay-controls map-canvas-controls" onPointerDown={(e) => e.stopPropagation()}>
      <div className="map-control-groups" role="toolbar" aria-label="Map canvas controls">
        {!editEnabled ? (
          <div className="map-control-group">
            <div className="map-control-group-label muted">
              <FontAwesomeIcon icon={faPenToSquare} />
              <span>Mode</span>
            </div>
            <div className="map-control-actions">
              <Button
                variant="secondary"
                type="button"
                className="map-control-btn"
                onClick={() => setEditEnabled(true)}
                title="Edit map"
                aria-label="Enable edit mode"
              >
                <span className="btn-icon"><FontAwesomeIcon icon={faPenToSquare} /></span>
                <span className="btn-label">Edit</span>
              </Button>
            </div>
          </div>
        ) : null}

        <div className="map-control-group">
          <div className="map-control-group-label muted">
            <FontAwesomeIcon icon={faMagnifyingGlass} />
            <span>View</span>
          </div>
          <div className="map-control-actions">
            <Button
              variant="secondary"
              type="button"
              className="map-control-btn"
              onClick={() => zoomBy(1.12)}
              title="Zoom in"
              aria-label="Zoom in"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faMagnifyingGlassPlus} /></span>
              <span className="btn-label">Zoom in</span>
            </Button>
            <Button
              variant="secondary"
              type="button"
              className="map-control-btn"
              onClick={resetView}
              title="Reset canvas view"
              aria-label="Reset view"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faUpRightAndDownLeftFromCenter} /></span>
              <span className="btn-label">Reset view</span>
            </Button>
            <Button
              variant="secondary"
              type="button"
              className="map-control-btn"
              onClick={() => zoomBy(1 / 1.12)}
              title="Zoom out"
              aria-label="Zoom out"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faMagnifyingGlassMinus} /></span>
              <span className="btn-label">Zoom out</span>
            </Button>
          </div>
        </div>

        <div className="map-control-group">
          <div className="map-control-group-label muted">
            <FontAwesomeIcon icon={faFont} />
            <span>Labels</span>
          </div>
          <div className="map-control-actions">
            <Button
              variant="secondary"
              type="button"
              className="map-control-btn"
              onClick={increaseLabelScale}
              title="Increase label size"
              aria-label="Increase label size"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faMagnifyingGlassPlus} /></span>
              <span className="btn-label">Larger labels</span>
            </Button>
            <Button
              variant="secondary"
              type="button"
              className="map-control-btn"
              onClick={resetLabelScale}
              title="Reset label size"
              aria-label="Reset label size"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faTextHeight} /></span>
              <span className="btn-label">Reset labels</span>
            </Button>
            <Button
              variant="secondary"
              type="button"
              className="map-control-btn"
              onClick={decreaseLabelScale}
              title="Decrease label size"
              aria-label="Decrease label size"
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faMagnifyingGlassMinus} /></span>
              <span className="btn-label">Smaller labels</span>
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
