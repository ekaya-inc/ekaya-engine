import { ArrowLeft, Info } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';

import { useProject } from '../contexts/ProjectContext';

import { Button } from './ui/Button';

interface AppPageHeaderProps {
  title: string;
  slug: string;
  icon: React.ReactNode;
  description?: string;
}

const AppPageHeader = ({ title, slug, icon, description }: AppPageHeaderProps) => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { urls } = useProject();

  const origin = urls.projectsPageUrl
    ? new URL(urls.projectsPageUrl).origin
    : 'https://us.ekaya.ai';

  return (
    <div className="mb-6">
      <Button
        variant="ghost"
        onClick={() => navigate(`/projects/${pid}`)}
        className="mb-4"
      >
        <ArrowLeft className="mr-2 h-4 w-4" />
        Back to Dashboard
      </Button>
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold text-text-primary flex items-center gap-2">
          {icon}
          {title}
        </h1>
        <a
          href={`${origin}/apps/${slug}`}
          target="_blank"
          rel="noopener noreferrer"
          title={`${title} documentation`}
        >
          <Info className="h-7 w-7 text-text-secondary hover:text-brand-purple transition-colors" />
        </a>
      </div>
      {description && (
        <p className="mt-2 text-text-secondary">{description}</p>
      )}
    </div>
  );
};

export default AppPageHeader;
