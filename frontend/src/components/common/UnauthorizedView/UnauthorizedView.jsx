import React, { useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faRightToBracket, faChevronDown, faChevronUp } from '@fortawesome/free-solid-svg-icons';
import PageHeader from '../PageHeader/PageHeader';
import GlassCard from '../GlassCard/GlassCard';
import GlassPill from '../GlassPill/GlassPill';
import './UnauthorizedView.css';

export default function UnauthorizedView({
  title = 'Unauthorized',
  message = 'You do not have permission to view this page.',
  className = '',
  hideHeader = false,
}) {
  const [showWhy, setShowWhy] = useState(false);
  return (
    <div className={['unauthorized-page', className].filter(Boolean).join(' ')}>
      {!hideHeader && <PageHeader title={title} subtitle="Sign in with a resident account to continue." />}
      <GlassCard interactive={false} className="unauthorized-card">
        <div className="unauthorized-body">
          <div className="unauthorized-icon" aria-hidden="true">
            <span className="unauthorized-icon-badge">🔒</span>
            <span className="unauthorized-pulse" />
          </div>
          <div className="unauthorized-message">{message}</div>
          <div className="unauthorized-actions">
            <GlassPill
              icon={faRightToBracket}
              text="Log in"
              tone="success"
              onClick={() => window.dispatchEvent(new CustomEvent('homenavi:open-auth'))}
              className="unauthorized-login-pill"
            />
            <button
              type="button"
              className="glass-pill glass-pill-default glass-pill-clickable unauthorized-why-pill"
              onClick={() => setShowWhy(prev => !prev)}
              aria-expanded={showWhy}
              aria-controls="unauthorized-why-panel"
            >
              <span>Why?</span>
              <span className="unauthorized-why-chevron" aria-hidden="true">
                {showWhy ? <FontAwesomeIcon icon={faChevronUp} /> : <FontAwesomeIcon icon={faChevronDown} />}
              </span>
            </button>
          </div>
          <div
            id="unauthorized-why-panel"
            className={`unauthorized-why-panel${showWhy ? ' open' : ''}`}
          >
            <p>This area is reserved for resident accounts and administrators.</p>
            <p>Use a resident login, or ask an admin to grant you access.</p>
          </div>
          <div className="unauthorized-note">
            Tip: click the profile icon in the top-right to sign in.
          </div>
        </div>
      </GlassCard>
    </div>
  );
}
