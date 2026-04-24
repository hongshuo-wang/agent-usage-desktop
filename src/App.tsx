import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Suspense, lazy } from "react";
import Layout from "./components/Layout";

const Config = lazy(() => import("./pages/Config"));
const Dashboard = lazy(() => import("./pages/Dashboard"));
const Sessions = lazy(() => import("./pages/Sessions"));
const Settings = lazy(() => import("./pages/Settings"));
const FilesBackups = lazy(() => import("./pages/config/FilesBackups"));
const MCPServers = lazy(() => import("./pages/config/MCPServers"));
const Providers = lazy(() => import("./pages/config/Providers"));
const Skills = lazy(() => import("./pages/config/Skills"));

function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Suspense fallback={null}>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/sessions" element={<Sessions />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/config" element={<Config />}>
              <Route path="providers" element={<Providers />} />
              <Route path="mcp" element={<MCPServers />} />
              <Route path="skills" element={<Skills />} />
              <Route path="files" element={<FilesBackups />} />
            </Route>
          </Routes>
        </Suspense>
      </Layout>
    </BrowserRouter>
  );
}

export default App;
