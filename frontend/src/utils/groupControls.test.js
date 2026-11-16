import { describe, expect, it } from 'vitest';

import {
  buildInitialControlValues,
  buildNormalizedInputs,
  buildPayloadForInput,
  buildPresentationInputs,
  buildSharedFieldCapabilities,
  buildSharedFieldState,
  canToggleDevice,
  collectCommonFieldKeys,
  findToggleInput,
  intersectSharedInputs,
} from './groupControls';
import { applyDevicePresentation } from './devicePresentation';

describe('buildInitialControlValues', () => {
  it('does not invent numeric values for missing or null state', () => {
    const inputs = [
      { id: 'target', property: 'target', type: 'slider', range: { min: 5, max: 35, step: 0.5 }, ui: { control: 'stepper' } },
      { id: 'level', property: 'level', type: 'number' },
    ];
    expect(buildInitialControlValues({ state: { level: null } }, inputs)).toEqual({ target: '', level: '' });
  });
});

it('preserves explicit range controls and their limits', () => {
  const property = 'temperature.FRIDGE.C.targetTemperature';
  const range = { min: 1, max: 7, step: 1, except: [3] };
  const device = {
    inputs: [{ id: 'fridge-temperature', type: 'range', property, capability_id: 'fridge', range }],
    capabilities: [{ id: 'fridge', property, value_type: 'number', access: { write: true } }],
  };
  expect(buildNormalizedInputs(device)).toMatchObject([{ id: 'fridge-temperature', type: 'slider', property, range }]);
  device.meta = { presentation: { controls: { 'fridge-temperature': 'stepper' } } };
  expect(applyDevicePresentation(device).inputs[0].ui.control).toBe('stepper');
});

