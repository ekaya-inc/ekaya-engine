interface EnabledTool {
  name: string;
  description: string;
}

interface MCPEnabledToolsProps {
  tools: EnabledTool[];
}

export default function MCPEnabledTools({ tools }: MCPEnabledToolsProps) {
  if (tools.length === 0) {
    return (
      <div className="text-sm text-text-secondary italic">
        No tools enabled. Enable a tool group above.
      </div>
    );
  }

  return (
    <div className="mt-4 border-t border-border-light pt-4">
      <h4 className="text-sm font-medium text-text-primary mb-2">Tools Enabled</h4>
      <table className="w-full text-sm">
        <tbody>
          {tools.map((tool) => (
            <tr key={tool.name} className="border-b border-border-light last:border-0">
              <td className="py-1.5 font-mono text-text-primary">{tool.name}</td>
              <td className="py-1.5 text-text-secondary">{tool.description}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
