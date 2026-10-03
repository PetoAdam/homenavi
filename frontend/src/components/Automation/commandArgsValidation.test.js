import { describe, expect, it } from 'vitest';
import { buildArgsFromBuilder, validateCommandArgs } from './commandArgsValidation';

const writableCapabilities = [
  { id: 'on', property: 'on', label: 'Power', type: 'binary' },
  { id: 'brightness', property: 'brightness', label: 'Brightness', type: 'numeric', min: 0, max: 100 },
];

describe('validateCommandArgs', () => {
  it('converts a Power on Builder selection into command JSON', () => {
    expect(buildArgsFromBuilder({ capability_id: 'on', capability_property: 'on', capability_value: true })).toEqual({ valid: true, args: { on: true } });
  });

  it('converts and validates multiple writable capability patches', () => {
    expect(buildArgsFromBuilder({ capability_commands: [{ id: 'on', property: 'on', value: true }, { id: 'brightness', property: 'brightness', value: 80 }] })).toEqual({ valid: true, args: { on: true, brightness: 80 } });
    expect(validateCommandArgs('{"on":true,"brightness":80}', { writableCapabilities }).valid).toBe(true);
  });

  it('accepts a matching writable capability and typed value', () => {
    expect(validateCommandArgs('{"brightness": 80}', { writableCapabilities })).toMatchObject({ valid: true, capability: { id: 'brightness' } });
  });

  it('rejects malformed, unknown, and invalid typed values', () => {
    expect(validateCommandArgs('{', { writableCapabilities }).valid).toBe(false);
    expect(validateCommandArgs('{"unknown": true}', { writableCapabilities }).valid).toBe(false);
    expect(validateCommandArgs('{"on": "true"}', { writableCapabilities }).valid).toBe(false);
  });
});