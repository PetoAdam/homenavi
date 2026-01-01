import React from 'react';
import Button from '../common/Button/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faCheck,
  faRotateLeft,
  faRotateRight,
} from '@fortawesome/free-solid-svg-icons';

export default function MapEditorTopbar({
  canUndo,
  undo,
  canRedo,
  redo,
  done,
}) {
  return (
    <div className="automation-topbar" aria-label="Map editor toolbar">
      <div className="automation-topbar-left">
        <div className="automation-topbar-row automation-topbar-row-primary">
          <div className="automation-topbar-label muted">Map editor</div>
        </div>
      </div>

      <div className="automation-topbar-right">
        <div className="automation-topbar-group">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn"
            type="button"
            disabled={!canUndo}
            onClick={undo}
            aria-label="Undo"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faRotateLeft} /></span>
            <span className="btn-label">Undo</span>
          </Button>
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn"
            type="button"
            disabled={!canRedo}
            onClick={redo}
            aria-label="Redo"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faRotateRight} /></span>
            <span className="btn-label">Redo</span>
          </Button>
        </div>

        <div className="automation-topbar-group">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn"
            type="button"
            onClick={done}
            aria-label="Done"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faCheck} /></span>
            <span className="btn-label">Done</span>
          </Button>
        </div>
      </div>
    </div>
  );
}
