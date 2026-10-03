import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faPlus, faTrash } from '@fortawesome/free-solid-svg-icons';
import BaseNodeEditor from './BaseNodeEditor';
import { operatorsForCapability } from '../../capabilityModel';
import { CapabilityIcon, DeviceIcon } from '../AutomationIcons';
import GlassSelect from '../../../common/GlassSelect/GlassSelect';
import Button from '../../../common/Button/Button';
import { triggerValueFromBuilder, triggerValueToBuilder } from '../../triggerValueConversion';

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

function conditionSummary(label, op, value) {
  const verb = { eq: 'is', neq: 'is not', gt: 'is greater than', gte: 'is at least', lt: 'is less than', lte: 'is at most', exists: 'has a value', changed: 'changes' }[op] || op;
  if (op === 'exists' || op === 'changed') return `${label} ${verb}`;
  return `${label} ${verb} ${String(value ?? 'a value')}`;
}

function valueTypeForCapability(capability) {
  if (!capability) return '';
  if (capability.type === 'binary') return 'boolean';
  if (capability.type === 'numeric') return 'number';
  if (capability.type === 'enum' || capability.type === 'string') return 'text';
  return '';
}

function additionalConditionValue(condition) {
  if (String(condition?.value_mode || 'json') !== 'builder') return condition?.value_text || 'a value';
  const type = String(condition?.value_type || 'boolean').toLowerCase();
  if (type === 'boolean') return Boolean(condition?.value_bool);
  if (type === 'number') return condition?.value_number;
  return condition?.value_string;
}

export default class TriggerEditor extends BaseNodeEditor {
  showToast(message) {
    this.props.onToast?.(message);
  }

  switchValueMode(mode, capability) {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return;
    const ui = selectedNode.data?.ui || {};
    if (mode === 'json') {
      const conversion = triggerValueFromBuilder(ui, capability);
      this.setSelectedNodeUI({ value_mode: 'json', value_text: conversion.valid ? conversion.text : '' });
      this.showToast(conversion.valid ? 'Builder value converted to JSON.' : `Builder could not be converted: ${conversion.message}`);
      return;
    }
    const conversion = triggerValueToBuilder(ui.value_text, capability);
    this.setSelectedNodeUI(conversion.valid
      ? { ...conversion.ui, value_mode: 'builder' }
      : { value_bool: true, value_number: '', value_string: '', value_mode: 'builder' });
    this.showToast(conversion.valid ? 'JSON value converted to Builder.' : `Cannot convert JSON to Builder: ${conversion.message} A blank Builder is ready.`);
  }

  updateAdditionalCondition(index, patch) {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return;
    const conditions = Array.isArray(selectedNode.data?.additional_conditions) ? selectedNode.data.additional_conditions : [];
    this.setSelectedNodeData({
      additional_conditions: conditions.map((condition, conditionIndex) => conditionIndex === index ? { ...condition, ...patch } : condition),
    });
  }

