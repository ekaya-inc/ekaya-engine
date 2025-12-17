/**
 * SQL Editor Component
 * CodeMirror-based SQL editor with dialect-aware syntax highlighting
 */

import { sql, PostgreSQL, MySQL, SQLite, MSSQL } from '@codemirror/lang-sql';
import { oneDark } from '@codemirror/theme-one-dark';
import CodeMirror from '@uiw/react-codemirror';
import { useEffect, useMemo, useState } from 'react';


import type { SqlDialect } from '../types';

import { useTheme } from './ThemeProvider';

type ValidationStatus = 'idle' | 'validating' | 'valid' | 'invalid';

interface SqlEditorProps {
  value: string;
  onChange: (value: string) => void;
  dialect: SqlDialect;
  schema?: Record<string, readonly string[]>;
  readOnly?: boolean;
  validationStatus?: ValidationStatus;
  validationError?: string | undefined;
  placeholder?: string;
  minHeight?: string;
}

/**
 * Maps our SqlDialect type to CodeMirror dialect configurations
 */
function getCodeMirrorDialect(dialect: SqlDialect) {
  switch (dialect) {
    case 'PostgreSQL':
      return PostgreSQL;
    case 'MySQL':
      return MySQL;
    case 'SQLite':
      return SQLite;
    case 'MSSQL':
      return MSSQL;
    default:
      return PostgreSQL;
  }
}

/**
 * Get border color class based on validation status
 */
function getBorderClass(status: ValidationStatus): string {
  switch (status) {
    case 'valid':
      return 'border-green-500 dark:border-green-400';
    case 'invalid':
      return 'border-red-500 dark:border-red-400';
    case 'validating':
      return 'border-yellow-500 dark:border-yellow-400';
    default:
      return 'border-gray-300 dark:border-gray-600';
  }
}

export function SqlEditor({
  value,
  onChange,
  dialect,
  schema,
  readOnly = false,
  validationStatus = 'idle',
  validationError,
  placeholder = 'Enter SQL query...',
  minHeight = '200px',
}: SqlEditorProps) {
  const { theme } = useTheme();
  const [isDark, setIsDark] = useState(false);

  // Determine if we're in dark mode
  useEffect(() => {
    if (theme === 'dark') {
      setIsDark(true);
    } else if (theme === 'light') {
      setIsDark(false);
    } else {
      // System theme - check media query
      setIsDark(window.matchMedia('(prefers-color-scheme: dark)').matches);
    }
  }, [theme]);

  // Listen for system theme changes when using system theme
  useEffect(() => {
    if (theme !== 'system') return;

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = (e: MediaQueryListEvent) => setIsDark(e.matches);
    mediaQuery.addEventListener('change', handler);
    return () => mediaQuery.removeEventListener('change', handler);
  }, [theme]);

  // Memoize extensions to avoid recreating on every render
  // Recreate when dialect or schema changes
  const extensions = useMemo(
    () => [
      sql({
        dialect: getCodeMirrorDialect(dialect),
        upperCaseKeywords: true,
        ...(schema && { schema }),
      }),
    ],
    [dialect, schema]
  );

  const borderClass = getBorderClass(validationStatus);

  return (
    <div className="space-y-1">
      <div
        className={`rounded-md border-2 overflow-hidden transition-colors ${borderClass}`}
        style={{ minHeight }}
      >
        <CodeMirror
          value={value}
          onChange={onChange}
          extensions={extensions}
          theme={isDark ? oneDark : 'light'}
          readOnly={readOnly}
          placeholder={placeholder}
          basicSetup={{
            lineNumbers: true,
            highlightActiveLineGutter: true,
            highlightSpecialChars: true,
            foldGutter: true,
            dropCursor: true,
            allowMultipleSelections: true,
            indentOnInput: true,
            bracketMatching: true,
            closeBrackets: true,
            autocompletion: true,
            rectangularSelection: true,
            crosshairCursor: true,
            highlightActiveLine: true,
            highlightSelectionMatches: true,
            closeBracketsKeymap: true,
            defaultKeymap: true,
            searchKeymap: true,
            historyKeymap: true,
            foldKeymap: true,
            completionKeymap: true,
            lintKeymap: true,
          }}
          style={{ minHeight }}
        />
      </div>
      {validationStatus === 'invalid' && validationError && (
        <p className="text-sm text-red-500 dark:text-red-400">
          {validationError}
        </p>
      )}
      {validationStatus === 'validating' && (
        <p className="text-sm text-yellow-600 dark:text-yellow-400">
          Validating SQL...
        </p>
      )}
      {validationStatus === 'valid' && (
        <p className="text-sm text-green-600 dark:text-green-400">
          SQL is valid
        </p>
      )}
    </div>
  );
}
