import { describe, expect, it } from 'vitest';
import { requiresTwoFactor } from './authModalState';

describe('requiresTwoFactor', () => {
  it('prioritizes a two-factor challenge even when credentials were accepted', () => {
    expect(requiresTwoFactor({ success: true, twoFA: true, type: 'email' })).toBe(true);
  });

  it('does not treat a completed sign-in as a two-factor challenge', () => {
    expect(requiresTwoFactor({ success: true })).toBe(false);
  });
});