function clampDay(year, monthIndex, day) {
  const lastDay = new Date(year, monthIndex + 1, 0).getDate()
  return Math.min(Math.max(day, 1), lastDay)
}

export function resolveTrafficCycleStartDay(settings) {
  const configured = Number(settings?.traffic_cycle_start_day || 0)
  if (configured >= 1 && configured <= 31) return configured
  const deploy = String(settings?.deploy_date || '')
  if (deploy.length >= 10) {
    const day = Number(deploy.slice(8, 10))
    if (day >= 1 && day <= 31) return day
  }
  return 1
}

export function currentTrafficCycleRange(now = new Date(), startDay = 1) {
  const day = startDay >= 1 && startDay <= 31 ? startDay : 1
  const y = now.getFullYear()
  const m = now.getMonth()
  const today = new Date(y, m, now.getDate())

  if (day === 1) {
    return {
      from: new Date(y, m, 1),
      to: today
    }
  }

  const thisStart = new Date(y, m, clampDay(y, m, day))
  if (today >= thisStart) {
    return { from: thisStart, to: today }
  }

  const prevMonth = m === 0 ? 11 : m - 1
  const prevYear = m === 0 ? y - 1 : y
  return {
    from: new Date(prevYear, prevMonth, clampDay(prevYear, prevMonth, day)),
    to: today
  }
}

export function formatTrafficCycleDate(d) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

export function trafficCycleRangeFromSettings(settings, now = new Date()) {
  const startDay = resolveTrafficCycleStartDay(settings)
  const { from, to } = currentTrafficCycleRange(now, startDay)
  return {
    startDay,
    from: formatTrafficCycleDate(from),
    to: formatTrafficCycleDate(to)
  }
}
