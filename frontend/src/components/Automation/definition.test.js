import { describe, expect, it } from 'vitest';

import { buildDefinitionFromEditor, canWorkflowRunNow, workflowHasManualTrigger } from './definition';

describe('automation sleep node', () => {
  it('keeps fractional sleep durations in the saved definition', () => {
    const definition = buildDefinitionFromEditor({
      workflowName: 'Staggered lights',
      nodes: [
        { id: 'trigger-1', kind: 'trigger.manual', x: 0, y: 0, data: {} },
        { id: 'sleep-1', kind: 'logic.sleep', x: 120, y: 0, data: { duration_sec: 0.2 } },
      ],
      edges: [{ from: 'trigger-1', to: 'sleep-1' }],
    });

    expect(definition.nodes.find((node) => node.id === 'sleep-1')?.data?.duration_sec).toBe(0.2);
  });

  it('rejects negative sleep durations', () => {
    expect(() => buildDefinitionFromEditor({
      workflowName: 'Invalid stagger',
      nodes: [
        { id: 'trigger-1', kind: 'trigger.manual', x: 0, y: 0, data: {} },
        { id: 'sleep-1', kind: 'logic.sleep', x: 120, y: 0, data: { duration_sec: -0.2 } },
      ],
      edges: [{ from: 'trigger-1', to: 'sleep-1' }],
    })).toThrow('Sleep node duration must be >= 0');
  });
});

describe('workflow run availability', () => {
  it('detects manual triggers in workflow definitions', () => {
    expect(workflowHasManualTrigger({
      definition: {
        version: 'automation',
        nodes: [{ id: 'trigger-1', kind: 'trigger.manual', data: {} }],
        edges: [],
      },
    })).toBe(true);

    expect(workflowHasManualTrigger({
      definition: {
        version: 'automation',
        nodes: [{ id: 'trigger-1', kind: 'trigger.schedule', data: {} }],
        edges: [],
      },
    })).toBe(false);
  });

  it('only allows direct runs for enabled workflows with a manual trigger', () => {
    expect(canWorkflowRunNow({
      enabled: true,
      definition: {
        version: 'automation',
        nodes: [{ id: 'trigger-1', kind: 'trigger.manual', data: {} }],
        edges: [],
      },
    })).toBe(true);

    expect(canWorkflowRunNow({
      enabled: false,
      definition: {
        version: 'automation',
        nodes: [{ id: 'trigger-1', kind: 'trigger.manual', data: {} }],
        edges: [],
      },
    })).toBe(false);

    expect(canWorkflowRunNow({
      enabled: true,
      definition: {
        version: 'automation',
        nodes: [{ id: 'trigger-1', kind: 'trigger.schedule', data: {} }],
        edges: [],
      },
    })).toBe(false);
  });
});

describe('capability-driven command actions', () => {
  it('preserves the capability value instead of replacing it with legacy state fields', () => {
    const definition = buildDefinitionFromEditor({
      nodes: [
        { id: 'trigger-1', kind: 'trigger.manual', x: 0, y: 0, data: {} },
        { id: 'action-1', kind: 'action.send_command', x: 0, y: 0, data: {
          targets: { type: 'device', ids: ['device-1'] }, command: 'set_state',
          capability_id: 'setpoint', capability_property: 'setpoint', capability_value: 21,
          ui: { args_mode: 'builder', state: 'ON' },
        } },
      ],
      edges: [{ from: 'trigger-1', to: 'action-1' }],
    });
    expect(definition.nodes.find(node => node.id === 'action-1')?.data.args).toEqual({ setpoint: 21 });
  });
});

describe('same-device trigger conditions', () => {
  it('serializes the primary and additional capability conditions as an AND array', () => {
    const definition = buildDefinitionFromEditor({
      nodes: [{ id: 'trigger-1', kind: 'trigger.device_state', x: 0, y: 0, data: {
        targets: { type: 'device', ids: ['device-1'] }, capability_id: 'brightness', capability_property: 'brightness',
        key: 'brightness', op: 'gte', ui: { value_mode: 'json', value_text: '80' },
        additional_conditions: [{ capability_id: 'color_mode', key: 'color_mode', op: 'eq', value_text: '"warm"' }],
      } }],
      edges: [],
    });
    expect(definition.nodes[0].data.conditions).toEqual([
      { key: 'brightness', op: 'gte', value: 80 },
      { key: 'color_mode', op: 'eq', value: 'warm' },
    ]);
  });

  it('serializes Builder values for additional conditions', () => {
    const definition = buildDefinitionFromEditor({
      nodes: [{ id: 'trigger-1', kind: 'trigger.device_state', x: 0, y: 0, data: {
        targets: { type: 'device', ids: ['device-1'] }, key: 'power', op: 'eq', ui: { value_mode: 'builder', value_type: 'boolean', value_bool: true },
        additional_conditions: [{ key: 'temperature', op: 'gte', value_mode: 'builder', value_type: 'number', value_number: '21' }],
      } }],
      edges: [],
    });
    expect(definition.nodes[0].data.conditions).toEqual([
      { key: 'power', op: 'eq', value: true },
      { key: 'temperature', op: 'gte', value: 21 },
    ]);
  });
});