import React from 'react';
import Button from '../../common/Button/Button';
import GlassSwitch from '../../common/GlassSwitch/GlassSwitch';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faArrowsRotate,
  faBroom,
  faChevronDown,
  faPlay,
  faPlus,
  faRotateLeft,
  faRotateRight,
  faTrash,
} from '@fortawesome/free-solid-svg-icons';

export default function AutomationTopbar({
  workflows,
  selectedId,
  onSelectId,
  startNewWorkflow,
  selectedWorkflow,
  saving,
  lastSavedAt,
  loading,
  devicesLoading,
  refreshAllData,
  clearCanvas,
  canUndo,
  undo,
  canRedo,
  redo,
  toggleEnabled,
  runNow,
  removeWorkflow,
  isAdmin,
}) {
  return (
    <div className="automation-topbar">
      <div className="automation-topbar-left">
        <div className="automation-topbar-row automation-topbar-row-select">
          <div className="automation-topbar-label muted">Workflow</div>

          <div className="automation-topbar-select-wrap" aria-label="Workflow selector">
            <select
              className="input automation-topbar-select"
              value={selectedId || ''}
              onChange={(e) => {
                const v = String(e.target.value || '');
                onSelectId(v || null);
              }}
              aria-label="Select workflow"
            >
              <option value="">(new workflow)</option>
              {workflows.map((wf) => (
                <option key={wf.id} value={wf.id}>{wf.name}</option>
              ))}
            </select>
            <span className="automation-topbar-select-icon" aria-hidden="true">
              <FontAwesomeIcon icon={faChevronDown} />
            </span>
          </div>

          {(saving || lastSavedAt) && (
            <span className="automation-topbar-saved muted">
              {saving ? 'Saving…' : `Saved ${new Date(lastSavedAt).toLocaleTimeString()}`}
            </span>
          )}
        </div>

        <div className="automation-topbar-row automation-topbar-row-primary">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn automation-topbar-new"
            onClick={startNewWorkflow}
            title="Create new workflow"
            aria-label="Create new workflow"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faPlus} /></span>
            <span className="btn-label">New</span>
          </Button>

          {selectedWorkflow && (
            <div className="automation-topbar-status">
              <span className={`badge ${selectedWorkflow.enabled ? 'success' : 'muted'}`}>
                {selectedWorkflow.enabled ? 'Enabled' : 'Disabled'}
              </span>
              <div className="automation-topbar-switch" title="Enable/disable workflow">
                <GlassSwitch
                  checked={!!selectedWorkflow.enabled}
                  disabled={saving}
                  onChange={() => toggleEnabled()}
                />
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="automation-topbar-right">
        <div className="automation-topbar-group">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn"
            disabled={loading || devicesLoading}
            onClick={refreshAllData}
            title="Refresh workflows + devices"
            aria-label="Refresh workflows + devices"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faArrowsRotate} /></span>
            <span className="btn-label">{(loading || devicesLoading) ? 'Refreshing…' : 'Refresh'}</span>
          </Button>
        </div>

        <div className="automation-topbar-group">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn"
            onClick={clearCanvas}
            title="Clear nodes from canvas"
            aria-label="Clear nodes from canvas"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faBroom} /></span>
            <span className="btn-label">Clear</span>
          </Button>
        </div>

        <div className="automation-topbar-group">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn"
            type="button"
            disabled={!canUndo}
            onClick={undo}
            title="Undo (Ctrl/Cmd+Z)"
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
            title="Redo (Ctrl/Cmd+Shift+Z or Ctrl/Cmd+Y)"
            aria-label="Redo"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faRotateRight} /></span>
            <span className="btn-label">Redo</span>
          </Button>
        </div>

        <div className="automation-topbar-group">
          {selectedWorkflow && (
            <>
              <Button
                variant="secondary"
                className="automation-topbar-iconbtn"
                onClick={runNow}
                disabled={saving}
                title="Run workflow now"
                aria-label="Run workflow now"
              >
                <span className="btn-icon"><FontAwesomeIcon icon={faPlay} /></span>
                <span className="btn-label">Run</span>
              </Button>
              {isAdmin && (
                <Button
                  variant="secondary"
                  className="automation-topbar-iconbtn"
                  onClick={removeWorkflow}
                  title="Delete workflow"
                  aria-label="Delete workflow"
                >
                  <span className="btn-icon"><FontAwesomeIcon icon={faTrash} /></span>
                  <span className="btn-label">Delete</span>
                </Button>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
