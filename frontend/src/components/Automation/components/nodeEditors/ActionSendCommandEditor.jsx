import React from 'react';
import BaseNodeEditor from './BaseNodeEditor';

export default class ActionSendCommandEditor extends BaseNodeEditor {
  render() {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return null;

    const { deviceOptions, applyEditorUpdate } = this.props;
    const cmd = String(selectedNode.data?.command || '').trim() || 'set_state';
    const commandMode = String(selectedNode.data?.ui?.command_mode || (cmd === 'set_state' ? 'set_state' : 'custom'));
    const effectiveArgsMode = commandMode === 'custom' ? 'json' : (selectedNode.data?.ui?.args_mode || 'builder');

    const targetsType = String(selectedNode.data?.targets?.type || 'device').toLowerCase();
    const selectedDeviceId = targetsType === 'device' && Array.isArray(selectedNode.data?.targets?.ids)
      ? String(selectedNode.data?.targets?.ids?.[0] || '')
      : '';

    return (
      <div className="automation-props">
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

        <div className="field">
          <label className="label">Args mode</label>
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
                        ? { ...n, data: { ...(n.data || {}), ui: { ...(n.data?.ui || {}), args_mode: 'json' } } }
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
              placeholder='e.g. { "state": "ON", "brightness": 120 }'
            />
          </div>
        ) : (
          <div className="automation-slide">
            <div className={`automation-slide-inner ${effectiveArgsMode === 'builder' ? 'mode-builder' : 'mode-json'}`}>
              <div className="automation-slide-pane">
                <div className="field">
                  <label className="label">State</label>
                  <select
                    className="input"
                    value={selectedNode.data?.ui?.state || ''}
                    onChange={(e) => {
                      const v = e.target.value;
                      this.setSelectedNodeUI({ state: v });
                    }}
                  >
                    <option value="">(unset)</option>
                    <option value="ON">ON</option>
                    <option value="OFF">OFF</option>
                  </select>
                </div>
                <div className="field">
                  <label className="label">Brightness (0-255)</label>
                  <input
                    className="input"
                    type="number"
                    min="0"
                    max="255"
                    value={selectedNode.data?.ui?.brightness || ''}
                    onChange={(e) => {
                      const v = e.target.value;
                      this.setSelectedNodeUI({ brightness: v });
                    }}
                  />
                </div>
                <div className="field">
                  <label className="label">Transition (ms)</label>
                  <input
                    className="input"
                    type="number"
                    min="0"
                    value={selectedNode.data?.ui?.transition_ms || ''}
                    onChange={(e) => {
                      const v = e.target.value;
                      this.setSelectedNodeUI({ transition_ms: v });
                    }}
                  />
                </div>

                <div className="field">
                  <label className="label">Color mode</label>
                  <select
                    className="input"
                    value={selectedNode.data?.ui?.color_mode || 'none'}
                    onChange={(e) => {
                      const v = e.target.value;
                      this.setSelectedNodeUI({ color_mode: v });
                    }}
                  >
                    <option value="none">None</option>
                    <option value="color_temp">Color temperature</option>
                    <option value="hs">Hue / saturation</option>
                  </select>
                </div>

                {(selectedNode.data?.ui?.color_mode || 'none') === 'color_temp' && (
                  <div className="field">
                    <label className="label">Color temperature (mired)</label>
                    <input
                      className="input"
                      type="number"
                      min="0"
                      value={selectedNode.data?.ui?.color_temp || ''}
                      onChange={(e) => {
                        const v = e.target.value;
                        this.setSelectedNodeUI({ color_temp: v });
                      }}
                    />
                  </div>
                )}

                {(selectedNode.data?.ui?.color_mode || 'none') === 'hs' && (
                  <>
                    <div className="field">
                      <label className="label">Hue (0-360)</label>
                      <input
                        className="input"
                        type="number"
                        min="0"
                        max="360"
                        value={selectedNode.data?.ui?.hue || ''}
                        onChange={(e) => {
                          const v = e.target.value;
                          this.setSelectedNodeUI({ hue: v });
                        }}
                      />
                    </div>
                    <div className="field">
                      <label className="label">Saturation (0-100)</label>
                      <input
                        className="input"
                        type="number"
                        min="0"
                        max="100"
                        value={selectedNode.data?.ui?.saturation || ''}
                        onChange={(e) => {
                          const v = e.target.value;
                          this.setSelectedNodeUI({ saturation: v });
                        }}
                      />
                    </div>
                  </>
                )}
              </div>

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
                    placeholder='e.g. { "state": "ON", "brightness": 120 }'
                  />
                </div>
              </div>
            </div>
          </div>
        )}

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
