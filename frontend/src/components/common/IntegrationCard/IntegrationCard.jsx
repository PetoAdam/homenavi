import React from 'react';
import './IntegrationCard.css';

export function IntegrationCardHeader({
  eyebrow,
  title,
  subtitle,
  version,
  badges,
  icon,
}) {
  const pillItems = [];
  if (version) {
    const label = String(version).startsWith('v') ? version : `v${version}`;
    pillItems.push(
      <span className="integration-card-pill" key="version">
        {label}
      </span>
    );
  }
  if (Array.isArray(badges)) {
    badges.forEach((badge, index) => {
      if (!badge) return;
      pillItems.push(
        <span className="integration-card-pill" key={`badge-${index}`}>
          {badge}
        </span>
      );
    });
  }
  return (
    <div className="integration-card-header">
      <div className="integration-card-heading">
        {eyebrow ? <div className="integration-card-eyebrow">{eyebrow}</div> : null}
        <div className="integration-card-title">{title}</div>
        {pillItems.length ? (
          <div className="integration-card-pills">
            {pillItems}
          </div>
        ) : null}
        {subtitle ? <div className="integration-card-sub">{subtitle}</div> : null}
      </div>
      <div className="integration-card-icon">{icon}</div>
    </div>
  );
}

export default function IntegrationCard({
  header,
  description,
  meta,
  actions,
  footer,
  className = '',
  onClick,
}) {
  const interactive = typeof onClick === 'function';
  return (
    <div
      className={`integration-card${interactive ? ' interactive' : ''} ${className}`.trim()}
      onClick={onClick}
      role={interactive ? 'button' : undefined}
      tabIndex={interactive ? 0 : undefined}
      onKeyDown={(event) => {
        if (!interactive) return;
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          onClick(event);
        }
      }}
    >
      {header}
      {description ? <div className="integration-card-desc">{description}</div> : null}
      {meta}
      {actions ? (
        <div className="integration-card-actions" onClick={(e) => e.stopPropagation()}>
          {actions}
        </div>
      ) : null}
      {footer}
    </div>
  );
}
