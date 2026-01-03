import React, { useState } from 'react';
import { useAuth } from '../../context/AuthContext';
import AssistantPanel from './AssistantPanel';
import './AssistantButton.css';

// AI Icon - Apple Intelligence inspired
const AIIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
    <defs>
      <linearGradient id="ai-gradient" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%" stopColor="#22c55e" />
        <stop offset="50%" stopColor="#7850dc" />
        <stop offset="100%" stopColor="#3b82f6" />
      </linearGradient>
      <linearGradient id="ai-gradient-light" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%" stopColor="rgba(255,255,255,0.9)" />
        <stop offset="50%" stopColor="rgba(34,197,94,0.8)" />
        <stop offset="100%" stopColor="rgba(120,80,220,0.7)" />
      </linearGradient>
    </defs>
    {/* Central orb */}
    <circle cx="12" cy="12" r="4" fill="url(#ai-gradient-light)" />
    {/* Outer ring */}
    <circle 
      cx="12" 
      cy="12" 
      r="8" 
      stroke="url(#ai-gradient)" 
      strokeWidth="1.5" 
      fill="none"
      opacity="0.6"
    />
    {/* Sparkle lines */}
    <path 
      d="M12 2V5M12 19V22M2 12H5M19 12H22" 
      stroke="url(#ai-gradient)" 
      strokeWidth="2" 
      strokeLinecap="round"
      opacity="0.8"
    />
    {/* Diagonal sparkles */}
    <path 
      d="M5.64 5.64L7.76 7.76M16.24 16.24L18.36 18.36M5.64 18.36L7.76 16.24M16.24 7.76L18.36 5.64" 
      stroke="url(#ai-gradient)" 
      strokeWidth="1.5" 
      strokeLinecap="round"
      opacity="0.5"
    />
  </svg>
);

export default function AssistantButton() {
  const [isOpen, setIsOpen] = useState(false);
  const [isProcessing, setIsProcessing] = useState(false);
  const { user, accessToken } = useAuth();

  // Only show for authenticated users with appropriate role
  if (!user) {
    return null;
  }

  // Check if user has at least resident role
  const allowedRoles = ['resident', 'admin', 'service'];
  if (!allowedRoles.includes(user.role)) {
    return null;
  }

  const handleClick = () => {
    setIsOpen(!isOpen);
  };

  return (
    <>
      <button
        className={`assistant-button ${isProcessing ? 'processing' : ''} ${isOpen ? 'open' : ''}`}
        onClick={handleClick}
        aria-label={isOpen ? 'Close AI Assistant' : 'Open AI Assistant'}
        title="Homenavi AI Assistant"
      >
        <div className="assistant-button-icon">
          <AIIcon />
        </div>
      </button>

      <AssistantPanel
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        onProcessingChange={setIsProcessing}
        accessToken={accessToken}
        userName={user?.name || user?.email || 'User'}
      />
    </>
  );
}
