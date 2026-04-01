import { Outlet } from "react-router-dom";

import Header from "./Header";

const Layout = () => {
  return (
    <div className="min-h-screen bg-surface-primary">
      <Header />
      <main className="container mx-auto px-4 py-8">
        <Outlet />
      </main>
    </div>
  );
};

export default Layout;
