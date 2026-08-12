export function automationUiInitialState(workflowIdParam = '') {
  return {
    viewMode: workflowIdParam ? 'edit' : 'overview',
    isNarrow: false,
    hasOverviewSelection: false,
    workflowSearch: '',
    loadedWorkflowId: null,
    selectedNodeId: 'workflow',
    workflowDrag: {
      dragId: '',
      overId: '',
      position: 'after',
    },
    editAutoFitDoneKey: '',
    previewAutoFitDoneKey: '',
  };
}

export function automationUiReducer(state, action) {
  switch (action?.type) {
    case 'set-view-mode':
      return { ...state, viewMode: action.value };
    case 'set-is-narrow':
      return { ...state, isNarrow: Boolean(action.value) };
    case 'set-has-overview-selection':
      return { ...state, hasOverviewSelection: Boolean(action.value) };
    case 'set-workflow-search':
      return { ...state, workflowSearch: `${action.value || ''}` };
    case 'set-loaded-workflow-id':
      return { ...state, loadedWorkflowId: action.value ?? null };
    case 'set-selected-node-id':
      return { ...state, selectedNodeId: `${action.value || ''}` || 'workflow' };
    case 'set-workflow-drag':
      return {
        ...state,
        workflowDrag: {
          ...(state.workflowDrag || {}),
          ...(action.value && typeof action.value === 'object' ? action.value : {}),
        },
      };
    case 'reset-workflow-drag':
      return {
        ...state,
        workflowDrag: {
          dragId: '',
          overId: '',
          position: 'after',
        },
      };
    case 'set-edit-autofit-done-key':
      return { ...state, editAutoFitDoneKey: `${action.value || ''}` };
    case 'set-preview-autofit-done-key':
      return { ...state, previewAutoFitDoneKey: `${action.value || ''}` };
    case 'reset-autofit-done-keys':
      return { ...state, editAutoFitDoneKey: '', previewAutoFitDoneKey: '' };
    default:
      return state;
  }
}
