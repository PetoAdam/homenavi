const defaultForm = {
  protocol: '',
  name: '',
  type: '',
  manufacturer: '',
  model: '',
  description: '',
  icon: 'auto',
};

export function createDefaultAddDeviceForm() {
  return { ...defaultForm };
}

export function createAddDeviceModalInitialState() {
  return {
    form: createDefaultAddDeviceForm(),
    showAdvanced: false,
    flowStep: 'form',
    activePairing: null,
    pairingNotice: '',
    pairingError: null,
    secondsRemaining: null,
    stopPending: false,
    pairingStartPending: false,
    pairingSetupOverride: false,
    pairingRecoveryDraft: null,
    pairingTerminalContext: null,
    pairedDevicesSummary: [],
  };
}

export function addDeviceModalReducer(state, action) {
  switch (action.type) {
    case 'reset-flow':
      return {
        ...state,
        showAdvanced: false,
        flowStep: 'form',
        activePairing: null,
        pairingNotice: '',
        pairingError: null,
        secondsRemaining: null,
        stopPending: false,
        pairingStartPending: false,
        pairingSetupOverride: false,
        pairingRecoveryDraft: null,
        pairingTerminalContext: null,
        pairedDevicesSummary: [],
      };
    case 'reset-all':
      return createAddDeviceModalInitialState();
    case 'set-form':
      return {
        ...state,
        form: {
          ...createDefaultAddDeviceForm(),
          ...(action.form || {}),
        },
      };
    case 'update-form': {
      const updater = action.updater;
      if (typeof updater !== 'function') {
        return state;
      }
      return {
        ...state,
        form: updater(state.form),
      };
    }
    case 'set-show-advanced':
      return {
        ...state,
        showAdvanced: Boolean(action.value),
      };
    case 'patch':
      return {
        ...state,
        ...(action.partial || {}),
      };
    default:
      return state;
  }
}
