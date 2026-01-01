import React from 'react';
import Button from '../common/Button/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faMinus,
  faPenToSquare,
  faPlus,
  faUpRightAndDownLeftFromCenter,
} from '@fortawesome/free-solid-svg-icons';

export default function MapCanvasControls({
  editEnabled,
  setEditEnabled,
  zoomBy,
  resetView,
}) {
  return (
    <div className="automation-canvas-controls" onPointerDown={(e) => e.stopPropagation()}>
      <div className="actions compact">
        {!editEnabled ? (
          <Button
            variant="secondary"
            type="button"
            onClick={() => setEditEnabled(true)}
            title="Edit map"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faPenToSquare} /></span>
            <span className="btn-label">Edit</span>
          </Button>
        ) : null}
        <Button
          variant="secondary"
          type="button"
          onClick={() => zoomBy(1.12)}
          title="Zoom in"
        >
          <span className="btn-icon"><FontAwesomeIcon icon={faPlus} /></span>
          <span className="btn-label">Zoom</span>
        </Button>
        <Button
          variant="secondary"
          type="button"
          onClick={() => zoomBy(1 / 1.12)}
          title="Zoom out"
        >
          <span className="btn-icon"><FontAwesomeIcon icon={faMinus} /></span>
          <span className="btn-label">Zoom</span>
        </Button>
        <Button
          variant="secondary"
          type="button"
          onClick={resetView}
          title="Reset canvas view"
        >
          <span className="btn-icon"><FontAwesomeIcon icon={faUpRightAndDownLeftFromCenter} /></span>
          <span className="btn-label">View</span>
        </Button>
      </div>
    </div>
  );
}
