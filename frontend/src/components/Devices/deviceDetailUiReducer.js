export const deviceDetailUiInitialState = {
  pendingCommand: null,
  commandError: '',
  managementActionPending: '',
  managementActionState: null,
  managementActionError: '',
  ersMetaSaving: false,
  ersMetaError: '',
  groupingEditing: false,
  editRoomId: '',
  editTagIds: [],
  newTagName: '',
  favoriteSaving: false,
  favoriteError: '',
  favoriteFields: [],
};

export function deviceDetailUiReducer(state, action) {
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
    default:
      return state;
  }
}
