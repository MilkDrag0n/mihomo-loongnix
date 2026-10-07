import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Card, Stat, Button, Input } from "@holosama/ui/react";
import { Activity, Globe, Signal, RefreshCw } from "lucide-react";
import { api } from "./api";
import { useLoad, useAction } from "./hooks";
import { Confirm, ErrorNotice } from "./ui";
import type { Status, Metrics, Group } from "./types";
import { bytes, trafficSample, type Sample } from "./telemetry";
export function Overview() {
  const status = useLoad<Status>("/status", 3000),
    groups = useLoad<Group[]>("/proxy-groups", 15000);
  const s = status.data;
  const [samples, setSamples] = useState<Sample[]>([]),
    [port, setPort] = useState("7890"),
    [confirm, setConfirm] = useState<"core" | "tun" | "port">();
  const previous = useRef<Metrics>(undefined),
    edited = useRef(false);
  const { pending, act } = useAction();
  useEffect(() => {
    if (!s) return;
    if (!edited.current) setPort(String(s.proxy_port));
    const point = trafficSample(
      previous.current,
      status.error ? undefined : s.metrics,
    );
    previous.current = status.error ? undefined : s.metrics;
    setSamples((old) => (point ? [...old, point].slice(-60) : []));
  }, [s, status.error]);
  const m = status.error ? undefined : s?.metrics;
  async function apply() {
    if (!s || !confirm) return;
    const jobs = {
      core: () =>
        api("/core/" + (s.core.service_active ? "stop" : "start"), "POST"),
      tun: () => api("/tun", "PUT", { enabled: !s.tun.configured }),
      port: () => api("/proxy-port", "PUT", { port: Number(port) }),
    };
    const okay = await act(jobs[confirm], status.refresh);
    if (okay) {
      edited.current = false;
      setConfirm(undefined);
    }
  }
  return (
    <>
      <header className="holo-topbar">
        <h1>
          <Activity size={19} />
          运行概览
        </h1>
        <Button
          size="sm"
          variant="outline"
          disabled={pending}
          onClick={() => void status.refresh()}
        >
          <RefreshCw size={14} />
          刷新状态
        </Button>
      </header>
      <ErrorNotice
        error={status.error && status.error + " · 以下为最近一次状态"}
      />
      {s ? (
        <>
          <div className="overview-top">
            <div className="overview-left">
              <Card className="account">
                <div className="account-icon">
                  <Globe size={28} />
                </div>
                <div>
                  <h2>Mihomo</h2>
                  <span className="small muted">网络代理</span>
                </div>
                <span
                  className={
                    "holo-status" +
                    (s.core.running && !status.error ? " good" : "")
                  }
                >
                  {status.error
                    ? "待确认"
                    : s.core.running
                      ? "运行中"
                      : "未就绪"}
                </span>
              </Card>
              <Card>
                <h2 className="holo-section-title">
                  <Signal size={19} />
                  代理状态
                </h2>
                <div className="holo-fields">
                  {[
                    ["当前出口", s.current_node || "未选择"],
                    ["代理组", s.current_group || "暂无"],
                    ["活动配置", s.active_profile?.name || "未导入"],
                    ["代理端口", s.proxy_port],
                  ].map(([label, value]) => (
                    <div className="holo-field" key={label}>
                      <span>{label}</span>
                      <strong>{value}</strong>
                    </div>
                  ))}
                </div>
                <Link className="text-link manage-link" to="/proxies">
                  管理代理节点 →
                </Link>
              </Card>
            </div>
            <Card className="traffic-card">
              <div className="panel-top">
                <h2 className="holo-section-title">
                  <Activity size={19} />
                  实时流量
                </h2>
                <span className="small muted">
                  ● 下载 <span className="purple">● 上传</span>
                </span>
              </div>
              <TrafficChart samples={samples} />
              {!s.metrics && !status.error && (
                <p className="small muted">内核暂未返回统计数据</p>
              )}
            </Card>
          </div>
          <div className="holo-stats">
            <Stat label="活动连接" value={m?.connections ?? "—"} />
            <Stat label="本次下载" value={bytes(m?.download_total)} />
            <Stat label="本次上传" value={bytes(m?.upload_total)} />
            <Stat label="内核内存" value={bytes(m?.memory_bytes)} />
          </div>
          <Card>
            <div className="panel-top">
              <h2 className="holo-section-title">
                <Globe size={19} />
                代理组
              </h2>
              <Link className="text-link" to="/proxies">
                节点与测速 →
              </Link>
            </div>
            <ErrorNotice error={groups.error} />
            <div className="proxy-grid">
              {(groups.data ?? []).map((g) => (
                <Link
                  to="/proxies"
                  className={
                    "proxy" + (g.name === s.current_group ? " selected" : "")
                  }
                  key={g.name}
                >
                  <div className="panel-top">
                    <strong>{g.name}</strong>
                    <span className="small muted">{g.type}</span>
                  </div>
                  <p>{g.now || "未选择节点"}</p>
                  <div className="panel-top small">
                    <span className="muted">{g.nodes?.length ?? 0} 个节点</span>
                    <span>{delay(g)}</span>
                  </div>
                </Link>
              ))}
            </div>
            {!groups.data?.length && !groups.error && (
              <p className="holo-empty">暂无代理组，请先导入配置并启动内核。</p>
            )}
          </Card>
          <div className="control-grid">
            <Card>
              <h2 className="holo-section-title">代理内核</h2>
              <div className="holo-fields">
                <div className="holo-field">
                  <span>系统服务</span>
                  <strong>
                    {s.core.state_query_ok === false
                      ? "未知"
                      : s.core.service_active
                        ? "运行中"
                        : "未运行"}
                  </strong>
                </div>
                <div className="holo-field">
                  <span>控制接口</span>
                  <strong>
                    {s.core.controller_healthy ? "可连接" : "不可连接"}
                  </strong>
                </div>
              </div>
              <Button
                variant="outline"
                disabled={pending || !!status.error}
                onClick={() => setConfirm("core")}
              >
                {s.core.service_active ? "停止代理" : "启动代理"}
              </Button>
            </Card>
            <Card>
              <h2 className="holo-section-title">TUN 网络</h2>
              <div className="holo-fields">
                <div className="holo-field">
                  <span>期望配置</span>
                  <strong>{s.tun.configured ? "开启" : "关闭"}</strong>
                </div>
                <div className="holo-field">
                  <span>实际状态</span>
                  <strong>
                    {s.tun.enabled
                      ? "已生效"
                      : s.tun.configured
                        ? "待生效"
                        : "已关闭"}
                  </strong>
                </div>
              </div>
              <Button
                variant="outline"
                disabled={pending || !!status.error}
                onClick={() => setConfirm("tun")}
              >
                {s.tun.configured ? "关闭 TUN" : "开启 TUN"}
              </Button>
            </Card>
            <Card className="port-panel">
              <h2 className="holo-section-title">代理端口</h2>
              <p className="small muted">
                HTTP 与 SOCKS
                共用混合代理端口。仅修改代理端口，不修改网页访问端口。
              </p>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  if (Number(port) >= 1 && Number(port) <= 65535)
                    setConfirm("port");
                }}
                className="actions"
              >
                <Input
                  label="代理端口"
                  type="number"
                  min={1}
                  max={65535}
                  isRequired
                  value={port}
                  onValueChange={(v) => {
                    edited.current = true;
                    setPort(v);
                  }}
                />
                <Button type="submit" disabled={pending || !!status.error}>
                  应用端口
                </Button>
              </form>
            </Card>
          </div>
        </>
      ) : (
        <Card>
          <div className="holo-empty">
            {status.error ? "暂时无法读取状态" : "正在读取连接状态…"}
          </div>
        </Card>
      )}
      {confirm && s && (
        <Confirm
          busy={pending}
          title={
            confirm === "port"
              ? "修改代理端口？"
              : confirm === "tun"
                ? "切换 TUN？"
                : s.core.service_active
                  ? "停止代理？"
                  : "启动代理？"
          }
          onClose={() => setConfirm(undefined)}
          onConfirm={() => void apply()}
        >
          {confirm === "port"
            ? "使用旧代理端口的客户端需要更新设置。"
            : confirm === "tun"
              ? "开启 TUN 将接管系统流量；代理停止时仅保存配置。"
              : "此操作会影响代理连接。"}
        </Confirm>
      )}
    </>
  );
}
function delay(g: Group) {
  const n = g.nodes?.find((n) => n.name === g.now);
  return !n
    ? "暂无延迟"
    : n.delay > 0
      ? n.delay + " ms"
      : n.delay === -3
        ? "未测试"
        : "超时";
}
function TrafficChart({ samples }: { samples: Sample[] }) {
  const last = samples.at(-1),
    max = Math.max(1024, ...samples.flatMap((s) => [s.down, s.up])) * 1.15;
  const line = (key: "down" | "up") =>
    samples
      .map(
        (s, i) =>
          `${i ? "L" : "M"}${36 + (i / Math.max(samples.length - 1, 1)) * 520},${140 - (s[key] / max) * 124}`,
      )
      .join(" ");
  const time = (at?: number) =>
    at ? new Date(at).toLocaleTimeString("zh-CN", { hour12: false }) : "—";
  return (
    <div className="traffic-chart">
      <div className="traffic-speeds">
        <div>
          <span>↓ 下载速率</span>
          <strong>
            {bytes(last?.down)}
            <small>/s</small>
          </strong>
        </div>
        <div>
          <span>↑ 上传速率</span>
          <strong>
            {bytes(last?.up)}
            <small>/s</small>
          </strong>
        </div>
      </div>
      {samples.length < 2 ? (
        <div className="traffic-empty">
          正在采集流量，收到连续数据后显示曲线
        </div>
      ) : (
        <svg
          viewBox="0 0 570 160"
          preserveAspectRatio="none"
          role="img"
          aria-label="最近三分钟的下载和上传流量"
        >
          <defs>
            <linearGradient id="traffic-fill" x1="0" y1="0" x2="0" y2="1">
              <stop stopColor="#f54180" stopOpacity=".24" />
              <stop offset="1" stopColor="#f54180" stopOpacity="0" />
            </linearGradient>
          </defs>
          <path
            className="chart-grid"
            d="M36 16H556M36 57H556M36 99H556M36 140H556"
          />
          <text x="0" y="12">
            {bytes(max)}
          </text>
          <text x="12" y="143">
            0
          </text>
          <path
            className="chart-area"
            d={line("down") + " L556 140 L36 140 Z"}
            style={{ fill: "url(#traffic-fill)" }}
          />
          <path d={line("down")} stroke="var(--holo-accent-text)" />
          <path d={line("up")} stroke="#b198e9" />
        </svg>
      )}
      <div className="chart-time">
        <span>{time(samples[0]?.at)}</span>
        <span>按采样间隔计算 · B/s</span>
        <span>{time(last?.at)}</span>
      </div>
    </div>
  );
}
