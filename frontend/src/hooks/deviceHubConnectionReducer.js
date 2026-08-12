export function createDeviceHubConnectionInitialState(metadataMode = 'rest') {
  return {
    metadataStatus: { connected: false, source: metadataMode === 'ws' ? 'ws' : 'rest' },
    stateStatus: { connected: false, subscribed: false, firstStateReceived: false },
    realtimeMetrics: {
      authReadyMs: null,
      socketOpenMs: null,
      subscribeCompleteMs: null,
      firstStateReceivedMs: null,
    },
  };
}

export function deviceHubConnectionReducer(state, action) {
  switch (action?.type) {
    case 'reset':
      return createDeviceHubConnectionInitialState(action.metadataMode);
    case 'set-metadata-status':
      return {
        ...state,
        metadataStatus: {
          ...state.metadataStatus,
          ...(action.value && typeof action.value === 'object' ? action.value : {}),
        },
      };
    case 'set-state-status': {
      const nextValue = action.value;
      if (typeof nextValue === 'function') {
        const updated = nextValue(state.stateStatus);
        if (updated === state.stateStatus) return state;
        return { ...state, stateStatus: updated };
      }
      return {
        ...state,
        stateStatus: {
          ...state.stateStatus,
          ...(nextValue && typeof nextValue === 'object' ? nextValue : {}),
        },
      };
    }
    case 'set-realtime-metrics': {
      const nextValue = action.value;
      if (typeof nextValue === 'function') {
        const updated = nextValue(state.realtimeMetrics);
        if (updated === state.realtimeMetrics) return state;
        return { ...state, realtimeMetrics: updated };
      }
      return {
        ...state,
        realtimeMetrics: {
          ...state.realtimeMetrics,
          ...(nextValue && typeof nextValue === 'object' ? nextValue : {}),
        },
      };
    }
    case 'mark-realtime-metric': {
      const key = action.key;
      if (!key || state.realtimeMetrics[key] != null) {
        return state;
      }
      const elapsed = Number.isFinite(action.elapsed) ? Math.max(0, Math.round(action.elapsed)) : null;
      if (elapsed == null) return state;
      return {
        ...state,
        realtimeMetrics: {
          ...state.realtimeMetrics,
          [key]: elapsed,
        },
      };
    }
    case 'set-first-state-received':
      if (state.stateStatus.firstStateReceived) {
        return state;
      }
      return {
        ...state,
        stateStatus: {
          ...state.stateStatus,
          firstStateReceived: Boolean(action.value),
        },
      };
    default:
      return state;
  }
}
