import { FormEvent, useState } from "react";
import { Activity } from "lucide-react";
import { api } from "../lib/api";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";

interface AuthPageProps {
  mode: "setup" | "login";
  onAuthed: () => Promise<void>;
}

export function AuthPage({ mode, onAuthed }: AuthPageProps) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const isSetup = mode === "setup";

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setLoading(true);
    try {
      if (isSetup) await api.setup(username, password);
      else await api.login(username, password);
      await onAuthed();
    } catch (err) {
      setError(err instanceof Error ? err.message : "请求失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="auth-layout">
      <div className="auth-brand">
        <span className="brand-mark"><Activity /></span>
        <div>
          <h1>CFST Dashboard</h1>
          <p>HTTPing control plane for edge hosts.</p>
        </div>
      </div>
      <Card className="auth-card">
        <CardHeader>
          <CardTitle>{isSetup ? "初始化管理员" : "欢迎回来"}</CardTitle>
          <CardDescription>{isSetup ? "创建第一位管理员后即可开始添加主机。" : "登录后继续管理 agent 和测速曲线。"}</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="auth-form" onSubmit={submit}>
            <Label>
              用户名
              <Input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" required />
            </Label>
            <Label>
              密码
              <Input
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                type="password"
                minLength={8}
                autoComplete={isSetup ? "new-password" : "current-password"}
                required
              />
            </Label>
            {error && <p className="form-error">{error}</p>}
            <Button disabled={loading}>{loading ? "处理中..." : isSetup ? "创建管理员" : "登录"}</Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
