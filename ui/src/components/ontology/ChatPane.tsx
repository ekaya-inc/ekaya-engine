/**
 * ChatPane Component
 * Interactive streaming chat interface for ontology building
 */

import { Send, MessageCircle, Loader2, RotateCcw, Bot, User, Wrench, Sparkles } from 'lucide-react';
import { useState, useRef, useEffect, useCallback } from 'react';

import ontologyApi, { type ChatMessage, type ChatEvent } from '../../services/ontologyApi';

interface ChatPaneProps {
  projectId: string;
  onOntologyUpdate?: (update: { entity: string; field: string; summary: string }) => void;
  onKnowledgeStored?: (fact: { factType: string; key: string; value: string }) => void;
}

/**
 * Clean model output by removing think blocks, tool_call tags, and Qwen tokens.
 * This runs on the final content for display.
 */
function cleanModelOutput(content: string): string {
  // Remove <think>...</think> blocks
  content = content.replace(/<think>[\s\S]*?<\/think>/g, '');
  // Remove orphan tags
  content = content.replace(/<\/?think>/g, '');
  // Remove <tool_call>...</tool_call> blocks
  content = content.replace(/<tool_call>[\s\S]*?<\/tool_call>/g, '');
  // Remove Qwen tokens
  content = content.replace(/✿FUNCTION✿:[\s\S]*?(?:✿RETURN✿|✿RESULT✿|$)/g, '');
  content = content.replace(/✿(FUNCTION|ARGS|RESULT|RETURN)✿/g, '');
  // Clean whitespace
  content = content.trim().replace(/\n{3,}/g, '\n\n');
  return content;
}

interface DisplayMessage {
  id: string;
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string;
  isStreaming?: boolean;
  toolCalls?: Array<{ id: string; name: string; status: 'pending' | 'complete' }>;
}

