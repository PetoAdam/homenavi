export function dashboardUiInitialState() {
  return {
    editMode: false,
    addModalOpen: false,
    settingsModalOpen: false,
    selectedWidgetId: null,
    dragOverTrash: false,
    currentBreakpoint: 'lg',
    desiredRowsByInstanceId: {},
  };
}

export function dashboardUiReducer(state, action) {
  switch (action?.type) {
    case 'set-edit-mode':
      return { ...state, editMode: action.value };
    case 'set-add-modal-open':
      return { ...state, addModalOpen: action.value };
    case 'set-settings-modal-open':
      return { ...state, settingsModalOpen: action.value };
    case 'set-selected-widget-id':
      return { ...state, selectedWidgetId: action.value };
    case 'set-drag-over-trash':
      return { ...state, dragOverTrash: action.value };
    case 'set-current-breakpoint':
      return { ...state, currentBreakpoint: action.value };
    case 'set-desired-row-height': {
      const instanceId = action.instanceId;
      if (!instanceId) return state;
      const next = { ...state.desiredRowsByInstanceId };
      if (action.value == null) {
        delete next[instanceId];
      } else {
        next[instanceId] = action.value;
      }
      return { ...state, desiredRowsByInstanceId: next };
    }
    case 'reset':
      return dashboardUiInitialState();
    default:
      return state;
  }
}
