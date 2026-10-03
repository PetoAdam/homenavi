import React from 'react';
import BaseNodeEditor from './BaseNodeEditor';
import { operatorsForCapability } from '../../capabilityModel';

const conditionLabels = {
  exists: 'Has a value',
  changed: 'Changes',
  eq: 'Is equal to',
  neq: 'Is not equal to',
  gt: 'Is greater than',
  gte: 'Is at least',
  lt: 'Is less than',
  lte: 'Is at most',
};

function valueTypeForCapability(capability) {
  if (!capability) return '';
  if (capability.type === 'binary') return 'boolean';
  if (capability.type === 'numeric') return 'number';
  if (capability.type === 'enum' || capability.type === 'string') return 'text';
  return '';
}

export default class TriggerEditor extends BaseNodeEditor {
  buildCronFromSimple(ui) {
    const preset = String(ui?.schedule_preset || 'every_n_minutes');
    const clampInt = (raw, min, max, fallback) => {
      const n = Number(raw);
      if (!Number.isFinite(n)) return fallback;
      const i = Math.floor(n);
      return Math.max(min, Math.min(max, i));
    };
    const parseHHMM = (raw, fallbackH = 8, fallbackM = 0) => {
      const v = String(raw || '').trim();
      const m = v.match(/^(\d{1,2}):(\d{2})$/);
      if (!m) return { h: fallbackH, min: fallbackM };
      const h = clampInt(m[1], 0, 23, fallbackH);
      const min = clampInt(m[2], 0, 59, fallbackM);
      return { h, min };
    };

    // robfig/cron with seconds: sec min hour dom month dow
    if (preset === 'every_n_minutes') {
      const every = clampInt(ui?.every_minutes, 1, 59, 5);
      return `0 */${every} * * * *`;
    }
    if (preset === 'hourly_at') {
      const minute = clampInt(ui?.at_minute, 0, 59, 0);
      return `0 ${minute} * * * *`;
    }
    if (preset === 'daily_at') {
      const { h, min } = parseHHMM(ui?.at_time, 8, 0);
      return `0 ${min} ${h} * * *`;
    }
    if (preset === 'weekly_at') {
      const { h, min } = parseHHMM(ui?.at_time, 8, 0);
      const dow = clampInt(ui?.weekday, 0, 6, 1);
      return `0 ${min} ${h} * * ${dow}`;
    }
    // Fallback
    return '0 */5 * * * *';
  }

  setScheduleUI(patch) {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return;
    const nextUI = { ...(selectedNode.data?.ui || {}), ...(patch || {}) };
    const mode = String(nextUI.schedule_mode || 'simple');
    this.setSelectedNodeUI(patch);
    if (mode === 'simple') {
      const cron = this.buildCronFromSimple(nextUI);
      this.setSelectedNodeData({ cron });
    }
  }

