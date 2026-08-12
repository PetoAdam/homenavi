import { describe, expect, it } from 'vitest';
import { automationUiInitialState, automationUiReducer } from './automationUiReducer';

describe('automationUiReducer', () => {
  it('initializes edit mode from the route param', () => {
    const state = automationUiInitialState('workflow-1');
    expect(state.viewMode).toBe('edit');
    expect(state.selectedNodeId).toBe('workflow');
  });

  it('tracks view, search and selection changes', () => {
    const next = automationUiReducer(automationUiInitialState(), { type: 'set-view-mode', value: 'edit' });
    const searched = automationUiReducer(next, { type: 'set-workflow-search', value: 'kitchen' });
    const selected = automationUiReducer(searched, { type: 'set-selected-node-id', value: 'node-1' });

    expect(next.viewMode).toBe('edit');
    expect(searched.workflowSearch).toBe('kitchen');
    expect(selected.selectedNodeId).toBe('node-1');
  });

  it('updates drag state and resets it cleanly', () => {
    const dragged = automationUiReducer(automationUiInitialState(), {
      type: 'set-workflow-drag',
      value: { dragId: 'a', overId: 'b', position: 'before' },
    });
    const reset = automationUiReducer(dragged, { type: 'reset-workflow-drag' });

    expect(dragged.workflowDrag).toEqual({ dragId: 'a', overId: 'b', position: 'before' });
    expect(reset.workflowDrag).toEqual({ dragId: '', overId: '', position: 'after' });
  });
});
