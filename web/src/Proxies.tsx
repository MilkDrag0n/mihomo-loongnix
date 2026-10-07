import { useEffect, useRef, useState } from "react";
import { Card, Button, Input, Select } from "@holosama/ui/react";
import { Globe } from "lucide-react";
import { api, message } from "./api";
import { useLoad, useAction } from "./hooks";
import { ErrorNotice } from "./ui";
import type { Group, Node } from "./types";
export function Proxies() {
  const list = useLoad<Group[]>("/proxy-groups"),
    { act, pending } = useAction();
  const [chosen, setChosen] = useState(""),
    [query, setQuery] = useState(""),
    [testing, setTesting] = useState(""),
    [batch, setBatch] = useState(false),
    [tested, setTested] = useState(0),
    [error, setError] = useState(""),
    [delays, setDelays] = useState<Record<string, number>>({});
  const alive = useRef(true),
    batchRef = useRef(false),
    testLock = useRef(false);
  useEffect(
    () => () => {
      alive.current = false;
      batchRef.current = false;
    },
    [],
  );
  const groups = list.data ?? [],
    group = groups.find((g) => g.name === chosen) ?? groups[0];
  const nodes = (group?.nodes ?? []).filter((n) =>
    (n.name + " " + n.type).toLowerCase().includes(query.toLowerCase()),
  );
  async function select(n: Node) {
    if (!group) return;
    await act(
      () =>
        api("/proxy-groups/" + encodeURIComponent(group.name), "PUT", {
          name: n.name,
        }),
      list.refresh,
    );
  }
  async function measure(n: Node, g: string) {
    if (testLock.current) return;
    testLock.current = true;
    setTesting(n.name);
    try {
      const data = await api<{ delay: number }>("/proxy-delay", "POST", {
        group: g,
        name: n.name,
      });
      if (alive.current)
        setDelays((old) => ({ ...old, [g + "\0" + n.name]: data.delay }));
    } catch (e) {
      if (alive.current) {
        setError(message(e));
        setDelays((old) => ({ ...old, [g + "\0" + n.name]: -1 }));
      }
    } finally {
      testLock.current = false;
      if (alive.current) setTesting("");
    }
  }
  async function all() {
    if (batchRef.current) {
      batchRef.current = false;
      setBatch(false);
      return;
    }
    if (!group || testLock.current) return;
    batchRef.current = true;
    setBatch(true);
    setTested(0);
    const snapshot = [...nodes],
      name = group.name;
    for (const n of snapshot) {
      if (!batchRef.current || !alive.current) break;
      await measure(n, name);
      if (alive.current) setTested((i) => i + 1);
    }
    batchRef.current = false;
    if (alive.current) setBatch(false);
  }
  function latency(n: Node) {
    const d = delays[(group?.name ?? "") + "\0" + n.name] ?? n.delay;
    return testing === n.name
      ? "测试中…"
      : d > 0
        ? d + " ms"
        : d === -3
          ? "未测试"
          : "超时";
  }
  return (
    <>
      <header className="holo-topbar">
        <h1>
          <Globe size={19} />
          代理节点
        </h1>
        <Button
          size="sm"
          variant="outline"
          disabled={batch || !!testing}
          onClick={() => void list.refresh()}
        >
          刷新节点
        </Button>
      </header>
      <ErrorNotice error={error || list.error} />
      <Card className="node-toolbar">
        <Select
          aria-label="代理组"
          value={group?.name ?? ""}
          options={groups.map((g) => ({
            value: g.name,
            label: g.name + " · " + (g.nodes?.length ?? 0) + " 节点",
          }))}
          onValueChange={setChosen}
          disabled={batch || !!testing}
        />
        <Input
          aria-label="搜索节点"
          placeholder="名称或类型"
          value={query}
          onValueChange={setQuery}
        />
        <Button
          disabled={!group || (!batch && !!testing)}
          onClick={() => void all()}
        >
          {batch ? "停止测速 · " + tested : "测速筛选结果"}
        </Button>
      </Card>
      <div className="section-label">
        <span>
          {nodes.length} 个节点 ／ {group?.type ?? "暂无代理组"}
        </span>
        <span className="small muted">当前：{group?.now ?? "未选择"}</span>
      </div>
      <div className="node-grid">
        {nodes.map((n) => (
          <Card
            key={n.name}
            className={"node-card" + (group?.now === n.name ? " selected" : "")}
          >
            <div className="panel-top">
              <span className="small muted">{n.type}</span>
              {group?.now === n.name && (
                <span className="small accent">✓ 当前</span>
              )}
            </div>
            <button
              className="node-title"
              disabled={pending}
              onClick={() => void select(n)}
            >
              {n.name}
            </button>
            <div className="node-bottom">
              <span className="small">{latency(n)}</span>
              <Button
                size="sm"
                variant="ghost"
                disabled={!!testing || batch}
                onClick={() => group && void measure(n, group.name)}
              >
                测速
              </Button>
              <Button
                size="sm"
                disabled={pending || group?.now === n.name}
                onClick={() => void select(n)}
              >
                {group?.now === n.name ? "已选择" : "使用"}
              </Button>
            </div>
          </Card>
        ))}
      </div>
      {!nodes.length && (
        <Card>
          <div className="holo-empty">
            {list.loading ? "正在读取节点…" : "没有符合条件的节点。"}
          </div>
        </Card>
      )}
    </>
  );
}
