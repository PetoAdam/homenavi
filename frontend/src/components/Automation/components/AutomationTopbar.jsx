import React from 'react';
import Button from '../../common/Button/Button';
import GlassSwitch from '../../common/GlassSwitch/GlassSwitch';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import '../../common/Toolbar/Toolbar.css';
import {
  faArrowsRotate,
  faBroom,
  faCheck,
  faChevronDown,
  faPen,
  faPlay,
  faPlus,
  faRotateLeft,
  faRotateRight,
  faTrash,
  faXmark,
} from '@fortawesome/free-solid-svg-icons';

export default function AutomationTopbar({
  workflows,
  selectedId,
  onSelectId,
  startNewWorkflow,
  selectedWorkflow,
  workflowName,
  onWorkflowNameChange,
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
  done,
  isAdmin,
}) {
  const [isRenaming, setIsRenaming] = React.useState(false);
  const [renameDraft, setRenameDraft] = React.useState(String(workflowName || ''));

  React.useEffect(() => {
    setRenameDraft(String(workflowName || ''));
  }, [workflowName]);

  const canRename = Boolean(selectedWorkflow);

  const commitRename = () => {
    if (!canRename) {
      setIsRenaming(false);
      return;
    }
    const trimmed = String(renameDraft || '').trim();
    if (!trimmed) return;
    onWorkflowNameChange(trimmed);
    setIsRenaming(false);
  };

  const cancelRename = () => {
    setRenameDraft(String(workflowName || ''));
    setIsRenaming(false);
  };

  return (
    <div className="automation-topbar hn-toolbar">
      <div className="automation-topbar-left hn-toolbar-left">
        <div className="automation-topbar-row automation-topbar-row-select">
          <div className="automation-topbar-label muted">Workflow</div>

          <div className="automation-topbar-select-wrap" aria-label="Workflow selector">
            {isRenaming ? (
              <input
                className="input automation-topbar-select automation-topbar-rename-input"
                value={renameDraft}
                onChange={(e) => setRenameDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') commitRename();
                  if (e.key === 'Escape') cancelRename();
                }}
                placeholder="Workflow name"
                aria-label="Rename workflow"
                autoFocus
              />
            ) : (
              <>
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
              </>
            )}
          </div>

          <div className="automation-topbar-rename-actions">
            {isRenaming ? (
              <>
                <button
                  type="button"
                  className="automation-topbar-rename-btn"
                  title="Save name"
                  aria-label="Save name"
                  onClick={commitRename}
                >
                  <FontAwesomeIcon icon={faCheck} />
                </button>
                <button
                  type="button"
                  className="automation-topbar-rename-btn"
                  title="Cancel rename"
                  aria-label="Cancel rename"
                  onClick={cancelRename}
                >
                  <FontAwesomeIcon icon={faXmark} />
                </button>
              </>
            ) : (
              <button
                type="button"
                className="automation-topbar-rename-btn"
                title={canRename ? 'Rename workflow' : 'Select a workflow first'}
                aria-label="Rename workflow"
                disabled={!canRename}
                onClick={() => {
                  if (!canRename) return;
                  setRenameDraft(String(workflowName || ''));
                  setIsRenaming(true);
                }}
              >
                <FontAwesomeIcon icon={faPen} />
              </button>
            )}
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

      <div className="automation-topbar-right hn-toolbar-right">
        <div className="automation-topbar-group hn-toolbar-group">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn hn-toolbar-iconbtn"
            type="button"
            disabled={saving}
            onClick={done}
            title="Save and exit editor"
            aria-label="Done"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faCheck} /></span>
            <span className="btn-label">Done</span>
          </Button>
        </div>

        <div className="automation-topbar-group hn-toolbar-group">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn hn-toolbar-iconbtn"
            disabled={loading || devicesLoading}
            onClick={refreshAllData}
            title="Refresh workflows + devices"
            aria-label="Refresh workflows + devices"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faArrowsRotate} /></span>
            <span className="btn-label">{(loading || devicesLoading) ? 'Refreshing…' : 'Refresh'}</span>
          </Button>
        </div>

        <div className="automation-topbar-group hn-toolbar-group">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn hn-toolbar-iconbtn"
            onClick={clearCanvas}
            title="Clear nodes from canvas"
            aria-label="Clear nodes from canvas"
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faBroom} /></span>
            <span className="btn-label">Clear</span>
          </Button>
        </div>

        <div className="automation-topbar-group hn-toolbar-group">
          <Button
            variant="secondary"
            className="automation-topbar-iconbtn hn-toolbar-iconbtn"
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
            className="automation-topbar-iconbtn hn-toolbar-iconbtn"
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

        <div className="automation-topbar-group hn-toolbar-group">
          {selectedWorkflow && (
            <>
              <Button
                variant="secondary"
                className="automation-topbar-iconbtn hn-toolbar-iconbtn"
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
                  className="automation-topbar-iconbtn hn-toolbar-iconbtn"
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
