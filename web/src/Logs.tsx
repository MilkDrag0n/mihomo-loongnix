import { useEffect, useRef, useState } from "react";
import { Card, Button, Select } from "@holosama/ui/react";
import { FileText } from "lucide-react";
import { api } from "./api";
import { useLoad, useAction } from "./hooks";
import { ErrorNotice } from "./ui";
import { bytes } from "./telemetry";
import { openLogStream } from "./logStream";
import type { Log, Logging } from "./types";
export function Logs() {
  const logging = useLoad<Logging>("/logging/status"),
    { act, pending } = useAction();
  const [level, setLevel] = useState("info"),
    [paused, setPaused] = useState(false),
    [connection, setConnection] = useState("正在连接"),
    [logs, setLogs] = useState<Log[]>([]);
  const pauseRef = useRef(false);
  pauseRef.current = paused;
  useEffect(
    () =>
      openLogStream(
        level,
        (l) => {
          if (!pauseRef.current) setLogs((old) => [...old, l].slice(-1000));
        },
        setConnection,
      ),
    [level],
  );
  return (
    <>
      <header className="holo-topbar">
        <h1>
          <FileText size={19} />
          运行日志
        </h1>
        <span
          className={"holo-status" + (connection === "实时连接" ? " good" : "")}
        >
          {connection}
        </span>
      </header>
      <ErrorNotice error={logging.error} />
      <Card className="panel-top">
        <div>
          <h2>保存到磁盘</h2>
          <p className="small muted">
            {logging.data?.enabled ? "已启用" : "未启用"} · 已用{" "}
            {bytes(logging.data?.total_bytes)}
            {logging.data?.has_error && (
              <span className="error"> · 记录器异常</span>
            )}
          </p>
        </div>
        <Button
          variant="outline"
          disabled={pending || !logging.data}
          onClick={() =>
            void act(
              () => api("/logging", "PUT", { enabled: !logging.data?.enabled }),
              logging.refresh,
            )
          }
        >
          {logging.data?.enabled ? "关闭记录" : "开启记录"}
        </Button>
      </Card>
      <Card>
        <div className="table-toolbar">
          <Select
            aria-label="日志级别"
            value={level}
            onValueChange={setLevel}
            options={[
              { value: "debug", label: "调试" },
              { value: "info", label: "信息" },
              { value: "warning", label: "警告" },
              { value: "error", label: "错误" },
            ]}
          />
          <div className="actions">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setPaused(!paused)}
            >
              {paused ? "继续显示" : "暂停显示"}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setLogs([])}>
              清空显示
            </Button>
          </div>
        </div>
        <div className="log-lines" role="log" aria-label="运行日志">
          {logs.map((l, i) => (
            <div className="log-line" key={i}>
              <time>{new Date(l.received_at).toLocaleTimeString("zh-CN")}</time>
              <span className={"log-level " + l.level}>{l.level}</span>
              <span>{l.message}</span>
            </div>
          ))}
          {!logs.length && (
            <div className="holo-empty">
              {paused ? "显示已暂停" : "正在等待新的日志记录…"}
            </div>
          )}
        </div>
        <div className="small muted">
          {logs.length} / 1000 条记录 · 清空显示不会删除磁盘日志
        </div>
      </Card>
    </>
  );
}
