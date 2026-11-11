/** Formats millicores: "73m" below one core, "1.5 cores" above. */
export function formatCPU(millicores: number): string {
  if (millicores < 1000) return `${Math.round(millicores)}m`
  const cores = millicores / 1000
  const text = Number.isInteger(cores) ? String(cores) : cores.toFixed(1)
  return `${text} ${cores === 1 ? 'core' : 'cores'}`
}

const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']

/** Formats bytes in binary units, matching how Kubernetes expresses memory. */
export function formatBytes(bytes: number): string {
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024
    unit++
  }
  const digits = unit === 0 || value >= 100 ? 0 : 1
  return `${value.toFixed(digits)} ${BYTE_UNITS[unit]}`
}

/** Share of `total` as a 0–100 number; 0 when there is no total. */
export function percent(value: number, total: number): number {
  return total > 0 ? (value / total) * 100 : 0
}

/** Compact age such as "45s", "12m", "3h" or "9d", like kubectl's AGE column. */
export function formatAge(iso: string, now: number = Date.now()): string {
  const seconds = Math.max(0, Math.floor((now - new Date(iso).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 48) return `${hours}h`
  return `${Math.floor(hours / 24)}d`
}

export function formatClock(iso: string): string {
  return new Date(iso).toLocaleTimeString()
}

/** Milliseconds elapsed since `iso`. */
export function msSince(iso: string, now: number = Date.now()): number {
  return now - new Date(iso).getTime()
}
