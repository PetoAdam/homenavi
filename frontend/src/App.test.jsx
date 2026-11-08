// @vitest-environment jsdom

import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

describe('App scrollbar styles', () => {
  it('provides thin, muted scrollbars with a WebKit fallback', () => {
    const style = document.createElement('style');
    style.textContent = readFileSync('src/App.css', 'utf8');
    document.head.appendChild(style);
    try {
      const rules = Array.from(style.sheet.cssRules);
      const findRule = (selector) => rules.find((rule) => rule.selectorText === selector);
      expect(findRule('*')?.style.getPropertyValue('scrollbar-width')).toBe('thin');
      expect(findRule('*')?.style.getPropertyValue('scrollbar-color')).toBe('rgba(255, 255, 255, 0.2) transparent');
      expect(findRule('*::-webkit-scrollbar')?.style.getPropertyValue('width')).toBe('0.4rem');
      expect(findRule('*::-webkit-scrollbar')?.style.getPropertyValue('height')).toBe('0.4rem');
      expect(findRule('*::-webkit-scrollbar-thumb')?.style.getPropertyValue('background')).toBe('rgba(255, 255, 255, 0.2)');
      expect(findRule('*::-webkit-scrollbar-track')?.style.getPropertyValue('background')).toBe('transparent');
    } finally {
      style.remove();
    }
  });
});