function withMapEntry(state, key, id, value) {
  return {
    ...state,
    [key]: {
      ...(state[key] || {}),
      [id]: value,
    },
  };
}

function withInstallStatus(state, id, patch) {
  return {
    ...state,
    installStatus: {
      ...(state.installStatus || {}),
      [id]: {
        ...(state.installStatus?.[id] || {}),
        ...patch,
      },
    },
  };
}

export const integrationsAdminOperationsInitialState = {
  reloading: false,
  restartingAll: false,
  restartAllTargets: [],
  installing: {},
  uninstalling: {},
  updating: {},
  restarting: {},
  installStatus: {},
};

export function integrationsAdminOperationsReducer(state, action) {
  switch (action.type) {
    case 'set-reloading':
      return {
        ...state,
        reloading: Boolean(action.value),
      };
    case 'set-restarting-all':
      return {
        ...state,
        restartingAll: Boolean(action.value),
      };
    case 'set-restart-all-targets':
      return {
        ...state,
        restartAllTargets: Array.isArray(action.ids) ? action.ids : [],
      };
    case 'set-installing':
      return withMapEntry(state, 'installing', action.id, Boolean(action.value));
    case 'set-uninstalling':
      return withMapEntry(state, 'uninstalling', action.id, Boolean(action.value));
    case 'set-updating':
      return withMapEntry(state, 'updating', action.id, Boolean(action.value));
    case 'set-restarting':
      return withMapEntry(state, 'restarting', action.id, Boolean(action.value));
    case 'set-install-status':
      return withInstallStatus(state, action.id, action.status || {});
    case 'queue-install':
      return withInstallStatus(
        withMapEntry(state, 'installing', action.id, true),
        action.id,
        { id: action.id, stage: 'queued', progress: 5, message: 'Queued' }
      );
    case 'queue-restart':
      return withInstallStatus(
        withMapEntry(state, 'restarting', action.id, true),
        action.id,
        { id: action.id, stage: 'queued', progress: 10, message: 'Queued for restart' }
      );
    case 'queue-update':
      return withInstallStatus(
        withMapEntry(state, 'updating', action.id, true),
        action.id,
        {
          id: action.id,
          stage: 'queued',
          progress: action.progress ?? 5,
          message: action.message || 'Queued',
        }
      );
    case 'queue-restart-all': {
      const ids = Array.isArray(action.ids) ? action.ids.filter(Boolean) : [];
      let next = {
        ...state,
        restartingAll: true,
        restartAllTargets: ids,
      };
      ids.forEach((id) => {
        next = withMapEntry(next, 'restarting', id, true);
        next = withInstallStatus(next, id, {
          id,
          stage: 'queued',
          progress: 10,
          message: 'Queued for restart',
        });
      });
      return next;
    }
    default:
      return state;
  }
}
