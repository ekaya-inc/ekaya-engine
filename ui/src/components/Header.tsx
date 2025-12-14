import { Settings, HelpCircle } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';

import { useProject } from '../contexts/ProjectContext';

const Header = () => {
  const { projectName, projectId, papiUrl } = useProject();
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

  // Build the project page URL: {papiUrl}/projects/{projectId}
  const projectPageUrl =
    papiUrl && projectId ? `${papiUrl}/projects/${projectId}` : null;

  // Display project name if available, otherwise show "Ekaya Project"
  const displayName = projectName ?? 'Ekaya Project';

  return (
    <header className="border-b border-border-light bg-header-primary">
      <div className="container mx-auto flex h-16 items-center justify-between px-4">
        {projectPageUrl ? (
          <a
            href={projectPageUrl}
            className="text-xl font-semibold text-text-primary hover:text-accent-primary transition-colors"
            title={`Open ${displayName} in Ekaya Central`}
          >
            {displayName}
          </a>
        ) : (
          <h1 className="text-xl font-semibold text-text-primary">
            {displayName}
          </h1>
        )}
        {pid && (
          <div className="flex items-center gap-2">
            <button
              onClick={() => navigate(`/projects/${pid}/settings`)}
              className="p-2 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-secondary transition-colors"
              title="Settings"
            >
              <Settings className="h-5 w-5" />
            </button>
            <button
              onClick={() => navigate(`/projects/${pid}/help`)}
              className="p-2 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-secondary transition-colors"
              title="Help"
            >
              <HelpCircle className="h-5 w-5" />
            </button>
          </div>
        )}
      </div>
    </header>
  );
};

export default Header;
