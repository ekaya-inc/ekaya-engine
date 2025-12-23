import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';

import ErrorBoundary from './components/ErrorBoundary';
import Layout from './components/Layout';
import ProjectDataLoader from './components/ProjectDataLoader';
import { ThemeProvider } from './components/ThemeProvider';
import { ConfigProvider } from './contexts/ConfigContext';
import { DatasourceConnectionProvider } from './contexts/DatasourceConnectionContext';
import { ProjectProvider } from './contexts/ProjectContext';
import { ToastProviderComponent } from './hooks/useToast';
import DatasourcePage from './pages/DatasourcePage';
import HelpPage from './pages/HelpPage';
import HomePage from './pages/HomePage';
import MCPServerPage from './pages/MCPServerPage';
import OAuthCallbackPage from './pages/OAuthCallbackPage';
import OntologyPage from './pages/OntologyPage';
import ProjectDashboard from './pages/ProjectDashboard';
import QueriesPage from './pages/QueriesPage';
import RelationshipsPage from './pages/RelationshipsPage';
import SchemaPage from './pages/SchemaPage';
import SecurityPage from './pages/SecurityPage';
import SettingsPage from './pages/SettingsPage';

const App = (): JSX.Element => {
  return (
    <ConfigProvider>
      <ThemeProvider>
        <DatasourceConnectionProvider>
          <ToastProviderComponent>
            <Router>
              <ErrorBoundary>
              <Routes>
                <Route path="/oauth/callback" element={<OAuthCallbackPage />} />
                <Route path="/" element={<HomePage />} />
                <Route path="/projects/:pid" element={<ProjectProvider><ProjectDataLoader><Layout /></ProjectDataLoader></ProjectProvider>}>
                  <Route index element={<ProjectDashboard />} />
                  <Route path="datasource" element={<DatasourcePage />} />
                  <Route path="schema" element={<SchemaPage />} />
                  <Route path="relationships" element={<RelationshipsPage />} />
                  <Route path="ontology" element={<OntologyPage />} />
                  <Route path="security" element={<SecurityPage />} />
                  <Route path="queries" element={<QueriesPage />} />
                  <Route path="mcp-server" element={<MCPServerPage />} />
                  <Route path="settings" element={<SettingsPage />} />
                  <Route path="help" element={<HelpPage />} />
                </Route>
              </Routes>
              </ErrorBoundary>
            </Router>
          </ToastProviderComponent>
        </DatasourceConnectionProvider>
      </ThemeProvider>
    </ConfigProvider>
  );
};

export default App;
