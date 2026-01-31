import React from 'react';
import { createRoot } from 'react-dom/client';
import '../shared/hn.css';
import { fetchCounter, incrementCounter } from '../shared/api';

function WidgetApp() {
  const [count, setCount] = React.useState(0);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    let alive = true;
    fetchCounter()
      .then((data) => { if (alive) setCount(data?.count ?? 0); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, []);

  return (
    <div className="hn-shell" style={{ padding: 10 }}>
      <div className="hn-card" style={{ padding: 12 }}>
        <div className="hn-row">
          <div>
            <h1 className="hn-title" style={{ fontSize: 16 }}>Hello Widget</h1>
            <div className="hn-subtitle">Matches the tab UI style</div>
          </div>
          <span className="hn-pill">widget</span>
        </div>

        <div className="hn-section" style={{ marginTop: 12, padding: 12 }}>
          <div className="hn-counter">
            <div className="hn-stat">
              <div className="hn-stat-label">Clicked</div>
              <div className="hn-stat-value">{loading ? '…' : count}</div>
            </div>
            <button
              className="hn-btn"
              onClick={async () => {
                const data = await incrementCounter(1);
                setCount(data?.count ?? count + 1);
              }}
            >
              Increase
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

createRoot(document.getElementById('root')).render(<WidgetApp />);
