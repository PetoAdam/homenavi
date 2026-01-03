import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import * as assistantService from './assistantService';

// Mock the httpClient module at the top level
vi.mock('./httpClient', () => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn(),
}));

// Import the mocked module
import * as http from './httpClient';

describe('assistantService', () => {
  const mockToken = 'mock-jwt-token';
  
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  describe('getConversations', () => {
    it('fetches conversations with auth token', async () => {
      const mockConversations = { conversations: [
        { id: '1', title: 'Test Conversation' },
        { id: '2', title: 'Another Conversation' },
      ]};
      
      http.get.mockResolvedValueOnce(mockConversations);
      
      const result = await assistantService.getConversations(mockToken);
      
      expect(http.get).toHaveBeenCalledWith(
        '/api/assistant/conversations',
        { token: mockToken },
      );
      expect(result).toEqual(mockConversations);
    });
  });

  describe('createConversation', () => {
    it('creates a new conversation', async () => {
      const mockConversation = { id: '123', title: 'New Chat' };
      
      http.post.mockResolvedValueOnce(mockConversation);
      
      const result = await assistantService.createConversation(mockToken, 'New Chat');
      
      expect(http.post).toHaveBeenCalledWith(
        '/api/assistant/conversations',
        { title: 'New Chat' },
        { token: mockToken },
      );
      expect(result).toEqual(mockConversation);
    });

    it('uses default title if not provided', async () => {
      const mockConversation = { id: '123', title: 'New Conversation' };
      
      http.post.mockResolvedValueOnce(mockConversation);
      
      await assistantService.createConversation(mockToken);
      
      expect(http.post).toHaveBeenCalledWith(
        '/api/assistant/conversations',
        { title: 'New Conversation' },
        { token: mockToken },
      );
    });
  });

  describe('getConversation', () => {
    it('fetches a specific conversation by ID', async () => {
      const mockConversation = {
        id: '123',
        title: 'Test',
        messages: [{ id: '1', content: 'Hello' }],
      };
      
      http.get.mockResolvedValueOnce(mockConversation);
      
      const result = await assistantService.getConversation(mockToken, '123');
      
      expect(http.get).toHaveBeenCalledWith(
        '/api/assistant/conversations/123',
        { token: mockToken },
      );
      expect(result).toEqual(mockConversation);
    });
  });

  describe('updateConversation', () => {
    it('updates a conversation title', async () => {
      http.put.mockResolvedValueOnce({});
      
      await assistantService.updateConversation(mockToken, '123', 'Updated Title');
      
      expect(http.put).toHaveBeenCalledWith(
        '/api/assistant/conversations/123',
        { title: 'Updated Title' },
        { token: mockToken },
      );
    });
  });

  describe('deleteConversation', () => {
    it('deletes a conversation by ID', async () => {
      http.del.mockResolvedValueOnce({});
      
      await assistantService.deleteConversation(mockToken, '123');
      
      expect(http.del).toHaveBeenCalledWith(
        '/api/assistant/conversations/123',
        { token: mockToken },
      );
    });
  });

  describe('checkHealth', () => {
    it('checks service health status', async () => {
      http.get.mockResolvedValueOnce({ status: 'ok', ollama: true });
      
      const result = await assistantService.checkHealth();
      
      expect(http.get).toHaveBeenCalledWith('/api/assistant/health');
      expect(result.status).toBe('ok');
      expect(result.ollama).toBe(true);
    });
  });

  describe('getAdminStatus', () => {
    it('fetches admin status', async () => {
      const mockStatus = { activeConversations: 5, totalMessages: 100 };
      
      http.get.mockResolvedValueOnce(mockStatus);
      
      const result = await assistantService.getAdminStatus(mockToken);
      
      expect(http.get).toHaveBeenCalledWith(
        '/api/assistant/admin/status',
        { token: mockToken },
      );
      expect(result).toEqual(mockStatus);
    });
  });

  describe('getAdminModels', () => {
    it('fetches available models', async () => {
      const mockModels = { models: [
        { name: 'llama3.1:8b', size: 5000000000 },
        { name: 'llama3.2:3b', size: 2000000000 },
      ]};
      
      http.get.mockResolvedValueOnce(mockModels);
      
      const result = await assistantService.getAdminModels(mockToken);
      
      expect(http.get).toHaveBeenCalledWith(
        '/api/assistant/admin/models',
        { token: mockToken },
      );
      expect(result).toEqual(mockModels);
    });
  });

  describe('sendWSMessage', () => {
    it('sends message when WebSocket is open', () => {
      const mockWs = {
        readyState: 1, // WebSocket.OPEN
        send: vi.fn(),
      };
      
      const result = assistantService.sendWSMessage(mockWs, 'message', 'Hello', { foo: 'bar' });
      
      expect(mockWs.send).toHaveBeenCalledWith(JSON.stringify({
        type: 'message',
        content: 'Hello',
        data: { foo: 'bar' },
      }));
      expect(result).toBe(true);
    });

    it('returns false when WebSocket is not open', () => {
      const mockWs = {
        readyState: 3, // WebSocket.CLOSED
        send: vi.fn(),
      };
      
      const result = assistantService.sendWSMessage(mockWs, 'message', 'Hello');
      
      expect(mockWs.send).not.toHaveBeenCalled();
      expect(result).toBe(false);
    });

    it('returns false when WebSocket is null', () => {
      const result = assistantService.sendWSMessage(null, 'message', 'Hello');
      
      expect(result).toBe(false);
    });
  });

  describe('WS_MESSAGE_TYPES', () => {
    it('exports all message types', () => {
      expect(assistantService.WS_MESSAGE_TYPES).toEqual({
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
      });
    });
  });
});
