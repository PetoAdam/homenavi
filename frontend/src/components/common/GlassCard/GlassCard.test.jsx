// @vitest-environment jsdom

import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

describe('GlassCard styles', () => {
  it('layers card content without changing the positioning boundary', () => {
    const style = document.createElement('style');
    style.textContent = readFileSync('src/components/common/GlassCard/GlassCard.css', 'utf8');
    document.head.appendChild(style);
    try {
      const contentRule = Array.from(style.sheet.cssRules).find(
        (rule) => rule.selectorText === '.glass-card .card-content',
      );
      expect(contentRule?.style.getPropertyValue('isolation')).toBe('isolate');
      expect(contentRule?.style.getPropertyValue('position')).toBe('');
    } finally {
      style.remove();
    }
  });
});