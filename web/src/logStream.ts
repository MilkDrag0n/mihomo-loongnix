import { session } from "./api";
import type { Log } from "./types";
/** Lifecycle owned by one mounted page; bounded exponential retries survive short opens. */
export function openLogStream(
  level: string,
  onLog: (log: Log) => void,
  onState: (state: string) => void,
) {
  let stream: EventSource | undefined,
    timer: ReturnType<typeof setTimeout> | undefined,
    alive = true,
    backoff = 1000;
  function connect() {
    stream?.close();
    clearTimeout(timer);
    if (!alive || document.hidden) return;
    onState("正在连接");
    const source = new EventSource(
      "/api/v1/logs/stream?level=" + encodeURIComponent(level),
    );
    stream = source;
    let openedAt = 0;
    source.onopen = () => {
      if (!alive || stream !== source) return;
      openedAt = Date.now();
      onState("等待日志");
    };
    source.addEventListener("log", (e) => {
      if (!alive || stream !== source) return;
      try {
        const log = JSON.parse((e as MessageEvent).data);
        if (typeof log.message !== "string" || typeof log.level !== "string")
          return;
        backoff = 1000;
        onState("实时连接");
        onLog(log);
      } catch {}
    });
    source.addEventListener("gap", () => {
      if (alive && stream === source)
        onLog({
          level: "提示",
          message: "日志连接有间断，可能遗漏部分记录。",
          received_at: new Date().toISOString(),
        });
    });
    source.onerror = async () => {
      if (!alive || stream !== source) return;
      source.close();
      if (openedAt && Date.now() - openedAt >= 30000) backoff = 1000;
      onState("连接已断开");
      try {
        await session();
      } catch {
        return;
      }
      if (alive && stream === source && !document.hidden) {
        timer = setTimeout(connect, backoff);
        backoff = Math.min(30000, backoff * 2);
      }
    };
  }
  function visibility() {
    if (document.hidden) {
      stream?.close();
      stream = undefined;
      clearTimeout(timer);
      onState("后台已暂停");
    } else {
      onLog({
        level: "提示",
        message: "页面恢复，后台期间的日志不补发。",
        received_at: new Date().toISOString(),
      });
      connect();
    }
  }
  connect();
  document.addEventListener("visibilitychange", visibility);
  return () => {
    alive = false;
    stream?.close();
    clearTimeout(timer);
    document.removeEventListener("visibilitychange", visibility);
  };
}
