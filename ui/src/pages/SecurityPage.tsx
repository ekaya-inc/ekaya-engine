import { useParams } from 'react-router-dom';

/**
 * SecurityPage - Manage security settings
 * This page allows users to configure security policies and access controls.
 */
const SecurityPage = () => {
  const { pid } = useParams<{ pid: string }>();

  return (
    <div className="mx-auto max-w-6xl">
      <h1 className="text-2xl font-semibold mb-4">Security</h1>
      <p className="text-text-secondary">
        Project: {pid}
      </p>
    </div>
  );
};

export default SecurityPage;