describe('intersectSharedInputs', () => {
  it('omits matching IDs that target different command properties', () => {
    const members = ['heat', 'cool'].map(property => ({
      state: { [property]: 20 },
      inputs: [{ id: 'set_target', property, type: 'number', range: { min: 0, max: 30, step: 1 } }],
    }));
    expect(intersectSharedInputs(members).inputs).toEqual([]);
    members[1].inputs[0].property = 'Heat';
    expect(intersectSharedInputs(members).inputs).toEqual([]);
    members[1].inputs[0].property = 'heat';
    expect(intersectSharedInputs(members).inputs).toHaveLength(1);
  });

  it('omits ambiguous IDs and incompatible toggle payloads', () => {
    const members = [false, true].map(togglePowerString => ({
      state: { on: true },
      inputs: [{ id: 'switch', property: 'on', type: 'toggle', metadata: { togglePowerString } }],
    }));
    expect(intersectSharedInputs(members).inputs).toEqual([]);
    members[1].inputs[0].metadata.togglePowerString = false;
    expect(intersectSharedInputs(members).inputs).toHaveLength(1);
    members[1].inputs.push({ id: 'SWITCH', property: 'heat', type: 'toggle' });
    expect(intersectSharedInputs(members).inputs).toEqual([]);
  });

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

  it('uses the default control when members disagree on presentation', () => {
    const input = { id: 'target', type: 'slider', property: 'target', range: { min: 5, max: 35, step: 0.5 } };
    const devices = [
      { state: { target: 20 }, inputs: [{ ...input, ui: { control: 'stepper' } }] },
      { state: { target: 20 }, inputs: [input] },
    ];
    expect(intersectSharedInputs(devices).inputs[0].ui.control).toBeUndefined();
  });

  it.each([
    [[{ min: 0, max: 30, step: 2 }, { min: 0, max: 30, step: 3 }], { min: 0, max: 30, step: 6 }],
    [[{ min: 0, max: 30, step: 2 }, { min: 1, max: 30, step: 3 }], { min: 4, max: 30, step: 6 }],
    [[{ min: 0, max: 3, step: 0.2 }, { min: 0.1, max: 3, step: 0.3 }], { min: 0.4, max: 3, step: 0.6 }],
    [[{ max: -1, step: 2 }, { max: -1, step: 3 }], { max: -1, step: 6 }],
    [[{ min: 0, max: 200000000, step: 10001 }, { min: 0, max: 200000000, step: 10002 }], { min: 0, max: 200000000, step: 100030002 }],
    [[{ min: 0, max: 200000000, step: 10001 }, { min: 1, max: 200000000, step: 10002 }], { min: 100020001, max: 200000000, step: 100030002 }],
    [[{ min: 0, max: 1, step: 0.000000001 }], { min: 0, max: 1, step: 0.000000001 }],
  ])('intersects numeric step grids', (ranges, expected) => {
    const devices = ranges.map(range => ({ state: { target: 0 }, inputs: [{ id: 'target', type: 'number', range, ui: { control: 'stepper' } }] }));
    expect(intersectSharedInputs(devices).inputs[0].range).toEqual(expected);
  });

  it('omits a group control when member step grids cannot intersect', () => {
    const devices = [0, 1].map(min => ({ state: { target: min }, inputs: [{ id: 'target', type: 'number', range: { min, max: 10, step: 2 } }] }));
    expect(intersectSharedInputs(devices).inputs).toEqual([]);
  });

  it('combines excluded values across member ranges', () => {
    const devices = [1, 2].map(excluded => ({ state: { target: 0 }, inputs: [{ id: 'target', type: 'number', range: { min: 0, max: 2, step: 1, except: [excluded] } }] }));
    expect(intersectSharedInputs(devices).inputs[0].range.except).toEqual([1, 2]);
    devices[0].inputs[0].range.except.push(0);
    expect(intersectSharedInputs(devices).inputs).toEqual([]);
  });

  it('omits grids that cannot be represented safely at their precision', () => {
    const device = { state: { target: 0 }, inputs: [{ id: 'target', type: 'number', range: { min: 1000000.0000000001, max: 1000010, step: 1 } }] };
    expect(intersectSharedInputs([device]).inputs).toEqual([]);
  });

  it('omits grids that cannot be scaled to safe integers', () => {
    const device = { state: { target: 1000000000000000.1 }, inputs: [{ id: 'target', type: 'number', range: { min: 1000000000000000.1, max: 1000000000000010, step: 1 } }] };
    expect(intersectSharedInputs([device]).inputs).toEqual([]);
  });

  it('retains a shared power control when select preferences disagree', () => {
    const input = { id: 'power', property: 'power', type: 'select', options: [{ value: 'on' }, { value: 'off' }] };
    const devices = [{ state: { power: 'on' }, inputs: [input] }, { state: { power: 'off' }, inputs: [{ ...input, ui: { control: 'radio' } }] }];
    const control = intersectSharedInputs(devices).inputs[0];
    expect(control).toMatchObject({ type: 'toggle', metadata: { togglePowerString: true } });
    expect(buildPayloadForInput(control, true)).toEqual({ state: { power: 'on' } });
  });

  it('applies saved preferences to inferred writable controls', () => {
    const device = { state: { target: 20 }, capabilities: [{ id: 'target', kind: 'numeric', value_type: 'number', range: { min: 0, max: 100 }, access: { write: true } }], meta: { presentation: { controls: { target: 'stepper' } } } };
    expect(intersectSharedInputs([device, device]).inputs[0].ui.control).toBe('stepper');
  });

  it('does not duplicate an explicit input with its inferred capability', () => {
    const device = {
      inputs: [{ id: 'set_target_temperature', capability_id: 'climate.target_temperature', property: 'target_temperature', type: 'slider', range: { min: 5, max: 35 } }],
      capabilities: [{ id: 'climate.target_temperature', property: 'target_temperature', kind: 'numeric', value_type: 'number', range: { min: 5, max: 35 }, access: { write: true } }],
      state: { target_temperature: 20 },
    };
    expect(buildPresentationInputs(device).map(input => input.id)).toEqual(['set_target_temperature']);
    expect(intersectSharedInputs([device, device]).inputs.map(input => input.id)).toEqual(['set_target_temperature']);
  });

  it('retains independently writable namespaced capabilities', () => {
    const capabilities = ['left.temperature', 'right.temperature'].map(id => ({ id, property: id, kind: 'numeric', value_type: 'number', access: { write: true } }));
    expect(buildPresentationInputs({ capabilities }).map(input => input.id)).toEqual(['left.temperature', 'right.temperature']);
  });

  it('disables relative group controls when a member has no value', () => {
    const input = { id: 'target', type: 'number', ui: { control: 'stepper' } };
    const result = intersectSharedInputs([{ state: { target: 20 }, inputs: [input] }, { state: {}, inputs: [input] }]);
    expect(result.inputs[0].mixed).toBe(true);
    expect(result.mixedKeys).toEqual(['target']);
  });

  it('uses the explicitly linked capability when validating a preference', () => {
    const device = {
      inputs: [{ id: 'set_target', capability_id: 'numeric.target', property: 'target', type: 'number' }],
      capabilities: [{ id: 'binary.target', property: 'target', value_type: 'boolean' }, { id: 'numeric.target', property: 'target', value_type: 'number' }],
      meta: { presentation: { controls: { set_target: 'stepper' } } },
    };
    expect(applyDevicePresentation(device).inputs[0].ui.control).toBe('stepper');
    device.inputs[0].capability_id = 'missing.target';
    expect(applyDevicePresentation(device).inputs[0].ui).toBeUndefined();
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

  it('shares display hints only when all members agree on compatible metadata', () => {
    const capability = { id: 'temperature', value_type: 'number', unit: 'C', range: { min: 0, max: 100 }, ui: { display: 'gauge' } };
    const members = [{ capabilities: [capability] }, { capabilities: [{ ...capability }] }];
    expect(buildSharedFieldCapabilities(members, ['temperature'])).toEqual([capability]);
    members[1].capabilities[0] = { ...capability, ui: { display: 'badge' } };
    expect(buildSharedFieldCapabilities(members, ['temperature'])).toEqual([]);
    members[1].capabilities[0] = { ...capability, range: { min: 0, max: 50 } };
    expect(buildSharedFieldCapabilities(members, ['temperature'])).toEqual([]);
  });
});

describe('buildPayloadForInput', () => {
  it('converts toggle power controls to on/off payloads', () => {
    expect(buildPayloadForInput({ id: 'power', type: 'toggle', property: 'power' }, true)).toEqual({
      state: { power: 'on' },
    });
  });
});

describe('toggle helpers', () => {
  it('finds a fallback toggle input from writable capabilities', () => {
    const device = {
      state: { on: true },
      capabilities: [
        { id: 'on', property: 'on', kind: 'binary', value_type: 'boolean', access: { write: true } },
      ],
    };

    expect(findToggleInput(device)).toMatchObject({ id: 'on', type: 'toggle', property: 'on' });
    expect(canToggleDevice(device)).toBe(true);
  });
});