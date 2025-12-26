import React from 'react';
import BaseNodeEditor from './BaseNodeEditor';

export default class LogicIfEditor extends BaseNodeEditor {
  render() {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return null;

    return (
      <div className="automation-props">
        <div className="field">
          <label className="label">Path</label>
          <input
            className="input"
            value={selectedNode.data?.path || ''}
            onChange={(e) => {
              const v = e.target.value;
              this.setSelectedNodeData({ path: v });
            }}
            placeholder="e.g. state.motion"
          />
        </div>
        <div className="field">
          <label className="label">Operator</label>
          <select
            className="input"
            value={selectedNode.data?.op || 'exists'}
            onChange={(e) => {
              const v = e.target.value;
              this.setSelectedNodeData({ op: v });
            }}
          >
            <option value="exists">exists</option>
            <option value="eq">eq</option>
            <option value="neq">neq</option>
            <option value="gt">gt</option>
            <option value="gte">gte</option>
            <option value="lt">lt</option>
            <option value="lte">lte</option>
          </select>
        </div>
        <div className="field">
          <label className="label">Value (JSON, optional)</label>
          <textarea
            className="input textarea"
            rows={4}
            value={selectedNode.data?.ui?.value_text || ''}
            onChange={(e) => {
              const v = e.target.value;
              this.setSelectedNodeUI({ value_text: v });
            }}
            placeholder='e.g. true, 42, "ON"'
          />
        </div>
      </div>
    );
  }
}
