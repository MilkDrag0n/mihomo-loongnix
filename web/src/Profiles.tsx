import { useState } from "react";
import { Card, Button, Input } from "@holosama/ui/react";
import { Folder, Plus } from "lucide-react";
import { api } from "./api";
import { useLoad, useAction } from "./hooks";
import { Confirm, ErrorNotice } from "./ui";
import type { Profile } from "./types";
export function Profiles() {
  const list = useLoad<Profile[]>("/profiles"),
    { act, pending } = useAction();
  const [adding, setAdding] = useState(false),
    [name, setName] = useState(""),
    [url, setUrl] = useState(""),
    [editing, setEditing] = useState<Profile>(),
    [newName, setNewName] = useState(""),
    [removing, setRemoving] = useState<Profile>();
  async function importProfile() {
    const body = { name, url };
    setUrl("");
    if (await act(() => api("/profiles", "POST", body), list.refresh)) {
      setAdding(false);
      setName("");
    }
  }
  return (
    <>
      <header className="holo-topbar">
        <h1>
          <Folder size={19} />
          订阅配置
        </h1>
        <Button size="sm" onClick={() => setAdding(true)}>
          <Plus size={15} />
          导入配置
        </Button>
      </header>
      <ErrorNotice error={list.error} />
      {adding && (
        <Card>
          <form
            className="import-form"
            onSubmit={(e) => {
              e.preventDefault();
              void importProfile();
            }}
          >
            <h2>从订阅链接导入</h2>
            <Input
              label="配置名称"
              value={name}
              onValueChange={setName}
              maxLength={120}
            />
            <Input
              label="订阅链接"
              type="url"
              value={url}
              onValueChange={setUrl}
              isRequired
              autoComplete="off"
            />
            <p className="small muted">
              链接仅用于本次导入，不保存在浏览器中。
            </p>
            <div className="actions">
              <Button type="submit" disabled={pending}>
                导入
              </Button>
              <Button
                variant="ghost"
                disabled={pending}
                onClick={() => {
                  setUrl("");
                  setAdding(false);
                }}
              >
                取消
              </Button>
            </div>
          </form>
        </Card>
      )}
      <div className="section-label">
        <span>{list.data?.length ?? 0} 份配置</span>
        <Button variant="ghost" size="sm" onClick={() => void list.refresh()}>
          刷新
        </Button>
      </div>
      <div className="profile-grid">
        {(list.data ?? []).map((p) => (
          <Card
            key={p.id}
            className={"profile-card" + (p.active ? " selected" : "")}
          >
            <div className="panel-top">
              <Folder size={23} />
              <span className={"holo-status" + (p.active ? " good" : "")}>
                {p.active ? "当前使用" : "未激活"}
              </span>
            </div>
            <h2>{p.name}</h2>
            <p className="muted break-word">{p.source}</p>
            <p className="small muted">更新于 {p.updated_at || "暂无记录"}</p>
            <div className="actions wrap">
              <Button
                size="sm"
                disabled={pending || p.active}
                onClick={() =>
                  void act(
                    () =>
                      api(
                        "/profiles/" + encodeURIComponent(p.id) + "/activate",
                        "POST",
                      ),
                    list.refresh,
                  )
                }
              >
                {p.active ? "已激活" : "使用此配置"}
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={() =>
                  void act(
                    () =>
                      api(
                        "/profiles/" + encodeURIComponent(p.id) + "/update",
                        "POST",
                      ),
                    list.refresh,
                  )
                }
              >
                更新
              </Button>
              <Button
                size="sm"
                variant="ghost"
                disabled={pending}
                onClick={() => {
                  setEditing(p);
                  setNewName(p.name);
                }}
              >
                重命名
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={pending}
                onClick={() => setRemoving(p)}
              >
                删除
              </Button>
            </div>
          </Card>
        ))}
      </div>
      {!list.data?.length && !list.error && (
        <Card>
          <div className="holo-empty">
            {list.loading
              ? "正在读取配置…"
              : "还没有配置，先导入一条订阅链接。"}
          </div>
        </Card>
      )}
      {editing && (
        <Confirm
          title="重命名配置"
          busy={pending}
          onClose={() => setEditing(undefined)}
          onConfirm={() => {
            if (newName.trim())
              void act(
                () =>
                  api("/profiles/" + encodeURIComponent(editing.id), "PATCH", {
                    name: newName.trim(),
                  }),
                list.refresh,
              ).then((ok) => {
                if (ok) setEditing(undefined);
              });
          }}
        >
          <Input
            label="新的配置名称"
            value={newName}
            onValueChange={setNewName}
            maxLength={120}
            isRequired
          />
        </Confirm>
      )}
      {removing && (
        <Confirm
          title={"删除配置“" + removing.name + "”？"}
          busy={pending}
          onClose={() => setRemoving(undefined)}
          onConfirm={() =>
            void act(
              () =>
                api("/profiles/" + encodeURIComponent(removing.id), "DELETE"),
              list.refresh,
            ).then((ok) => {
              if (ok) setRemoving(undefined);
            })
          }
        >
          {removing.active
            ? "这是当前活动配置，删除可能改变代理连接。"
            : "删除后不能通过此面板恢复该配置。"}
        </Confirm>
      )}
    </>
  );
}
