import { ref } from "vue";
export const csrf = ref("");
export const authenticated = ref(false);
export class APIError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T>(
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
        ...(csrf.value ? { "X-CSRF-Token": csrf.value } : {}),
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
        message: "网页服务返回异常，请稍后刷新",
      },
    }));
  if (!response.ok) {
    if (response.status === 401) {
      authenticated.value = false;
      csrf.value = "";
    }
    throw new APIError(
      payload.error?.code || "ERROR",
      payload.error?.message || "操作未完成",
    );
  }
  return payload.data as T;
}
export async function session() {
  const result = await api<{ csrf_token: string }>("/auth/session");
  csrf.value = result.csrf_token;
  authenticated.value = true;
}
export async function login(password: string) {
  const result = await api<{ csrf_token: string }>("/auth/login", "POST", {
    password,
  });
  csrf.value = result.csrf_token;
  authenticated.value = true;
}
export async function logout() {
  await api("/auth/logout", "POST");
  authenticated.value = false;
  csrf.value = "";
}
