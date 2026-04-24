import { BrowserRouter, Routes, Route } from "react-router-dom";
import Layout from "./components/Layout";
import Config from "./pages/Config";
import Dashboard from "./pages/Dashboard";
import Sessions from "./pages/Sessions";
import Settings from "./pages/Settings";
import FilesBackups from "./pages/config/FilesBackups";
import MCPServers from "./pages/config/MCPServers";
import Providers from "./pages/config/Providers";
import Skills from "./pages/config/Skills";

function App() {
  return (
    <BrowserRouter>
      <Layout>
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
      </Layout>
    </BrowserRouter>
  );
}

export default App;
