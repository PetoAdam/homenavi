import React from 'react';
import './Toolbar.css';

export default function Toolbar({
  left,
  right,
  className = '',
}) {
  return (
    <div className={['hn-toolbar', className].filter(Boolean).join(' ')}>
      {left ? <div className="hn-toolbar-left">{left}</div> : null}
      {right ? <div className="hn-toolbar-right">{right}</div> : null}
    </div>
  );
}
