import { describe, expect, it } from 'vitest';
import { resolveControlComponent, resolveDisplayComponent } from './deviceControlUtils';

describe('control presentation compatibility', () => {
  it('keeps the input type while selecting a compatible numeric control', () => {
    const input = { type: 'slider', capability: { value_type: 'number' }, ui: { control: 'stepper' } };
    expect(resolveControlComponent(input)).toBe('stepper');
    expect(input.type).toBe('slider');
  });

  it('falls back for incompatible types, values, and unknown components', () => {
    expect(resolveControlComponent({ type: 'select', ui: { control: 'stepper' } })).toBe('select');
    expect(resolveControlComponent({ type: 'number', capability: { value_type: 'boolean' }, ui: { control: 'stepper' } })).toBe('number');
    expect(resolveControlComponent({ type: 'toggle', ui: { control: 'unknown' } })).toBe('toggle');
    expect(resolveControlComponent({ type: 'number', ui: { control: 'slider' } })).toBe('number');
    expect(resolveControlComponent({ type: 'slider', range: { min: 0, max: 10, step: 0 }, ui: { control: 'stepper' } })).toBe('slider');
    expect(resolveControlComponent({ type: 'slider', capability: { value_type: 'integer' }, range: { min: 0, max: 10, step: 0.5 }, ui: { control: 'stepper' } })).toBe('slider');
  });
});

describe('state presentation compatibility', () => {
  it('accepts a bounded numeric gauge and rejects incompatible displays', () => {
    const capability = { value_type: 'number', range: { min: 0, max: 100 }, ui: { display: 'gauge' } };
    expect(resolveDisplayComponent(capability, 42)).toBe('gauge');
    expect(resolveDisplayComponent(capability, '42')).toBeNull();
    expect(resolveDisplayComponent({ ...capability, value_type: 'boolean' }, true)).toBeNull();
    expect(resolveDisplayComponent({ ...capability, range: {} }, 42)).toBeNull();
    expect(resolveDisplayComponent({ ...capability, ui: { display: 'unknown' } }, 42)).toBeNull();
  });
});