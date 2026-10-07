import { useState } from "react";
import { Card, Button, Input } from "@holosama/ui/react";
import { ShieldCheck } from "lucide-react";
import { useLoad } from "./hooks";
import { ErrorNotice } from "./ui";
import type { Rule } from "./types";
export function Rules() {
  const list = useLoad<Rule[]>("/rules");
  const [query, setQuery] = useState(""),
    [page, setPage] = useState(1);
  const filtered = (list.data ?? []).filter((r) =>
    (r.type + " " + r.content + " " + r.policy)
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  const pages = Math.max(1, Math.ceil(filtered.length / 30)),
    current = Math.min(page, pages);
  return (
    <>
      <header className="holo-topbar">
        <h1>
          <ShieldCheck size={19} />
          规则管理
        </h1>
        <Button size="sm" variant="outline" onClick={() => void list.refresh()}>
          刷新规则
        </Button>
      </header>
      <ErrorNotice error={list.error} />
      <Card>
        <div className="table-toolbar">
          <Input
            label="筛选规则"
            placeholder="域名、规则类型或策略"
            value={query}
            onValueChange={(v) => {
              setQuery(v);
              setPage(1);
            }}
          />
          <span className="small muted">
            {filtered.length} / {list.data?.length ?? 0} 条规则 · 只读
          </span>
        </div>
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>序号</th>
                <th>类型</th>
                <th>匹配内容</th>
                <th>策略</th>
              </tr>
            </thead>
            <tbody>
              {filtered.slice((current - 1) * 30, current * 30).map((r, i) => (
                <tr key={i}>
                  <td>{(current - 1) * 30 + i + 1}</td>
                  <td>{r.type}</td>
                  <td>{r.content || "所有剩余流量"}</td>
                  <td>{r.policy}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {!filtered.length && (
          <div className="holo-empty">
            {list.loading ? "正在读取规则…" : "暂无匹配规则"}
          </div>
        )}
        <div className="pagination">
          <Button
            size="sm"
            variant="outline"
            disabled={current <= 1}
            onClick={() => setPage(current - 1)}
          >
            上一页
          </Button>
          <span>
            {current} / {pages}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={current >= pages}
            onClick={() => setPage(current + 1)}
          >
            下一页
          </Button>
        </div>
      </Card>
    </>
  );
}
