import { describe, expect, it } from 'vitest';

import { deriveLiveRunNodeStatesFromRunPayload, isTriggerRunEvent } from './runStreamUtils';

describe('isTriggerRunEvent', () => {
  it('identifies manual and event trigger nodes', () => {
    expect(isTriggerRunEvent({ node_kind: 'trigger.manual' })).toBe(true);
    expect(isTriggerRunEvent({ node_kind: 'trigger.device_state' })).toBe(true);
  });

  it('does not identify action or logic nodes as triggers', () => {
    expect(isTriggerRunEvent({ node_kind: 'action.send_command' })).toBe(false);
    expect(isTriggerRunEvent({ node_kind: 'logic.sleep' })).toBe(false);
  });

  it('derives active, done, and failed node states from polled run payloads', () => {
    expect(deriveLiveRunNodeStatesFromRunPayload({
      steps: [
        { node_id: 'trigger-1', status: 'success' },
        { node_id: 'sleep-1', status: 'running' },
        { node_id: 'action-1', status: 'failed' },
      ],
    })).toEqual({
      'trigger-1': 'done',
      'sleep-1': 'active',
      'action-1': 'failed',
    });
  });
});