  render() {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return null;

    const { deviceOptions, groupOptions, tagOptions, selectedCapabilities = [] } = this.props;
    const kind = String(selectedNode.kind || '');
    const selectedCapability = selectedCapabilities.find(capability => capability.id === selectedNode.data?.capability_id) || null;
    const operators = operatorsForCapability(selectedCapability);
    const targetsType = String(selectedNode.data?.targets?.type || 'device').toLowerCase();
    const selector = String(selectedNode.data?.targets?.selector || '').trim();
    const isCollectionSelector = targetsType === 'selector' && /^(group|tag):/.test(selector.toLowerCase());
    const targetMode = String(selectedNode.data?.ui?.target_mode || (targetsType === 'device' ? 'device' : (isCollectionSelector ? 'collection' : 'selector')));
    const collectionOptions = [
      ...(Array.isArray(groupOptions) ? groupOptions.map(item => ({ ...item, kind: 'Group' })) : []),
      ...(Array.isArray(tagOptions) ? tagOptions.map(item => ({ ...item, kind: 'Tag' })) : []),
    ];
    const selectedDeviceId = targetsType === 'device' ? String(selectedNode.data?.targets?.ids?.[0] || '') : '';

    return (
      <div className="automation-props">
        <div className="field">
          <label className="label">Trigger type</label>
          <select
            className="input"
            value={kind}
            onChange={(e) => {
              const k = e.target.value;
              this.setSelectedNodeKind(k);
            }}
          >
            <option value="trigger.manual">Manual</option>
            <option value="trigger.device_state">Device state</option>
            <option value="trigger.schedule">Schedule (cron)</option>
          </select>
        </div>

        {kind === 'trigger.manual' ? (
          <div className="muted" style={{ fontSize: '0.9rem' }}>
            Manual triggers only run when you press <strong>Run</strong>.
          </div>
        ) : kind === 'trigger.schedule' ? (
          <>
            <div className="field">
              <label className="label">Schedule editor</label>
              <div
                className="automation-segmented slider"
                role="tablist"
                aria-label="Schedule editor mode"
                style={{ '--seg-pos': (selectedNode.data?.ui?.schedule_mode || 'simple') === 'simple' ? 0 : 1 }}
              >
                <button
                  type="button"
                  role="tab"
                  aria-selected={(selectedNode.data?.ui?.schedule_mode || 'simple') === 'simple'}
                  className={(selectedNode.data?.ui?.schedule_mode || 'simple') === 'simple' ? 'active' : ''}
                  onClick={() => this.setSelectedNodeUI({ schedule_mode: 'simple' })}
                >
                  Builder
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={(selectedNode.data?.ui?.schedule_mode || 'simple') === 'cron'}
                  className={(selectedNode.data?.ui?.schedule_mode || 'simple') === 'cron' ? 'active' : ''}
                  onClick={() => this.setSelectedNodeUI({ schedule_mode: 'cron' })}
                >
                  Cron
                </button>
              </div>
            </div>

            <div className="automation-slide">
              <div className={`automation-slide-inner ${(selectedNode.data?.ui?.schedule_mode || 'simple') === 'simple' ? 'mode-builder' : 'mode-json'}`}>
                <div className="automation-slide-pane">
                  <div className="field">
                    <label className="label">Runs</label>
                    <select
                      className="input"
                      value={selectedNode.data?.ui?.schedule_preset || 'every_n_minutes'}
                      onChange={(e) => {
                        const preset = e.target.value;
                        this.setScheduleUI({ schedule_preset: preset });
                      }}
                    >
                      <option value="every_n_minutes">Every N minutes</option>
                      <option value="hourly_at">Hourly (at minute)</option>
                      <option value="daily_at">Daily (at time)</option>
                      <option value="weekly_at">Weekly (day + time)</option>
                    </select>
                  </div>

                  {String(selectedNode.data?.ui?.schedule_preset || 'every_n_minutes') === 'every_n_minutes' && (
                    <div className="field">
                      <label className="label">Every (minutes)</label>
                      <input
                        className="input"
                        type="number"
                        min="1"
                        max="59"
                        value={Number(selectedNode.data?.ui?.every_minutes ?? 5)}
                        onChange={(e) => {
                          const v = Number(e.target.value);
                          this.setScheduleUI({ every_minutes: v });
                        }}
                      />
                    </div>
                  )}

                  {String(selectedNode.data?.ui?.schedule_preset || 'every_n_minutes') === 'hourly_at' && (
                    <div className="field">
                      <label className="label">Minute</label>
                      <input
                        className="input"
                        type="number"
                        min="0"
                        max="59"
                        value={Number(selectedNode.data?.ui?.at_minute ?? 0)}
                        onChange={(e) => {
                          const v = Number(e.target.value);
                          this.setScheduleUI({ at_minute: v });
                        }}
                      />
                    </div>
                  )}

                  {String(selectedNode.data?.ui?.schedule_preset || 'every_n_minutes') === 'daily_at' && (
                    <div className="field">
                      <label className="label">Time</label>
                      <input
                        className="input"
                        type="time"
                        value={String(selectedNode.data?.ui?.at_time || '08:00')}
                        onChange={(e) => {
                          this.setScheduleUI({ at_time: e.target.value });
                        }}
                      />
                    </div>
                  )}

                  {String(selectedNode.data?.ui?.schedule_preset || 'every_n_minutes') === 'weekly_at' && (
                    <>
                      <div className="field">
                        <label className="label">Day</label>
                        <select
                          className="input"
                          value={String(selectedNode.data?.ui?.weekday ?? 1)}
                          onChange={(e) => {
                            const v = Number(e.target.value);
                            this.setScheduleUI({ weekday: v });
                          }}
                        >
                          <option value="1">Monday</option>
                          <option value="2">Tuesday</option>
                          <option value="3">Wednesday</option>
                          <option value="4">Thursday</option>
                          <option value="5">Friday</option>
                          <option value="6">Saturday</option>
                          <option value="0">Sunday</option>
                        </select>
                      </div>
                      <div className="field">
                        <label className="label">Time</label>
                        <input
                          className="input"
                          type="time"
                          value={String(selectedNode.data?.ui?.at_time || '08:00')}
                          onChange={(e) => {
                            this.setScheduleUI({ at_time: e.target.value });
                          }}
                        />
                      </div>
                    </>
                  )}

                  <div className="field">
                    <label className="label">Cron preview</label>
                    <input className="input" value={selectedNode.data?.cron || this.buildCronFromSimple(selectedNode.data?.ui)} readOnly />
                    <div className="muted">Uses cron with seconds.</div>
                  </div>
                </div>

                <div className="automation-slide-pane">
                  <div className="field">
                    <label className="label">Cron (with seconds)</label>
                    <input
                      className="input"
                      value={selectedNode.data?.cron || ''}
                      onChange={(e) => {
                        const v = e.target.value;
                        this.setSelectedNodeData({ cron: v });
                      }}
                      placeholder="0 */5 * * * *"
                    />
                    <div className="muted">Example: <strong>0 */5 * * * *</strong> (every 5 minutes)</div>
                  </div>
                </div>
              </div>
            </div>

            <div className="field">
              <label className="label">Cooldown (sec)</label>
              <input
                className="input"
                type="number"
                min="0"
                value={Number(selectedNode.data?.cooldown_sec ?? 1)}
                onChange={(e) => {
                  const v = Number(e.target.value);
                  this.setSelectedNodeData({ cooldown_sec: v });
                }}
              />
            </div>
          </>
        ) : (
          <>
            <div className="field">
              <label className="label">Target type</label>
              <select className="input" value={targetMode} onChange={(e) => {
                const nextTargetMode = e.target.value;
                this.setSelectedNodeData({ targets: nextTargetMode === 'device' ? { type: 'device', ids: [], selector: '' } : { type: 'selector', ids: [], selector: nextTargetMode === 'selector' ? selector : '' } });
                this.setSelectedNodeUI({ target_mode: nextTargetMode });
              }}>
                <option value="device">Device</option>
                <option value="collection">Group or tag</option>
                <option value="selector">Advanced selector</option>
              </select>
            </div>
            {targetMode === 'device' && <div className="field">
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
            </div>}
            {targetMode === 'collection' && <div className="field"><label className="label">Group or tag</label><select className="input" value={selector} onChange={e => this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector: e.target.value } })}><option value="">Select a group or tag…</option>{collectionOptions.map(item => <option key={`${item.kind}:${item.id}`} value={item.selector}>{item.kind}: {item.label}</option>)}</select></div>}
            {targetMode === 'selector' && <div className="field"><label className="label">Selector</label><input className="input" value={selector} onChange={e => this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector: e.target.value } })} placeholder="e.g. tag:kitchen" /></div>}
            <div className="field">
              <label className="label">Capability</label>
              <select className="input" value={selectedNode.data?.capability_id || ''} onChange={(e) => {
                const capability = selectedCapabilities.find(item => item.id === e.target.value);
                this.setSelectedNodeData({ capability_id: capability?.id || '', capability_property: capability?.property || '', key: capability?.property || '', op: 'exists' });
                if (capability) this.setSelectedNodeUI({ value_type: valueTypeForCapability(capability) });
              }}>
                <option value="">Select what to watch…</option>
                {selectedCapabilities.map(capability => <option key={capability.id} value={capability.id}>{capability.label}</option>)}
              </select>
            </div>
            <div className="field">
              <label className="label">When it</label>
              <select
                className="input"
                value={selectedNode.data?.op || 'exists'}
                onChange={(e) => {
                  const v = e.target.value;
                  this.setSelectedNodeData({ op: v });
                }}
              >
                {operators.map(operator => <option key={operator} value={operator}>{conditionLabels[operator] || operator}</option>)}
              </select>
            </div>

            {targetMode !== 'device' && <div className="field"><label className="label">For this target</label><select className="input" value={selectedNode.data?.aggregation || 'any'} onChange={e => this.setSelectedNodeData({ aggregation: e.target.value })}><option value="any">One or more devices</option><option value="all">All devices</option></select></div>}

            {!['exists', 'changed'].includes(String(selectedNode.data?.op || 'exists')) && (
              <>
                <div className="field">
                  <label className="label">Value editor</label>
                  <div
                    className="automation-segmented slider"
                    role="tablist"
                    aria-label="Value editor mode"
                    style={{ '--seg-pos': (selectedNode.data?.ui?.value_mode || 'builder') === 'builder' ? 0 : 1 }}
                  >
                    <button
                      type="button"
                      role="tab"
                      aria-selected={(selectedNode.data?.ui?.value_mode || 'builder') === 'builder'}
                      className={(selectedNode.data?.ui?.value_mode || 'builder') === 'builder' ? 'active' : ''}
                      onClick={() => this.setSelectedNodeUI({ value_mode: 'builder' })}
                    >
                      Builder
                    </button>
                    <button
                      type="button"
                      role="tab"
                      aria-selected={(selectedNode.data?.ui?.value_mode || 'builder') === 'json'}
                      className={(selectedNode.data?.ui?.value_mode || 'builder') === 'json' ? 'active' : ''}
                      onClick={() => this.setSelectedNodeUI({ value_mode: 'json' })}
                    >
                      JSON
                    </button>
                  </div>
                </div>

                <div className="automation-slide">
                  <div className={`automation-slide-inner ${(selectedNode.data?.ui?.value_mode || 'builder') === 'builder' ? 'mode-builder' : 'mode-json'}`}>
                    <div className="automation-slide-pane">
                      {!selectedCapability && <div className="field">
                        <label className="label">Type</label>
                        <select
                          className="input"
                          value={selectedNode.data?.ui?.value_type || 'boolean'}
                          onChange={(e) => {
                            const t = e.target.value;
                            this.setSelectedNodeUI({ value_type: t });
                          }}
                        >
                          <option value="boolean">True/False</option>
                          <option value="number">Number</option>
                          <option value="text">Text</option>
                        </select>
                      </div>}

                      {selectedCapability?.type === 'binary' && (
                        <div className="field">
                          <label className="label">Value</label>
                          <select className="input" value={String(selectedNode.data?.ui?.value_bool ?? true)} onChange={e => this.setSelectedNodeUI({ value_bool: e.target.value === 'true' })}>
                            <option value="true">On / true</option>
                            <option value="false">Off / false</option>
                          </select>
                        </div>
                      )}

                      {selectedCapability?.type === 'numeric' && (
                        <div className="field">
                          <label className="label">Value {selectedCapability.unit ? `(${selectedCapability.unit})` : ''}</label>
                          <input className="input" type="number" min={selectedCapability.min ?? undefined} max={selectedCapability.max ?? undefined} step={selectedCapability.step ?? 'any'} value={selectedNode.data?.ui?.value_number ?? ''} onChange={e => this.setSelectedNodeUI({ value_number: e.target.value })} />
                        </div>
                      )}

                      {selectedCapability?.type === 'enum' && (
                        <div className="field">
                          <label className="label">Value</label>
                          <select className="input" value={selectedNode.data?.ui?.value_string ?? ''} onChange={e => this.setSelectedNodeUI({ value_string: e.target.value })}>
                            <option value="">Select…</option>
                            {selectedCapability.enumValues.map(value => <option key={value} value={value}>{value}</option>)}
                          </select>
                        </div>
                      )}

                      {selectedCapability?.type === 'string' && (
                        <div className="field"><label className="label">Value</label><input className="input" value={selectedNode.data?.ui?.value_string ?? ''} onChange={e => this.setSelectedNodeUI({ value_string: e.target.value })} /></div>
                      )}

                      {!selectedCapability && String(selectedNode.data?.ui?.value_type || 'boolean') === 'boolean' && (
                        <div className="field">
                          <label className="label">Value</label>
                          <select
                            className="input"
                            value={String(selectedNode.data?.ui?.value_bool ?? true)}
                            onChange={(e) => {
                              const v = e.target.value === 'true';
                              this.setSelectedNodeUI({ value_bool: v });
                            }}
                          >
                            <option value="true">true</option>
                            <option value="false">false</option>
                          </select>
                        </div>
                      )}

                      {!selectedCapability && String(selectedNode.data?.ui?.value_type || 'boolean') === 'number' && (
                        <div className="field">
                          <label className="label">Value</label>
                          <input
                            className="input"
                            type="number"
                            value={selectedNode.data?.ui?.value_number ?? ''}
                            onChange={(e) => {
                              this.setSelectedNodeUI({ value_number: e.target.value });
                            }}
                            placeholder="e.g. 42"
                          />
                        </div>
                      )}

                      {!selectedCapability && String(selectedNode.data?.ui?.value_type || 'boolean') === 'text' && (
                        <div className="field">
                          <label className="label">Value</label>
                          <input
                            className="input"
                            value={selectedNode.data?.ui?.value_string ?? ''}
                            onChange={(e) => {
                              this.setSelectedNodeUI({ value_string: e.target.value });
                            }}
                            placeholder="e.g. ON"
                          />
                        </div>
                      )}
                    </div>

                    <div className="automation-slide-pane">
                      <div className="field">
                        <label className="label">Value (JSON)</label>
                        <textarea
                          className="input textarea"
                          rows={5}
                          value={selectedNode.data?.ui?.value_text || ''}
                          onChange={(e) => {
                            const v = e.target.value;
                            this.setSelectedNodeUI({ value_text: v });
                          }}
                          placeholder={'e.g. true or 42 or {\n  "state": "ON"\n}'}
                        />
                      </div>
                    </div>
                  </div>
                </div>
              </>
            )}

            <div className="field">
              <label className="label">Debounce (sec)</label>
              <input className="input" type="number" min="0" value={Number(selectedNode.data?.debounce_sec ?? 0)} onChange={e => this.setSelectedNodeData({ debounce_sec: Math.max(0, Number(e.target.value) || 0) })} />
            </div>

            <div className="field">
              <label className="label">Cooldown (sec)</label>
              <input
                className="input"
                type="number"
                min="0"
                value={Number(selectedNode.data?.cooldown_sec ?? 2)}
                onChange={(e) => {
                  const v = Number(e.target.value);
                  this.setSelectedNodeData({ cooldown_sec: v });
                }}
              />
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={!!selectedNode.data?.ignore_retained}
                  onChange={(e) => {
                    const v = e.target.checked;
                    this.setSelectedNodeData({ ignore_retained: v });
                  }}
                />
                Ignore retained messages
              </label>
            </div>
          </>
        )}
      </div>
    );
  }
}
