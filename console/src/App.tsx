import { Navigate, Route, Routes } from 'react-router-dom';
import { Layout } from './components/Layout';
import { ConnectionProvider, useConnection } from './connection/ConnectionContext';
import { AuditPage } from './pages/AuditPage';
import { ConnectionPage } from './pages/ConnectionPage';
import { DashboardPage } from './pages/DashboardPage';
import { DevicesPage } from './pages/DevicesPage';
import { LicensesPage } from './pages/LicensesPage';
import { ProductsPage } from './pages/ProductsPage';
import { SigningKeysPage } from './pages/SigningKeysPage';
import { VpsPage } from './pages/VpsPage';

function ProtectedLayout() {
  const connection = useConnection();
  if (connection.mode === 'booting') return <div className="connection-loading">Restoring connection…</div>;
  if (connection.mode === 'disconnected') return <Navigate to="/connection" replace />;
  return <Layout />;
}

export function App({ initialDemo = false }: { initialDemo?: boolean }) {
  return (
    <ConnectionProvider initialDemo={initialDemo}><Routes>
      <Route path="connection" element={<ConnectionPage />} />
      <Route element={<ProtectedLayout />}>
        <Route index element={<DashboardPage />} />
        <Route path="products" element={<ProductsPage />} />
        <Route path="licenses" element={<LicensesPage />} />
        <Route path="devices" element={<DevicesPage />} />
        <Route path="signing-keys" element={<SigningKeysPage />} />
        <Route path="vps-backups" element={<VpsPage />} />
        <Route path="audit" element={<AuditPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes></ConnectionProvider>
  );
}
