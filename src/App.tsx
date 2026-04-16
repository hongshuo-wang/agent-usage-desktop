import { BrowserRouter, Routes, Route } from "react-router-dom";
import Layout from "./components/Layout";

function Dashboard() {
  return <div>Dashboard placeholder</div>;
}
function Sessions() {
  return <div>Sessions placeholder</div>;
}
function Settings() {
  return <div>Settings placeholder</div>;
}

function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/sessions" element={<Sessions />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
}

export default App;
