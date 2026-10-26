import { useEffect, useState, type FormEvent } from "react";
import { createRoot } from "react-dom/client";
import {
  BrowserRouter,
  Routes,
  Route,
  NavLink,
  useNavigate,
  Navigate,
} from "react-router-dom";
import { HeroUIProvider, Sidebar, Button, Input } from "@holosama/ui/react";
import {
  LayoutDashboard,
  Folder,
  Globe,
  ShieldCheck,
  FileText,
  LogOut,
} from "lucide-react";
import { api, login, logout, session, message } from "./api";
import { ActionProvider, useAction } from "./hooks";
import { ErrorNotice } from "./ui";
import { Overview } from "./Overview";
import { Profiles } from "./Profiles";
import { Proxies } from "./Proxies";
import { Rules } from "./Rules";
import { Logs } from "./Logs";
import "./style.css";
import "@holosama/ui/theme.css";
document.body.classList.add("holo-app");
const pages = [
  ["/overview", "运行概览", LayoutDashboard],
  ["/profiles", "订阅配置", Folder],
  ["/proxies", "代理节点", Globe],
  ["/rules", "规则管理", ShieldCheck],
  ["/logs", "运行日志", FileText],
] as const;
function App() {
  const [mode, setMode] = useState<"password" | "external">(),
    [authenticated, setAuthenticated] = useState(false),
    [ready, setReady] = useState(false),
    [error, setError] = useState(""),
    [password, setPassword] = useState(""),
    [busy, setBusy] = useState(false),
    [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    let alive = true;
    async function init() {
      try {
        const m = await api<{ auth_mode: string }>("/auth/mode");
        if (m.auth_mode !== "password" && m.auth_mode !== "external")
          throw Error("网页认证配置不可用");
        if (alive) setMode(m.auth_mode);
        await session();
        if (alive) {
          setAuthenticated(true);
          setError("");
        }
      } catch (e) {
        if (alive) setError(message(e));
      } finally {
        if (alive) setReady(true);
      }
    }
    void init();
    const expired = () => setAuthenticated(false);
    window.addEventListener("mihomo-session-expired", expired);
    return () => {
      alive = false;
      window.removeEventListener("mihomo-session-expired", expired);
    };
  }, []);
  useEffect(() => {
    if (!authenticated) return;
    let touched = 0;
    const activity = (e: Event) => {
      if (!e.isTrusted || Date.now() - touched < 60000) return;
      touched = Date.now();
      void api("/auth/refresh", "POST").catch(() => {});
    };
    window.addEventListener("pointerdown", activity);
    window.addEventListener("keydown", activity);
    return () => {
      window.removeEventListener("pointerdown", activity);
      window.removeEventListener("keydown", activity);
    };
  }, [authenticated]);
  async function signIn(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      let chosen = mode;
      if (!chosen) {
        const m = await api<{ auth_mode: "password" | "external" }>(
          "/auth/mode",
        );
        if (!["password", "external"].includes(m.auth_mode))
          throw Error("网页认证配置不可用");
        chosen = m.auth_mode;
        setMode(chosen);
      }
      if (chosen === "external") await session();
      else if (password) await login(password);
      else return;
      setAuthenticated(true);
      navigate("/overview");
    } catch (e) {
      setError(message(e));
    } finally {
      setPassword("");
      setBusy(false);
    }
  }
  if (!ready) return <div className="loading">正在连接控制台…</div>;
  if (!authenticated)
    return (
      <main className="login-layout">
        <form className="holo-card login-card" onSubmit={signIn}>
          <div className="holo-brand">Mihomo</div>
          <h1>连接你的网络</h1>
          <p className="muted">
            {mode === "password"
              ? "使用管理员密码登录控制台。"
              : "通过既有外部访问验证连接控制台。"}
          </p>
          {mode === "password" && (
            <Input
              label="管理员密码"
              type="password"
              value={password}
              onValueChange={setPassword}
              isRequired
              autoComplete="current-password"
            />
          )}
          <ErrorNotice error={error} />
          <Button type="submit" disabled={busy}>
            {busy
              ? "正在连接…"
              : mode === "password"
                ? "进入控制台"
                : "重新连接"}
          </Button>
        </form>
      </main>
    );
  return (
    <ActionProvider>
      <div className={"shell" + (collapsed ? " nav-collapsed" : "")}>
        <Sidebar
          name="Mihomo"
          collapsed={collapsed}
          onCollapse={() => setCollapsed(!collapsed)}
          footer={
            mode === "password" ? (
              <Button
                variant="ghost"
                onClick={async () => {
                  try {
                    await logout();
                    setAuthenticated(false);
                  } catch (e) {
                    setError(message(e));
                  }
                }}
              >
                <LogOut size={16} />
                退出登录
              </Button>
            ) : (
              <span className="small muted">关闭网页不会停止代理</span>
            )
          }
        >
          {pages.map(([to, label, Icon]) => (
            <NavLink
              key={to}
              to={to}
              aria-label={label}
              title={label}
              className={({ isActive }) =>
                "holo-nav-link" + (isActive ? " active" : "")
              }
            >
              <Icon size={20} />
              <span className="nav-text">{label}</span>
            </NavLink>
          ))}
        </Sidebar>
        <main className="main">
          <ErrorNotice error={error} />
          <Notice />
          <Routes>
            <Route path="/overview" element={<Overview />} />
            <Route path="/profiles" element={<Profiles />} />
            <Route path="/proxies" element={<Proxies />} />
            <Route path="/rules" element={<Rules />} />
            <Route path="/logs" element={<Logs />} />
            <Route path="*" element={<Navigate to="/overview" replace />} />
          </Routes>
          <footer>
            Mihomo · Holosama UI<span>关闭网页不会停止代理</span>
          </footer>
        </main>
      </div>
    </ActionProvider>
  );
}
function Notice() {
  const { notice, failed, clear } = useAction();
  return notice ? (
    <div className={"notice" + (failed ? " error" : "")} role="status">
      {notice}
      <Button size="icon" variant="ghost" aria-label="关闭提示" onClick={clear}>
        ×
      </Button>
    </div>
  ) : null;
}
createRoot(document.getElementById("root")!).render(
  <HeroUIProvider>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </HeroUIProvider>,
);
