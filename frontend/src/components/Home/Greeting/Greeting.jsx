import React from 'react';
import './Greeting.css';
import { useAuth } from '../../../context/AuthContext';

export default function Greeting({ children, showProfileTextButton }) {
  let user = {};
  try {
    user = useAuth()?.user || {};
  } catch (e) {
    user = {};
  }
  const name = user?.first_name || user?.user_name || user?.name;
  return (
    <div
      className={`dashboard-greeting${showProfileTextButton ? ' dashboard-greeting--with-profile-btn' : ''}`}
    >
      {children ? children : `Welcome back${name ? `, ${name}` : ''}!👋`}
    </div>
  );
}
