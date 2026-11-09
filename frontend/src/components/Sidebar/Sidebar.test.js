import { readFileSync } from 'node:fs';
import postcss from 'postcss';
import { describe, expect, it } from 'vitest';

const css = postcss.parse(readFileSync(new URL('./Sidebar.css', import.meta.url), 'utf8'));

function sidebarDeclarations(mediaQuery) {
  const media = css.nodes.find((node) => node.type === 'atrule' && node.params === mediaQuery);
  const rule = media.nodes.find((node) => node.type === 'rule' && node.selector === '.sidebar');
  return Object.fromEntries(rule.nodes.filter((node) => node.type === 'decl').map((node) => [node.prop, node.value]));
}

describe('sidebar viewport spacing', () => {
  it('uses equal top and bottom gaps on desktop', () => {
    const sidebar = sidebarDeclarations('(min-width: 1399px)');
    const top = Number.parseFloat(sidebar.top);
    expect(sidebar.top).toMatch(/^\d+(?:\.\d+)?rem$/);
    expect(sidebar.height).toBe(`calc(100vh - ${top * 2}rem)`);
    expect(sidebar['min-height']).toBe(sidebar.height);
  });

  it('keeps the mobile overlay full-height', () => {
    const sidebar = sidebarDeclarations('(max-width: 1399px)');
    expect(sidebar.top).toBe('0');
    expect(sidebar.height).toBe('100vh');
    expect(sidebar['min-height']).toBe('100vh');
  });
});