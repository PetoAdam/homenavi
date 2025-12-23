import React from 'react';
import BaseNodeEditor from './BaseNodeEditor';

export default class LogicSleepEditor extends BaseNodeEditor {
  render() {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return null;

    return (
      <div className="automation-props">
        <div className="field">
          <label className="label">Duration (sec)</label>
          <input
            className="input"
            type="number"
            min="1"
            value={Number(selectedNode.data?.duration_sec ?? 5)}
            onChange={(e) => {
              const v = Number(e.target.value);
              this.setSelectedNodeData({ duration_sec: v });
            }}
          />
        </div>
      </div>
    );
  }
}
