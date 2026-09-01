import { describe, expect, it } from 'vitest';

import { isTriggerRunEvent } from './runStreamUtils';

describe('isTriggerRunEvent', () => {
  it('identifies manual and event trigger nodes', () => {
    expect(isTriggerRunEvent({ node_kind: 'trigger.manual' })).toBe(true);
    expect(isTriggerRunEvent({ node_kind: 'trigger.device_state' })).toBe(true);
  });

  it('does not identify action or logic nodes as triggers', () => {
    expect(isTriggerRunEvent({ node_kind: 'action.send_command' })).toBe(false);
    expect(isTriggerRunEvent({ node_kind: 'logic.sleep' })).toBe(false);
  });
});