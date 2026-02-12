import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import './IntegrationIcon.css';

const isImageIconPath = (iconName) => {
  const raw = (iconName || '').toString().trim();
  if (!raw) return false;
  return raw.startsWith('/integrations/') || raw.startsWith('http://') || raw.startsWith('https://');
};

export default function IntegrationIcon({
  icon,
  faIcon,
  fallbackIcon,
  className = '',
  imgClassName = '',
  onError,
}) {
  if (isImageIconPath(icon)) {
    const handleError = onError || ((event) => { event.currentTarget.style.display = 'none'; });
    return (
      <img
        className={`integration-icon-img ${imgClassName}`.trim()}
        src={icon}
        alt=""
        aria-hidden="true"
        onError={handleError}
      />
    );
  }

  const iconToRender = faIcon || fallbackIcon;
  if (!iconToRender) {
    return null;
  }

  return (
    <span className={`integration-icon ${className}`.trim()} aria-hidden="true">
      <FontAwesomeIcon icon={iconToRender} />
    </span>
  );
}
