import { describe, expect, it } from 'vitest';
import { buildPairingPhaseStates, resolveCurrentPairingStep } from './pairingSchema';

describe('pairingSchema zigbee flow', () => {
  it('maps a realistic zigbee pairing sequence to stable step progression', () => {
    const statuses = [
      'active',
      'device_joined',
      'device_detected',
      'interviewing',
      'interview_complete',
      'completed',
    ];

    const indexes = statuses.map(status => resolveCurrentPairingStep(status).index);

    expect(indexes).toEqual([0, 1, 1, 2, 2, 3]);
    expect(indexes.every((index, position) => position === 0 || index >= indexes[position - 1])).toBe(true);
  });

  it('keeps completed as the only active step once the flow finishes', () => {
    const phases = buildPairingPhaseStates('completed');

    expect(phases.map(phase => phase.state)).toEqual([
      'complete',
      'complete',
      'complete',
      'active',
    ]);
  });
});