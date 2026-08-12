export function userSettingsInitialState(user = null) {
  return {
    emailVerified: Boolean(user?.email_confirmed),
    twoFAEnabled: Boolean(user?.two_factor_enabled),
    status: '',
    emailCode: '',
    twoFACode: '',
    showEmailCodeInput: false,
    show2FACodeInput: false,
    editingProfile: false,
    profileForm: {
      firstName: user?.first_name || '',
      lastName: user?.last_name || '',
    },
    showPasswordReset: false,
    passwordForm: {
      currentPassword: '',
      newPassword: '',
      confirmPassword: '',
    },
    showProfilePictureModal: false,
    profilePictureFile: null,
  };
}

export function userSettingsReducer(state, action) {
  switch (action?.type) {
    case 'reset-from-user':
      return {
        ...state,
        emailVerified: Boolean(action.user?.email_confirmed),
        twoFAEnabled: Boolean(action.user?.two_factor_enabled),
        profileForm: {
          firstName: action.user?.first_name || '',
          lastName: action.user?.last_name || '',
        },
      };
    case 'set-field':
      return { ...state, [action.key]: action.value };
    case 'update-profile-form':
      return {
        ...state,
        profileForm: {
          ...state.profileForm,
          ...(action.value && typeof action.value === 'object' ? action.value : {}),
        },
      };
    case 'update-password-form':
      return {
        ...state,
        passwordForm: {
          ...state.passwordForm,
          ...(action.value && typeof action.value === 'object' ? action.value : {}),
        },
      };
    case 'reset-password-form':
      return {
        ...state,
        passwordForm: {
          currentPassword: '',
          newPassword: '',
          confirmPassword: '',
        },
      };
    default:
      return state;
  }
}
