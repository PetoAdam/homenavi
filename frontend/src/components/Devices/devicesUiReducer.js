export const devicesUiInitialState = {
  pendingCommands: {},
  commandError: null,
  showAddModal: false,
};

export function devicesUiReducer(state, action) {
  switch (action.type) {
    case 'set-command-error':
      return {
        ...state,
        commandError: action.message || null,
      };
    case 'clear-command-error':
      return {
        ...state,
        commandError: null,
      };
    case 'open-add-modal':
      return {
        ...state,
        showAddModal: true,
      };
    case 'close-add-modal':
      return {
        ...state,
        showAddModal: false,
      };
    case 'set-pending-command':
      return {
        ...state,
        pendingCommands: {
          ...state.pendingCommands,
          [action.id]: action.pending,
        },
      };
    case 'clear-pending-command': {
      if (!state.pendingCommands[action.id]) return state;
      const next = { ...state.pendingCommands };
      delete next[action.id];
      return {
        ...state,
        pendingCommands: next,
      };
    }
    case 'update-pending-commands': {
      const updater = action.updater;
      if (typeof updater !== 'function') return state;
      const nextPending = updater(state.pendingCommands);
      if (!nextPending || typeof nextPending !== 'object') {
        return state;
      }
      if (nextPending === state.pendingCommands) {
        return state;
      }
      return {
        ...state,
        pendingCommands: nextPending,
      };
    }
    default:
      return state;
  }
}
