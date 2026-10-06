import { BrowserRouter, Navigate, Routes, Route } from "react-router-dom";
import { Component, lazy, Suspense, type ErrorInfo, type ReactNode } from "react";
import Layout from "./components/Layout";

const Dashboard = lazy(() => import("./pages/Dashboard"));
const Sessions = lazy(() => import("./pages/Sessions"));
const Settings = lazy(() => import("./pages/Settings"));

class AppErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Application render failed", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <main className="flex min-h-screen items-center justify-center bg-background p-6 text-foreground">
          <section className="w-full max-w-md border border-border bg-card p-6">
            <h1 className="text-base font-semibold">Agent Usage 无法加载</h1>
            <p className="mt-2 break-words text-sm text-muted-foreground">{this.state.error.message}</p>
            <button type="button" className="btn btn-primary mt-5" onClick={() => window.location.reload()}>
              重新加载
            </button>
          </section>
        </main>
      );
    }
    return this.props.children;
  }
}

function App() {
  return (
    <AppErrorBoundary>
      <BrowserRouter>
        <Layout>
          <Suspense fallback={<div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">正在加载...</div>}>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/sessions" element={<Sessions />} />
            <Route path="/settings" element={<Navigate to="/settings/data-sources" replace />} />
            <Route path="/settings/data-sources" element={<Settings section="data-sources" />} />
            <Route path="/settings/pricing" element={<Settings section="pricing" />} />
            <Route path="/settings/index-diagnostics" element={<Settings section="index-diagnostics" />} />
            <Route path="/settings/session-content" element={<Settings section="session-content" />} />
            <Route path="/settings/preferences" element={<Settings section="preferences" />} />
            <Route path="/settings/about" element={<Settings section="about" />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
          </Suspense>
        </Layout>
      </BrowserRouter>
    </AppErrorBoundary>
  );
}

export default App;
