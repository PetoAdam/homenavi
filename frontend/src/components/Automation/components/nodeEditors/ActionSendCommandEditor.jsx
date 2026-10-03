import React from 'react';
import BaseNodeEditor from './BaseNodeEditor';

export default class ActionSendCommandEditor extends BaseNodeEditor {
  render() {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return null;

    const { deviceOptions, groupOptions, applyEditorUpdate, selectedCapabilities = [] } = this.props;
    const writableCapabilities = selectedCapabilities.filter(capability => capability.writable && capability.type !== 'unsupported');
    const selectedCapability = writableCapabilities.find(capability => capability.id === selectedNode.data?.capability_id) || null;
    const cmd = String(selectedNode.data?.command || '').trim() || 'set_state';
    const commandMode = String(selectedNode.data?.ui?.command_mode || (cmd === 'set_state' ? 'set_state' : 'custom'));
    const effectiveArgsMode = commandMode === 'custom' ? 'json' : (selectedNode.data?.ui?.args_mode || 'builder');

    const targetsType = String(selectedNode.data?.targets?.type || 'device').toLowerCase();
    const selector = String(selectedNode.data?.targets?.selector || '').trim();
    const isGroupSelector = targetsType === 'selector' && selector.toLowerCase().startsWith('group:');
    const targetMode = targetsType === 'device' ? 'device' : (isGroupSelector ? 'group' : 'selector');
    const selectedDeviceId = targetsType === 'device' && Array.isArray(selectedNode.data?.targets?.ids)
      ? String(selectedNode.data?.targets?.ids?.[0] || '')
      : '';
    const selectedGroupSelector = isGroupSelector ? selector : '';

    return (
      <div className="automation-props">
        <div className="field">
          <label className="label">Target type</label>
          <select
            className="input"
            value={targetMode}
            onChange={(e) => {
              const value = e.target.value;
              if (value === 'device') {
                this.setSelectedNodeData({ targets: { type: 'device', ids: [], selector: '' } });
                return;
              }
              if (value === 'group') {
                this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector: '' } });
                return;
              }
              this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector } });
            }}
          >
            <option value="device">Device</option>
            <option value="group">Group</option>
            <option value="selector">Advanced selector</option>
          </select>
        </div>

        {targetMode === 'device' && (
          <div className="field">
            <label className="label">Device ID</label>
            <select
              className="input"
              value={selectedDeviceId}
              onChange={(e) => {
                const v = e.target.value;
                this.setSelectedNodeData({ targets: { type: 'device', ids: v ? [v] : [], selector: '' } });
              }}
            >
              <option value="">Select a device…</option>
              {deviceOptions.map((d) => (
                <option key={d.id} value={d.id}>{d.label}</option>
              ))}
            </select>
          </div>
        )}

        {targetMode === 'group' && (
          <div className="field">
            <label className="label">Group</label>
            <select
              className="input"
              value={selectedGroupSelector}
              onChange={(e) => {
                const v = e.target.value;
                this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector: v } });
              }}
            >
              <option value="">Select a group…</option>
              {(Array.isArray(groupOptions) ? groupOptions : []).map((group) => (
                <option key={group.id} value={group.selector}>{group.label}</option>
              ))}
            </select>
          </div>
        )}

        {targetMode === 'selector' && (
          <div className="field">
            <label className="label">Selector</label>
            <input
              className="input"
              value={selector}
              onChange={(e) => {
                this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector: e.target.value } });
              }}
              placeholder="e.g. tag:kitchen or group:kitchen-spots"
            />
          </div>
        )}

        <div className="field">
          <label className="label">Command</label>
          <select
            className="input"
            value={commandMode}
            onChange={(e) => {
              const v = e.target.value;
              if (v === 'set_state') {
                this.setSelectedNodeUI({ command_mode: 'set_state' });
                this.setSelectedNodeData({ command: 'set_state' });
                return;
              }
              // custom
              this.setSelectedNodeUI({ command_mode: 'custom', args_mode: 'json' });
              if (String(selectedNode.data?.command || '').trim().toLowerCase() === 'set_state') {
                this.setSelectedNodeData({ command: '' });
              }
            }}
          >
            <option value="set_state">Set state</option>
            <option value="custom">Custom…</option>
          </select>
        </div>

        {commandMode === 'set_state' && effectiveArgsMode !== 'json' && (
          <>
            {targetMode === 'selector' && !isGroupSelector && (
              <div className="muted">Capability metadata cannot be resolved for an arbitrary selector. Use advanced JSON.</div>
            )}
            {targetMode === 'group' && selectedGroupSelector && writableCapabilities.length === 0 && (
              <div className="muted">This group has no compatible writable capabilities.</div>
            )}
            <div className="field">
              <label className="label">Capability</label>
              <select
                className="input"
                value={selectedNode.data?.capability_id || ''}
                onChange={(e) => {
                  const capability = writableCapabilities.find(item => item.id === e.target.value);
                  this.setSelectedNodeData({
                    capability_id: capability?.id || '',
                    capability_property: capability?.property || '',
                    capability_value: capability?.type === 'binary' ? false : '',
                  });
                }}
              >
                <option value="">Select a writable capability…</option>
                {writableCapabilities.map(capability => <option key={capability.id} value={capability.id}>{capability.label}</option>)}
              </select>
            </div>
            {selectedCapability?.type === 'binary' && <div className="field"><label className="label">Value</label><input type="checkbox" checked={Boolean(selectedNode.data?.capability_value)} onChange={e => this.setSelectedNodeData({ capability_value: e.target.checked })} /></div>}
            {selectedCapability?.type === 'numeric' && <div className="field"><label className="label">Value {selectedCapability.unit ? `(${selectedCapability.unit})` : ''}</label><input className="input" type="number" min={selectedCapability.min ?? undefined} max={selectedCapability.max ?? undefined} step={selectedCapability.step ?? 'any'} value={selectedNode.data?.capability_value ?? ''} onChange={e => this.setSelectedNodeData({ capability_value: Number(e.target.value) })} /></div>}
            {selectedCapability?.type === 'enum' && <div className="field"><label className="label">Value</label><select className="input" value={selectedNode.data?.capability_value ?? ''} onChange={e => this.setSelectedNodeData({ capability_value: e.target.value })}><option value="">Select…</option>{selectedCapability.enumValues.map(value => <option key={value} value={value}>{value}</option>)}</select></div>}
            {selectedCapability?.type === 'string' && <div className="field"><label className="label">Value</label><input className="input" value={selectedNode.data?.capability_value ?? ''} onChange={e => this.setSelectedNodeData({ capability_value: e.target.value })} /></div>}
            {selectedCapability?.type === 'object' && <div className="field"><label className="label">Value (JSON)</label><textarea className="input textarea" rows={5} value={typeof selectedNode.data?.capability_value === 'string' ? selectedNode.data.capability_value : JSON.stringify(selectedNode.data?.capability_value ?? {}, null, 2)} onChange={e => { try { this.setSelectedNodeData({ capability_value: JSON.parse(e.target.value) }); } catch { this.setSelectedNodeData({ capability_value: e.target.value }); } }} /></div>}
          </>
        )}

        {commandMode === 'custom' && (
          <div className="field">
            <label className="label">Custom command</label>
            <input
              className="input"
              value={selectedNode.data?.command || ''}
              onChange={(e) => {
                const v = e.target.value;
                this.setSelectedNodeData({ command: v });
              }}
              placeholder="e.g. refresh"
            />
          </div>
        )}

        {!selectedCapability && commandMode === 'set_state' && effectiveArgsMode !== 'json' && (
          <div className="muted">Select a capability to build a device command.</div>
        )}

        {commandMode === 'set_state' && (
        <div className="field">
          <label className="label">Advanced args</label>
          {commandMode === 'custom' ? (
            <div className="muted">Custom commands use JSON args.</div>
          ) : (
            <div
              className="automation-segmented slider"
              role="tablist"
              aria-label="Args editor mode"
              style={{ '--seg-pos': effectiveArgsMode === 'builder' ? 0 : 1 }}
            >
              <button
                type="button"
                role="tab"
                aria-selected={effectiveArgsMode === 'builder'}
                className={effectiveArgsMode === 'builder' ? 'active' : ''}
                onClick={() => {
                  applyEditorUpdate((prev) => ({
                    ...prev,
                    nodes: (Array.isArray(prev.nodes) ? prev.nodes : []).map((n) =>
                      n?.id === selectedNode.id
                        ? { ...n, data: { ...(n.data || {}), ui: { ...(n.data?.ui || {}), args_mode: 'builder' } } }
                        : n
                    ),
                  }));
                }}
              >
                Builder
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={effectiveArgsMode === 'json'}
                className={effectiveArgsMode === 'json' ? 'active' : ''}
                onClick={() => {
                  applyEditorUpdate((prev) => ({
                    ...prev,
                    nodes: (Array.isArray(prev.nodes) ? prev.nodes : []).map((n) =>
                      n?.id === selectedNode.id
                        ? { ...n, data: { ...(n.data || {}), capability_id: '', capability_property: '', capability_value: undefined, ui: { ...(n.data?.ui || {}), args_mode: 'json' } } }
                        : n
                    ),
                  }));
                }}
              >
                JSON
              </button>
            </div>
          )}
        </div>
        )}

        {commandMode === 'custom' ? (
          <div className="field">
            <label className="label">Args (JSON)</label>
            <textarea
              className="input textarea"
              rows={8}
              value={selectedNode.data?.ui?.args_text || ''}
              onChange={(e) => {
                const v = e.target.value;
                this.setSelectedNodeUI({ args_text: v });
              }}
              placeholder='e.g. { "property_name": "value" }'
            />
          </div>
        ) : commandMode === 'set_state' && effectiveArgsMode === 'json' ? (
          <div className="automation-slide">
            <div className={`automation-slide-inner ${effectiveArgsMode === 'builder' ? 'mode-builder' : 'mode-json'}`}>
              <div className="automation-slide-pane">
                <div className="field">
                  <label className="label">Args (JSON)</label>
                  <textarea
                    className="input textarea"
                    rows={8}
                    value={selectedNode.data?.ui?.args_text || ''}
                    onChange={(e) => {
                      const v = e.target.value;
                      this.setSelectedNodeUI({ args_text: v });
                    }}
                    placeholder='e.g. { "property_name": "value" }'
                  />
                </div>
              </div>
            </div>
          </div>
        ) : null}

        <div className="field">
          <label className="checkbox">
            <input
              type="checkbox"
              checked={!!selectedNode.data?.wait_for_result}
              onChange={(e) => {
                const v = e.target.checked;
                this.setSelectedNodeData({ wait_for_result: v });
              }}
            />
            Wait for result (leaf-only)
          </label>
        </div>

        {selectedNode.data?.wait_for_result && (
          <div className="field">
            <label className="label">Result timeout (sec)</label>
            <input
              className="input"
              type="number"
              min="1"
              value={Number(selectedNode.data?.result_timeout_sec ?? 15)}
              onChange={(e) => {
                const v = Number(e.target.value);
                this.setSelectedNodeData({ result_timeout_sec: v });
              }}
            />
          </div>
        )}
      </div>
    );
  }
}
