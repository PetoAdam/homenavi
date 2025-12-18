import React from 'react';
import { useNavigate } from 'react-router-dom';
import { faArrowLeft } from '@fortawesome/free-solid-svg-icons';
import GlassPill from '../GlassPill/GlassPill';
import './PageHeader.css';

export default function PageHeader({
  title,
  subtitle,
  showBack = false,
  backText = 'Back',
  onBack,
  className = '',
  children,
}) {
  const navigate = useNavigate();

  const handleBack = () => {
    if (typeof onBack === 'function') {
      onBack();
      return;
    }
    navigate(-1);
  };

  return (
    <header className={[
      'page-header-flat',
      showBack ? 'page-header--with-back' : '',
      className,
    ].filter(Boolean).join(' ')}>
      <div className="page-header-row">
        {showBack ? (
          <GlassPill
            icon={faArrowLeft}
            text={backText}
            tone="default"
            onClick={handleBack}
            className="page-header-back-pill"
          />
        ) : null}

        <div className="page-header-text">
          <h1 className="page-title">{title}</h1>
          {subtitle ? <div className="page-subtitle">{subtitle}</div> : null}
        </div>
      </div>

      {children ? (
        <div className="page-header-extra">
          {children}
        </div>
      ) : null}
    </header>
  );
}
