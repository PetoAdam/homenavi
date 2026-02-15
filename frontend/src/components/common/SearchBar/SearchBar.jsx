import React, { useEffect, useMemo, useRef, useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faMagnifyingGlass, faXmark } from '@fortawesome/free-solid-svg-icons';
import './SearchBar.css';

export default function SearchBar({
  value,
  onChange,
  debounceMs = 200,
  placeholder = 'Search…',
  onClear,
  autoFocus = false,
  className = '',
  ariaLabel = 'Search',
}) {
  const normalizedValue = useMemo(() => String(value ?? ''), [value]);
  const canClear = typeof onClear === 'function' && normalizedValue.length > 0;

  const [draft, setDraft] = useState(normalizedValue);
  const lastEmittedRef = useRef(normalizedValue);

  // Keep local draft in sync with external value changes (e.g., parent resets).
  useEffect(() => {
    setDraft(normalizedValue);
    lastEmittedRef.current = normalizedValue;
  }, [normalizedValue]);

  // Debounce onChange to avoid excessive filtering/network calls.
  useEffect(() => {
    if (typeof onChange !== 'function') return undefined;
    if (draft === lastEmittedRef.current) return undefined;

    const ms = Number.isFinite(Number(debounceMs)) ? Math.max(0, Number(debounceMs)) : 0;
    if (ms === 0) {
      lastEmittedRef.current = draft;
      onChange(draft);
      return undefined;
    }

    const t = setTimeout(() => {
      lastEmittedRef.current = draft;
      onChange(draft);
    }, ms);

    return () => clearTimeout(t);
  }, [debounceMs, draft, onChange]);

  const isActive = draft.length > 0;

  return (
    <div className={`hn-searchbar ${isActive ? 'hn-searchbar--active' : ''} ${className}`.trim()}>
      <FontAwesomeIcon icon={faMagnifyingGlass} className="hn-searchbar__icon" />
      <input
        type="text"
        className="hn-searchbar__input"
        placeholder={placeholder}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        autoFocus={autoFocus}
        aria-label={ariaLabel}
      />
      {canClear ? (
        <button
          type="button"
          className="hn-searchbar__clear"
          onClick={() => {
            setDraft('');
            lastEmittedRef.current = '';
            onChange?.('');
            onClear();
          }}
          aria-label="Clear search"
          title="Clear"
        >
          <FontAwesomeIcon icon={faXmark} />
        </button>
      ) : null}
    </div>
  );
}
