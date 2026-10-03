import { describe, expect, it } from 'vitest';
import { capabilitiesForDevice, compatibleCapabilities, operatorsForCapability } from './capabilityModel';

const bulb = {
  capabilities: [
    { id: 'state', property: 'state', name: 'State', value_type: 'boolean', access: { read: true, write: true } },
    { id: 'brightness', property: 'brightness', value_type: 'number', access: { read: true, write: true }, range: { min: 0, max: 254, step: 1 } },
  ],
};

describe('automation capability model', () => {
  it('normalizes adapter-provided capabilities into typed controls', () => {
    expect(capabilitiesForDevice(bulb)).toMatchObject([
      { id: 'state', type: 'binary', writable: true },
      { id: 'brightness', type: 'numeric', min: 0, max: 254, step: 1 },
    ]);
  });

  it('only exposes compatible writable capabilities for groups', () => {
    expect(compatibleCapabilities([bulb, { capabilities: [bulb.capabilities[0]] }], { writable: true }).map(cap => cap.id)).toEqual(['state']);
  });

  it('limits comparison operators by capability type', () => {
    expect(operatorsForCapability(capabilitiesForDevice(bulb)[0])).toEqual(['exists', 'changed', 'eq', 'neq']);
    expect(operatorsForCapability(capabilitiesForDevice(bulb)[1])).toContain('gte');
  });

});