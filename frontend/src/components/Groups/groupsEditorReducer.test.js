import { describe, expect, it } from 'vitest';
import { groupsEditorInitialState, groupsEditorReducer } from './groupsEditorReducer';

describe('groupsEditorReducer', () => {
  it('opens the editor for create and edit flows', () => {
    const created = groupsEditorReducer(groupsEditorInitialState(), { type: 'open-create' });
    const edited = groupsEditorReducer(created, { type: 'open-edit', group: { id: 'g1' } });

    expect(created.editorOpen).toBe(true);
    expect(created.editingGroup).toBeNull();
    expect(edited.editingGroup).toEqual({ id: 'g1' });
  });

  it('blocks close while save is pending', () => {
    const saving = groupsEditorReducer(groupsEditorInitialState(), { type: 'set-save-pending', value: true });
    const closed = groupsEditorReducer(saving, { type: 'close-editor' });

    expect(closed).toBe(saving);
  });

  it('tracks delete target and pending status', () => {
    const opened = groupsEditorReducer(groupsEditorInitialState(), { type: 'open-delete', group: { id: 'g2' } });
    const pending = groupsEditorReducer(opened, { type: 'set-delete-pending', value: true });
    const blockedClose = groupsEditorReducer(pending, { type: 'close-delete' });

    expect(opened.deleteTarget).toEqual({ id: 'g2' });
    expect(pending.deletePending).toBe(true);
    expect(blockedClose).toBe(pending);
  });
});
