import React from 'react';
import Button from '../../common/Button/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faLinkSlash, faTrash } from '@fortawesome/free-solid-svg-icons';
import TriggerEditor from './nodeEditors/TriggerEditor';
import ActionSendCommandEditor from './nodeEditors/ActionSendCommandEditor';
import ActionNotifyEmailEditor from './nodeEditors/ActionNotifyEmailEditor';
import LogicSleepEditor from './nodeEditors/LogicSleepEditor';
import LogicIfEditor from './nodeEditors/LogicIfEditor';
import LogicForEditor from './nodeEditors/LogicForEditor';

export default function AutomationPropertiesPanel({
  selectedNodeId,
  selectedNode,
  selectedConnections,
  isTriggerNode,
  defaultNodeData,
  applyEditorUpdate,
  applyEditorUpdateBatched,
  deviceOptions,
  triggerKeyOptions,
  userOptions,
  isAdmin,
  currentUserId,
  disconnectIncoming,
  disconnectOutgoing,
  deleteSelectedNode,
}) {
  return (
    <div className="automation-right">
      <div className="automation-panel-title">Properties</div>

      {selectedNode && selectedNodeId !== 'workflow' && (
        <div className="automation-props">
          <div className="field">
            <label className="label">Node</label>
            <div className="automation-props-actions compact">
              <Button
                variant="secondary"
                type="button"
                onClick={deleteSelectedNode}
                className="automation-btn-mini"
                title="Delete node"
                aria-label="Delete node"
              >
                <span className="btn-icon"><FontAwesomeIcon icon={faTrash} /></span>
                <span className="btn-label">Delete</span>
              </Button>

              {selectedConnections.incomingCount > 0 && (
                <Button
                  variant="secondary"
                  type="button"
                  onClick={disconnectIncoming}
                  className="automation-btn-mini"
                  title={`Disconnect incoming (${selectedConnections.incomingCount})`}
                  aria-label={`Disconnect incoming (${selectedConnections.incomingCount})`}
                >
                  <span className="btn-icon"><FontAwesomeIcon icon={faLinkSlash} /></span>
                  <span className="btn-label">In ({selectedConnections.incomingCount})</span>
                </Button>
              )}

              {selectedConnections.outgoingCount > 0 && (
                <Button
                  variant="secondary"
                  type="button"
                  onClick={disconnectOutgoing}
                  className="automation-btn-mini"
                  title={`Disconnect outgoing (${selectedConnections.outgoingCount})`}
                  aria-label={`Disconnect outgoing (${selectedConnections.outgoingCount})`}
                >
                  <span className="btn-icon"><FontAwesomeIcon icon={faLinkSlash} /></span>
                  <span className="btn-label">Out ({selectedConnections.outgoingCount})</span>
                </Button>
              )}
            </div>
          </div>
        </div>
      )}

      {selectedNodeId === 'workflow' && (
        <div className="automation-props">
          <div className="muted" style={{ fontSize: '0.9rem' }}>
            Tip: drag from a node’s right dot to another node, or tap the right dot then tap a node.
          </div>
        </div>
      )}

      {selectedNode && isTriggerNode(selectedNode) && (
        <TriggerEditor
          selectedNode={selectedNode}
          applyEditorUpdate={applyEditorUpdate}
          applyEditorUpdateBatched={applyEditorUpdateBatched}
          defaultNodeData={defaultNodeData}
          deviceOptions={deviceOptions}
          triggerKeyOptions={triggerKeyOptions}
        />
      )}

      {selectedNode && String(selectedNode.kind || '') === 'action.send_command' && (
        <ActionSendCommandEditor
          selectedNode={selectedNode}
          applyEditorUpdate={applyEditorUpdate}
          applyEditorUpdateBatched={applyEditorUpdateBatched}
          defaultNodeData={defaultNodeData}
          deviceOptions={deviceOptions}
          triggerKeyOptions={triggerKeyOptions}
        />
      )}

      {selectedNode && String(selectedNode.kind || '') === 'action.notify_email' && (
        <ActionNotifyEmailEditor
          selectedNode={selectedNode}
          applyEditorUpdate={applyEditorUpdate}
          applyEditorUpdateBatched={applyEditorUpdateBatched}
          defaultNodeData={defaultNodeData}
          userOptions={userOptions}
          isAdmin={!!isAdmin}
          currentUserId={currentUserId}
        />
      )}

      {selectedNode && String(selectedNode.kind || '') === 'logic.sleep' && (
        <LogicSleepEditor
          selectedNode={selectedNode}
          applyEditorUpdate={applyEditorUpdate}
          applyEditorUpdateBatched={applyEditorUpdateBatched}
          defaultNodeData={defaultNodeData}
          deviceOptions={deviceOptions}
          triggerKeyOptions={triggerKeyOptions}
        />
      )}

      {selectedNode && String(selectedNode.kind || '') === 'logic.if' && (
        <LogicIfEditor
          selectedNode={selectedNode}
          applyEditorUpdate={applyEditorUpdate}
          applyEditorUpdateBatched={applyEditorUpdateBatched}
          defaultNodeData={defaultNodeData}
          deviceOptions={deviceOptions}
          triggerKeyOptions={triggerKeyOptions}
        />
      )}

      {selectedNode && String(selectedNode.kind || '') === 'logic.for' && (
        <LogicForEditor
          selectedNode={selectedNode}
          applyEditorUpdate={applyEditorUpdate}
          applyEditorUpdateBatched={applyEditorUpdateBatched}
          defaultNodeData={defaultNodeData}
          deviceOptions={deviceOptions}
          triggerKeyOptions={triggerKeyOptions}
        />
      )}

      {selectedNodeId !== 'workflow' && !selectedNode && (
        <div className="muted">Click a node to edit its fields.</div>
      )}
    </div>
  );
}
