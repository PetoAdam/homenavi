import * as http from './httpClient';

const BASE = '/api/assistant';

/**
 * Get list of user's conversations
 */
export async function getConversations(token) {
  return http.get(`${BASE}/conversations`, { token });
}

/**
 * Create a new conversation
 */
export async function createConversation(token, title = 'New Conversation') {
  return http.post(`${BASE}/conversations`, { title }, { token });
}

/**
 * Get a specific conversation with messages
 */
export async function getConversation(token, conversationId) {
  return http.get(`${BASE}/conversations/${conversationId}`, { token });
}

/**
 * Update conversation title
 */
export async function updateConversation(token, conversationId, title) {
  return http.put(`${BASE}/conversations/${conversationId}`, { title }, { token });
}

/**
 * Delete a conversation
 */
export async function deleteConversation(token, conversationId) {
  return http.del(`${BASE}/conversations/${conversationId}`, { token });
}

/**
 * Health check
 */
export async function checkHealth() {
  return http.get(`${BASE}/health`);
}

/**
 * Admin: Get assistant status
 */
export async function getAdminStatus(token) {
  return http.get(`${BASE}/admin/status`, { token });
}

/**
 * Admin: List available models
 */
export async function getAdminModels(token) {
  return http.get(`${BASE}/admin/models`, { token });
}

/**
 * Create WebSocket connection for real-time chat
 * @returns {WebSocket} WebSocket connection
 */
export function createChatWebSocket() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/ws/assistant`;
  return new WebSocket(wsUrl);
}

/**
 * WebSocket message types
 */
export const WS_MESSAGE_TYPES = {
  MESSAGE: 'message',
  TOKEN: 'token',
  DONE: 'done',
  ERROR: 'error',
  TYPING: 'typing',
  CANCEL: 'cancel',
  NEW_CONVERSATION: 'new_conversation',
  CONVERSATION_CREATED: 'conversation_created',
  CONVERSATION_CLEARED: 'conversation_cleared',
  CANCELLED: 'cancelled',
};

/**
 * Helper to send a WebSocket message
 */
export function sendWSMessage(ws, type, content = '', data = null) {
  // WebSocket.OPEN = 1
  if (ws && ws.readyState === 1) {
    ws.send(JSON.stringify({ type, content, data }));
    return true;
  }
  return false;
}
