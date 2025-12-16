import {
  AlertCircle,
  HelpCircle,
  Send,
  ChevronDown,
  ChevronUp,
  CheckCircle,
} from 'lucide-react';
import { useState } from 'react';

import { Button } from '../ui/Button';

type QuestionCategory = 'CRITICAL' | 'Important' | 'Good to Know';

interface Question {
  id: string | number;
  text: string;
  category: QuestionCategory;
}

interface QuestionStats {
  critical: unknown[];
  important: unknown[];
  goodToKnow: unknown[];
}

interface QuestionsViewProps {
  questions: Question[];
  setSelectedQuestion: (question: Question | null) => void;
  questionStats: QuestionStats;
  projectId?: string | undefined;
  workflowId?: string | null | undefined;
  isAwaitingAnswers?: boolean | undefined;
  onSubmitAnswers?: ((
    answers: Array<{ question_id: string; answer: string }>
  ) => void) | undefined;
}

const QuestionsView = ({
  questions,
  setSelectedQuestion: _setSelectedQuestion,
  questionStats,
  projectId: _projectId,
  workflowId: _workflowId,
  isAwaitingAnswers = false,
  onSubmitAnswers,
}: QuestionsViewProps) => {
  const [expandedQuestion, setExpandedQuestion] = useState<
    string | number | null
  >(null);
  const [responseText, setResponseText] = useState<string>('');
  const [userAnswers, setUserAnswers] = useState<Record<string | number, string>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleQuestionClick = (question: Question): void => {
    if (expandedQuestion === question.id) {
      setExpandedQuestion(null);
    } else {
      setExpandedQuestion(question.id);
      // Load existing answer if available
      setResponseText(userAnswers[question.id] ?? '');
    }
  };

  const handleAnswerChange = (questionId: string | number, value: string): void => {
    setResponseText(value);
    // Update the answer in real-time as user types
    setUserAnswers((prev) => ({
      ...prev,
      [questionId]: value,
    }));
  };

  const handleSubmitAllAnswers = async (): Promise<void> => {
    if (!onSubmitAnswers) return;

    setIsSubmitting(true);

    try {
      // Transform answers to API format
      const answers = Object.entries(userAnswers).map(
        ([questionId, answer]) => ({
          question_id: questionId.toString(),
          answer: answer,
        })
      );

      // Call parent's submit handler
      await onSubmitAnswers(answers);

      // Clear form
      setUserAnswers({});
    } catch (err) {
      console.error('Submit error:', err);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Section */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <HelpCircle className="h-5 w-5 text-purple-500" />
            <h2 className="text-2xl font-semibold leading-none tracking-tight">
              Clarification Questions
            </h2>
          </div>
          <div className="flex gap-2 text-sm">
            <span className="px-2 py-1 bg-red-500/10 text-red-500 rounded">
              {questionStats.critical.length} Critical
            </span>
            <span className="px-2 py-1 bg-yellow-500/10 text-yellow-500 rounded">
              {questionStats.important.length} Important
            </span>
            <span className="px-2 py-1 bg-blue-500/10 text-blue-500 rounded">
              {questionStats.goodToKnow.length} Good to Know
            </span>
          </div>
        </div>
        <p className="text-sm text-text-secondary">
          These are questions the system needs clarification on to understand
          your business better.
        </p>
      </div>

      {/* Questions List */}
      <div className="space-y-3">
        {questions.map((question) => (
          <div
            key={question.id}
            className={`rounded-lg border transition-all ${
              expandedQuestion === question.id
                ? 'border-purple-500/50 bg-surface-secondary'
                : 'border-border-light hover:border-purple-500/50 hover:bg-surface-secondary'
            }`}
          >
            <div
              className="p-3 cursor-pointer"
              onClick={() => handleQuestionClick(question)}
            >
              <div className="flex items-start gap-3">
                <div className="mt-0.5">
                  {userAnswers[question.id] ? (
                    <CheckCircle className="h-4 w-4 text-green-500" />
                  ) : (
                    <AlertCircle
                      className={`h-4 w-4 ${
                        question.category === 'CRITICAL'
                          ? 'text-red-500'
                          : question.category === 'Important'
                            ? 'text-yellow-500'
                            : 'text-blue-500'
                      }`}
                    />
                  )}
                </div>
                <div className="flex-1">
                  <div className="flex items-center gap-2 mb-1">
                    <span
                      className={`text-xs font-medium ${
                        question.category === 'CRITICAL'
                          ? 'text-red-500'
                          : question.category === 'Important'
                            ? 'text-yellow-500'
                            : 'text-blue-500'
                      }`}
                    >
                      {question.category}
                    </span>
                    {userAnswers[question.id] && (
                      <span className="text-xs text-green-500 font-medium">
                        • Answered
                      </span>
                    )}
                  </div>
                  <p className="text-sm text-text-primary">{question.text}</p>
                </div>
                <div className="mt-0.5">
                  {expandedQuestion === question.id ? (
                    <ChevronUp className="h-4 w-4 text-text-tertiary" />
                  ) : (
                    <ChevronDown className="h-4 w-4 text-text-tertiary" />
                  )}
                </div>
              </div>
            </div>

            {/* Expanded Response Section */}
            {expandedQuestion === question.id && (
              <div className="border-t border-border-light p-4 bg-surface-primary">
                <div className="space-y-3">
                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-2">
                      Your Response
                    </label>
                    <textarea
                      value={responseText}
                      onChange={(e) => handleAnswerChange(question.id, e.target.value)}
                      placeholder="Please provide your answer to help us understand your business better."
                      className="w-full px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary placeholder-text-tertiary focus:outline-none focus:ring-2 focus:ring-purple-500 min-h-[100px] resize-y"
                    />
                  </div>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Submit All Answers Button */}
      {isAwaitingAnswers && Object.keys(userAnswers).length > 0 && (
        <div className="mt-6 pt-6 border-t border-border-light">
          <div className="flex items-center justify-between">
            <div className="text-sm text-text-secondary">
              {Object.keys(userAnswers).length} of {questions.length} questions answered
            </div>
            <Button
              onClick={handleSubmitAllAnswers}
              disabled={isSubmitting}
              className="bg-green-600 text-white hover:bg-green-700 disabled:bg-gray-300 disabled:text-gray-500"
            >
              {isSubmitting ? (
                <>
                  <div className="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent" />
                  Submitting...
                </>
              ) : (
                <>
                  <Send className="h-4 w-4 mr-2" />
                  Submit All Answers & Continue Workflow
                </>
              )}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
};

export default QuestionsView;
