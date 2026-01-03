import React from 'react';

export default function TypingIndicator({ visible = true }) {
  if (!visible) {
    return null;
  }

  return (
    <div 
      className="typing-indicator"
      data-testid="typing-indicator"
      role="status"
      aria-label="AI is typing"
    >
      <span data-testid="typing-dot"></span>
      <span data-testid="typing-dot"></span>
      <span data-testid="typing-dot"></span>
    </div>
  );
}
