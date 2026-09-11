// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createRoot } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const authState = {
  user: { role: 'resident' },
  accessToken: 'token',
  bootstrapping: false,
};

const dashboardHookState = {
  dashboard: { id: 'dash-1', layout_version: 1 },
  doc: {
    items: [
      { instance_id: 'weather-1', widget_type: 'homenavi.weather', enabled: true, settings: {} },
      { instance_id: 'device-1', widget_type: 'homenavi.device', enabled: true, settings: {} },
    ],
    layoutsByCols: {
      '4': [
        { i: 'weather-1', x: 0, y: 0, w: 2, h: 5, minW: 1, minH: 2 },
        { i: 'device-1', x: 2, y: 0, w: 1, h: 4, minW: 1, minH: 2 },
      ],
      '3': [
        { i: 'weather-1', x: 0, y: 0, w: 2, h: 5, minW: 1, minH: 2 },
        { i: 'device-1', x: 2, y: 0, w: 1, h: 4, minW: 1, minH: 2 },
      ],
      '2': [
        { i: 'weather-1', x: 0, y: 0, w: 2, h: 5, minW: 1, minH: 2 },
        { i: 'device-1', x: 0, y: 5, w: 1, h: 4, minW: 1, minH: 2 },
      ],
      '1': [
        { i: 'weather-1', x: 0, y: 0, w: 1, h: 5, minW: 1, minH: 2 },
        { i: 'device-1', x: 0, y: 5, w: 1, h: 4, minW: 1, minH: 2 },
      ],
    },
  },
  catalog: [
    { id: 'homenavi.weather', display_name: 'Weather', default_height: 5 },
    { id: 'homenavi.device', display_name: 'Device', default_height: 4 },
  ],
  loading: false,
  saving: false,
  error: '',
  saveParsedDoc: vi.fn(),
  flushSave: vi.fn(),
};

const originalCrypto = globalThis.crypto;

vi.mock('../../../context/AuthContext', () => ({
  useAuth: () => authState,
}));

vi.mock('../../../hooks/useDashboard', () => ({
  default: () => dashboardHookState,
}));

vi.mock('react-grid-layout', () => {
  const MockGridLayout = ({ children }) => <div data-testid="grid-layout">{children}</div>;
  return {
    default: MockGridLayout,
    WidthProvider: (Component) => Component,
  };
});

vi.mock('./WidgetRenderer', () => ({
  default: ({ instanceId, widgetType, onSettings, onRemove }) => (
    <div data-widget-id={instanceId}>
      <span>{widgetType}</span>
      <button onClick={onSettings}>Settings</button>
      <button onClick={onRemove}>Remove</button>
    </div>
  ),
}));

vi.mock('./AddWidgetModal', () => ({
  default: ({ open, onAdd }) => (open ? <button onClick={() => onAdd('homenavi.device', {})}>Add device widget</button> : null),
}));

vi.mock('./WidgetSettingsModal', () => ({
  default: () => null,
}));

import Dashboard from './Dashboard';

function renderIntoDom(element) {
  const container = document.createElement('div');
  const modalRoot = document.createElement('div');
  modalRoot.id = 'modal-root';
  document.body.appendChild(container);
  document.body.appendChild(modalRoot);
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
      modalRoot.remove();
    },
  };
}

async function flushEffects() {
  await act(async () => {
    await Promise.resolve();
  });
}

describe('Dashboard component', () => {
  beforeEach(() => {
    dashboardHookState.saveParsedDoc.mockReset();
    dashboardHookState.flushSave.mockReset();
    window.localStorage.clear();
    document.body.innerHTML = '';
    Object.defineProperty(window, 'innerWidth', { value: 1400, configurable: true, writable: true });
    Object.defineProperty(globalThis, 'crypto', {
      value: { ...originalCrypto, randomUUID: () => 'new-widget-1' },
      configurable: true,
    });
  });

  afterEach(() => {
    document.body.innerHTML = '';
    Object.defineProperty(globalThis, 'crypto', {
      value: originalCrypto,
      configurable: true,
    });
  });

  it('shows the first-time tutorial, supports dismissal persistence, and reopens edit mode without it later', async () => {
    const view = renderIntoDom(<Dashboard />);

    act(() => {
      document.querySelector('button[title="Edit dashboard"]').click();
    });

    expect(document.body.textContent).toContain('Welcome to dashboard editing');

    act(() => {
      Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('Do not show again')).click();
    });

    expect(window.localStorage.getItem('homenavi:dashboard:tutorial-dismissed:v1')).toBe('1');

    act(() => {
      document.querySelector('button[title="Done editing"]').click();
    });

    expect(dashboardHookState.flushSave).toHaveBeenCalledTimes(1);

    act(() => {
      document.querySelector('button[title="Edit dashboard"]').click();
    });

    expect(document.body.textContent).not.toContain('Dashboard editing');

    view.unmount();
  });

  it('imports another column mode into the current layout and persists the result', async () => {
    dashboardHookState.doc.layoutsByCols['4'] = [
      { i: 'device-1', x: 0, y: 0, w: 1, h: 4, minW: 1, minH: 2 },
      { i: 'weather-1', x: 1, y: 0, w: 2, h: 5, minW: 1, minH: 2 },
    ];

    const view = renderIntoDom(<Dashboard />);

    act(() => {
      document.querySelector('button[title="Edit dashboard"]').click();
    });

    act(() => {
      Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('Open layout tools')).click();
    });

    expect(document.body.textContent).toContain('Layout Tools');

    act(() => {
      document.querySelector('#dashboard-tools-import-source').value = '4';
      document.querySelector('#dashboard-tools-import-source').dispatchEvent(new Event('change', { bubbles: true }));
      Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('Import into current')).click();
    });

    await flushEffects();

    expect(dashboardHookState.saveParsedDoc).toHaveBeenCalled();
    const latestDoc = dashboardHookState.saveParsedDoc.mock.calls.at(-1)[0];
    expect(latestDoc.layoutsByCols).toBeTruthy();
    expect(latestDoc.layoutsByCols['3'][0].i).toBe('device-1');
    expect(latestDoc.layoutsByCols['3'][1].i).toBe('weather-1');

    view.unmount();
  });

  it('adds a new widget at the end without changing existing widget geometry', async () => {
    const view = renderIntoDom(<Dashboard />);

    act(() => {
      document.querySelector('button[title="Edit dashboard"]').click();
    });

    act(() => {
      Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('Skip')).click();
    });

    act(() => {
      document.querySelector('button[title="Add widget"]').click();
    });

    act(() => {
      Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('Add device widget')).click();
    });

    await flushEffects();

    const latestDoc = dashboardHookState.saveParsedDoc.mock.calls.at(-1)[0];
    expect(latestDoc.items.at(-1).instance_id).toBe('new-widget-1');
    expect(latestDoc.layoutsByCols['3'][0]).toMatchObject({ i: 'weather-1', x: 0, y: 0, w: 2, h: 5 });
    expect(latestDoc.layoutsByCols['3'][1]).toMatchObject({ i: 'device-1', x: 2, y: 0, w: 1, h: 4 });
    expect(latestDoc.layoutsByCols['3'][2]).toMatchObject({ i: 'new-widget-1', x: 0, y: 5, w: 1, h: 4 });

    view.unmount();
  });
});