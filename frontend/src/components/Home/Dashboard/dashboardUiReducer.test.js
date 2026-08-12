import { describe, expect, it } from 'vitest';
import { dashboardUiInitialState, dashboardUiReducer } from './dashboardUiReducer';

describe('dashboardUiReducer', () => {
  it('updates modal and edit state transitions', () => {
    const base = dashboardUiInitialState();

    const editState = dashboardUiReducer(base, { type: 'set-edit-mode', value: true });
    expect(editState.editMode).toBe(true);

    const modalState = dashboardUiReducer(editState, { type: 'set-add-modal-open', value: true });
    expect(modalState.addModalOpen).toBe(true);

    const selectedState = dashboardUiReducer(modalState, { type: 'set-selected-widget-id', value: 'widget-42' });
    expect(selectedState.selectedWidgetId).toBe('widget-42');
  });

  it('updates breakpoint and desired row sizing state', () => {
    const base = dashboardUiInitialState();

    const breakpointState = dashboardUiReducer(base, { type: 'set-current-breakpoint', value: 'sm' });
    expect(breakpointState.currentBreakpoint).toBe('sm');

    const rowState = dashboardUiReducer(breakpointState, {
      type: 'set-desired-row-height',
      instanceId: 'widget-1',
      value: 5,
    });

    expect(rowState.desiredRowsByInstanceId['widget-1']).toBe(5);
  });

  it('resets the dashboard ui state back to the initial shape', () => {
    const state = dashboardUiReducer(dashboardUiInitialState(), {
      type: 'set-edit-mode',
      value: true,
    });

    const reset = dashboardUiReducer(state, { type: 'reset' });
    expect(reset).toEqual(dashboardUiInitialState());
  });
});