const ChatPane = ({ projectId, onOntologyUpdate, onKnowledgeStored }: ChatPaneProps) => {
  const [messages, setMessages] = useState<DisplayMessage[]>([]);
  const [input, setInput] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const [isInitializing, setIsInitializing] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Auto-scroll to bottom when messages change
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // Initialize chat on mount
  useEffect(() => {
    const initChat = async () => {
      try {
        setIsInitializing(true);
        setError(null);

        // Try to get existing history first
        const historyResponse = await ontologyApi.getChatHistory(projectId, 50);

        if (historyResponse.messages && historyResponse.messages.length > 0) {
          // Convert to display format
          const displayMessages = historyResponse.messages.map((msg: ChatMessage) => ({
            id: msg.id,
            role: msg.role,
            content: msg.content,
          }));
          setMessages(displayMessages);

          // IMPORTANT: Even with history, check for NEW pending questions
          // (questions may have been generated after the last chat message)
          try {
            const initResponse = await ontologyApi.initializeChat(projectId);
            if (initResponse.has_pending_questions && initResponse.pending_count && initResponse.pending_count > 0) {
              // There are pending questions - generate and show a message about them
              const openingMsg = initResponse.opening_message;
              if (openingMsg) {
                setMessages(prev => [...prev, {
                  id: 'questions-' + Date.now(),
                  role: 'assistant' as const,
                  content: openingMsg,
                }]);
              }
            }
          } catch (initErr) {
            // Ignore init errors when we have history - non-critical
            console.debug('Could not check for pending questions:', initErr);
          }
        } else {
          // No history - initialize chat to get opening message
          const initResponse = await ontologyApi.initializeChat(projectId);
          if (initResponse.opening_message) {
            setMessages([{
              id: 'init-' + Date.now(),
              role: 'assistant',
              content: initResponse.opening_message,
            }]);
          }
        }
      } catch (err) {
        console.error('Failed to initialize chat:', err);
        // Start with a default message
        setMessages([{
          id: 'default-' + Date.now(),
          role: 'assistant',
          content: "Hello! I'm here to help you build your data ontology. What would you like to tell me about your data?",
        }]);
      } finally {
        setIsInitializing(false);
      }
    };

    initChat();
  }, [projectId]);

  // Handle incoming SSE events
  const handleEvent = useCallback((event: ChatEvent) => {
    switch (event.type) {
      case 'text':
        // Append text to current assistant message
        setMessages(prev => {
          const updated = [...prev];
          const lastMsg = updated[updated.length - 1];
          if (lastMsg?.role === 'assistant' && lastMsg.isStreaming) {
            lastMsg.content += event.content ?? '';
          }
          return updated;
        });
        break;

      case 'tool_call':
        // Show tool call in progress
        if (event.data) {
          const toolData = event.data as { id: string; name: string };
          setMessages(prev => {
            const updated = [...prev];
            const lastMsg = updated[updated.length - 1];
            if (lastMsg?.role === 'assistant') {
              lastMsg.toolCalls = [
                ...(lastMsg.toolCalls ?? []),
                { id: toolData.id, name: toolData.name, status: 'pending' }
              ];
            }
            return updated;
          });
        }
        break;

      case 'tool_result':
        // Mark tool call as complete
        if (event.data) {
          const resultData = event.data as { tool_call_id: string };
          setMessages(prev => {
            const updated = [...prev];
            const lastMsg = updated[updated.length - 1];
            if (lastMsg?.role === 'assistant' && lastMsg.toolCalls) {
              const toolCall = lastMsg.toolCalls.find(tc => tc.id === resultData.tool_call_id);
              if (toolCall) {
                toolCall.status = 'complete';
              }
            }
            return updated;
          });
        }
        break;

      case 'ontology_update':
        // Notify parent of ontology update
        if (event.data && onOntologyUpdate) {
          const updateData = event.data as { entity: string; field: string; summary: string };
          onOntologyUpdate(updateData);
        }
        break;

      case 'knowledge_stored':
        // Notify parent of knowledge stored
        if (event.data && onKnowledgeStored) {
          const factData = event.data as { fact_type: string; key: string; value: string };
          onKnowledgeStored({
            factType: factData.fact_type,
            key: factData.key,
            value: factData.value,
          });
        }
        break;

      case 'error':
        setError(event.content ?? 'An error occurred');
        setIsStreaming(false);
        break;

      case 'done':
        // Mark streaming complete
        setMessages(prev => {
          const updated = [...prev];
          const lastMsg = updated[updated.length - 1];
          if (lastMsg?.role === 'assistant') {
            lastMsg.isStreaming = false;
          }
          return updated;
        });
        setIsStreaming(false);
        break;
    }
  }, [onOntologyUpdate, onKnowledgeStored]);

  // Send message
  const sendMessage = useCallback(async () => {
    if (!input.trim() || isStreaming) return;

    const userMessage = input.trim();
    setInput('');
    setError(null);

    // Add user message
    const userMsgId = 'user-' + Date.now();
    setMessages(prev => [...prev, {
      id: userMsgId,
      role: 'user',
      content: userMessage,
    }]);

    // Add placeholder for assistant response
    const assistantMsgId = 'assistant-' + Date.now();
    setMessages(prev => [...prev, {
      id: assistantMsgId,
      role: 'assistant',
      content: '',
      isStreaming: true,
    }]);

    setIsStreaming(true);

    // Send message with SSE
    abortControllerRef.current = ontologyApi.sendChatMessage(
      projectId,
      userMessage,
      handleEvent,
      (err) => {
        setError(err.message);
        setIsStreaming(false);
        // Remove empty assistant message on error
        setMessages(prev => prev.filter(m => m.id !== assistantMsgId || m.content.length > 0));
      },
      () => {
        setIsStreaming(false);
      }
    );
  }, [input, isStreaming, projectId, handleEvent]);

  // Handle Enter key
  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  // Clear chat
  const handleClearChat = async () => {
    if (isStreaming) return;

    try {
      await ontologyApi.clearChatHistory(projectId);
      setMessages([{
        id: 'cleared-' + Date.now(),
        role: 'assistant',
        content: "Chat history cleared. How can I help you with your data ontology?",
      }]);
    } catch (err) {
      console.error('Failed to clear chat:', err);
    }
  };

  // Cancel streaming
  const handleCancel = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      setIsStreaming(false);
    }
  };

  if (isInitializing) {
    return (
      <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm">
        <div className="p-4 border-b border-border-light">
          <h3 className="font-semibold text-text-primary flex items-center gap-2">
            <MessageCircle className="h-5 w-5 text-purple-500" />
            Ontology Chat
          </h3>
        </div>
        <div className="p-8 flex flex-col items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-purple-500 mb-3" />
          <p className="text-text-secondary">Initializing chat...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm flex flex-col h-[600px]">
      {/* Header */}
      <div className="p-4 border-b border-border-light flex items-center justify-between">
        <h3 className="font-semibold text-text-primary flex items-center gap-2">
          <MessageCircle className="h-5 w-5 text-purple-500" />
          Ontology Chat
        </h3>
        <button
          onClick={handleClearChat}
          disabled={isStreaming}
          className="p-2 text-text-tertiary hover:text-text-secondary disabled:opacity-50 rounded-lg hover:bg-surface-secondary transition-colors"
          title="Clear chat history"
        >
          <RotateCcw className="h-4 w-4" />
        </button>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.map((message) => (
          <ChatBubble key={message.id} message={message} />
        ))}
        {error && (
          <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
            {error}
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="border-t border-border-light p-4">
        <div className="flex gap-2">
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type your response..."
            className="flex-1 resize-none rounded-lg border border-border-light bg-surface-secondary px-3 py-2 text-sm text-text-primary placeholder:text-text-tertiary focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent"
            rows={2}
            disabled={isStreaming}
          />
          <button
            onClick={isStreaming ? handleCancel : sendMessage}
            disabled={!isStreaming && !input.trim()}
            className={`px-4 py-2 rounded-lg font-medium transition-colors flex items-center gap-2 ${
              isStreaming
                ? 'bg-red-500 hover:bg-red-600 text-white'
                : 'bg-purple-600 hover:bg-purple-700 text-white disabled:opacity-50 disabled:cursor-not-allowed'
            }`}
          >
            {isStreaming ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Stop
              </>
            ) : (
              <>
                <Send className="h-4 w-4" />
                Send
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};

// Chat bubble component
interface ChatBubbleProps {
  message: DisplayMessage;
}

const ChatBubble = ({ message }: ChatBubbleProps) => {
  const isUser = message.role === 'user';
  const isAssistant = message.role === 'assistant';

  return (
    <div className={`flex gap-3 ${isUser ? 'flex-row-reverse' : ''}`}>
      {/* Avatar */}
      <div className={`flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center ${
        isUser ? 'bg-blue-100' : 'bg-purple-100'
      }`}>
        {isUser ? (
          <User className="h-4 w-4 text-blue-600" />
        ) : (
          <Bot className="h-4 w-4 text-purple-600" />
        )}
      </div>

      {/* Content */}
      <div className={`flex-1 ${isUser ? 'text-right' : ''}`}>
        <div className={`inline-block rounded-lg px-4 py-2 max-w-[85%] ${
          isUser
            ? 'bg-blue-600 text-white'
            : 'bg-surface-secondary text-text-primary'
        }`}>
          {/* Tool calls indicator */}
          {isAssistant && message.toolCalls && message.toolCalls.length > 0 && (
            <div className="mb-2 pb-2 border-b border-border-light">
              {message.toolCalls.map((tc) => (
                <div key={tc.id} className="flex items-center gap-2 text-xs text-text-secondary">
                  <Wrench className="h-3 w-3" />
                  <span>{formatToolName(tc.name)}</span>
                  {tc.status === 'pending' ? (
                    <Loader2 className="h-3 w-3 animate-spin" />
                  ) : (
                    <Sparkles className="h-3 w-3 text-green-500" />
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Message content */}
          <p className="text-sm whitespace-pre-wrap">{cleanModelOutput(message.content)}</p>

          {/* Streaming indicator */}
          {message.isStreaming && !cleanModelOutput(message.content) && (
            <div className="flex items-center gap-2 text-sm text-text-tertiary">
              <Loader2 className="h-4 w-4 animate-spin" />
              <span>Thinking...</span>
            </div>
          )}
          {message.isStreaming && cleanModelOutput(message.content) && (
            <span className="inline-block w-1 h-4 bg-purple-500 animate-pulse ml-1" />
          )}
        </div>
      </div>
    </div>
  );
};

// Format tool name for display
const formatToolName = (name: string): string => {
  const nameMap: Record<string, string> = {
    query_column_values: 'Querying column values',
    query_schema_metadata: 'Checking schema',
    store_knowledge: 'Storing knowledge',
    update_entity: 'Updating entity',
    update_domain: 'Updating domain',
  };
  return nameMap[name] ?? name;
};

export default ChatPane;