  switchAdditionalValueMode(index, mode, capability) {
    const selectedNode = this.selectedNode;
    const condition = selectedNode?.data?.additional_conditions?.[index];
    if (!condition) return;
    if (mode === 'json') {
      const conversion = triggerValueFromBuilder(condition, capability);
      this.updateAdditionalCondition(index, { value_mode: 'json', value_text: conversion.valid ? conversion.text : '' });
      this.showToast(conversion.valid ? 'Builder value converted to JSON.' : `Builder could not be converted: ${conversion.message}`);
      return;
    }
    const conversion = triggerValueToBuilder(condition.value_text, capability);
    this.updateAdditionalCondition(index, conversion.valid
      ? { ...conversion.ui, value_mode: 'builder' }
      : { value_bool: true, value_number: '', value_string: '', value_mode: 'builder' });
    this.showToast(conversion.valid ? 'JSON value converted to Builder.' : `Cannot convert JSON to Builder: ${conversion.message} A blank Builder is ready.`);
  }

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
    const selectedDevice = deviceOptions.find(device => device.id === selectedDeviceId);
    const primaryValue = selectedCapability?.type === 'binary' ? Boolean(selectedNode.data?.ui?.value_bool)
      : selectedCapability?.type === 'numeric' ? selectedNode.data?.ui?.value_number
        : selectedNode.data?.ui?.value_string;
    const triggerSummary = selectedCapability ? [conditionSummary(selectedCapability.label, selectedNode.data?.op || 'exists', primaryValue), ...(Array.isArray(selectedNode.data?.additional_conditions) ? selectedNode.data.additional_conditions.filter(condition => condition.key).map(condition => conditionSummary(condition.key, condition.op || 'eq', additionalConditionValue(condition))) : [])].join(' AND ') : '';
    const additionalConditionsEditor = targetMode !== 'selector' ? (
      <div className="field automation-additional-conditions">
        <label className="label">Additional conditions (AND)</label>
        {(Array.isArray(selectedNode.data?.additional_conditions) ? selectedNode.data.additional_conditions : []).map((condition, index) => {
          const capability = selectedCapabilities.find(item => item.id === condition.capability_id);
          const update = (patch) => this.updateAdditionalCondition(index, patch);
          const requiresValue = !['exists', 'changed'].includes(condition.op || 'eq');
          const valueMode = String(condition.value_mode || 'json');
          const valueType = capability?.type === 'binary' ? 'boolean'
            : capability?.type === 'numeric' ? 'number'
              : capability?.type === 'enum' || capability?.type === 'string' ? 'text'
                : String(condition.value_type || 'boolean');
          return <div className="field automation-capability-row" key={`${condition.capability_id || 'condition'}-${index}`}>
            <div className="automation-capability-row-heading">
              <label className="label">Additional capability {index + 1}</label>
              <Button
                type="button"
                variant="secondary"
                className="automation-btn-mini automation-capability-remove"
                aria-label={`Remove capability condition ${index + 1}`}
                title="Remove capability condition"
                onClick={() => this.setSelectedNodeData({ additional_conditions: selectedNode.data.additional_conditions.filter((_, itemIndex) => itemIndex !== index) })}
              >
                <span className="btn-icon"><FontAwesomeIcon icon={faTrash} /></span>
              </Button>
            </div>
            <GlassSelect value={condition.capability_id || ''} onChange={(value) => { const next = selectedCapabilities.find(item => item.id === value); update({ capability_id: next?.id || '', key: next?.property || '', op: 'eq', value_mode: 'builder', value_type: valueTypeForCapability(next) || 'boolean', value_bool: true, value_number: '', value_string: '', value_text: '' }); }} placeholder="Select another capability…" ariaLabel={`Additional capability ${index + 1}`} options={selectedCapabilities.map(item => ({ value: item.id, label: <><CapabilityIcon capability={item} /> {item.label}</> }))} />
            <select className="input" value={condition.op || 'eq'} onChange={e => update({ op: e.target.value })}>{operatorsForCapability(capability).filter(op => op !== 'changed').map(op => <option key={op} value={op}>{conditionLabels[op] || op}</option>)}</select>
            {requiresValue && <>
              <div className="automation-segmented slider" role="tablist" aria-label={`Additional condition ${index + 1} value editor mode`} style={{ '--seg-pos': valueMode === 'builder' ? 0 : 1 }}>
                <button type="button" role="tab" aria-selected={valueMode === 'builder'} className={valueMode === 'builder' ? 'active' : ''} onClick={() => this.switchAdditionalValueMode(index, 'builder', capability)}>Builder</button>
                <button type="button" role="tab" aria-selected={valueMode === 'json'} className={valueMode === 'json' ? 'active' : ''} onClick={() => this.switchAdditionalValueMode(index, 'json', capability)}>JSON</button>
              </div>
              {valueMode === 'builder' ? <>
                {!capability && <select className="input" value={valueType} onChange={e => update({ value_type: e.target.value })}><option value="boolean">True/False</option><option value="number">Number</option><option value="text">Text</option></select>}
                {valueType === 'boolean' && <select className="input" value={String(condition.value_bool ?? true)} onChange={e => update({ value_bool: e.target.value === 'true' })}><option value="true">On / true</option><option value="false">Off / false</option></select>}
                {valueType === 'number' && <input className="input" type="number" min={capability?.min ?? undefined} max={capability?.max ?? undefined} step={capability?.step ?? 'any'} value={condition.value_number ?? ''} onChange={e => update({ value_number: e.target.value })} />}
                {capability?.type === 'enum' && <select className="input" value={condition.value_string ?? ''} onChange={e => update({ value_string: e.target.value })}><option value="">Select…</option>{capability.enumValues.map(value => <option key={value} value={value}>{value}</option>)}</select>}
                {valueType === 'text' && capability?.type !== 'enum' && <input className="input" value={condition.value_string ?? ''} onChange={e => update({ value_string: e.target.value })} />}
              </> : <textarea className="input textarea" rows={3} value={condition.value_text || ''} onChange={e => update({ value_text: e.target.value })} placeholder="e.g. true, 35, or &quot;away&quot;" />}
            </>}
          </div>;
        })}
        <div className="automation-props-actions">
          <Button
            type="button"
            className="automation-btn-mini"
            disabled={!selectedNode.data?.capability_id}
            title={!selectedNode.data?.capability_id ? 'Choose a primary capability first' : 'Add capability condition'}
            onClick={() => this.setSelectedNodeData({ additional_conditions: [...(selectedNode.data?.additional_conditions || []), { capability_id: '', key: '', op: 'eq', value_mode: 'builder', value_type: 'boolean', value_bool: true, value_number: '', value_string: '', value_text: '' }] })}
          >
            <span className="btn-icon"><FontAwesomeIcon icon={faPlus} /></span>
            <span className="btn-label">Add capability</span>
          </Button>
        </div>
        <div className="muted">All conditions must match each selected device. For logic across multiple devices, use an IF node.</div>
      </div>
    ) : null;

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
              <GlassSelect
                value={selectedDeviceId}
                onChange={(value) => {
                  const v = value;
                  this.setSelectedNodeData({ targets: { type: 'device', ids: v ? [v] : [], selector: '' } });
                }}
                placeholder="Select a device…"
                ariaLabel="Device ID"
                options={deviceOptions.map(device => ({ value: device.id, label: <><DeviceIcon device={device.raw} /> {device.label}</> }))}
              />
            </div>}
            {selectedDevice && <div className="muted"><DeviceIcon device={selectedDevice.raw} /> {selectedDevice.label}</div>}
            {targetMode === 'collection' && <div className="field"><label className="label">Group or tag</label><select className="input" value={selector} onChange={e => this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector: e.target.value } })}><option value="">Select a group or tag…</option>{collectionOptions.map(item => <option key={`${item.kind}:${item.id}`} value={item.selector}>{item.kind}: {item.label}</option>)}</select></div>}
            {targetMode === 'selector' && <div className="field"><label className="label">Selector</label><input className="input" value={selector} onChange={e => this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector: e.target.value } })} placeholder="e.g. tag:kitchen" /></div>}
            <div className="field">
              <label className="label">State editor</label>
              <div className="automation-segmented slider" role="tablist" aria-label="Value editor mode" style={{ '--seg-pos': (selectedNode.data?.ui?.value_mode || 'builder') === 'builder' ? 0 : 1 }}>
                <button type="button" role="tab" aria-selected={(selectedNode.data?.ui?.value_mode || 'builder') === 'builder'} className={(selectedNode.data?.ui?.value_mode || 'builder') === 'builder' ? 'active' : ''} onClick={() => this.switchValueMode('builder', selectedCapability)}>Builder</button>
                <button type="button" role="tab" aria-selected={(selectedNode.data?.ui?.value_mode || 'builder') === 'json'} className={(selectedNode.data?.ui?.value_mode || 'builder') === 'json' ? 'active' : ''} onClick={() => this.switchValueMode('json', selectedCapability)}>JSON</button>
              </div>
            </div>
            <div className="field">
              <label className="label">Capability</label>
              <GlassSelect value={selectedNode.data?.capability_id || ''} onChange={(value) => {
                const capability = selectedCapabilities.find(item => item.id === value);
                this.setSelectedNodeData({ capability_id: capability?.id || '', capability_property: capability?.property || '', key: capability?.property || '', op: 'exists' });
                if (capability) this.setSelectedNodeUI({ value_type: valueTypeForCapability(capability) });
              }}
                placeholder="Select what to watch…"
                ariaLabel="Capability"
                options={selectedCapabilities.map(capability => ({ value: capability.id, label: <><CapabilityIcon capability={capability} /> {capability.label}</> }))}
              />
            </div>
            {selectedCapability && <div className="muted"><CapabilityIcon capability={selectedCapability} /> {selectedCapability.label}</div>}
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
                {(selectedNode.data?.ui?.value_mode || 'builder') === 'builder' ? (
                  <div>
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
                ) : (
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
                      placeholder="e.g. true, 35, or &quot;away&quot;"
                    />
                    <div className="muted">Use a JSON value matching the selected capability. For a numeric capability, <strong>35</strong> is correct.</div>
                  </div>
                )}
              </>
            )}

            {additionalConditionsEditor}
            {triggerSummary && <div className="muted automation-trigger-summary" role="status">{triggerSummary}</div>}

            <div className="automation-props-section automation-settings-section">
              <div className="automation-props-section-title">Trigger behavior</div>
              <div className="automation-props-section-subtitle">Control when this trigger is allowed to run.</div>
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
            </div>
          </>
        )}
      </div>
    );
  }
}
