let csrf = "";
export class APIError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T = unknown>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch("/api/v1" + path, {
      method,
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        ...(csrf ? { "X-CSRF-Token": csrf } : {}),
      },
      ...(method !== "GET" ? { body: JSON.stringify(body ?? {}) } : {}),
    });
  } catch {
    throw new APIError(
      method === "GET" ? "OFFLINE" : "RESULT_UNKNOWN",
      method === "GET"
        ? "暂时无法连接网页服务"
        : "结果待确认，请刷新状态，不要重复提交",
    );
  }
  const payload = await response
    .json()
    .catch(() => ({
      error: {
        code: "INVALID_RESPONSE",
        message: "网页服务返回异常，请刷新确认",
      },
    }));
  if (!response.ok) {
    if (response.status === 401) {
      csrf = "";
      window.dispatchEvent(new Event("mihomo-session-expired"));
    }
    throw new APIError(
      payload.error?.code || "ERROR",
      payload.error?.message || "操作未完成",
    );
  }
  return payload.data as T;
}
export async function session() {
  const s = await api<{ csrf_token: string }>("/auth/session");
  csrf = s.csrf_token;
}
export async function login(password: string) {
  const s = await api<{ csrf_token: string }>("/auth/login", "POST", {
    password,
  });
  csrf = s.csrf_token;
}
export async function logout() {
  await api("/auth/logout", "POST");
  csrf = "";
}
export const message = (e: unknown) =>
  e instanceof Error ? e.message : "请求未完成，请刷新确认";
