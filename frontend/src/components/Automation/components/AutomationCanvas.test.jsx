// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createRoot } from 'react-dom/client';

import AutomationCanvas from './AutomationCanvas';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

function renderIntoDom(element) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(element);
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

function buildProps(overrides = {}) {
  return {
    canvasRef: { current: null },
    onCanvasDragOver: vi.fn(),
    onCanvasDrop: vi.fn(),
    onCanvasPointerDown: vi.fn(),
    onCanvasPointerMove: vi.fn(),
    onCanvasPointerUp: vi.fn(),
    onCanvasPointerCancel: vi.fn(),
    GRID_SIZE: 24,
    viewport: { x: 0, y: 0, scale: 1 },
    setViewport: vi.fn(),
    svgWorldSize: { x: 0, y: 0, w: 1000, h: 1000 },
    edgesToRender: [],
    connectMode: null,
    connectHoverId: null,
    setConnectHoverId: vi.fn(),
    connectModeRef: { current: null },
    setConnectMode: vi.fn(),
    cancelConnect: vi.fn(),
    deleteEdge: vi.fn(),
    editorNodes: [
      { id: 'trigger-1', kind: 'trigger.manual', x: 10, y: 10, data: {} },
      { id: 'action-1', kind: 'action.send_command', x: 280, y: 10, data: {} },
      { id: 'logic-1', kind: 'logic.sleep', x: 550, y: 10, data: {} },
    ],
    selectedNodeId: 'workflow',
    setSelectedNodeId: vi.fn(),
    NODE_WIDTH: 240,
    NODE_HEADER_HEIGHT: 52,
    isTriggerNode: (node) => String(node?.kind || '').startsWith('trigger.'),
    nodeTitle: (kind) => kind,
    nodeSubtitle: () => '',
    nodeBodyText: () => 'body',
    iconForNodeKind: () => null,
    deviceNameById: new Map(),
    liveRunNodeStates: {},
    commitConnection: vi.fn(),
    startConnectFromNode: vi.fn(),
    onNodePointerDown: vi.fn(),
    executeFromNodeTitle: 'Run from here',
    canExecuteFromNode: false,
    runNow: vi.fn(),
    canvasSize: { width: 1200, height: 800 },
    zoomAroundPoint: (current) => current,
    autoFitKey: '',
    autoFitDataReady: true,
    onAutoFitComplete: undefined,
    renderDelayMs: 0,
    readOnly: false,
    ...overrides,
  };
}

describe('AutomationCanvas', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('renders live run state classes on matching nodes', () => {
    const view = renderIntoDom(
      <AutomationCanvas
        {...buildProps({
          liveRunNodeStates: {
            'trigger-1': 'active',
            'action-1': 'done',
            'logic-1': 'failed',
          },
        })}
      />,
    );

    expect(document.querySelector('.automation-node.run-live-active')).not.toBeNull();
    expect(document.querySelector('.automation-node.run-live-done')).not.toBeNull();
    expect(document.querySelector('.automation-node.run-live-failed')).not.toBeNull();

    view.unmount();
  });
});