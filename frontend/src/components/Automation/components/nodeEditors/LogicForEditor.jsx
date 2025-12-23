import React from 'react';
import BaseNodeEditor from './BaseNodeEditor';

export default class LogicForEditor extends BaseNodeEditor {
  render() {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return null;

    return (
      <div className="automation-props">
        <div className="field">
          <label className="label">Count</label>
          <input
            className="input"
            type="number"
            min="1"
            value={Number(selectedNode.data?.count ?? 3)}
            onChange={(e) => {
              const v = Number(e.target.value);
              this.setSelectedNodeData({ count: v });
            }}
          />
        </div>
      </div>
    );
  }
}
