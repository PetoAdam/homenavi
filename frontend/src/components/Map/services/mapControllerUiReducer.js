export function mapControllerUiInitialState() {
  return {
    mode: 'select',
    editEnabled: false,
    activeRoomId: '',
    activeVertexIndex: null,
    draft: null,
    hoverPoint: null,
    selectedDeviceId: '',
    activeWallIndex: null,
    mapError: '',
    opPending: false,
    roomNameEdit: '',
    expandedDeviceKey: '',
    favoritesEditorKey: '',
    insertCornerPreview: null,
    labelScale: 1,
    view: { scale: 1, tx: 0, ty: 0 },
    isMapPrepared: false,
    snapSettings: {
      vertex: true,
      edge: true,
      align: true,
      ortho: true,
      grid: false,
    },
    snapGuide: null,
  };
}

export function mapControllerUiReducer(state, action) {
  switch (action?.type) {
    case 'set-field': {
      const key = action.key;
      if (!key || !(key in state)) return state;
      const nextValue = action.value;
      const currentValue = state[key];
      if (typeof nextValue === 'function') {
        const updated = nextValue(currentValue);
        if (updated === currentValue) return state;
        return { ...state, [key]: updated };
      }
      if (currentValue === nextValue) return state;
      return { ...state, [key]: nextValue };
    }
    case 'merge-snap-settings':
      return {
        ...state,
        snapSettings: {
          ...state.snapSettings,
          ...(action.value && typeof action.value === 'object' ? action.value : {}),
        },
      };
    case 'reset-draft':
      return {
        ...state,
        draft: null,
        hoverPoint: null,
        activeWallIndex: null,
        mode: 'select',
      };
    case 'reset-favorites-editor':
      return {
        ...state,
        favoritesEditorKey: '',
      };
    default:
      return state;
  }
}
