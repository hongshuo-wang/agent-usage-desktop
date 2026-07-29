import { BrowserRouter, Navigate, Routes, Route } from "react-router-dom";
import { Suspense, lazy } from "react";
import Layout from "./components/Layout";

const Dashboard = lazy(() => import("./pages/Dashboard"));
const Sessions = lazy(() => import("./pages/Sessions"));
const Settings = lazy(() => import("./pages/Settings"));

function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Suspense fallback={null}>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/sessions" element={<Sessions />} />
            <Route path="/settings" element={<Navigate to="/settings/data-sources" replace />} />
            <Route path="/settings/data-sources" element={<Settings section="data-sources" />} />
            <Route path="/settings/pricing" element={<Settings section="pricing" />} />
            <Route path="/settings/index-diagnostics" element={<Settings section="index-diagnostics" />} />
            <Route path="/settings/preferences" element={<Settings section="preferences" />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </Layout>
    </BrowserRouter>
  );
}

export default App;
