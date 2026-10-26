import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api, message } from "./api";
export function useLoad<T>(path: string, interval = 0) {
  const [data, setData] = useState<T>(),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true);
  const alive = useRef(false),
    busy = useRef(false);
  const refresh = useCallback(async () => {
    if (busy.current) return;
    busy.current = true;
    try {
      const d = await api<T>(path);
      if (alive.current) {
        setData(d);
        setError("");
      }
    } catch (e) {
      if (alive.current) setError(message(e));
    } finally {
      busy.current = false;
      if (alive.current) setLoading(false);
    }
  }, [path]);
  useEffect(() => {
    alive.current = true;
    let timer: ReturnType<typeof setTimeout>;
    async function tick() {
      if (!document.hidden) await refresh();
      if (alive.current && interval) timer = setTimeout(tick, interval);
    }
    void tick();
    return () => {
      alive.current = false;
      clearTimeout(timer);
    };
  }, [refresh, interval]);
  return { data, error, loading, refresh };
}
type Actions = {
  pending: boolean;
  notice: string;
  failed: boolean;
  clear: () => void;
  act: (
    job: () => Promise<unknown>,
    refresh?: () => Promise<unknown>,
  ) => Promise<boolean>;
};
const ActionContext = createContext<Actions>(null!);
export function ActionProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState(false),
    [notice, setNotice] = useState(""),
    [failed, setFailed] = useState(false);
  const lock = useRef(false),
    alive = useRef(true);
  useEffect(
    () => () => {
      alive.current = false;
    },
    [],
  );
  const act = useCallback(
    async (job: () => Promise<unknown>, refresh?: () => Promise<unknown>) => {
      if (lock.current) return false;
      lock.current = true;
      setPending(true);
      setNotice("");
      let okay = false;
      try {
        await job();
        okay = true;
        if (alive.current) {
          setFailed(false);
          setNotice("操作请求已完成，请以页面实际状态为准");
        }
      } catch (e) {
        if (alive.current) {
          setFailed(true);
          setNotice(message(e));
        }
      } finally {
        await refresh?.().catch(() => {});
        lock.current = false;
        if (alive.current) setPending(false);
      }
      return okay;
    },
    [],
  );
  return (
    <ActionContext.Provider
      value={{ pending, notice, failed, clear: () => setNotice(""), act }}
    >
      {children}
    </ActionContext.Provider>
  );
}
export const useAction = () => useContext(ActionContext);
