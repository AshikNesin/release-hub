import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";

dayjs.extend(relativeTime);

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`;
}

export const fromNow = (iso: string) => dayjs(iso).fromNow();
export const formatDateTime = (iso: string) => dayjs(iso).format("MMM D, YYYY HH:mm");

export const platformLabel = (p: string) => (p === "ios" ? "iOS" : p === "android" ? "Android" : p);
