import React, { useEffect } from 'react';
import { createPortal } from 'react-dom';
import { getModalRoot } from '../Modal/modalRoot';
import './BaseModal.css';

export default function BaseModal({
  open,
  onClose,
  children,
  backdropClassName = '',
  dialogClassName = '',
  closeButtonClassName = 'auth-modal-close',
  showClose = true,
  closeAriaLabel = 'Close dialog',
  onBackdropMouseDown,
  disableBackdropClose = false,
}) {
  useEffect(() => {
    if (!open) return undefined;
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        if (onClose) {
          onClose();
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, onClose]);

  useEffect(() => {
    if (!open) return undefined;
    const originalOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = originalOverflow;
    };
  }, [open]);

  if (!open) return null;

  const handleBackdrop = (event) => {
    if (onBackdropMouseDown) {
      onBackdropMouseDown(event);
      return;
    }
    if (disableBackdropClose) return;
    if (event.target === event.currentTarget && onClose) {
      onClose();
    }
  };

  return createPortal(
    <div
      className={`auth-modal-backdrop base-modal-backdrop open ${backdropClassName}`.trim()}
      onMouseDown={handleBackdrop}
    >
      <div className={`auth-modal-glass base-modal-glass open ${dialogClassName}`.trim()}>
        {showClose ? (
          <button
            type="button"
            className={closeButtonClassName}
            onClick={onClose}
            aria-label={closeAriaLabel}
          >
            ×
          </button>
        ) : null}
        {children}
      </div>
    </div>,
    getModalRoot()
  );
}
