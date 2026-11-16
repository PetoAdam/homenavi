// @vitest-environment jsdom

import React, { act, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DeviceControlRenderer } from './DeviceControlRenderer';
import { StateMetric } from '../DeviceMetricsRenderer/DeviceMetricsRenderer';
import DeviceTile from '../../Devices/DeviceTile';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('device presentation components', () => {
  let root;
  let container;

  function render(element) {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(element));
  }

  afterEach(() => {
    if (root) act(() => root.unmount());
    container?.remove();
  });

  it('steps a numeric slider input without changing its command type', () => {
    const commit = vi.fn();
    const input = { id: 'target', type: 'slider', label: 'Target', property: 'target_temperature', range: { min: 5, max: 35, step: 0.5 }, capability: { value_type: 'number', unit: 'C' }, ui: { control: 'stepper' } };
    function Example() {
      const [value, setValue] = useState(20);
      return <DeviceControlRenderer input={input} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    act(() => container.querySelector('[aria-label="Increase Target"]').click());
    expect(commit).toHaveBeenCalledOnce();
    expect(commit).toHaveBeenCalledWith(20.5);
    expect(input.type).toBe('slider');
    expect(container.querySelector('input').value).toBe('20.5');
  });

  it.each(['number', 'stepper'])('preserves fine %s steps and skips excluded values', control => {
    const commit = vi.fn();
    function Example() {
      const [value, setValue] = useState(0);
      return <DeviceControlRenderer input={{ type: 'number', label: 'Target', range: { min: 0, max: 1, step: 1e-11, except: [1e-11] }, ui: { control } }} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    act(() => container.querySelector('[aria-label="Increase Target"]').click());
    expect(commit).toHaveBeenCalledWith(2e-11);
  });

  it('skips an excluded slider value', () => {
    const commit = vi.fn();
    function Example() {
      const [value, setValue] = useState(2);
      return <DeviceControlRenderer input={{ type: 'slider', label: 'Target', range: { min: 1, max: 7, step: 1, except: [3] } }} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    const field = container.querySelector('input');
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field, '3');
      field.dispatchEvent(new Event('input', { bubbles: true }));
      field.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
    });
    expect(commit).toHaveBeenCalledOnce();
    expect(commit).toHaveBeenCalledWith(4);
  });

  it('disables relative stepping for mixed group values', () => {
    const commit = vi.fn();
    render(<DeviceControlRenderer input={{ type: 'number', label: 'Target', mixed: true, ui: { control: 'stepper' } }} value={20} onChange={() => {}} onCommit={commit} />);
    expect(container.querySelector('[aria-label="Increase Target"]').disabled).toBe(true);
    expect(commit).not.toHaveBeenCalled();
  });

  it('displays units and commits an edited number only once', () => {
    const commit = vi.fn();
    const input = { type: 'number', label: 'Humidity', unit: '%', range: { min: 0, max: 100, step: 1 } };
    function Example() {
      const [value, setValue] = useState(20);
      return <DeviceControlRenderer input={input} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    const field = container.querySelector('input');
    expect(container.textContent).toContain('%');
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field, '21');
      field.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => field.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true })));
    act(() => field.dispatchEvent(new FocusEvent('focusout', { bubbles: true })));
    expect(commit).toHaveBeenCalledOnce();
    expect(commit).toHaveBeenCalledWith(21);
  });

  it('steps a number with stacked arrows and respects its bounds', () => {
    const commit = vi.fn();
    const input = { id: 'target', type: 'number', label: 'Target', range: { min: 5, max: 6, step: 0.5 } };
    function Example() {
      const [value, setValue] = useState(5.5);
      return <DeviceControlRenderer input={input} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    act(() => container.querySelector('[aria-label="Increase Target"]').click());
    expect(commit).toHaveBeenLastCalledWith(6);
    expect(container.querySelector('[aria-label="Increase Target"]').disabled).toBe(true);
    act(() => container.querySelector('[aria-label="Decrease Target"]').click());
    expect(commit).toHaveBeenLastCalledWith(5.5);
    expect(commit).toHaveBeenCalledTimes(2);
  });

  it('prevents number arrow commands for mixed values', () => {
    const commit = vi.fn();
    render(<DeviceControlRenderer input={{ type: 'number', label: 'Target', mixed: true }} value={20} onChange={() => {}} onCommit={commit} />);
    expect(container.querySelector('[aria-label="Increase Target"]').disabled).toBe(true);
    expect(container.querySelector('input').value).toBe('');
    expect(commit).not.toHaveBeenCalled();
  });

  it('sends only the adjusted value when editing then pressing an arrow', () => {
    const commit = vi.fn();
    function Example() {
      const [value, setValue] = useState(20);
      return <DeviceControlRenderer input={{ type: 'number', label: 'Target', range: { step: 0.5 } }} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    const field = container.querySelector('input');
    const increase = container.querySelector('[aria-label="Increase Target"]');
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field, '21');
      field.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => field.dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: increase })));
    act(() => increase.click());
    expect(commit).toHaveBeenCalledOnce();
    expect(commit).toHaveBeenCalledWith(21.5);
  });

  it.each(['number', 'stepper'])('commits %s edits when focus leaves its arrows', control => {
    const commit = vi.fn();
    function Example() {
      const [value, setValue] = useState(20);
      return <DeviceControlRenderer input={{ type: 'number', label: 'Target', ui: { control } }} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    const field = container.querySelector('input');
    const increase = container.querySelector('[aria-label="Increase Target"]');
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field, '21');
      field.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => field.dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: increase })));
    expect(commit).not.toHaveBeenCalled();
    act(() => increase.dispatchEvent(new FocusEvent('focusout', { bubbles: true })));
    expect(commit).toHaveBeenCalledOnce();
    expect(Number(commit.mock.calls[0][0])).toBe(21);
  });

  it.each(['number', 'stepper'])('rejects %s edits outside the step grid without a minimum', control => {
    const commit = vi.fn();
    function Example() {
      const [value, setValue] = useState(1);
      return <DeviceControlRenderer input={{ type: 'number', label: 'Target', range: { step: 6 }, ui: { control } }} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    const field = container.querySelector('input');
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field, '7');
      field.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => field.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true })));
    expect(commit).not.toHaveBeenCalled();
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field, '6.000000001');
      field.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => field.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true })));
    expect(commit).not.toHaveBeenCalled();
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field, '6');
      field.dispatchEvent(new Event('input', { bubbles: true }));
    });
    Object.defineProperty(field, 'validity', { configurable: true, value: { valid: false, badInput: false } });
    act(() => field.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true })));
    expect(commit).toHaveBeenCalledOnce();
    expect(Number(commit.mock.calls[0][0])).toBe(6);
  });

  it('disables tile stepping when state is missing', () => {
    const device = { id: 'demo/zone', displayName: 'Zone', online: true, state: {}, inputs: [{ id: 'target', type: 'slider', label: 'Target', property: 'target', range: { min: 5, max: 35, step: 0.5 }, ui: { control: 'stepper' } }] };
    render(<DeviceTile device={device} onCommand={vi.fn()} />);
    expect(container.querySelector('[aria-label="Increase Target"]').disabled).toBe(true);
    expect(container.querySelector('input[type="number"]').value).toBe('');
  });

  it('does not send a synthetic slider minimum on blur', () => {
    const commit = vi.fn();
    function Example() {
      const [value, setValue] = useState('');
      return <DeviceControlRenderer input={{ type: 'slider', label: 'Target', range: { min: 5, max: 35 } }} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    const field = container.querySelector('input');
    expect(container.textContent).toContain('No reported value');
    act(() => field.dispatchEvent(new FocusEvent('focusout', { bubbles: true })));
    expect(commit).not.toHaveBeenCalled();
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(field, '12');
      field.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => field.dispatchEvent(new MouseEvent('mouseup', { bubbles: true })));
    act(() => field.dispatchEvent(new FocusEvent('focusout', { bubbles: true })));
    expect(commit).toHaveBeenCalledOnce();
    expect(commit).toHaveBeenCalledWith(12);
  });

  it('keeps distinct namespaced inferred controls in the tile', () => {
    const capabilities = ['left.temperature', 'right.temperature'].map(id => ({ id, property: id, kind: 'numeric', value_type: 'number', access: { write: true }, range: { min: 0, max: 35 } }));
    const device = { id: 'demo/zone', displayName: 'Zone', online: true, state: { 'left.temperature': 20, 'right.temperature': 21 }, capabilities };
    render(<DeviceTile device={device} onCommand={vi.fn()} />);
    expect(container.querySelectorAll('.device-controls input[type="range"]')).toHaveLength(2);
  });

  it('disables number arrows while pending or without a value', () => {
    render(<DeviceControlRenderer input={{ type: 'number', label: 'Target' }} value={null} pending onChange={() => {}} onCommit={() => {}} />);
    expect(container.querySelector('[aria-label="Increase Target"]').disabled).toBe(true);
    expect(container.querySelector('[aria-label="Decrease Target"]').disabled).toBe(true);
  });

  it('opens the color dialog and cancels without sending a command', () => {
    const commit = vi.fn();
    const parentKeyHandler = vi.fn();
    window.addEventListener('keydown', parentKeyHandler);
    render(<DeviceControlRenderer input={{ type: 'color', label: 'Lamp color' }} value="#39AA79" onChange={() => {}} onCommit={commit} />);
    const trigger = container.querySelector('.hn-color-toggle');
    act(() => trigger.click());
    const dialog = document.querySelector('dialog');
    expect(dialog.getAttribute('aria-labelledby')).toBeTruthy();
    expect(document.body.style.overflow).toBe('hidden');
    act(() => dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
    expect(parentKeyHandler).not.toHaveBeenCalled();
    act(() => dialog.dispatchEvent(new Event('cancel', { bubbles: true, cancelable: true })));
    expect(document.querySelector('dialog')).toBeNull();
    expect(document.body.style.overflow).toBe('');
    expect(document.activeElement).toBe(trigger);
    expect(commit).not.toHaveBeenCalled();
    window.removeEventListener('keydown', parentKeyHandler);
  });

  it('validates and applies a color from the dialog', () => {
    const change = vi.fn();
    const commit = vi.fn();
    render(<DeviceControlRenderer input={{ type: 'color', label: 'Lamp color' }} value="#39AA79" onChange={change} onCommit={commit} />);
    act(() => container.querySelector('.hn-color-toggle').click());
    const field = document.querySelector('.hn-color-input');
    const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
    act(() => { setValue.call(field, '#12'); field.dispatchEvent(new Event('input', { bubbles: true })); });
    expect(document.querySelector('.hn-color-apply').disabled).toBe(true);
    act(() => { setValue.call(field, '#336699'); field.dispatchEvent(new Event('input', { bubbles: true })); });
    act(() => document.querySelector('.hn-color-apply').click());
    expect(change).toHaveBeenCalledWith('#336699');
    expect(commit).toHaveBeenCalledOnce();
    expect(commit).toHaveBeenCalledWith('#336699');
  });

  it('represents a mixed checkbox without selecting either value', () => {
    render(<DeviceControlRenderer input={{ type: 'toggle', label: 'Power', mixed: true, ui: { control: 'checkbox' } }} value={true} onChange={() => {}} onCommit={() => {}} />);
    const checkbox = container.querySelector('input');
    expect(checkbox.indeterminate).toBe(true);
    expect(checkbox.getAttribute('aria-checked')).toBe('mixed');
  });

  it('renders only compatible bounded state meters', () => {
    const capability = { value_type: 'number', range: { min: 0, max: 100 }, ui: { display: 'gauge' } };
    render(<StateMetric metric={{ key: 'battery', label: 'Battery', value: 42, rawValue: 42, unit: '%', capability }} />);
    expect(container.querySelector('meter').value).toBe(42);
    expect(container.querySelector('.dmr-meter-fill').style.width).toBe('42%');
    expect(container.textContent).toContain('42 %');
  });

  it('renders compatible enum choices and commits their original values', () => {
    const commit = vi.fn();
    const input = { type: 'select', label: 'Mode', capability: { value_type: 'enum' }, ui: { control: 'segmented' }, options: [{ value: 'heat', label: 'Heat' }, { value: 'off', label: 'Off' }] };
    render(<DeviceControlRenderer input={input} value="off" onChange={() => {}} onCommit={commit} />);
    act(() => container.querySelector('input').click());
    expect(commit).toHaveBeenCalledWith('heat');
  });

  it('keeps radio choices single-select and commits the chosen value', () => {
    const commit = vi.fn();
    const input = { type: 'select', label: 'Mode', ui: { control: 'radio' }, options: [{ value: 'heat', label: 'Heat' }, { value: 'off', label: 'Off' }] };
    function Example() {
      const [value, setValue] = useState('off');
      return <DeviceControlRenderer input={input} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    const choices = container.querySelectorAll('input[type="radio"]');
    act(() => choices[0].click());
    expect(choices[0].checked).toBe(true);
    expect(choices[1].checked).toBe(false);
    expect(commit).toHaveBeenCalledWith('heat');
  });

  it('sends an action on every button press', () => {
    const commit = vi.fn();
    const input = { type: 'select', label: 'Action', ui: { control: 'buttons' }, options: [{ value: 'stop', label: 'Stop' }] };
    render(<DeviceControlRenderer input={input} value="stop" onChange={() => {}} onCommit={commit} />);
    act(() => { container.querySelector('button').click(); container.querySelector('button').click(); });
    expect(commit).toHaveBeenCalledTimes(2);
    expect(commit).toHaveBeenCalledWith('stop');
  });

  it('commits text on blur without changing its type', () => {
    const commit = vi.fn();
    const input = { type: 'text', label: 'Message', capability: { value_type: 'string' }, ui: { control: 'textarea' } };
    function Example() {
      const [value, setValue] = useState('old');
      return <DeviceControlRenderer input={input} value={value} onChange={setValue} onCommit={commit} />;
    }
    render(<Example />);
    const textarea = container.querySelector('textarea');
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(textarea, 'new');
      textarea.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => textarea.dispatchEvent(new FocusEvent('focusout', { bubbles: true })));
    expect(commit).toHaveBeenCalledWith('new');
    expect(input.type).toBe('text');
  });

  it('sends an absolute temperature command from the device tile', () => {
    const command = vi.fn();
    const capability = { id: 'climate.target_temperature', property: 'target_temperature', name: 'Target', value_type: 'number', access: { read: true, write: true }, range: { min: 5, max: 35, step: 0.5 }, unit: 'C' };
    const device = { id: 'emodul/zone', displayName: 'Thermostat', online: true, state: { target_temperature: 20 }, capabilities: [capability], inputs: [{ id: 'set_target_temperature', type: 'slider', label: 'Target', property: 'target_temperature', capability_id: capability.id, range: capability.range, ui: { control: 'stepper' } }] };
    render(<DeviceTile device={device} onCommand={command} />);
    act(() => container.querySelector('[aria-label="Increase Target"]').click());
    expect(command).toHaveBeenCalledOnce();
    expect(command).toHaveBeenCalledWith(device, { state: { target_temperature: 20.5 } });
  });

  it('steps a default number control from the device tile', () => {
    const command = vi.fn();
    const device = { id: 'demo/zone', displayName: 'Zone', online: true, state: { target: 20 }, inputs: [{ id: 'target', type: 'number', label: 'Target', property: 'target', range: { min: 5, max: 35, step: 0.5 } }] };
    render(<DeviceTile device={device} onCommand={command} />);
    act(() => container.querySelector('[aria-label="Increase Target"]').click());
    expect(command).toHaveBeenCalledOnce();
    expect(command).toHaveBeenCalledWith(device, { state: { target: 20.5 } });
  });
});