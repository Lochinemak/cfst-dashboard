import { LogOut } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../lib/api";
import type { Host, Measurement, Target } from "../types";
import { Button } from "../components/ui/button";
import { Sidebar } from "../components/dashboard/sidebar";
import { HostDetail } from "../components/dashboard/host-detail";

interface DashboardPageProps {
  onLoggedOut: () => void;
}

export function DashboardPage({ onLoggedOut }: DashboardPageProps) {
  const navigate = useNavigate();
  const params = useParams();
  const selectedHostID = params.hostID ? Number(params.hostID) : undefined;
  const [hosts, setHosts] = useState<Host[]>([]);
  const [targets, setTargets] = useState<Target[]>([]);
  const [measurements, setMeasurements] = useState<Measurement[]>([]);
  const selectedHost = useMemo(() => hosts.find((host) => host.id === selectedHostID), [hosts, selectedHostID]);

  async function loadHosts() {
    const nextHosts = (await api.hosts()) ?? [];
    setHosts(nextHosts);
    if (!selectedHostID && nextHosts.length > 0) {
      navigate(`/hosts/${nextHosts[0].id}`, { replace: true });
    }
  }

  async function loadHostData(hostID = selectedHostID) {
    if (!hostID) {
      setTargets([]);
      setMeasurements([]);
      return;
    }
    const since = new Date(Date.now() - 30 * 24 * 3600 * 1000).toISOString();
    const [nextTargets, nextMeasurements] = await Promise.all([
      api.targets(hostID),
      api.measurements(hostID, since),
    ]);
    setTargets(nextTargets ?? []);
    setMeasurements(nextMeasurements ?? []);
  }

  useEffect(() => {
    void loadHosts();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void loadHostData(selectedHostID);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedHostID]);

  async function createHost(name: string) {
    const host = await api.createHost(name);
    await loadHosts();
    navigate(`/hosts/${host.id}`);
  }

  async function renameHost(hostID: number, name: string) {
    const host = await api.updateHost(hostID, name);
    setHosts((current) => current.map((item) => (item.id === hostID ? host : item)));
  }

  async function deleteHost(hostID: number) {
    await api.deleteHost(hostID);
    const remainingHosts = hosts.filter((host) => host.id !== hostID);
    setHosts(remainingHosts);
    setTargets([]);
    setMeasurements([]);
    if (remainingHosts.length > 0) {
      navigate(`/hosts/${remainingHosts[0].id}`, { replace: true });
    } else {
      navigate("/hosts", { replace: true });
    }
  }

  async function logout() {
    await api.logout();
    onLoggedOut();
  }

  return (
    <main className="app-shell">
      <Sidebar
        hosts={hosts}
        selectedHostID={selectedHostID}
        onSelect={(hostID) => navigate(`/hosts/${hostID}`)}
        onCreate={createHost}
        onRefresh={loadHosts}
      />
      <section className="workspace">
        <header className="workspace-topbar">
          <div>
            <h1>CFST Dashboard</h1>
            <p>CloudflareSpeedTest HTTPing Control Plane</p>
          </div>
          <Button variant="outline" size="icon" onClick={logout} title="退出登录">
            <LogOut />
          </Button>
        </header>
        <HostDetail
          host={selectedHost}
          targets={targets}
          measurements={measurements}
          onChanged={() => loadHostData(selectedHostID)}
          onRename={renameHost}
          onDelete={deleteHost}
        />
      </section>
    </main>
  );
}
