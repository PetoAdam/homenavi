import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faCircleCheck, faPlus, faTrash } from '@fortawesome/free-solid-svg-icons';
import BaseNodeEditor from './BaseNodeEditor';
import { CapabilityIcon, DeviceIcon } from '../AutomationIcons';
import Button from '../../../common/Button/Button';
import GlassSelect from '../../../common/GlassSelect/GlassSelect';
import { buildArgsFromBuilder, validateCommandArgs } from '../../commandArgsValidation';

export default class ActionSendCommandEditor extends BaseNodeEditor {
  state = { jsonValidation: null };

  showToast(message) {
    this.props.onToast?.(message);
  }

  switchArgsMode(mode, writableCapabilities) {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return;
    const data = selectedNode.data || {};
    if (mode === 'json') {
      const conversion = buildArgsFromBuilder(data);
      const args = conversion.valid ? conversion.args : {};
      this.showToast(conversion.valid ? 'Builder converted to JSON.' : `Builder could not be converted: ${conversion.message}`);
      this.setState({ jsonValidation: null });
      this.props.applyEditorUpdate((previous) => ({
        ...previous,
        nodes: (previous.nodes || []).map(node => node?.id === selectedNode.id ? {
          ...node,
          data: { ...(node.data || {}), ui: { ...(node.data?.ui || {}), args_mode: 'json', args_text: JSON.stringify(args, null, 2) } },
        } : node),
      }));
      return;
    }
    const validation = validateCommandArgs(data?.ui?.args_text, { writableCapabilities });
    const capability = validation.capability;
    this.showToast(validation.valid ? 'JSON converted to Builder.' : `Cannot convert JSON to Builder: ${validation.message} A blank Builder is ready.`);
    this.setState({ jsonValidation: null });
    this.props.applyEditorUpdate((previous) => ({
      ...previous,
      nodes: (previous.nodes || []).map(node => node?.id === selectedNode.id ? {
        ...node,
        data: {
          ...(node.data || {}),
          capability_id: capability?.id || '',
          capability_property: capability?.property || '',
          capability_value: capability ? validation.args[capability.property] : undefined,
          ui: { ...(node.data?.ui || {}), args_mode: 'builder' },
        },
      } : node),
    }));
  }

  validateJson(commandMode, writableCapabilities) {
    const validation = validateCommandArgs(this.selectedNode?.data?.ui?.args_text, { commandMode, writableCapabilities });
    this.setState({ jsonValidation: validation });
    this.showToast(validation.valid ? 'JSON command is valid.' : validation.message);
  }

