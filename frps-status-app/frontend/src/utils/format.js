export function humanBytes(v) {
  if (!v) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let n = Number(v), i = 0
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(2)} ${units[i]}`
}

export function percent(value, totalGB) {
  if (!totalGB || totalGB <= 0) return 0
  const totalBytes = totalGB * 1024 * 1024 * 1024
  return Math.min(100, Math.round((Number(value) / totalBytes) * 100))
}

export function certClass(days) {
  if (days == null) return 'bad'
  if (days < 7) return 'bad'
  if (days < 15) return 'warn'
  return 'ok'
}

export function fmtDate(iso) {
  if (!iso) return '-'
  return new Date(iso).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })
}
