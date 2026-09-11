// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createRoot } from 'react-dom/client';

import useRunStream from './useRunStream';

let messageHandler = null;
let statusHandler = null;

vi.mock('../../../services/realtime/sharedWebSocket', () => ({
  wsUrlForPath: (path) => `ws://test${path}`,
  getSharedWebSocket: () => ({
    subscribe: (cb) => {
      messageHandler = cb;
      return () => {
        messageHandler = null;
      };
    },
    onStatus: (cb) => {
      statusHandler = cb;
      return () => {
        statusHandler = null;
      };
    },
  }),
}));

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

function renderHarness() {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  const state = { current: null };

  function Harness() {
    state.current = useRunStream();
    return null;
  }

  act(() => {
    root.render(<Harness />);
  });

  return {
    state,
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

describe('useRunStream', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
      cb(Date.now());
      return 1;
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
    messageHandler = null;
    statusHandler = null;
    document.body.innerHTML = '';
  });

  it('keeps trigger highlights visible until the post-run grace period expires', async () => {
    const view = renderHarness();

    await act(async () => {
      await view.state.current.startRunStream({
        runId: 'run-1',
        workflowId: 'wf-1',
        refreshRuns: vi.fn(),
      });
    });

    act(() => {
      messageHandler?.({ data: JSON.stringify({ type: 'node_started', run_id: 'run-1', node_id: 'trigger-1', node_kind: 'trigger.schedule', status: 'running' }) });
    });

    expect(view.state.current.liveRunNodeStates).toEqual({ 'trigger-1': 'active' });

    act(() => {
      messageHandler?.({ data: JSON.stringify({ type: 'node_finished', run_id: 'run-1', node_id: 'trigger-1', node_kind: 'trigger.schedule', status: 'success' }) });
    });

    expect(view.state.current.liveRunNodeStates['trigger-1']).toBe('active');

    act(() => {
      vi.advanceTimersByTime(699);
    });

    expect(view.state.current.liveRunNodeStates['trigger-1']).toBe('active');

    act(() => {
      vi.advanceTimersByTime(1);
    });

    expect(view.state.current.liveRunNodeStates['trigger-1']).toBe('done');

    act(() => {
      messageHandler?.({ data: JSON.stringify({ type: 'run_finished', run_id: 'run-1', status: 'success' }) });
    });

    expect(view.state.current.liveRunNodeStates['trigger-1']).toBe('done');

    act(() => {
      vi.advanceTimersByTime(4999);
    });

    expect(view.state.current.liveRunNodeStates).toEqual({ 'trigger-1': 'done' });

    act(() => {
      vi.advanceTimersByTime(1);
    });

    expect(view.state.current.liveRunNodeStates).toEqual({});

    view.unmount();
  });

  it('does not wipe the trigger highlight when polled run payloads only include non-trigger steps', async () => {
    const getRun = vi.fn().mockResolvedValue({
      success: true,
      data: {
        run: { status: 'running' },
        steps: [
          { node_id: 'sleep-1', status: 'running' },
        ],
      },
    });
    const view = renderHarness();

    await act(async () => {
      await view.state.current.startRunStream({
        runId: 'run-2',
        workflowId: 'wf-2',
        accessToken: 'token',
        getRun,
        refreshRuns: vi.fn(),
      });
    });

    act(() => {
      messageHandler?.({ data: JSON.stringify({ type: 'node_started', run_id: 'run-2', node_id: 'trigger-1', node_kind: 'trigger.schedule', status: 'running' }) });
      messageHandler?.({ data: JSON.stringify({ type: 'node_finished', run_id: 'run-2', node_id: 'trigger-1', node_kind: 'trigger.schedule', status: 'success' }) });
    });

    expect(view.state.current.liveRunNodeStates['trigger-1']).toBe('active');

    await act(async () => {
      vi.advanceTimersByTime(700);
      await Promise.resolve();
    });

    expect(view.state.current.liveRunNodeStates['trigger-1']).toBe('done');

    await act(async () => {
      vi.advanceTimersByTime(850);
      await Promise.resolve();
    });

    expect(getRun).toHaveBeenCalled();
    expect(view.state.current.liveRunNodeStates).toEqual({
      'trigger-1': 'done',
      'sleep-1': 'active',
    });

    view.unmount();
  });
});