import React from 'react';
import { createRoot } from 'react-dom/client';
import '../shared/hn.css';
import Player from '../shared/Player';

function TabApp() {
  return (
    <div className="hn-shell hn-tab-shell">
      <Player variant="tab" showSearch showQueue />
    </div>
  );
}

createRoot(document.getElementById('root')).render(<TabApp />);
