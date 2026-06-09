import { useEffect, useState } from "react";
import { Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { api } from "./lib/api";
import { AuthPage } from "./routes/auth-page";
import { DashboardPage } from "./routes/dashboard-page";

type BootState = "loading" | "setup" | "authed" | "anonymous";

export function App() {
  const navigate = useNavigate();
  const [bootState, setBootState] = useState<BootState>("loading");

  async function boot() {
    const status = await api.setupStatus();
    if (status.setup_required) {
      setBootState("setup");
      navigate("/setup", { replace: true });
      return;
    }
    try {
      await api.hosts();
      setBootState("authed");
    } catch {
      setBootState("anonymous");
      navigate("/login", { replace: true });
    }
  }

  useEffect(() => {
    void boot();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (bootState === "loading") {
    return <div className="boot-screen">Loading CFST Dashboard...</div>;
  }

  return (
    <Routes>
      <Route path="/setup" element={<AuthPage mode="setup" onAuthed={async () => { setBootState("authed"); navigate("/hosts", { replace: true }); }} />} />
      <Route path="/login" element={<AuthPage mode="login" onAuthed={async () => { setBootState("authed"); navigate("/hosts", { replace: true }); }} />} />
      <Route path="/hosts" element={bootState === "authed" ? <DashboardPage onLoggedOut={() => { setBootState("anonymous"); navigate("/login", { replace: true }); }} /> : <Navigate to="/login" replace />} />
      <Route path="/hosts/:hostID" element={bootState === "authed" ? <DashboardPage onLoggedOut={() => { setBootState("anonymous"); navigate("/login", { replace: true }); }} /> : <Navigate to="/login" replace />} />
      <Route path="*" element={<Navigate to={bootState === "authed" ? "/hosts" : bootState === "setup" ? "/setup" : "/login"} replace />} />
    </Routes>
  );
}
