import React from 'react';

export default function Greeting({ children }) {
  return (
    <div className="dashboard-greeting">
      {children}
    </div>
  );
}
