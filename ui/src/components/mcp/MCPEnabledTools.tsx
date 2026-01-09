import { ChevronDown, ChevronRight } from 'lucide-react';
import { useState } from 'react';

interface EnabledTool {
  name: string;
  description: string;
}

interface MCPEnabledToolsProps {
  tools: EnabledTool[];
}

export default function MCPEnabledTools({ tools }: MCPEnabledToolsProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  if (tools.length === 0) {
    return (
      <div className="text-sm text-text-secondary italic">
        No tools enabled. Enable a tool group above.
      </div>
    );
  }

  return (
    <div className="mt-4 border-t border-border-light pt-4">
      <button
        type="button"
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center gap-2 text-base font-semibold text-text-primary hover:text-brand-purple transition-colors"
      >
        {isExpanded ? (
          <ChevronDown className="h-5 w-5" />
        ) : (
          <ChevronRight className="h-5 w-5" />
        )}
        Tools Enabled ({tools.length})
      </button>
      {isExpanded && (
        <table className="w-full text-sm mt-3">
          <tbody>
            {tools.map((tool) => (
              <tr key={tool.name} className="border-b border-border-light last:border-0">
                <td className="py-1.5 font-mono text-text-primary">{tool.name}</td>
                <td className="py-1.5 text-text-secondary">{tool.description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
