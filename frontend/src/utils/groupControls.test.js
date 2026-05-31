import { describe, expect, it } from 'vitest';

import {
  buildPayloadForInput,
  buildSharedFieldState,
  collectCommonFieldKeys,
  intersectSharedInputs,
} from './groupControls';

describe('intersectSharedInputs', () => {
  it('keeps controls shared across all group members and marks mixed values', () => {
    const devices = [
      {
        id: 'zigbee/a',
        state: { on: true, brightness: 20 },
        inputs: [
          { id: 'on', type: 'toggle', property: 'on' },
          { id: 'brightness', type: 'slider', property: 'brightness', range: { min: 0, max: 100, step: 1 } },
        ],
      },
      {
        id: 'zigbee/b',
        state: { on: false, brightness: 80 },
        inputs: [
          { id: 'on', type: 'toggle', property: 'on' },
          { id: 'brightness', type: 'slider', property: 'brightness', range: { min: 10, max: 90, step: 5 } },
        ],
      },
    ];

    const result = intersectSharedInputs(devices);

    expect(result.inputs.map((input) => input.id)).toEqual(['on', 'brightness']);
    expect(result.mixedKeys).toEqual(['on', 'brightness']);
    expect(result.inputs[1].range).toMatchObject({ min: 10, max: 90, step: 5 });
  });
});

describe('group shared fields', () => {
  const devices = [
    { state: { temperature: 21.5, humidity: 40, on: true } },
    { state: { temperature: 21.5, humidity: 55, on: false } },
  ];

  it('collects fields common to all members', () => {
    expect(collectCommonFieldKeys(devices)).toEqual(['humidity', 'on', 'temperature']);
  });

  it('marks shared field values as mixed when members diverge', () => {
    expect(buildSharedFieldState(devices, ['temperature', 'humidity'])).toEqual({
      temperature: 21.5,
      humidity: 'Mixed',
    });
  });
});

describe('buildPayloadForInput', () => {
  it('converts toggle power controls to on/off payloads', () => {
    expect(buildPayloadForInput({ id: 'power', type: 'toggle', property: 'power' }, true)).toEqual({
      state: { power: 'on' },
    });
  });
});