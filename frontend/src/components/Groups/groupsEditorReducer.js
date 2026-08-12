export function groupsEditorInitialState() {
  return {
    editorOpen: false,
    editingGroup: null,
    editorError: '',
    savePending: false,
    deleteTarget: null,
    deletePending: false,
    deleteError: '',
  };
}

export function groupsEditorReducer(state, action) {
  switch (action?.type) {
    case 'open-create':
      return {
        ...state,
        editorOpen: true,
        editingGroup: null,
        editorError: '',
      };
    case 'open-edit':
      return {
        ...state,
        editorOpen: true,
        editingGroup: action.group || null,
        editorError: '',
      };
    case 'close-editor':
      if (state.savePending) return state;
      return {
        ...state,
        editorOpen: false,
        editingGroup: null,
        editorError: '',
      };
    case 'set-editor-error':
      return {
        ...state,
        editorError: `${action.value || ''}`,
      };
    case 'set-save-pending':
      return {
        ...state,
        savePending: Boolean(action.value),
      };
    case 'open-delete':
      return {
        ...state,
        deleteTarget: action.group || null,
        deleteError: '',
      };
    case 'close-delete':
      if (state.deletePending) return state;
      return {
        ...state,
        deleteTarget: null,
        deleteError: '',
      };
    case 'set-delete-error':
      return {
        ...state,
        deleteError: `${action.value || ''}`,
      };
    case 'set-delete-pending':
      return {
        ...state,
        deletePending: Boolean(action.value),
      };
    default:
      return state;
  }
}
