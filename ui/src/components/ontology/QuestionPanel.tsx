/**
 * QuestionPanel Component
 * Application-controlled state machine for ontology questions.
 * Shows one question at a time with Answer/Skip/Delete buttons.
 */

import { HelpCircle, Loader2, CheckCircle, SkipForward, Trash2, Send, AlertCircle, MessageCircle, ChevronDown, ChevronUp, Brain } from 'lucide-react';
import { useState, useEffect, useCallback } from 'react';

import { useToast } from '../../hooks/useToast';
import ontologyApi from '../../services/ontologyApi';
import type { QuestionDTO, QuestionPanelState } from '../../types';

interface QuestionPanelProps {
  projectId: string;
  onAllComplete?: () => void;
  onQuestionAnswered?: (questionId: string, actionsSummary: string) => void;
}

const QuestionPanel = ({ projectId, onAllComplete, onQuestionAnswered }: QuestionPanelProps) => {
  const { toast } = useToast();
  const [state, setState] = useState<QuestionPanelState>('loading');
  const [currentQuestion, setCurrentQuestion] = useState<QuestionDTO | null>(null);
  const [answer, setAnswer] = useState('');
  const [followUp, setFollowUp] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [thinking, setThinking] = useState<string | null>(null);
  const [thinkingExpanded, setThinkingExpanded] = useState(false);

  const loadNextQuestion = useCallback(async () => {
    setState('loading');
    setError(null);
    setFollowUp(null);
    setThinking(null);
    setThinkingExpanded(false);
    setAnswer('');

    try {
      const response = await ontologyApi.getNextQuestion(projectId);

      if (response.all_complete) {
        setState('all_complete');
        setCurrentQuestion(null);
        onAllComplete?.();
      } else if (response.question) {
        setCurrentQuestion(response.question);
        setState('showing_question');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load question');
      setState('showing_question');
    }
  }, [projectId, onAllComplete]);

  // Load the first question on mount
  useEffect(() => {
    loadNextQuestion();
  }, [loadNextQuestion]);

  const handleSubmitAnswer = async () => {
    if (!currentQuestion || !answer.trim()) return;

    setState('waiting_for_llm');
    setError(null);

    try {
      const response = await ontologyApi.answerQuestion(
        projectId,
        currentQuestion.id,
        answer.trim()
      );

      // Store thinking if available
      if (response.thinking) {
        setThinking(response.thinking);
      }

      if (response.follow_up) {
        // LLM needs clarification
        setFollowUp(response.follow_up);
        setAnswer('');
        setState('showing_follow_up');
      } else {
        // Question is done - show toast notification
        if (response.actions_summary) {
          toast({
            title: 'Ontology Updated',
            description: response.actions_summary,
            variant: 'success',
            duration: 4000,
          });
          onQuestionAnswered?.(currentQuestion.id, response.actions_summary);
        }

        if (response.all_complete) {
          setState('all_complete');
          setCurrentQuestion(null);
          onAllComplete?.();
        } else if (response.next_question) {
          setCurrentQuestion(response.next_question);
          setAnswer('');
          setFollowUp(null);
          setThinking(null);
          setThinkingExpanded(false);
          setState('showing_question');
        } else {
          // No next question returned, fetch it
          loadNextQuestion();
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to process answer');
      setState('showing_question');
    }
  };

  const handleSkip = async () => {
    if (!currentQuestion) return;

    setState('waiting_for_llm');
    setError(null);

    try {
      const response = await ontologyApi.skipQuestion(projectId, currentQuestion.id);

      if (response.all_complete) {
        setState('all_complete');
        setCurrentQuestion(null);
        onAllComplete?.();
      } else if (response.next_question) {
        setCurrentQuestion(response.next_question);
        setAnswer('');
        setFollowUp(null);
        setState('showing_question');
      } else {
        loadNextQuestion();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to skip question');
      setState('showing_question');
    }
  };

  const handleDelete = async () => {
    if (!currentQuestion) return;

    setState('waiting_for_llm');
    setError(null);

    try {
      const response = await ontologyApi.deleteQuestion(projectId, currentQuestion.id);

      if (response.all_complete) {
        setState('all_complete');
        setCurrentQuestion(null);
        onAllComplete?.();
      } else if (response.next_question) {
        setCurrentQuestion(response.next_question);
        setAnswer('');
        setFollowUp(null);
        setState('showing_question');
      } else {
        loadNextQuestion();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete question');
      setState('showing_question');
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmitAnswer();
    }
  };

  // Loading state
  if (state === 'loading') {
    return (
      <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm p-8">
        <div className="flex flex-col items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-purple-500 mb-3" />
          <p className="text-text-secondary">Loading questions...</p>
        </div>
      </div>
    );
  }

  // All complete state
  if (state === 'all_complete') {
    return (
      <div className="rounded-lg border border-green-200 bg-green-50 shadow-sm p-8">
        <div className="flex flex-col items-center justify-center">
          <CheckCircle className="h-12 w-12 text-green-500 mb-4" />
          <h3 className="text-lg font-semibold text-green-800 mb-2">All Questions Answered!</h3>
          <p className="text-green-700 text-center">
            Great work! You&apos;ve answered all the ontology questions.
            Your data model is now more complete and accurate.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm">
      {/* Header */}
      <div className="p-4 border-b border-border-light flex items-center justify-between">
        <h3 className="font-semibold text-text-primary flex items-center gap-2">
          <HelpCircle className="h-5 w-5 text-purple-500" />
          Ontology Question
        </h3>
        {currentQuestion && (
          <div className="flex items-center gap-2">
            <span className={`px-2 py-1 text-xs font-medium rounded-full ${
              currentQuestion.priority <= 2
                ? 'bg-red-100 text-red-700'
                : currentQuestion.priority <= 3
                ? 'bg-yellow-100 text-yellow-700'
                : 'bg-gray-100 text-gray-700'
            }`}>
              Priority {currentQuestion.priority}
            </span>
            <span className="px-2 py-1 text-xs font-medium rounded-full bg-purple-100 text-purple-700">
              {currentQuestion.category}
            </span>
          </div>
        )}
      </div>

      {/* Question content */}
      <div className="p-6">
        {/* Error display */}
        {error && (
          <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm flex items-start gap-2">
            <AlertCircle className="h-4 w-4 mt-0.5 flex-shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {currentQuestion && (
          <>
            {/* Question text */}
            <div className="mb-4">
              <p className="text-lg text-text-primary">{currentQuestion.text}</p>
              {currentQuestion.reasoning && (
                <p className="mt-2 text-sm text-text-secondary italic">
                  {currentQuestion.reasoning}
                </p>
              )}
            </div>

            {/* Affected tables/columns */}
            {((currentQuestion.affected_tables?.length ?? 0) > 0 || (currentQuestion.affected_columns?.length ?? 0) > 0) && (
              <div className="mb-4 p-3 bg-surface-secondary rounded-lg">
                <p className="text-xs text-text-tertiary mb-2">Affects:</p>
                <div className="flex flex-wrap gap-2">
                  {currentQuestion.affected_tables?.map((table) => (
                    <span key={table} className="px-2 py-1 text-xs bg-blue-100 text-blue-700 rounded">
                      {table}
                    </span>
                  ))}
                  {currentQuestion.affected_columns?.map((col) => (
                    <span key={col} className="px-2 py-1 text-xs bg-purple-100 text-purple-700 rounded">
                      {col}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* LLM Thinking (collapsible - for debugging) */}
            {thinking && (
              <div className="mb-4">
                <button
                  onClick={() => setThinkingExpanded(!thinkingExpanded)}
                  className="w-full flex items-center justify-between p-3 bg-slate-100 border border-slate-200 rounded-lg hover:bg-slate-150 transition-colors text-left"
                >
                  <div className="flex items-center gap-2">
                    <Brain className="h-4 w-4 text-slate-500" />
                    <span className="text-sm font-medium text-slate-600">LLM Thinking Process</span>
                  </div>
                  {thinkingExpanded ? (
                    <ChevronUp className="h-4 w-4 text-slate-500" />
                  ) : (
                    <ChevronDown className="h-4 w-4 text-slate-500" />
                  )}
                </button>
                {thinkingExpanded && (
                  <div className="mt-2 p-4 bg-slate-50 border border-slate-200 rounded-lg">
                    <pre className="text-xs text-slate-600 whitespace-pre-wrap font-mono overflow-x-auto">
                      {thinking}
                    </pre>
                  </div>
                )}
              </div>
            )}

            {/* Follow-up from LLM */}
            {followUp && (
              <div className="mb-4 p-4 bg-amber-50 border border-amber-200 rounded-lg">
                <div className="flex items-start gap-3">
                  <MessageCircle className="h-5 w-5 text-amber-600 mt-0.5 flex-shrink-0" />
                  <div>
                    <p className="text-sm font-medium text-amber-800 mb-1">Follow-up Question</p>
                    <p className="text-sm text-amber-700">{followUp}</p>
                  </div>
                </div>
              </div>
            )}

            {/* Answer input */}
            <div className="mb-4">
              <textarea
                value={answer}
                onChange={(e) => setAnswer(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder={followUp ? "Provide clarification..." : "Type your answer..."}
                className="w-full resize-none rounded-lg border border-border-light bg-surface-secondary px-4 py-3 text-sm text-text-primary placeholder:text-text-tertiary focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent"
                rows={4}
                disabled={state === 'waiting_for_llm'}
              />
            </div>

            {/* Action buttons */}
            <div className="flex justify-between items-center">
              <div className="flex gap-2">
                <button
                  onClick={handleSkip}
                  disabled={state === 'waiting_for_llm'}
                  className="px-4 py-2 text-sm font-medium text-text-secondary bg-surface-secondary rounded-lg hover:bg-surface-tertiary transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                >
                  <SkipForward className="h-4 w-4" />
                  Skip
                </button>
                <button
                  onClick={handleDelete}
                  disabled={state === 'waiting_for_llm'}
                  className="px-4 py-2 text-sm font-medium text-red-600 bg-red-50 rounded-lg hover:bg-red-100 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                >
                  <Trash2 className="h-4 w-4" />
                  Don&apos;t Ask Again
                </button>
              </div>

              <button
                onClick={handleSubmitAnswer}
                disabled={!answer.trim() || state === 'waiting_for_llm'}
                className="px-6 py-2 text-sm font-medium text-white bg-purple-600 rounded-lg hover:bg-purple-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
              >
                {state === 'waiting_for_llm' ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    Processing...
                  </>
                ) : (
                  <>
                    <Send className="h-4 w-4" />
                    Submit Answer
                  </>
                )}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

export default QuestionPanel;
