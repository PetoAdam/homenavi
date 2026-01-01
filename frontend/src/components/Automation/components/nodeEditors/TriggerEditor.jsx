import React from 'react';
import BaseNodeEditor from './BaseNodeEditor';

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

    const { deviceOptions, triggerKeyOptions } = this.props;
    const kind = String(selectedNode.kind || '');

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
              <label className="label">Device ID</label>
              <select
                className="input"
                value={String(selectedNode.data?.targets?.type || 'device').toLowerCase() === 'device' ? String(selectedNode.data?.targets?.ids?.[0] || '') : ''}
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
              <label className="label">State key (optional)</label>
              {triggerKeyOptions.length > 0 ? (
                <select
                  className="input"
                  value={selectedNode.data?.key || ''}
                  onChange={(e) => {
                    const v = e.target.value;
                    this.setSelectedNodeData({ key: v });
                  }}
                >
                  <option value="">(any key)</option>
                  {triggerKeyOptions.map((k) => (
                    <option key={k} value={k}>{k}</option>
                  ))}
                </select>
              ) : (
                <input
                  className="input"
                  value={selectedNode.data?.key || ''}
                  onChange={(e) => {
                    const v = e.target.value;
                    this.setSelectedNodeData({ key: v });
                  }}
                  placeholder="e.g. motion"
                />
              )}
            </div>
            <div className="field">
              <label className="label">Op</label>
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

            {String(selectedNode.data?.op || 'exists') !== 'exists' && (
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
                      <div className="field">
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
                      </div>

                      {String(selectedNode.data?.ui?.value_type || 'boolean') === 'boolean' && (
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

                      {String(selectedNode.data?.ui?.value_type || 'boolean') === 'number' && (
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

                      {String(selectedNode.data?.ui?.value_type || 'boolean') === 'text' && (
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
