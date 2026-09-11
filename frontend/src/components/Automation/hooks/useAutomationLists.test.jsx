// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createRoot } from 'react-dom/client';

import useAutomationLists from './useAutomationLists';

const listRunsMock = vi.fn();
const listWorkflowsMock = vi.fn();

vi.mock('../../../services/automationService', () => ({
  listRuns: (...args) => listRunsMock(...args),
  listWorkflows: (...args) => listWorkflowsMock(...args),
}));

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

function renderHarness(stateRef) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  function Harness() {
    stateRef.current = useAutomationLists({ accessToken: 'token', onError: vi.fn() });
    return null;
  }

  act(() => {
    root.render(<Harness />);
  });

  return {
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

describe('useAutomationLists', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    listRunsMock.mockReset();
    listWorkflowsMock.mockReset();
    listWorkflowsMock.mockResolvedValue({ success: true, data: { workflows: [{ id: 'wf-1', updated_at: '2026-09-06T00:00:00Z' }] } });
    listRunsMock.mockResolvedValue({ success: true, data: { runs: [] } });
  });

  afterEach(() => {
    vi.useRealTimers();
    document.body.innerHTML = '';
  });

  it('polls runs for the selected workflow so external trigger runs can be discovered', async () => {
    const stateRef = { current: null };
    const view = renderHarness(stateRef);

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(listWorkflowsMock).toHaveBeenCalledTimes(1);
    expect(listRunsMock).toHaveBeenCalledTimes(1);
    expect(listRunsMock).toHaveBeenLastCalledWith('wf-1', 'token', 6);
    expect(stateRef.current.runsLoading).toBe(false);

    await act(async () => {
      vi.advanceTimersByTime(3000);
      await Promise.resolve();
    });

    expect(listRunsMock).toHaveBeenCalledTimes(2);
    expect(stateRef.current.runsLoading).toBe(false);

    view.unmount();
  });
});