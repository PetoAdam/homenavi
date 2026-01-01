import React from 'react';
import { faRightToBracket, faLock } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import GlassPill from '../GlassPill/GlassPill';
import './NoPermissionWidget.css';

export default function NoPermissionWidget({
  title = 'No access',
  message = 'You do not have permission to view this.',
  showLogin = true,
  className = '',
}) {
  return (
    <div
      className={['no-permission-widget', className].filter(Boolean).join(' ')}
      onClick={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
      role="group"
      aria-label={title}
    >
      <div className="no-permission-widget__badge" aria-hidden="true">
        <FontAwesomeIcon icon={faLock} />
      </div>
      <div className="no-permission-widget__body">
        <div className="no-permission-widget__title">{title}</div>
        <div className="no-permission-widget__message">{message}</div>
        {showLogin ? (
          <div className="no-permission-widget__actions">
            <GlassPill
              icon={faRightToBracket}
              text="Sign in"
              tone="success"
              onClick={() => window.dispatchEvent(new CustomEvent('homenavi:open-auth'))}
              className="no-permission-widget__login"
            />
          </div>
        ) : null}
      </div>
    </div>
  );
}
