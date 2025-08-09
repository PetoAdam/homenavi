import React from 'react';
import './Button.css';

export default function Button({
  variant = 'primary',
  type = 'button',
  disabled = false,
  onClick,
  className = '',
  children,
  ...props
}) {
  const classes = ['btn'];
  if (variant && variant !== 'primary') classes.push(variant);
  if (className) classes.push(className);
  return (
    <button
      type={type}
      className={classes.join(' ')}
      disabled={disabled}
      onClick={onClick}
      {...props}
    >
      {children}
    </button>
  );
}
