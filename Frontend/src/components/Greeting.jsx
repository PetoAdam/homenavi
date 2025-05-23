import React from 'react';
import './Greeting.css';

export default function Greeting({ children }) {
  return (
    <div className="dashboard-greeting">
      {children}
    </div>
  );
}
