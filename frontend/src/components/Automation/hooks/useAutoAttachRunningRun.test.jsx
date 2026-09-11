// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createRoot } from 'react-dom/client';

import useAutoAttachRunningRun, { findLatestRunningRun } from './useAutoAttachRunningRun';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

function renderHarness(propsRef) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  function Harness() {
    useAutoAttachRunningRun(propsRef.current);
    return null;
  }

  act(() => {
    root.render(<Harness />);
  });

  return {
    rerender() {
      act(() => {
        root.render(<Harness />);
      });
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

describe('useAutoAttachRunningRun', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('finds the newest running run candidate from the selected workflow list', () => {
    expect(findLatestRunningRun([
      { id: 'run-1', status: 'success' },
      { id: 'run-2', status: 'running' },
      { id: 'run-3', status: 'failed' },
    ])).toMatchObject({ id: 'run-2', status: 'running' });
  });

  it('auto-attaches to a newly discovered running run once', () => {
    const propsRef = {
      current: {
        accessToken: 'token',
        selectedWorkflow: { id: 'wf-1' },
        runs: [],
        fetchRuns: vi.fn(),
        clearLiveRunHighlights: vi.fn(),
        closeRunWs: vi.fn(),
        startRunStream: vi.fn(),
        getRun: vi.fn(),
      },
    };

    const view = renderHarness(propsRef);

    propsRef.current = {
      ...propsRef.current,
      runs: [{ id: 'run-2', status: 'running' }],
    };

    view.rerender();

    expect(propsRef.current.clearLiveRunHighlights).toHaveBeenCalledTimes(1);
    expect(propsRef.current.closeRunWs).toHaveBeenCalledTimes(1);
    expect(propsRef.current.startRunStream).toHaveBeenCalledTimes(1);
    expect(propsRef.current.startRunStream).toHaveBeenCalledWith({
      runId: 'run-2',
      accessToken: 'token',
      workflowId: 'wf-1',
      refreshRuns: expect.any(Function),
      getRun: propsRef.current.getRun,
    });

    view.rerender();
    expect(propsRef.current.startRunStream).toHaveBeenCalledTimes(1);

    view.unmount();
  });
});