import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faUserCircle } from '@fortawesome/free-solid-svg-icons';

export default function UserAvatar({ user, size = 38, background = "var(--color-primary)" }) {
  if (!user) {
    return <FontAwesomeIcon icon={faUserCircle} style={{ fontSize: size, color: 'var(--color-secondary-xlight)', background: 'transparent' }} />;
  }
  let initials = '';
  if (user?.first_name || user?.last_name) {
    initials = (user?.first_name?.[0]?.toUpperCase() || '') + (user?.last_name?.[0]?.toUpperCase() || '');
  } else if (user?.user_name) {
    initials = user.user_name.slice(0, 2).toUpperCase();
  }
  if (user?.avatar) {
    return (
      <img
        src={user.avatar}
        alt="Profile"
        style={{ width: size, height: size, borderRadius: '50%', objectFit: 'cover', background }}
      />
    );
  }
  // Calculate font size based on avatar size
  const fontSize = size > 36 ? '1.6rem' : `${0.4 + (size / 38) * 0.6}rem`;
  return (
    <span
      style={{
        display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: 700,
        fontSize, background, borderRadius: '50%', width: size, height: size, color: '#fff', WebkitTextStroke: '0.5px rgba(0,0,0,0.08)', textShadow: '0 1px 2px rgba(0,0,0,0.12)',
      }}
    >
      {initials}
    </span>
  );
}
