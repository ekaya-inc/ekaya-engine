import { useState, useEffect } from 'react';

import DatasourceAdapterSelection from '../components/DatasourceAdapterSelection';
import DatasourceConfiguration from '../components/DatasourceConfiguration';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';

const DatasourcePage = () => {
  const { datasources, selectedDatasource } = useDatasourceConnection();
  const [selectedAdapter, setSelectedAdapter] = useState<string | null>(null);
  const [showSetup, setShowSetup] = useState<boolean>(false);

  // Set selected adapter and show config when selectedDatasource exists
  useEffect(() => {
    if (selectedDatasource) {
      setSelectedAdapter(selectedDatasource.type);
      setShowSetup(true);
    }
  }, [selectedDatasource]);

  const handleAdapterSelect = (adapterId: string): void => {
    setSelectedAdapter(adapterId);
    setShowSetup(true);
  };

  const handleBackToSelection = (): void => {
    setShowSetup(false);
    setSelectedAdapter(null);
  };

  if (showSetup) {
    return (
      <DatasourceConfiguration
        selectedAdapter={selectedAdapter}
        onBackToSelection={handleBackToSelection}
      />
    );
  }

  return (
    <DatasourceAdapterSelection
      selectedAdapter={selectedAdapter}
      onAdapterSelect={handleAdapterSelect}
      datasources={datasources}
    />
  );
};

export default DatasourcePage;
