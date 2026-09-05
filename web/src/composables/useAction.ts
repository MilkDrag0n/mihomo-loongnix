import { ref } from "vue";
export const pending = ref(false);
export const notice = ref("");
export const noticeError = ref(false);
export async function action(
  job: () => Promise<unknown>,
  refresh: () => Promise<unknown>,
  message = "操作请求已完成，请以页面实际状态为准",
) {
  if (pending.value) return;
  pending.value = true;
  notice.value = "";
  try {
    await job();
    noticeError.value = false;
    notice.value = message;
  } catch (e) {
    noticeError.value = true;
    notice.value = (e as Error).message;
  } finally {
    await refresh().catch(() => {});
    pending.value = false;
  }
}
