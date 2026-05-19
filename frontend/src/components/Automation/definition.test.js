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