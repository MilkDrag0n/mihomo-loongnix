import type { Metrics } from "./types";
export type Sample = { at: number; down: number; up: number };
export function trafficSample(
  previous: Metrics | undefined,
  current: Metrics | undefined,
): Sample | null {
  if (!previous || !current) return null;
  const elapsed =
    (Date.parse(current.observed_at) - Date.parse(previous.observed_at)) / 1000;
  const numbers = [
    previous.download_total,
    previous.upload_total,
    current.download_total,
    current.upload_total,
  ];
  if (
    !numbers.every(
      (v) => typeof v === "number" && Number.isFinite(v) && v >= 0,
    ) ||
    !Number.isFinite(elapsed) ||
    elapsed < 0.2 ||
    elapsed > 15
  )
    return null;
  const down = current.download_total! - previous.download_total!,
    up = current.upload_total! - previous.upload_total!;
  if (down < 0 || up < 0) return null;
  return {
    at: Date.parse(current.observed_at),
    down: down / elapsed,
    up: up / elapsed,
  };
}
export function bytes(n: number | null | undefined) {
  if (typeof n !== "number" || !Number.isFinite(n) || n < 0) return "—";
  const unit =
    n >= 1073741824 ? "GB" : n >= 1048576 ? "MB" : n >= 1024 ? "KB" : "B";
  const divisor = { GB: 1073741824, MB: 1048576, KB: 1024, B: 1 }[unit];
  return `${(n / divisor).toFixed(unit === "B" ? 0 : 2)} ${unit}`;
}