  render() {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return null;

    const { deviceOptions, groupOptions, selectedCapabilities = [] } = this.props;
    const writableCapabilities = selectedCapabilities.filter(capability => capability.writable && capability.type !== 'unsupported');
    const capabilityCommands = Array.isArray(selectedNode.data?.capability_commands) && selectedNode.data.capability_commands.length > 0
      ? selectedNode.data.capability_commands
      : [{ id: selectedNode.data?.capability_id || '', property: selectedNode.data?.capability_property || '', value: selectedNode.data?.capability_value }];
    const selectedCapability = writableCapabilities.find(capability => capability.id === capabilityCommands[0]?.id) || null;
    const updateCapabilityCommand = (index, patch) => {
      const commands = capabilityCommands.map((command, commandIndex) => commandIndex === index ? { ...command, ...patch } : command);
      this.setSelectedNodeData({
        capability_commands: commands,
        capability_id: commands[0]?.id || '',
        capability_property: commands[0]?.property || '',
        capability_value: commands[0]?.value,
      });
    };
    const cmd = String(selectedNode.data?.command || '').trim() || 'set_state';
    const commandMode = String(selectedNode.data?.ui?.command_mode || (cmd === 'set_state' ? 'set_state' : 'custom'));
    const effectiveArgsMode = commandMode === 'custom' ? 'json' : (selectedNode.data?.ui?.args_mode || 'builder');

    const targetsType = String(selectedNode.data?.targets?.type || 'device').toLowerCase();
    const selector = String(selectedNode.data?.targets?.selector || '').trim();
    const isGroupSelector = targetsType === 'selector' && selector.toLowerCase().startsWith('group:');
    const targetMode = String(selectedNode.data?.ui?.target_mode || (targetsType === 'device' ? 'device' : (isGroupSelector ? 'group' : 'selector')));
    const selectedDeviceId = targetsType === 'device' && Array.isArray(selectedNode.data?.targets?.ids)
      ? String(selectedNode.data?.targets?.ids?.[0] || '')
      : '';
    const selectedGroupSelector = isGroupSelector ? selector : '';
    const selectedDevice = deviceOptions.find(device => device.id === selectedDeviceId);

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
                this.setSelectedNodeUI({ target_mode: 'device' });
                return;
              }
              if (value === 'group') {
                this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector: '' } });
                this.setSelectedNodeUI({ target_mode: 'group' });
                return;
              }
              this.setSelectedNodeData({ targets: { type: 'selector', ids: [], selector } });
              this.setSelectedNodeUI({ target_mode: 'selector' });
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
          </div>
        )}
        {selectedDevice && <div className="muted"><DeviceIcon device={selectedDevice.raw} /> {selectedDevice.label}</div>}

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

        {commandMode === 'set_state' && (
          <div className="field">
            <label className="label">State editor</label>
            <div className="automation-segmented slider" role="tablist" aria-label="Args editor mode" style={{ '--seg-pos': effectiveArgsMode === 'builder' ? 0 : 1 }}>
              <button type="button" role="tab" aria-selected={effectiveArgsMode === 'builder'} className={effectiveArgsMode === 'builder' ? 'active' : ''} onClick={() => this.switchArgsMode('builder', writableCapabilities)}>Builder</button>
              <button type="button" role="tab" aria-selected={effectiveArgsMode === 'json'} className={effectiveArgsMode === 'json' ? 'active' : ''} onClick={() => this.switchArgsMode('json', writableCapabilities)}>JSON</button>
            </div>
          </div>
        )}

        {commandMode === 'set_state' && effectiveArgsMode !== 'json' && (
          <>
            {targetMode === 'selector' && !isGroupSelector && (
              <div className="muted">Capability metadata cannot be resolved for an arbitrary selector. Use advanced JSON.</div>
            )}
            {targetMode === 'group' && selectedGroupSelector && writableCapabilities.length === 0 && (
              <div className="muted">This group has no compatible writable capabilities.</div>
            )}
            {capabilityCommands.map((command, index) => {
              const capability = writableCapabilities.find(item => item.id === command.id);
              return <div className={`field ${index > 0 ? 'automation-capability-row' : ''}`} key={`${command.id || 'new'}-${index}`}>
                <div className="automation-capability-row-heading">
                  <label className="label">{index === 0 ? 'Capability' : `Additional capability ${index + 1}`}</label>
                  {index > 0 && (
                    <Button
                      type="button"
                      variant="secondary"
                      className="automation-btn-mini automation-capability-remove"
                      aria-label={`Remove capability ${index + 1}`}
                      title="Remove capability"
                      onClick={() => this.setSelectedNodeData({ capability_commands: capabilityCommands.filter((_, commandIndex) => commandIndex !== index) })}
                    >
                      <span className="btn-icon"><FontAwesomeIcon icon={faTrash} /></span>
                    </Button>
                  )}
                </div>
                <GlassSelect value={command.id || ''} onChange={(value) => { const next = writableCapabilities.find(item => item.id === value); updateCapabilityCommand(index, { id: next?.id || '', property: next?.property || '', value: next?.type === 'binary' ? false : '' }); }} placeholder="Select a writable capability…" ariaLabel={`Capability ${index + 1}`} options={writableCapabilities.map(item => ({ value: item.id, label: <><CapabilityIcon capability={item} /> {item.label}</> }))} />
                {capability?.type === 'binary' && <label className="checkbox"><input type="checkbox" checked={Boolean(command.value)} onChange={e => updateCapabilityCommand(index, { value: e.target.checked })} /> On</label>}
                {capability?.type === 'numeric' && <input className="input" type="number" min={capability.min ?? undefined} max={capability.max ?? undefined} step={capability.step ?? 'any'} value={command.value ?? ''} onChange={e => updateCapabilityCommand(index, { value: Number(e.target.value) })} />}
                {capability?.type === 'enum' && <select className="input" value={command.value ?? ''} onChange={e => updateCapabilityCommand(index, { value: e.target.value })}><option value="">Select…</option>{capability.enumValues.map(value => <option key={value} value={value}>{value}</option>)}</select>}
                {capability?.type === 'string' && <input className="input" value={command.value ?? ''} onChange={e => updateCapabilityCommand(index, { value: e.target.value })} />}
                {capability?.type === 'object' && <textarea className="input textarea" rows={4} value={typeof command.value === 'string' ? command.value : JSON.stringify(command.value ?? {}, null, 2)} onChange={e => { try { updateCapabilityCommand(index, { value: JSON.parse(e.target.value) }); } catch { updateCapabilityCommand(index, { value: e.target.value }); } }} placeholder='e.g. { "h": 30, "s": 80 }' />}
              </div>;
            })}
            <div className="automation-props-actions">
              <Button
                type="button"
                className="automation-btn-mini"
                disabled={writableCapabilities.length === 0}
                title={writableCapabilities.length === 0 ? 'No writable capabilities are available' : 'Add capability'}
                onClick={() => this.setSelectedNodeData({ capability_commands: [...capabilityCommands, { id: '', property: '', value: undefined }] })}
              >
                <span className="btn-icon"><FontAwesomeIcon icon={faPlus} /></span>
                <span className="btn-label">Add capability</span>
              </Button>
            </div>
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
        ) : null}

        {(commandMode === 'custom' || effectiveArgsMode === 'json') && (
          <div className="field">
            <Button
              type="button"
              variant="secondary"
              className="automation-json-validate-btn"
              onClick={() => this.validateJson(commandMode, writableCapabilities)}
            >
              <span className="btn-icon"><FontAwesomeIcon icon={faCircleCheck} /></span>
              <span className="btn-label">Validate JSON</span>
            </Button>
            {this.state.jsonValidation && (
              <div className="muted" role="status" aria-live="polite">{this.state.jsonValidation.message}</div>
            )}
          </div>
        )}

        <div className="automation-props-section automation-settings-section">
          <div className="automation-props-section-title">Execution settings</div>
          <div className="automation-props-section-subtitle">Control how this command completes.</div>
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
      </div>
    );
  }
}
