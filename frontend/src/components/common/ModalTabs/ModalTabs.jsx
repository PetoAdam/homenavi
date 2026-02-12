import React from 'react';
import './ModalTabs.css';

export default function ModalTabs({ tabs, activeTab, onChange, className = '' }) {
  if (!Array.isArray(tabs) || !tabs.length) {
    return null;
  }

  return (
    <div className={`auth-modal-tabs ${className}`.trim()}>
      {tabs.map((tab) => (
        <button
          key={tab.id}
          className={activeTab === tab.id ? 'active' : ''}
          onClick={() => onChange(tab.id)}
          type="button"
        >
          {tab.icon ? <span className="modal-tabs-icon">{tab.icon}</span> : null}
          <span>{tab.label}</span>
        </button>
      ))}
    </div>
  );
}
