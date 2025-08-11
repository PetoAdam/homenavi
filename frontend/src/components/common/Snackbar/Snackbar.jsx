import React, { useEffect, useState } from 'react';
import './Snackbar.css';

export default function Snackbar({ message, duration = 2500, onClose }) {
  const [open, setOpen] = useState(!!message);
  useEffect(() => {
    if (message) {
      setOpen(true);
      const t = setTimeout(() => { setOpen(false); if (onClose) onClose(); }, duration);
      return () => clearTimeout(t);
    }
  }, [message, duration, onClose]);
  if (!message && !open) return null;
  return (
    <div className={`snackbar${open ? ' snackbar--show' : ''}`}>{message}</div>
  );
}
