export const deviceDetailHistoryInitialState = {
  rangePreset: '24h',
  fromLocal: '',
  toLocal: '',
  limitEnabled: false,
  limit: 300,
  order: 'desc',
  historyLoading: false,
  historyError: null,
  historyPoints: [],
  overlay: null,
  overlayPhase: '',
};

export function deviceDetailHistoryReducer(state, action) {
  switch (action.type) {
    case 'set-field': {
      const key = action.key;
      if (!(key in state)) return state;
      if (Object.is(state[key], action.value)) return state;
      return {
        ...state,
        [key]: action.value,
      };
    }
    case 'update-field': {
      const key = action.key;
      if (!(key in state)) return state;
      const updater = action.updater;
      if (typeof updater !== 'function') return state;
      const nextValue = updater(state[key]);
      if (Object.is(state[key], nextValue)) return state;
      return {
        ...state,
        [key]: nextValue,
      };
    }
    case 'set-overlay': {
      const overlay = action.overlay;
      const overlayPhase = action.overlayPhase ?? state.overlayPhase;
      if (Object.is(state.overlay, overlay) && Object.is(state.overlayPhase, overlayPhase)) {
        return state;
      }
      return {
        ...state,
        overlay,
        overlayPhase,
      };
    }
    case 'clear-overlay':
      if (!state.overlay && !state.overlayPhase) return state;
      return {
        ...state,
        overlay: null,
        overlayPhase: '',
      };
    default:
      return state;
  }
}
