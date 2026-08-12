import { describe, expect, it } from 'vitest';
import { userSettingsInitialState, userSettingsReducer } from './userSettingsReducer';

describe('userSettingsReducer', () => {
  it('initializes from the current user', () => {
    const state = userSettingsInitialState({
      email_confirmed: true,
      two_factor_enabled: false,
      first_name: 'Ada',
      last_name: 'Lovelace',
    });

    expect(state.emailVerified).toBe(true);
    expect(state.profileForm).toEqual({ firstName: 'Ada', lastName: 'Lovelace' });
  });

  it('updates nested form state', () => {
    const state = userSettingsReducer(userSettingsInitialState(), {
      type: 'update-profile-form',
      value: { firstName: 'Grace' },
    });
    const withPassword = userSettingsReducer(state, {
      type: 'update-password-form',
      value: { currentPassword: 'old' },
    });

    expect(state.profileForm.firstName).toBe('Grace');
    expect(withPassword.passwordForm.currentPassword).toBe('old');
  });

  it('resets password form', () => {
    const next = userSettingsReducer(userSettingsInitialState(), { type: 'reset-password-form' });
    expect(next.passwordForm).toEqual({ currentPassword: '', newPassword: '', confirmPassword: '' });
  });
});
