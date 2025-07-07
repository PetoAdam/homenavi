import React from 'react';
import './Greeting.css';

export default function Greeting({ children, showProfileTextButton }) {
  return (
    <div
      className={`dashboard-greeting${showProfileTextButton ? ' dashboard-greeting--with-profile-btn' : ''}`}
    >
      {children ? children : 'Welcome back, Adam!👋'}
    </div>
  );
}
