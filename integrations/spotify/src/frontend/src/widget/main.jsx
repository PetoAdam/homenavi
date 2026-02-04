import React from 'react';
import { createRoot } from 'react-dom/client';
import '../shared/hn.css';
import Player from '../shared/Player';

function WidgetApp() {
  return (
    <div className="hn-shell hn-widget-shell" style={{ padding: 0 }}>
      <Player variant="widget" showQueue />
    </div>
  );
}

createRoot(document.getElementById('root')).render(<WidgetApp />);
