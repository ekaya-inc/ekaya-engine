import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';

import ErrorBoundary from './components/ErrorBoundary';
import Layout from './components/Layout';
import ProjectDataLoader from './components/ProjectDataLoader';
import { ThemeProvider } from './components/ThemeProvider';
import { ConfigProvider } from './contexts/ConfigContext';
import { DatasourceConnectionProvider } from './contexts/DatasourceConnectionContext';
import { ProjectProvider } from './contexts/ProjectContext';
import { ToastProviderComponent } from './hooks/useToast';
import AIAgentsPage from './pages/AIAgentsPage';
import AIConfigPage from './pages/AIConfigPage';
import AIDataLiaisonPage from './pages/AIDataLiaisonPage';
import ApplicationsPage from './pages/ApplicationsPage';
import AuditPage from './pages/AuditPage';
import DatasourcePage from './pages/DatasourcePage';
import EnrichmentPage from './pages/EnrichmentPage';
import GlossaryPage from './pages/GlossaryPage';
import HelpPage from './pages/HelpPage';
import HomePage from './pages/HomePage';
import MCPServerPage from './pages/MCPServerPage';
import OAuthCallbackPage from './pages/OAuthCallbackPage';
import OntologyPage from './pages/OntologyPage';
import OntologyQuestionsPage from './pages/OntologyQuestionsPage';
import ProjectDashboard from './pages/ProjectDashboard';
import ProjectKnowledgePage from './pages/ProjectKnowledgePage';
import ProjectsRedirect from './pages/ProjectsRedirect';
import QueriesPage from './pages/QueriesPage';
import RelationshipsPage from './pages/RelationshipsPage';
import SchemaPage from './pages/SchemaPage';
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
                <Route path="/projects" element={<ProjectsRedirect />} />
                <Route path="/projects/:pid" element={<ProjectProvider><ProjectDataLoader><Layout /></ProjectDataLoader></ProjectProvider>}>
                  <Route index element={<ProjectDashboard />} />
                  <Route path="applications" element={<ApplicationsPage />} />
                  <Route path="ai-config" element={<AIConfigPage />} />
                  <Route path="ai-agents" element={<AIAgentsPage />} />
                  <Route path="ai-data-liaison" element={<AIDataLiaisonPage />} />
                  <Route path="audit" element={<AuditPage />} />
                  <Route path="datasource" element={<DatasourcePage />} />
                  <Route path="schema" element={<SchemaPage />} />
                  <Route path="relationships" element={<RelationshipsPage />} />
                  <Route path="enrichment" element={<EnrichmentPage />} />
                  <Route path="glossary" element={<GlossaryPage />} />
                  <Route path="project-knowledge" element={<ProjectKnowledgePage />} />
                  <Route path="ontology" element={<OntologyPage />} />
                  <Route path="ontology-questions" element={<OntologyQuestionsPage />} />
                  <Route path="queries/*" element={<QueriesPage />} />
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
