// @vitest-environment jsdom

import React from 'react';
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import DemoOverviewWidget from './DemoOverviewWidget';
import { getWidgetComponent, listLocalWidgetCatalog } from '../widgetRegistry';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

function renderIntoDom(element) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(element);
  });

  return {
    container,
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

describe('DemoOverviewWidget', () => {
  it('is exposed through the dashboard widget registry', () => {
    const catalog = listLocalWidgetCatalog();
    const meta = catalog.find((item) => item.id === 'homenavi.demo.overview');

    expect(meta).toMatchObject({
      id: 'homenavi.demo.overview',
      display_name: 'Welcome',
      source: 'first_party',
      verified: true,
    });
    expect(getWidgetComponent('homenavi.demo.overview')).toBe(DemoOverviewWidget);
  });

  it('renders orientation copy and links into existing app routes', () => {
    const view = renderIntoDom(
      <MemoryRouter>
        <DemoOverviewWidget />
      </MemoryRouter>
    );

    expect(document.body.textContent).toContain('Explore the seeded smart home.');
    expect(document.body.textContent).toContain('Live rooms, device state, scenes, and demo-safe controls are already wired in.');
    expect(document.body.textContent).toContain('Map');
    expect(document.body.textContent).toContain('Scenes');
    expect(document.body.textContent).toContain('About');

    const links = Array.from(view.container.querySelectorAll('a'));
    expect(links.map((link) => link.getAttribute('href'))).toEqual(['/map', '/automation', '/about']);

    view.unmount();
  });
});