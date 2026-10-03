import { describe, expect, it } from 'vitest';
import { triggerValueFromBuilder, triggerValueToBuilder } from './triggerValueConversion';

const temperature = { type: 'numeric', label: 'Temperature', min: -20, max: 50 };
const power = { type: 'binary', label: 'Power' };

describe('device state trigger value conversion', () => {
  it('converts a numeric Builder value to scalar JSON', () => {
    expect(triggerValueFromBuilder({ value_number: '35' }, temperature)).toEqual({ valid: true, value: 35, text: '35' });
  });

  it('converts JSON scalars to capability-matching Builder values', () => {
    expect(triggerValueToBuilder('35', temperature)).toEqual({ valid: true, value: 35, ui: { value_number: '35' } });
    expect(triggerValueToBuilder('true', power)).toEqual({ valid: true, value: true, ui: { value_bool: true } });
  });

  it('rejects malformed and incompatible trigger JSON', () => {
    expect(triggerValueToBuilder('{', temperature).valid).toBe(false);
    expect(triggerValueToBuilder('"35"', temperature).valid).toBe(false);
  });
});