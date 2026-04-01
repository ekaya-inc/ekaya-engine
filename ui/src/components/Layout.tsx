import { Outlet } from "react-router-dom";

import Header from "./Header";
import ProjectSetupWizardGate from "./ProjectSetupWizardGate";

const Layout = () => {
  return (
    <div className="min-h-screen bg-surface-primary">
      <Header />
      <main className="container mx-auto px-4 py-8">
        <Outlet />
      </main>
      <ProjectSetupWizardGate />
    </div>
  );
};

export default Layout;
