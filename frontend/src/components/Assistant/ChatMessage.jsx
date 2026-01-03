import React, { useMemo } from 'react';

export default function ChatMessage({ message }) {
  const { role, content, error, streaming, timestamp } = message;

  const getClassName = () => {
    let className = `chat-message ${role}`;
    if (error) className += ' error';
    if (streaming) className += ' streaming';
    return className;
  };

  // Enhanced markdown formatting with code blocks, lists, headers, and JSON
  const formatContent = useMemo(() => {
    if (!content) return '';
    
    let text = content;
    
    // Escape HTML to prevent XSS
    text = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    
    // Handle code blocks with triple backticks (```language\ncode```)
    text = text.replace(/```(\w*)\n?([\s\S]*?)```/g, (match, lang, code) => {
      // Try to detect JSON and format it
      let formattedCode = code.trim();
      if (lang === 'json' || (!lang && formattedCode.startsWith('{') || formattedCode.startsWith('['))) {
        try {
          const parsed = JSON.parse(formattedCode);
          formattedCode = JSON.stringify(parsed, null, 2);
        } catch (e) {
          // Not valid JSON, keep as-is
        }
      }
      const langClass = lang ? ` class="language-${lang}"` : '';
      return `<pre><code${langClass}>${formattedCode}</code></pre>`;
    });
    
    // Handle inline JSON objects/arrays that aren't in code blocks
    text = text.replace(/(?<!<code[^>]*>)(\{[\s\S]*?\}|\[[\s\S]*?\])(?![^<]*<\/code>)/g, (match) => {
      try {
        const parsed = JSON.parse(match);
        const formatted = JSON.stringify(parsed, null, 2);
        return `<pre><code class="language-json">${formatted}</code></pre>`;
      } catch (e) {
        return match;
      }
    });
    
    // Handle headers (## Header)
    text = text.replace(/^### (.+)$/gm, '<h4>$1</h4>');
    text = text.replace(/^## (.+)$/gm, '<h3>$1</h3>');
    text = text.replace(/^# (.+)$/gm, '<h2>$1</h2>');
    
    // Handle unordered lists (- item or * item)
    text = text.replace(/^[\-\*] (.+)$/gm, '<li>$1</li>');
    // Wrap consecutive <li> in <ul>
    text = text.replace(/(<li>.*<\/li>\n?)+/g, (match) => `<ul>${match}</ul>`);
    
    // Handle numbered lists (1. item)
    text = text.replace(/^\d+\. (.+)$/gm, '<li>$1</li>');
    // Note: This creates issues with ol vs ul - for simplicity, using ul for all
    
    // Convert **bold** to <strong>
    text = text.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
    
    // Convert *italic* to <em> (but not if already processed as list)
    text = text.replace(/(?<!\*)\*(?!\*)(.+?)(?<!\*)\*(?!\*)/g, '<em>$1</em>');
    
    // Convert `inline code` to <code> (but not if already in a code block)
    text = text.replace(/(?<!<code[^>]*>)`([^`]+)`(?![^<]*<\/code>)/g, '<code class="inline">$1</code>');
    
    // Convert horizontal rules (--- or ***)
    text = text.replace(/^[\-\*]{3,}$/gm, '<hr>');
    
    // Convert newlines to <br> (but not right after block elements)
    text = text.replace(/(?<!<\/pre>|<\/ul>|<\/li>|<\/h[234]>|<hr>)\n/g, '<br>');
    
    // Clean up extra <br> after block elements
    text = text.replace(/(<\/pre>|<\/ul>|<\/h[234]>|<hr>)<br>/g, '$1');
    
    return text;
  }, [content]);

  const formatTimestamp = (ts) => {
    if (!ts) return null;
    const date = ts instanceof Date ? ts : new Date(ts);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  return (
    <div className={getClassName()}>
      <div 
        className="chat-message-content"
        dangerouslySetInnerHTML={{ __html: formatContent }}
      />
      {timestamp && (
        <time className="chat-message-time" dateTime={timestamp instanceof Date ? timestamp.toISOString() : timestamp}>
          {formatTimestamp(timestamp)}
        </time>
      )}
      {streaming && <span className="streaming-cursor">▊</span>}
    </div>
  );
}
