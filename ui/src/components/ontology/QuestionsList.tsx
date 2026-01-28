import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  MessageCircleQuestion,
  Table2,
} from 'lucide-react';
import { useState } from 'react';

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '../ui/Card';

/**
 * Question data structure
 */
interface Question {
  id: string;
  text: string;
  priority: number;
  is_required: boolean;
  category: string;
  reasoning?: string;
  affected_tables?: string[];
  affected_columns?: string[];
  status: string;
  created_at: string;
}

interface QuestionsListProps {
  questions: Question[];
}

/**
 * Category display configuration
 */
const CATEGORY_CONFIG: Record<string, { label: string; order: number }> = {
  business_rules: { label: 'Business Rules', order: 1 },
  relationship: { label: 'Relationships', order: 2 },
  terminology: { label: 'Terminology', order: 3 },
  enumeration: { label: 'Enumerations', order: 4 },
  temporal: { label: 'Temporal Patterns', order: 5 },
  data_quality: { label: 'Data Quality', order: 6 },
};

/**
 * Get priority label and color
 */
function getPriorityInfo(priority: number): { label: string; colorClass: string } {
  switch (priority) {
    case 1:
      return { label: 'Critical', colorClass: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300' };
    case 2:
      return { label: 'High', colorClass: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300' };
    case 3:
      return { label: 'Medium', colorClass: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300' };
    default:
      return { label: 'Low', colorClass: 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-300' };
  }
}

/**
 * Get category display label
 */
function getCategoryLabel(category: string): string {
  return CATEGORY_CONFIG[category]?.label ?? category.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
}

/**
 * Group questions by category
 */
function groupByCategory(questions: Question[]): Map<string, Question[]> {
  const grouped = new Map<string, Question[]>();

  for (const question of questions) {
    const category = question.category || 'other';
    const existing = grouped.get(category);
    if (existing) {
      existing.push(question);
    } else {
      grouped.set(category, [question]);
    }
  }

  // Sort categories by configured order
  const sortedEntries = [...grouped.entries()].sort((a, b) => {
    const orderA = CATEGORY_CONFIG[a[0]]?.order ?? 99;
    const orderB = CATEGORY_CONFIG[b[0]]?.order ?? 99;
    return orderA - orderB;
  });

  return new Map(sortedEntries);
}

/**
 * QuestionItem - Individual question display
 */
const QuestionItem = ({ question }: { question: Question }) => {
  const [expanded, setExpanded] = useState(false);
  const priorityInfo = getPriorityInfo(question.priority);

  return (
    <div className="border-b last:border-b-0">
      <button
        className="w-full text-left p-4 hover:bg-surface-secondary transition-colors flex items-start gap-3"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="mt-0.5 text-muted-foreground">
          {expanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-start gap-2 flex-wrap">
            <span className="text-sm font-medium">{question.text}</span>
          </div>

          <div className="flex items-center gap-2 mt-2 flex-wrap">
            {/* Priority badge */}
            <span className={`text-xs px-2 py-0.5 rounded-full ${priorityInfo.colorClass}`}>
              {priorityInfo.label}
            </span>

            {/* Required indicator */}
            {question.is_required && (
              <span className="text-xs px-2 py-0.5 rounded-full bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">
                Required
              </span>
            )}

            {/* Affected tables */}
            {question.affected_tables && question.affected_tables.length > 0 && (
              <span className="text-xs text-muted-foreground flex items-center gap-1">
                <Table2 className="h-3 w-3" />
                {question.affected_tables.join(', ')}
              </span>
            )}
          </div>
        </div>
      </button>

      {/* Expanded content */}
      {expanded && (
        <div className="px-4 pb-4 pl-11 space-y-3">
          {question.reasoning && (
            <div className="text-sm">
              <span className="font-medium text-muted-foreground">Why this matters: </span>
              <span className="text-muted-foreground">{question.reasoning}</span>
            </div>
          )}

          {question.affected_columns && question.affected_columns.length > 0 && (
            <div className="text-sm">
              <span className="font-medium text-muted-foreground">Affected columns: </span>
              <span className="font-mono text-xs bg-surface-tertiary px-1.5 py-0.5 rounded">
                {question.affected_columns.join(', ')}
              </span>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

/**
 * CategorySection - Group of questions under a category
 */
const CategorySection = ({
  category,
  questions,
}: {
  category: string;
  questions: Question[];
}) => {
  const [collapsed, setCollapsed] = useState(false);

  return (
    <div className="mb-4 last:mb-0">
      <button
        className="w-full text-left flex items-center gap-2 py-2 px-1 hover:bg-surface-secondary rounded transition-colors"
        onClick={() => setCollapsed(!collapsed)}
      >
        {collapsed ? (
          <ChevronRight className="h-4 w-4 text-muted-foreground" />
        ) : (
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        )}
        <h3 className="font-medium">{getCategoryLabel(category)}</h3>
        <span className="text-sm text-muted-foreground">({questions.length})</span>
      </button>

      {!collapsed && (
        <div className="mt-2 border rounded-lg overflow-hidden">
          {questions.map((question) => (
            <QuestionItem key={question.id} question={question} />
          ))}
        </div>
      )}
    </div>
  );
};

/**
 * EmptyState - Shown when no questions exist
 */
const EmptyState = () => (
  <div className="flex flex-col items-center justify-center py-12 text-center">
    <div className="flex h-16 w-16 items-center justify-center rounded-full bg-green-500/10 mb-4">
      <CheckCircle2 className="h-8 w-8 text-green-500" />
    </div>
    <h3 className="text-lg font-medium mb-2">No pending questions</h3>
    <p className="text-sm text-muted-foreground max-w-md">
      All ontology questions have been answered. Run ontology extraction again
      after making schema changes to generate new questions.
    </p>
  </div>
);

/**
 * QuestionsList - Display pending ontology questions grouped by category
 */
export const QuestionsList = ({ questions }: QuestionsListProps) => {
  if (questions.length === 0) {
    return (
      <Card>
        <CardContent className="pt-6">
          <EmptyState />
        </CardContent>
      </Card>
    );
  }

  const grouped = groupByCategory(questions);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-amber-500/10 text-amber-500">
            <MessageCircleQuestion className="h-5 w-5" />
          </div>
          <div>
            <CardTitle className="text-lg">
              Pending Questions ({questions.length})
            </CardTitle>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {[...grouped.entries()].map(([category, categoryQuestions]) => (
          <CategorySection
            key={category}
            category={category}
            questions={categoryQuestions}
          />
        ))}
      </CardContent>
    </Card>
  );
};
