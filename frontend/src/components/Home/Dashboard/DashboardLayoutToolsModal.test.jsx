// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createRoot } from 'react-dom/client';

import DashboardLayoutToolsModal from './DashboardLayoutToolsModal';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

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
    container,
    modalRoot,
    rerender(nextElement) {
      act(() => {
        root.render(nextElement);
      });
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
      modalRoot.remove();
    },
  };
}

describe('DashboardLayoutToolsModal', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('renders import and auto-layout actions and forwards callbacks', () => {
    const onClose = vi.fn();
    const onImportSourceModeChange = vi.fn();
    const onImportIntoCurrent = vi.fn();
    const onAutoLayoutOptionChange = vi.fn();
    const onAutoLayoutCurrent = vi.fn();

    const view = renderIntoDom(
      <DashboardLayoutToolsModal
        open
        onClose={onClose}
        currentMode="3"
        importSourceMode="4"
        onImportSourceModeChange={onImportSourceModeChange}
        onImportIntoCurrent={onImportIntoCurrent}
        autoLayoutReorganize={true}
        autoLayoutGrowWidths={false}
        autoLayoutShrinkWidths={true}
        autoLayoutGrowHeights={true}
        autoLayoutShrinkHeights={false}
        onAutoLayoutOptionChange={onAutoLayoutOptionChange}
        onAutoLayoutCurrent={onAutoLayoutCurrent}
      />,
    );

    expect(document.body.textContent).toContain('Layout Tools');
    expect(document.body.textContent).toContain('Editing 3 columns.');

    act(() => {
      document.querySelector('#dashboard-tools-import-source').value = '2';
      document.querySelector('#dashboard-tools-import-source').dispatchEvent(new Event('change', { bubbles: true }));
      document.querySelectorAll('input[type="checkbox"]')[0].click();
      document.querySelectorAll('input[type="checkbox"]')[1].click();
      document.querySelectorAll('input[type="checkbox"]')[2].click();
      document.querySelectorAll('input[type="checkbox"]')[3].click();
      document.querySelectorAll('input[type="checkbox"]')[4].click();
      document.querySelector('button[aria-label="Close layout tools"]').click();
      Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('Import into current')).click();
      Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('Auto-layout current')).click();
    });

    expect(onImportSourceModeChange).toHaveBeenCalledWith('2');
    expect(onAutoLayoutOptionChange).toHaveBeenCalledWith('reorganize', false);
    expect(onAutoLayoutOptionChange).toHaveBeenCalledWith('growWidths', true);
    expect(onAutoLayoutOptionChange).toHaveBeenCalledWith('shrinkWidths', false);
    expect(onAutoLayoutOptionChange).toHaveBeenCalledWith('growHeights', false);
    expect(onAutoLayoutOptionChange).toHaveBeenCalledWith('shrinkHeights', true);
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onImportIntoCurrent).toHaveBeenCalledTimes(1);
    expect(onAutoLayoutCurrent).toHaveBeenCalledTimes(1);

    view.unmount();
  });
});