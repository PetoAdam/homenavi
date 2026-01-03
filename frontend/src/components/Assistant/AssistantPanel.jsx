import React, { useState, useEffect, useRef, useCallback } from 'react';
import ChatMessage from './ChatMessage';
import TypingIndicator from './TypingIndicator';
import ChatInput from './ChatInput';
import './AssistantPanel.css';

export default function AssistantPanel({ isOpen, onClose, onProcessingChange, accessToken, userName }) {
  const [messages, setMessages] = useState([]);
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [isTyping, setIsTyping] = useState(false);
  const [connectionError, setConnectionError] = useState(null);
  const wsRef = useRef(null);
  const messagesEndRef = useRef(null);
  const reconnectTimeoutRef = useRef(null);

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, isTyping, scrollToBottom]);

  // WebSocket connection management
  const connect = useCallback(() => {
    if (!isOpen || !accessToken || wsRef.current?.readyState === WebSocket.OPEN) {
      return;
    }

    setIsConnecting(true);
    setConnectionError(null);

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/assistant`;

    try {
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        setIsConnected(true);
        setIsConnecting(false);
        setConnectionError(null);
        console.log('Assistant WebSocket connected');
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          handleWSMessage(data);
        } catch (e) {
          console.error('Failed to parse WebSocket message:', e);
        }
      };

      ws.onclose = (event) => {
        setIsConnected(false);
        setIsConnecting(false);
        wsRef.current = null;

        if (event.code !== 1000 && isOpen) {
          setConnectionError('Connection lost. Reconnecting...');
          reconnectTimeoutRef.current = setTimeout(connect, 3000);
        }
      };

      ws.onerror = () => {
        setIsConnected(false);
        setIsConnecting(false);
        setConnectionError('Failed to connect to assistant');
      };
    } catch (error) {
      setIsConnecting(false);
      setConnectionError('Failed to create WebSocket connection');
    }
  }, [isOpen, accessToken]);

  const handleWSMessage = (data) => {
    switch (data.type) {
      case 'token':
        // Streaming token - append to last assistant message
        setMessages(prev => {
          const last = prev[prev.length - 1];
          if (last?.role === 'assistant' && last.streaming) {
            return [
              ...prev.slice(0, -1),
              { ...last, content: last.content + data.content }
            ];
          }
          // Start new assistant message
          return [...prev, { role: 'assistant', content: data.content, streaming: true }];
        });
        setIsTyping(false);
        break;

      case 'done':
        // Stream complete
        setMessages(prev => {
          const last = prev[prev.length - 1];
          if (last?.streaming) {
            return [...prev.slice(0, -1), { ...last, streaming: false }];
          }
          return prev;
        });
        setIsTyping(false);
        onProcessingChange(false);
        break;

      case 'typing':
        setIsTyping(true);
        break;

      case 'error':
        setMessages(prev => [...prev, { role: 'system', content: data.content, error: true }]);
        setIsTyping(false);
        onProcessingChange(false);
        break;

      case 'conversation_created':
        console.log('Conversation created:', data.content);
        break;

      case 'conversation_cleared':
        setMessages([]);
        break;

      case 'cancelled':
        setIsTyping(false);
        onProcessingChange(false);
        break;

      default:
        console.log('Unknown WS message type:', data.type);
    }
  };

  useEffect(() => {
    if (isOpen) {
      connect();
    }

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
    };
  }, [isOpen, connect]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (wsRef.current) {
        wsRef.current.close(1000);
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
    };
  }, []);

  const sendMessage = useCallback((content) => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      setConnectionError('Not connected. Trying to reconnect...');
      connect();
      return;
    }

    const trimmedContent = content.trim();
    if (!trimmedContent) return;

    // Add user message to UI immediately
    setMessages(prev => [...prev, { role: 'user', content: trimmedContent }]);
    setIsTyping(true);
    onProcessingChange(true);

    // Send to server
    wsRef.current.send(JSON.stringify({ type: 'message', content: trimmedContent }));
  }, [connect, onProcessingChange]);

  const handleQuickAction = (action) => {
    sendMessage(action);
  };

  const handleNewConversation = () => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: 'new_conversation' }));
    }
    setMessages([]);
  };

  return (
    <div className={`assistant-panel ${isOpen ? 'open' : ''}`}>
      {/* Header */}
      <div className="assistant-panel-header">
        <div className="assistant-panel-title">
          <div className="ai-title-icon">✨</div>
          <div>
            <div>Homenavi AI</div>
            <div className={`assistant-panel-status ${isConnected ? 'connected' : ''}`}>
              {isConnecting ? 'Connecting...' : isConnected ? 'Connected' : 'Disconnected'}
            </div>
          </div>
        </div>
        <button className="assistant-close-btn" onClick={onClose} aria-label="Close">
          ×
        </button>
      </div>

      {/* Connection error banner */}
      {connectionError && (
        <div className="connection-banner">
          {connectionError}
          <button onClick={connect}>Retry</button>
        </div>
      )}

      {/* Messages */}
      <div className="assistant-messages">
        {messages.length === 0 ? (
          <div className="assistant-welcome">
            <div className="welcome-icon">🏠</div>
            <h3>Welcome, {userName}!</h3>
            <p>I'm your Homenavi AI assistant. I can help you control devices, create automations, and answer questions about your home.</p>
            <div className="quick-actions">
              <button onClick={() => handleQuickAction("What devices are currently on?")}>
                💡 Active devices
              </button>
              <button onClick={() => handleQuickAction("Show system status")}>
                📊 System status
              </button>
              <button onClick={() => handleQuickAction("What can you help me with?")}>
                ❓ What can you do?
              </button>
            </div>
          </div>
        ) : (
          <>
            {messages.map((msg, index) => (
              <ChatMessage key={index} message={msg} />
            ))}
          </>
        )}

        {isTyping && <TypingIndicator />}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <ChatInput
        onSend={sendMessage}
        disabled={!isConnected || isTyping}
        placeholder={
          !isConnected 
            ? 'Connecting...' 
            : isTyping 
              ? 'AI is responding...' 
              : 'Ask me anything...'
        }
      />
    </div>
  );
}
