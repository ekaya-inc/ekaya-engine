import { Navigate, Outlet, useLocation, useParams } from 'react-router-dom';

import { useProject } from '../contexts/ProjectContext';

const ProjectRouteGate = () => {
  const { pid } = useParams<{ pid: string }>();
  const { shouldShowSetupWizard } = useProject();
  const location = useLocation();

  if (!pid) {
    return <Outlet />;
  }

  const setupPath = `/projects/${pid}/setup`;
  const isSetupPath = location.pathname === setupPath;

  if (shouldShowSetupWizard && !isSetupPath) {
    return <Navigate to={setupPath} replace />;
  }

  return <Outlet />;
};

export default ProjectRouteGate;
