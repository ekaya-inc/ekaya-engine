import {
  Brain,
  MessageSquare,
  Send,
  Upload,
  FileText,
  CheckCircle2,
  X,
  Edit2,
  Save,
} from 'lucide-react';
import { useState, useRef } from 'react';

import { Button } from '../ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/Card';

interface BusinessRule {
  id: number;
  text: string;
  category: string;
  createdAt: Date;
  verified: boolean;
}

interface ChatMessage {
  id: number;
  type: 'user' | 'system' | 'ai';
  text: string;
  timestamp: Date;
  isFile?: boolean;
  fileName?: string;
}

interface BusinessLogicViewProps {
  businessRules: BusinessRule[];
  setBusinessRules: React.Dispatch<React.SetStateAction<BusinessRule[]>>;
}

const BusinessLogicView = ({ businessRules, setBusinessRules }: BusinessLogicViewProps) => {
  const [currentInput, setCurrentInput] = useState<string>('');
  const [editingRule, setEditingRule] = useState<number | null>(null);
  const [editText, setEditText] = useState<string>('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([
    {
      id: 1,
      type: 'system',
      text: 'Tell me about your business logic and rules. For example: "Our fiscal year ends in June" or "We consider a user active if they logged in within 30 days".',
      timestamp: new Date()
    }
  ]);

  // Handle adding new business rule
  const handleSendMessage = (): void => {
    if (!currentInput.trim()) return;

    const newMessage: ChatMessage = {
      id: chatMessages.length + 1,
      type: 'user',
      text: currentInput,
      timestamp: new Date()
    };

    setChatMessages([...chatMessages, newMessage]);

    // Add to business rules
    const newRule: BusinessRule = {
      id: businessRules.length + 1,
      text: currentInput,
      category: determineCategory(currentInput),
      createdAt: new Date(),
      verified: false
    };

    setBusinessRules([...businessRules, newRule]);

    // Simulate AI response
    setTimeout(() => {
      const aiResponse: ChatMessage = {
        id: chatMessages.length + 2,
        type: 'ai',
        text: `Thank you. I've recorded this business rule: "${currentInput}". This will help me better understand your data queries.`,
        timestamp: new Date()
      };
      setChatMessages(prev => [...prev, aiResponse]);
    }, 1000);

    setCurrentInput('');
  };

  // Determine category based on keywords
  const determineCategory = (text: string): string => {
    const lowerText = text.toLowerCase();
    if (lowerText.includes('fiscal') || lowerText.includes('financial') || lowerText.includes('revenue')) {
      return 'Financial';
    }
    if (lowerText.includes('user') || lowerText.includes('customer') || lowerText.includes('active')) {
      return 'User Behavior';
    }
    if (lowerText.includes('inventory') || lowerText.includes('stock') || lowerText.includes('product')) {
      return 'Inventory';
    }
    if (lowerText.includes('order') || lowerText.includes('shipping') || lowerText.includes('delivery')) {
      return 'Operations';
    }
    return 'General';
  };

  // Handle file upload
  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>): void => {
    const file = e.target.files?.[0];
    if (file) {
      const uploadMessage: ChatMessage = {
        id: chatMessages.length + 1,
        type: 'user',
        text: `Uploaded file: ${file.name}`,
        isFile: true,
        fileName: file.name,
        timestamp: new Date()
      };
      setChatMessages([...chatMessages, uploadMessage]);

      // Simulate processing
      setTimeout(() => {
        const aiResponse: ChatMessage = {
          id: chatMessages.length + 2,
          type: 'ai',
          text: `I've analyzed ${file.name} and extracted business rules from your documentation.`,
          timestamp: new Date()
        };
        setChatMessages(prev => [...prev, aiResponse]);
      }, 1500);
    }
  };

  // Handle editing a rule
  const handleEditRule = (rule: BusinessRule): void => {
    setEditingRule(rule.id);
    setEditText(rule.text);
  };

  // Handle saving edited rule
  const handleSaveRule = (): void => {
    setBusinessRules(businessRules.map(rule =>
      rule.id === editingRule ? { ...rule, text: editText } : rule
    ));
    setEditingRule(null);
    setEditText('');
  };

  // Handle deleting a rule
  const handleDeleteRule = (id: number): void => {
    setBusinessRules(businessRules.filter(rule => rule.id !== id));
  };

  // Handle verifying a rule
  const handleVerifyRule = (id: number): void => {
    setBusinessRules(businessRules.map(rule =>
      rule.id === id ? { ...rule, verified: !rule.verified } : rule
    ));
  };

  return (
    <>
      {/* Business Rules List */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Brain className="h-5 w-5 text-purple-500" />
            <CardTitle>Business Logic & Rules</CardTitle>
          </div>
          <CardDescription>
            Your documented business rules that guide data interpretation - {businessRules.filter(r => r.verified).length}/{businessRules.length} verified
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3 max-h-64 overflow-y-auto mb-4">
            {businessRules.length === 0 ? (
              <p className="text-sm text-text-secondary text-center py-4">
                No business rules documented yet. Use the discussion below to add your business logic.
              </p>
            ) : (
              businessRules.map((rule) => (
                <div
                  key={rule.id}
                  className={`p-3 rounded-lg border transition-all ${
                    rule.verified
                      ? 'border-green-500/30 bg-green-500/5'
                      : 'border-border-light bg-surface-secondary'
                  }`}
                >
                  <div className="flex items-start gap-3">
                    <button
                      onClick={() => handleVerifyRule(rule.id)}
                      className="mt-0.5 cursor-pointer"
                    >
                      <CheckCircle2
                        className={`h-4 w-4 ${
                          rule.verified ? 'text-green-500' : 'text-gray-400'
                        }`}
                      />
                    </button>
                    <div className="flex-1">
                      <div className="flex items-center gap-2 mb-1">
                        <span className="text-xs font-medium text-purple-500">
                          {rule.category}
                        </span>
                        <span className="text-xs text-text-tertiary">
                          {rule.createdAt.toLocaleDateString()}
                        </span>
                      </div>
                      {editingRule === rule.id ? (
                        <div className="flex gap-2">
                          <input
                            type="text"
                            value={editText}
                            onChange={(e) => setEditText(e.target.value)}
                            className="flex-1 text-sm px-2 py-1 border border-border-light rounded bg-surface-primary"
                          />
                          <button onClick={handleSaveRule}>
                            <Save className="h-4 w-4 text-green-500" />
                          </button>
                          <button onClick={() => setEditingRule(null)}>
                            <X className="h-4 w-4 text-red-500" />
                          </button>
                        </div>
                      ) : (
                        <p className="text-sm text-text-primary">{rule.text}</p>
                      )}
                    </div>
                    {editingRule !== rule.id && (
                      <div className="flex gap-1">
                        <button onClick={() => handleEditRule(rule)}>
                          <Edit2 className="h-4 w-4 text-text-tertiary hover:text-text-primary" />
                        </button>
                        <button onClick={() => handleDeleteRule(rule.id)}>
                          <X className="h-4 w-4 text-text-tertiary hover:text-red-500" />
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              ))
            )}
          </div>

          <div className="border-t border-border-light pt-4">
            <h4 className="text-sm font-medium text-text-primary mb-2">Common Business Rules to Document:</h4>
            <div className="text-xs text-text-secondary space-y-1">
              <p>• Fiscal year start/end dates</p>
              <p>• User activation criteria (e.g., &quot;30 days since last login&quot;)</p>
              <p>• Revenue recognition rules</p>
              <p>• Return/refund policies</p>
              <p>• Inventory management thresholds</p>
              <p>• Customer segmentation criteria</p>
              <p>• Seasonal business patterns</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Interactive Discussion */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <MessageSquare className="h-5 w-5 text-purple-500" />
            <CardTitle>Business Logic Discussion</CardTitle>
          </div>
          <CardDescription>
            Tell us about your business rules, calculations, and special considerations
          </CardDescription>
        </CardHeader>
        <CardContent>
          {/* Chat Messages */}
          <div className="border border-border-light rounded-lg p-4 h-64 overflow-y-auto mb-4 bg-surface-secondary/30">
            {chatMessages.map((message) => (
              <div key={message.id} className={`mb-3 ${message.type === 'user' ? 'text-right' : ''}`}>
                <div className={`inline-block max-w-[80%] p-3 rounded-lg ${
                  message.type === 'user'
                    ? 'bg-blue-500 text-white'
                    : message.type === 'system'
                    ? 'bg-purple-500/10 text-text-primary'
                    : 'bg-surface-secondary text-text-primary'
                }`}>
                  {message.isFile && (
                    <div className="flex items-center gap-2 mb-1">
                      <FileText className="h-4 w-4" />
                      <span className="text-sm font-medium">{message.fileName}</span>
                    </div>
                  )}
                  <p className="text-sm">{message.text}</p>
                </div>
                <div className="text-xs text-text-tertiary mt-1">
                  {message.timestamp.toLocaleTimeString()}
                </div>
              </div>
            ))}
          </div>

          {/* Input Area */}
          <div className="flex gap-2">
            <input
              type="text"
              value={currentInput}
              onChange={(e) => setCurrentInput(e.target.value)}
              onKeyPress={(e) => e.key === 'Enter' && handleSendMessage()}
              placeholder='e.g., "Our fiscal year ends in June" or "We calculate user activation as..."'
              className="flex-1 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary placeholder-text-tertiary focus:outline-none focus:ring-2 focus:ring-purple-500"
            />
            <input
              ref={fileInputRef}
              type="file"
              onChange={handleFileUpload}
              className="hidden"
              accept=".txt,.md,.doc,.docx,.pdf"
            />
            <Button
              variant="outline"
              size="icon"
              onClick={() => fileInputRef.current?.click()}
            >
              <Upload className="h-4 w-4" />
            </Button>
            <Button onClick={handleSendMessage}>
              <Send className="h-4 w-4 mr-2" />
              Send
            </Button>
          </div>
        </CardContent>
      </Card>
    </>
  );
};

export default BusinessLogicView;
