<template>
  <div class="dashboard-shell">
    <div class="page-header">
      <div>
        <div class="page-title">数据看板</div>
        <div class="page-sub">
          <span>公网IP {{ hostPublicIP || '-' }} · 网卡 {{ hostIface || '-' }}</span>
          <span>{{ updatedAt ? '最后更新：' + updatedAt : '加载中…' }}</span>
        </div>
      </div>
      <div class="page-actions">
        <button class="btn btn-outline btn-sm icon-btn" title="日志" aria-label="日志" @click="openLogModal"><span class="btn-doc-icon" aria-hidden="true"></span></button>
        <button class="btn btn-outline btn-sm icon-btn" title="刷新" aria-label="刷新" :disabled="loading" @click="$emit('refresh')">
          <span class="refresh-glyph" :class="{ 'is-spinning': loading }" aria-hidden="true">↻</span>
        </button>
      </div>
    </div>

    <div class="page-body dashboard-page">
      <div class="dashboard-summary">
        <section class="summary-card service-card">
          <div class="summary-title">FRPS 服务</div>
          <div class="service-body">
            <div class="summary-icon server-icon" aria-hidden="true"><span></span><span></span><span></span></div>
            <div>
              <div class="service-status"><i class="status-dot" :class="bindOk ? 'ok' : 'bad'"></i>{{ bindOk ? '在线' : '离线' }}</div>
              <div class="service-tags">
                <span class="service-chip service-domain" :title="rootDomain">{{ rootDomain }}</span>
                <span class="service-chip service-health">连接 {{ bindLatency }}ms.面板 {{ dashOk ? dashLatency + 'ms' : '离线' }}</span>
              </div>
              <div class="service-uptime"><small>已运行</small><b>{{ runDays }}</b><span>天</span></div>
            </div>
          </div>
        </section>

        <section class="summary-card throughput-card">
          <div class="summary-title">本月流量吞吐</div>
          <div class="traffic-bars">
            <div class="traffic-line traffic-half traffic-in-row">
              <div class="traffic-label">
                <span class="traffic-lbl">入站</span>
                <b><span class="traffic-bytes">{{ humanBytesKB(ifaceMonthInKB) }}</span><span class="traffic-ratio" :class="inRatioTierClass"> ({{ inPctText }})</span></b>
              </div>
              <div class="thin-progress"><div class="thin-progress-fill" :class="inRatioTierClass" :style="{ width: inPct + '%' }"></div></div>
            </div>
            <div class="traffic-line traffic-half traffic-out-row">
              <div class="traffic-label">
                <span class="traffic-lbl">出站</span>
                <b><span class="traffic-bytes">{{ humanBytesKB(ifaceMonthOutKB) }}</span><span class="traffic-ratio" :class="outRatioTierClass"> ({{ outPctText }})</span></b>
              </div>
              <div class="thin-progress"><div class="thin-progress-fill" :class="outRatioTierClass" :style="{ width: outPct + '%' }"></div></div>
            </div>
            <div class="traffic-line traffic-total traffic-total-row">
              <div class="traffic-label">
                <span class="traffic-lbl">总量</span>
                <b><span class="traffic-bytes">{{ humanBytesKB(ifaceMonthInKB + ifaceMonthOutKB) }}</span><span class="traffic-ratio" :class="totalRatioTierClass"> ({{ totalPctText }})</span></b>
              </div>
              <div class="thin-progress"><div class="thin-progress-fill" :class="totalRatioTierClass" :style="{ width: totalPct + '%' }"></div></div>
            </div>
          </div>
        </section>

        <section class="summary-card online-card">
          <div class="summary-title">代理在线</div>
          <div class="online-head">
            <div><b>{{ onlineProxies }}</b><span>/ {{ totalProxies }}</span></div>
            <div class="summary-icon chart-icon" aria-hidden="true"></div>
          </div>
          <div class="summary-sub">在线率 {{ onlinePct }}%</div>
          <div class="summary-sub">TCP {{ proxyTypeMap.tcp || 0 }} · HTTP {{ proxyTypeMap.http || 0 }} · HTTPS {{ proxyTypeMap.https || 0 }}</div>
        </section>

        <section class="summary-card cert-card">
          <div class="summary-title">证书状态</div>
          <span class="cert-pill" :class="certHealthClass">{{ certHealthText }}</span>
          <div class="cert-body">
            <div class="summary-icon shield-icon" aria-hidden="true"></div>
            <div>
              <div class="cert-days"><b>{{ certSummary.min_days_left ?? '-' }}</b><span>天</span></div>
              <div class="summary-sub">WARN {{ certSummary.warn || 0 }} · FAIL {{ certSummary.fail || 0 }}</div>
            </div>
          </div>
        </section>

        <section class="summary-card storage-card">
          <div class="summary-title">存储空间</div>
          <div v-if="storageLoading && !storageHasData" class="storage-loading">加载中…</div>
          <div v-else-if="storageError && !storageHasData" class="storage-error">{{ storageError }}</div>
          <div v-else class="storage-body" :class="{ refreshing: storageLoading }">
            <div class="storage-donut-wrap">
              <div ref="storagePieEl" class="storage-donut"></div>
              <div v-if="storagePieCenterShow" class="storage-donut-center">
                <span class="storage-donut-pct">{{ storageFreePct }}%</span>
                <span class="storage-donut-lbl">可用</span>
              </div>
            </div>
            <div class="storage-meta">
              <div class="storage-stat total"><span>分区</span><b>{{ humanBytes(storageTotalBytes) }}</b></div>
              <div class="storage-stat used"><span>已用</span><b>{{ humanBytes(storageUsedBytes) }}</b></div>
              <div class="storage-stat free"><span>可用</span><b>{{ humanBytes(storageFreeBytes) }}</b></div>
            </div>
          </div>
        </section>
      </div>

      <div class="dashboard-main">
        <section class="card-area chart-block">
          <div class="card-head"><div class="section-title">本月流量趋势</div></div>
          <div ref="chartEl" class="trend-chart"></div>
        </section>
        <section class="card-area top5-block">
          <div class="card-head">
            <div class="section-title">本月总流量 Top 5</div>
          </div>
          <div class="top5-scroll">
            <table class="top5-table">
              <thead>
                <tr>
                  <th class="col-rank">排名</th><th class="col-name">代理名称</th><th class="col-num">入站</th><th class="col-num">出站</th><th class="col-num">总流量</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="!topProxies.length"><td colspan="5" class="empty">暂无数据</td></tr>
                <tr v-for="(p, i) in topProxies" :key="p.name + p.type">
                  <td class="col-rank"><b>{{ i + 1 }}</b></td>
                  <td class="col-name"><code>{{ p.name }}</code></td>
                  <td class="col-num">{{ humanBytes(p.month_in) }}</td>
                  <td class="col-num">{{ humanBytes(p.month_out) }}</td>
                  <td class="col-num"><b>{{ humanBytes((p.total != null ? p.total : (p.month_in + p.month_out))) }}</b></td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </div>

    <Teleport to="body">
      <Transition name="log-modal-fade">
        <div v-if="logOpen" class="log-modal-mask" @click.self="logOpen = false">
          <section class="log-modal" role="dialog" aria-modal="true" aria-labelledby="log-modal-title">
            <header class="log-modal-head">
              <div>
                <h2 id="log-modal-title">运行日志</h2>
                <p>{{ logMeta }}</p>
              </div>
              <div class="log-head-actions">
                <button class="btn btn-danger btn-sm" :disabled="logClearing" @click="clearLog">
                  {{ logClearing ? '清空中...' : '清空' }}
                </button>
                <button class="btn btn-outline btn-sm" :disabled="logLoading" @click="loadLog">
                  <span class="refresh-glyph" :class="{ 'is-spinning': logLoading }" aria-hidden="true">↻</span> 刷新
                </button>
                <button class="log-close" type="button" aria-label="关闭" @click="logOpen = false">×</button>
              </div>
            </header>
            <div class="log-toolbar">
              <div class="log-filter-group" aria-label="日志等级筛选">
                <button
                  v-for="item in logLevelOptions"
                  :key="item.value"
                  class="log-filter-btn"
                  :class="[item.value, { active: logLevelFilter === item.value }]"
                  type="button"
                  @click="logLevelFilter = item.value"
                >
                  {{ item.label }}
                </button>
              </div>
              <label class="log-follow-toggle">
                <input v-model="logAutoFollow" type="checkbox" />
                <span>自动显示最新</span>
              </label>
              <span class="log-toolbar-note">当前 {{ displayedLogRows.length }} / {{ logRows.length }} 行</span>
            </div>
            <div ref="logBodyEl" class="log-body">
              <div v-if="logLoading && !logContent" class="log-empty">日志加载中...</div>
              <div v-else-if="logError" class="log-empty error">{{ logError }}</div>
              <div v-else-if="displayedLogRows.length" class="log-table-wrap">
                <table class="log-table">
                  <thead>
                    <tr>
                      <th class="log-col-time">时间</th>
                      <th class="log-col-file">文件</th>
                      <th class="log-col-level">级别</th>
                      <th class="log-col-message">内容</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(row, index) in displayedLogRows" :key="index" :class="row.levelClass">
                      <td class="log-col-time">{{ row.time }}</td>
                      <td class="log-col-file">{{ row.file }}</td>
                      <td class="log-col-level"><span class="log-level">{{ row.level }}</span></td>
                      <td class="log-col-message">{{ row.message }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <div v-else class="log-empty">当前暂无日志</div>
            </div>
          </section>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import * as echarts from 'echarts'
import { humanBytes, percent } from '../utils/format.js'
import { api } from '../api/index.js'

const props = defineProps({ status: Object, daily: Array, loading: Boolean })
defineEmits(['refresh'])

const chartEl = ref(null)
const storagePieEl = ref(null)
const logOpen = ref(false)
const logLoading = ref(false)
const logContent = ref('')
const logPath = ref('')
const logSize = ref(0)
const logError = ref('')
const logBodyEl = ref(null)
const logAutoFollow = ref(true)
const logLevelFilter = ref('all')
const logClearing = ref(false)
let logTimer = null
let chart = null
let storageChart = null
let storageChartEl = null

const storagePartition = ref(null)
const storageApp = ref(null)
const storageLoading = ref(false)
const storageError = ref('')

const storageTotalBytes = computed(() => {
  const p = storagePartition.value?.partition
  if (!p) return 0
  return Number(p.total_bytes) || 0
})
const storageUsedBytes = computed(() => {
  const p = storagePartition.value?.partition
  if (!p) return 0
  return Number(p.used_bytes) || 0
})
const storageFreeBytes = computed(() => {
  const p = storagePartition.value?.partition
  if (!p) return 0
  return Number(p.free_bytes) || 0
})
const storageUsedPct = computed(() => {
  const t = storageTotalBytes.value
  if (!t) return 0
  return Math.round((storageUsedBytes.value / t) * 100)
})
const storageFreePct = computed(() => {
  const t = storageTotalBytes.value
  if (!t) return 0
  return Math.max(0, Math.min(100, 100 - storageUsedPct.value))
})
const storagePieCenterShow = computed(() => {
  return storagePartition.value?.ok && storageTotalBytes.value > 0
})
const storageHasData = computed(() => {
  return Boolean(storagePartition.value?.partition && storageTotalBytes.value > 0)
})

function fmtMB(v) {
  if (v == null || Number.isNaN(Number(v))) return '-'
  return Number(v).toFixed(2)
}

const updatedAt = computed(() => props.status
  ? new Date(props.status.generated_at).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })
  : '')

const bindOk = computed(() => props.status?.frps?.bind?.ok ?? false)
const bindLatency = computed(() => props.status?.frps?.bind?.latency_ms ?? '-')
const dashOk = computed(() => props.status?.frps?.dashboard?.ok ?? false)
const dashLatency = computed(() => props.status?.frps?.dashboard?.latency_ms ?? '-')

const limitInGB = computed(() => props.status?.settings?.limit_in_gb || 0)
const limitOutGB = computed(() => props.status?.settings?.limit_out_gb || 0)
const limitTotalGB = computed(() => props.status?.settings?.limit_total_gb || 0)
const inPct = computed(() => percent(ifaceMonthInKB.value * 1024, limitInGB.value))
const outPct = computed(() => percent(ifaceMonthOutKB.value * 1024, limitOutGB.value))
const totalPct = computed(() => percent((ifaceMonthInKB.value + ifaceMonthOutKB.value) * 1024, limitTotalGB.value))
const inPctText = computed(() => (limitInGB.value > 0 ? `${inPct.value}%` : '不限'))
const outPctText = computed(() => (limitOutGB.value > 0 ? `${outPct.value}%` : '不限'))
const totalPctText = computed(() => (limitTotalGB.value > 0 ? `${totalPct.value}%` : '不限'))

/** 相对「月度限额」的占用比例括号颜色：<50% / 50%~90% / ≥90%（无限额时为 neutral） */
function trafficRatioTierClass(pct, hasLimit) {
  if (!hasLimit) return 'traffic-ratio-tier-none'
  if (pct < 50) return 'traffic-ratio-tier-low'
  if (pct < 90) return 'traffic-ratio-tier-mid'
  return 'traffic-ratio-tier-high'
}
const inRatioTierClass = computed(() => trafficRatioTierClass(inPct.value, limitInGB.value > 0))
const outRatioTierClass = computed(() => trafficRatioTierClass(outPct.value, limitOutGB.value > 0))
const totalRatioTierClass = computed(() => trafficRatioTierClass(totalPct.value, limitTotalGB.value > 0))
const runDays = computed(() => {
  const raw = props.status?.settings?.deploy_date
  if (!raw) return '-'
  const start = new Date(`${raw}T00:00:00`)
  if (Number.isNaN(start.getTime())) return '-'
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  return Math.max(1, Math.floor((today - start) / 86400000) + 1)
})

const proxies = computed(() => props.status?.proxies ?? [])
const onlineProxies = computed(() => proxies.value.filter(p => p.online).length)
const totalProxies = computed(() => proxies.value.length)
const onlinePct = computed(() => totalProxies.value ? Math.round((onlineProxies.value / totalProxies.value) * 100) : 0)
const proxyTypeMap = computed(() => {
  const m = {}
  for (const p of proxies.value) m[p.type] = (m[p.type] || 0) + 1
  return m
})

const topProxies = computed(() => {
  const fromBackend = props.status?.dashboard?.top_proxies
  if (Array.isArray(fromBackend) && fromBackend.length) return fromBackend
  return [...proxies.value]
    .map(p => ({ name: p.name, type: p.type, month_in: Number(p.month_in || 0), month_out: Number(p.month_out || 0), total: Number(p.month_in || 0) + Number(p.month_out || 0) }))
    .sort((a, b) => b.total - a.total)
    .slice(0, 5)
})

const logLevelOptions = [
  { value: 'all', label: '全部' },
  { value: 'info', label: 'Info' },
  { value: 'warn', label: 'Warn' },
  { value: 'error', label: 'Error' }
]

const logRows = computed(() => parseLogRows(logContent.value))
const displayedLogRows = computed(() => {
  if (logLevelFilter.value === 'all') return logRows.value
  return logRows.value.filter(row => row.levelClass === logLevelFilter.value)
})
const logMeta = computed(() => {
  if (!logPath.value) return '当前日志文件'
  return `${logPath.value} · ${humanBytes(logSize.value)}`
})
const hostPublicIP = ref('')
const hostIface = ref('')
const ifaceMonthInKB = ref(0)
const ifaceMonthOutKB = ref(0)

function parseLogRows(content) {
  if (!content) return []
  return content
    .split(/\r?\n/)
    .filter(Boolean)
    .map((line) => {
      const match = line.match(/^\[([^\]]+)\]\s+(Info|Warn|Error)\s+(.*)$/)
      if (!match) {
        return { time: '-', file: '-', level: 'Info', levelClass: 'info', message: line }
      }
      const header = match[1]
      const splitAt = header.lastIndexOf(' ')
      const time = splitAt > -1 ? header.slice(0, splitAt) : header
      const file = splitAt > -1 ? header.slice(splitAt + 1) : '-'
      const level = match[2]
      return {
        time,
        file,
        level,
        levelClass: level.toLowerCase(),
        message: match[3]
      }
    })
}

async function openLogModal() {
  logOpen.value = true
  await loadLog()
  startLogAutoRefresh()
}

async function loadLog(options = {}) {
  logLoading.value = true
  if (!options.silent) logError.value = ''
  try {
    const res = await api.getCurrentLog()
    logContent.value = res.content || ''
    logPath.value = res.path || ''
    logSize.value = Number(res.size || 0)
    if (logAutoFollow.value) {
      await nextTick()
      await scrollLogToBottom()
    }
  } catch (e) {
    logError.value = e.message || '日志读取失败'
  } finally {
    logLoading.value = false
    if (logAutoFollow.value) await scrollLogToBottom()
  }
}

async function clearLog() {
  if (!confirm('确定清空当前日志文件内容？此操作不可恢复。')) return
  logClearing.value = true
  logError.value = ''
  try {
    await api.clearCurrentLog()
    await loadLog({ silent: true })
  } catch (e) {
    logError.value = e.message || '日志清空失败'
  } finally {
    logClearing.value = false
  }
}

async function scrollLogToBottom() {
  await nextTick()
  await new Promise(resolve => requestAnimationFrame(resolve))
  const el = logBodyEl.value
  if (el) el.scrollTop = el.scrollHeight
}

function startLogAutoRefresh() {
  stopLogAutoRefresh()
  if (!logAutoFollow.value) return
  logTimer = setInterval(() => {
    if (logOpen.value && logAutoFollow.value && !logLoading.value) loadLog({ silent: true })
  }, 5000)
}

function stopLogAutoRefresh() {
  if (logTimer) {
    clearInterval(logTimer)
    logTimer = null
  }
}

watch(logAutoFollow, async (enabled) => {
  if (enabled) {
    startLogAutoRefresh()
    await scrollLogToBottom()
  } else {
    stopLogAutoRefresh()
  }
})

watch(logOpen, (open) => {
  if (!open) stopLogAutoRefresh()
})

watch(displayedLogRows, async () => {
  if (logOpen.value && logAutoFollow.value) await scrollLogToBottom()
})

const certs = computed(() => {
  const arr = props.status?.certificates ?? []
  return [...arr].sort((a, b) => {
    const da = a.days_left == null ? 99999 : a.days_left
    const db = b.days_left == null ? 99999 : b.days_left
    return da - db
  })
})

const frpsDomain = computed(() => {
  const hit = certs.value.find(c => String(c?.domain || '').toLowerCase().startsWith('frps.'))
  return hit?.domain || '-'
})

const rootDomain = computed(() => {
  const domain = frpsDomain.value
  if (!domain || domain === '-') return '-'
  return domain.replace(/^frps\./i, '')
})

const certSummary = computed(() => props.status?.dashboard?.certificate || {
  total: certs.value.length,
  ok: certs.value.filter(c => c.ok && (c.days_left == null || c.days_left >= 15)).length,
  warn: certs.value.filter(c => c.ok && c.days_left != null && c.days_left < 15).length,
  fail: certs.value.filter(c => !c.ok).length,
  min_domain: certs.value[0]?.domain || '',
  min_days_left: certs.value[0]?.days_left ?? null
})

const certHealthText = computed(() => {
  if ((certSummary.value.fail || 0) > 0) return '异常'
  if ((certSummary.value.warn || 0) > 0) return '预警'
  return '正常'
})
const certHealthClass = computed(() => {
  if ((certSummary.value.fail || 0) > 0) return 'bad'
  if ((certSummary.value.warn || 0) > 0) return 'warn'
  return 'ok'
})

function buildChart(daily) {
  if (!chart) return
  const rows = Array.isArray(daily) ? daily : []
  const now = new Date()
  const monthStart = formatLocalDate(new Date(now.getFullYear(), now.getMonth(), 1))
  const today = formatLocalDate(now)
  const monthRows = rows.filter(r => r.day >= monthStart && r.day <= today)
  if (!monthRows.length) {
    chart.clear()
    return
  }
  const byDay = {}
  for (const r of monthRows) {
    byDay[r.day] ??= { in: 0, out: 0 }
    byDay[r.day].in += Number(r.rx_kb || 0)
    byDay[r.day].out += Number(r.tx_kb || 0)
  }
  const days = Object.keys(byDay).sort()
  chart.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      formatter: params => {
        const d = params[0]?.axisValue || ''
        return `${d}<br/>${params.map(p => `${p.marker}${p.seriesName}: <b>${humanBytesKB(p.value)}</b>`).join('<br/>')}`
      }
    },
    legend: { data: ['入站', '出站'], top: 0, left: 'center', textStyle: { color: '#334155', fontSize: 13 }, itemWidth: 22, itemHeight: 10 },
    grid: { left: 74, right: 22, top: 48, bottom: 42 },
    xAxis: {
      type: 'category',
      data: days,
      boundaryGap: false,
      axisLabel: { color: '#64748b', fontSize: 12 },
      axisLine: { lineStyle: { color: '#475569' } },
      axisTick: { show: false }
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#64748b', fontSize: 12, formatter: v => humanBytesKB(v) },
      splitLine: { lineStyle: { color: '#dbe3ee', type: 'dashed' } },
      axisLine: { show: false },
      axisTick: { show: false }
    },
    series: [
      { name: '入站', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => byDay[d].in), itemStyle: { color: '#1f7ae0' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(31,122,224,.08)' } },
      { name: '出站', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => byDay[d].out), itemStyle: { color: '#12b76a' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(18,183,106,.15)' } }
    ]
  })
}

function formatLocalDate(d) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function humanBytesKB(kb) {
  return humanBytes(Number(kb || 0) * 1024)
}

async function loadIfaceMonthSummary() {
  const now = new Date()
  const from = formatLocalDate(new Date(now.getFullYear(), now.getMonth(), 1))
  const to = formatLocalDate(now)
  try {
    const rows = await api.getDailyInterface({ from, to })
    let inKB = 0
    let outKB = 0
    for (const r of (rows || [])) {
      inKB += Number(r.rx_kb || 0)
      outKB += Number(r.tx_kb || 0)
    }
    ifaceMonthInKB.value = inKB
    ifaceMonthOutKB.value = outKB
    buildChart(rows)
  } catch {
    ifaceMonthInKB.value = 0
    ifaceMonthOutKB.value = 0
    buildChart([])
  }
}

async function loadHostNetwork() {
  try {
    const info = await api.getHostNetwork()
    hostPublicIP.value = info?.public_ip || ''
    hostIface.value = info?.iface || ''
  } catch {
    hostPublicIP.value = ''
    hostIface.value = ''
  }
}

watch(() => props.status?.generated_at, () => {
  loadHostNetwork()
  loadIfaceMonthSummary()
  loadStorageInfo()
})

function buildStoragePie() {
  ensureStorageChart()
  if (!storageChart) return
  const S = storagePartition.value
  if (!S?.ok || !S.partition) {
    storageChart.clear()
    return
  }
  const used = Number(S.partition.used_bytes) || 0
  const free = Number(S.partition.free_bytes) || 0
  const total = used + free
  if (total <= 0) {
    storageChart.clear()
    return
  }
  storageChart.setOption({
    backgroundColor: 'transparent',
    animation: true,
    tooltip: {
      trigger: 'item',
      formatter: (p) => `${p.marker}${p.name}<br/><b>${humanBytes(p.value)}</b>（${p.percent}%）`
    },
    series: [
      {
        type: 'pie',
        radius: ['52%', '82%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        itemStyle: { borderRadius: 3, borderColor: '#fff', borderWidth: 2 },
        label: { show: false },
        emphasis: { disabled: true },
        data: [
          { name: '已用', value: used, itemStyle: { color: '#2563eb' } },
          { name: '可用', value: free, itemStyle: { color: '#cbd5e1' } }
        ]
      }
    ]
  }, { notMerge: true })
}

function ensureStorageChart() {
  const el = storagePieEl.value
  if (!el) return
  if (storageChart && storageChartEl === el) return
  if (storageChart) {
    if (storageChartEl) ro?.unobserve(storageChartEl)
    storageChart.dispose()
  }
  storageChart = echarts.init(el)
  storageChartEl = el
  ro?.observe(el)
}

async function loadStorageInfo() {
  const hadData = storageHasData.value
  storageLoading.value = true
  storageError.value = ''
  try {
    const [disk, app] = await Promise.all([api.getStorage(), api.getStorageAppUsage()])
    storagePartition.value = disk
    storageApp.value = app
  } catch (e) {
    storageError.value = e.message || '存储信息加载失败'
    if (!hadData) {
      storagePartition.value = null
      storageApp.value = null
    }
  } finally {
    storageLoading.value = false
    await nextTick()
    buildStoragePie()
    storageChart?.resize()
  }
}

const ro = typeof ResizeObserver !== 'undefined'
  ? new ResizeObserver(() => {
      chart?.resize()
      storageChart?.resize()
    })
  : null

onMounted(async () => {
  await nextTick()
  chart = echarts.init(chartEl.value)
  ensureStorageChart()
  loadHostNetwork()
  loadIfaceMonthSummary()
  loadStorageInfo()
  ro?.observe(chartEl.value)
})

onUnmounted(() => {
  stopLogAutoRefresh()
  ro?.disconnect()
  chart?.dispose()
  storageChart?.dispose()
  storageChart = null
  storageChartEl = null
})
</script>

<style scoped>
.dashboard-page {
  display: grid;
  gap: 12px;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  grid-template-rows: auto minmax(0, 1fr);
  background: #f7f9fc;
}
.dashboard-shell {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}
.dashboard-shell :deep(.page-header) {
  min-height: auto;
}
.dashboard-shell :deep(.page-title) {
  font-size: var(--fs-page-title);
  line-height: var(--lh-title);
  font-weight: var(--fw-title);
}
.dashboard-shell :deep(.page-sub) {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
  align-items: center;
  margin-top: 4px;
  color: #64748b;
  font-size: var(--fs-body);
}
.page-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.page-actions .btn {
  height: 36px;
  width: 36px;
  padding: 0;
  justify-content: center;
  gap: 6px;
  border-color: #d7e1ed;
  background: #fff;
  color: #0f172a;
  font-size: 12px;
  font-weight: 500;
  box-shadow: none;
  border-radius: 8px;
}
.btn-doc-icon {
  width: 14px;
  height: 17px;
  display: inline-block;
  position: relative;
  border: 2px solid currentColor;
  border-radius: 2px;
}
.btn-doc-icon::before,
.btn-doc-icon::after {
  content: "";
  position: absolute;
  left: 3px;
  right: 3px;
  height: 2px;
  background: currentColor;
}
.btn-doc-icon::before { top: 4px; }
.btn-doc-icon::after { top: 9px; }
.dashboard-summary {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
}
.storage-card .summary-title {
  padding-right: 4px;
}
.storage-loading,
.storage-error {
  margin-top: 14px;
  font-size: 12px;
  color: #64748b;
}
.storage-error {
  color: #b91c1c;
}
.storage-body {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 6px;
  min-height: 118px;
  transition: opacity .16s ease;
}
.storage-body.refreshing {
  opacity: .72;
}
.storage-donut-wrap {
  position: relative;
  width: 112px;
  height: 112px;
  flex: 0 0 auto;
}
.storage-donut {
  width: 112px;
  height: 112px;
}
.storage-donut-center {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  pointer-events: none;
  line-height: 1.1;
}
.storage-donut-pct {
  font-size: 15px;
  font-weight: 800;
  color: #0f172a;
}
.storage-donut-lbl {
  margin-top: 2px;
  font-size: 10px;
  font-weight: 650;
  color: #64748b;
}
.storage-meta {
  flex: 1;
  min-width: 0;
  display: grid;
  gap: 4px;
}
.storage-meta .summary-sub {
  font-size: 11px;
  line-height: 1.35;
}
.storage-stat {
  display: flex;
  min-width: 0;
  align-items: baseline;
  justify-content: space-between;
  gap: 6px;
  padding: 0;
  border: 0;
  background: transparent;
  line-height: 1.2;
}
.storage-stat span {
  flex: 0 0 auto;
  color: #64748b;
  font-size: 10px;
  font-weight: 500;
}
.storage-stat b {
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  color: #0f172a;
  font-size: 11px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.storage-stat.total {
  border-color: transparent;
  background: transparent;
}
.storage-stat.used {
  border-color: transparent;
  background: transparent;
}
.storage-stat.used span,
.storage-stat.used b {
  color: #2563eb;
}
.storage-stat.free {
  border-color: transparent;
  background: transparent;
}
.storage-stat.free span,
.storage-stat.free b {
  color: #16a34a;
}
.storage-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  margin-top: 4px;
  font-size: 10px;
  color: #64748b;
}
.storage-legend span {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.storage-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
}
.storage-dot.used {
  background: #2563eb;
}
.storage-dot.free {
  background: #cbd5e1;
}
.storage-app-line {
  margin-top: 4px;
  font-size: 10px !important;
  color: #64748b !important;
}
.storage-app-line b {
  color: #0f172a;
  font-weight: 650;
}
.summary-card {
  position: relative;
  min-height: 172px;
  min-width: 0;
  padding: 14px 18px;
  background: #fff;
  border: 1px solid #dfe7f1;
  border-radius: 8px;
  box-shadow: 0 8px 22px rgba(15, 23, 42, .08);
}
.summary-title,
.section-title {
  color: #0f172a;
}
.summary-title {
  font-size: 13px;
  font-weight: 650;
}
.summary-sub {
  color: #475569;
  font-size: 12px;
}
.summary-icon {
  width: 54px;
  height: 54px;
  border-radius: 50%;
  background: #e7f6eb;
  color: #166534;
  flex: 0 0 auto;
}
.service-body {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 12px;
}
.service-body > div:last-child {
  min-width: 0;
}
.server-icon {
  display: grid;
  place-content: center;
  gap: 6px;
}
.server-icon span {
  display: block;
  width: 28px;
  height: 9px;
  border: 2px solid currentColor;
  border-radius: 5px;
  position: relative;
}
.server-icon span::before {
  content: "";
  position: absolute;
  left: 5px;
  top: 3px;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: currentColor;
}
.service-status {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
  font-size: 14px;
  font-weight: 800;
  color: #0f172a;
}
.service-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 7px;
}
.service-chip {
  display: inline-flex;
  max-width: 100%;
  min-width: 0;
  padding: 4px 8px;
  align-items: center;
  overflow: hidden;
  border: 1px solid #e2e8f0;
  border-radius: 7px;
  background: #eef2f7;
  color: #334155;
  font-size: 11px;
  font-weight: 650;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.service-health {
  border-color: #bbf7d0;
  background: #ecfdf3;
  color: #15803d;
}
.service-uptime small {
  color: #64748b;
  font-size: 10px;
  font-weight: 650;
}
.service-domain {
  border-color: #fed7aa;
  background: #fff7ed;
  color: #c2410c;
}
.service-uptime {
  display: inline-flex;
  align-items: baseline;
  gap: 5px;
  color: #475569;
  font-size: 11px;
}
.service-uptime small {
  display: inline;
  margin: 0;
}
.service-uptime b {
  color: #0f172a;
  font-size: 18px;
  font-weight: 850;
  line-height: 1;
}
.status-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;
}
.status-dot.ok { background: #12b76a; }
.status-dot.bad { background: #ef4444; }
.traffic-bars {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-top: 10px;
}
.iface-month-summary {
  margin-top: 10px;
  border-top: 1px solid #e2e8f0;
  padding-top: 8px;
  display: flex;
  align-items: center;
  gap: 10px;
  color: #475569;
  font-size: 12px;
  flex-wrap: wrap;
}
.iface-month-summary b {
  color: #0f172a;
  font-weight: 600;
}
.traffic-line {
  display: grid;
  gap: 8px;
  min-width: 0;
}
.traffic-total {
  grid-column: 1 / -1;
}
.traffic-label {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 8px;
  justify-content: space-between;
  font-size: 12px;
}
.traffic-label b {
  font-weight: 500;
  white-space: nowrap;
}
/* 本月流量吞吐：仅字体颜色层级，不改布局尺寸 */
.throughput-card .traffic-lbl {
  color: #64748b;
  font-weight: 650;
  font-size: 11px;
  letter-spacing: 0.03em;
}
.throughput-card .traffic-in-row .traffic-bytes {
  color: #1d4ed8;
  font-weight: 650;
}
.throughput-card .traffic-out-row .traffic-bytes {
  color: #047857;
  font-weight: 650;
}
.throughput-card .traffic-total-row .traffic-bytes {
  color: #0f172a;
  font-weight: 700;
}
.throughput-card .traffic-ratio {
  font-weight: 500;
}
.throughput-card .traffic-ratio.traffic-ratio-tier-none {
  color: #94a3b8;
}
.throughput-card .traffic-ratio.traffic-ratio-tier-low {
  color: #15803d;
}
.throughput-card .traffic-ratio.traffic-ratio-tier-mid {
  color: #b45309;
}
.throughput-card .traffic-ratio.traffic-ratio-tier-high {
  color: #b91c1c;
  font-weight: 650;
}
/* 吞吐进度条：与括号比例同一档位配色 */
.throughput-card .thin-progress .thin-progress-fill {
  min-width: 0;
}
.throughput-card .thin-progress .thin-progress-fill.traffic-ratio-tier-none {
  background: linear-gradient(90deg, #94a3b8, #cbd5e1);
}
.throughput-card .thin-progress .thin-progress-fill.traffic-ratio-tier-low {
  background: linear-gradient(90deg, #15803d, #22c55e);
}
.throughput-card .thin-progress .thin-progress-fill.traffic-ratio-tier-mid {
  background: linear-gradient(90deg, #d97706, #ea580c);
}
.throughput-card .thin-progress .thin-progress-fill.traffic-ratio-tier-high {
  background: linear-gradient(90deg, #dc2626, #ef4444);
}
.thin-progress,
.wide-progress {
  height: 6px;
  border-radius: 999px;
  background: #e5e7eb;
  overflow: hidden;
}
.thin-progress div,
.wide-progress div {
  height: 100%;
  border-radius: inherit;
}
.wide-progress div {
  background: linear-gradient(90deg, #12b76a, #16a34a);
}
.online-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 8px;
}
.online-head b {
  font-size: 28px;
  line-height: 1;
  color: #0f172a;
}
.online-head span {
  margin-left: 7px;
  font-size: 18px;
  font-weight: 700;
  color: #0f172a;
}
.online-card .summary-sub {
  margin-top: 6px;
}
.chart-icon {
  width: 42px;
  height: 42px;
  border-radius: 50%;
  position: relative;
}
.chart-icon::before {
  content: "";
  position: absolute;
  inset: 12px 11px 11px;
  border: 2px solid currentColor;
  border-radius: 4px;
}
.chart-icon::after {
  content: "";
  position: absolute;
  left: 14px;
  top: 22px;
  width: 16px;
  height: 10px;
  border-left: 2px solid currentColor;
  border-bottom: 2px solid currentColor;
  transform: skewX(-22deg);
}
.cert-pill {
  position: absolute;
  top: 32px;
  right: 18px;
  min-width: 46px;
  height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 750;
}
.cert-pill.ok { background: #dcfce7; color: #16a34a; }
.cert-pill.warn { background: #fef3c7; color: #b45309; }
.cert-pill.bad { background: #fee2e2; color: #dc2626; }
.cert-body {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 10px;
}
.shield-icon {
  position: relative;
}
.shield-icon::before {
  content: "";
  position: absolute;
  left: 18px;
  top: 12px;
  width: 20px;
  height: 24px;
  border: 3px solid currentColor;
  border-radius: 8px 8px 14px 14px;
  clip-path: polygon(50% 0, 100% 18%, 100% 62%, 50% 100%, 0 62%, 0 18%);
}
.shield-icon::after {
  content: "";
  position: absolute;
  left: 24px;
  top: 24px;
  width: 10px;
  height: 6px;
  border-left: 3px solid currentColor;
  border-bottom: 3px solid currentColor;
  transform: rotate(-45deg);
}
.cert-days {
  display: flex;
  align-items: baseline;
  gap: 6px;
  margin-bottom: 4px;
  color: #0f172a;
}
.cert-days b {
  font-size: 28px;
  line-height: 1;
}
.cert-days span {
  font-size: 16px;
  font-weight: 750;
}
.dashboard-main {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(420px, 1fr);
  grid-template-rows: 1fr;
  gap: 12px;
  min-height: 0;
  height: 100%;
  overflow: hidden;
}
.card-area {
  background: #fff;
  border: 1px solid #dfe7f1;
  border-radius: 8px;
  box-shadow: 0 8px 22px rgba(15, 23, 42, .08);
}
.chart-block {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 16px 6px;
}
.card-head .section-title {
  font-size: 15px;
  font-weight: 600;
}
.trend-chart {
  flex: 1;
  min-height: 0;
  padding: 0 8px 8px;
}
.top5-block { display: grid; grid-template-rows: auto 1fr; min-height: 0; height: 100%; }
.chip {
  font-size: 11px;
  color: var(--text-2);
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 999px;
  padding: 2px 8px;
}
.top5-scroll {
  min-height: 0;
  height: 100%;
  overflow: auto;
  padding: 0 12px 10px;
}
.top5-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
.top5-table th,
.top5-table td {
  height: 46px;
  padding: 8px 8px;
  border-bottom: 1px solid #dfe7f1;
  font-size: 13px;
  text-align: left;
}
.top5-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  height: 38px;
  background: #fff;
  font-size: 13px;
  font-weight: 650;
  color: #334155;
}
.top5-table tbody tr:hover td {
  background: #f8fafc;
}
.col-rank { width: 54px; }
.col-name { width: auto; }
.col-num { width: 88px; text-align: right !important; }
.top5-table .col-rank b,
.top5-table .col-num b {
  color: #0f172a;
  font-size: 13px;
}
.top5-table .col-name code {
  display: block;
  width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 4px 9px;
  border: 0;
  border-radius: 5px;
  background: linear-gradient(90deg, #f1f5f9, #eef2f7);
  color: #0f172a;
  font-size: 12px;
}
.log-modal-mask {
  position: fixed;
  inset: 0;
  z-index: 520;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(15, 23, 42, .58);
  color: #0f172a;
  --surface: #ffffff;
  --surface-2: #f8fafc;
  --border: #e2e8f0;
  --text: #0f172a;
  --text-2: #475569;
}
.log-modal {
  width: min(980px, 100%);
  height: min(720px, calc(100vh - 48px));
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr);
  background: #fff;
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  box-shadow: 0 24px 80px rgba(15, 23, 42, .34);
  overflow: hidden;
}
.log-modal-head {
  min-height: 66px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.log-modal-head h2 {
  font-size: 18px;
  line-height: 1.2;
  font-weight: 700;
}
.log-modal-head p {
  margin-top: 3px;
  color: var(--text-2);
  font-size: 12px;
  word-break: break-all;
}
.log-head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.log-close {
  width: 32px;
  height: 32px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: #fff;
  color: var(--text-2);
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
}
.log-close:hover { background: var(--surface-2); color: var(--text); }
.log-toolbar {
  min-height: 44px;
  padding: 8px 18px;
  border-bottom: 1px solid var(--border);
  background: var(--surface-2);
  display: flex;
  align-items: center;
  gap: 8px;
}
.log-filter-group {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--border);
  border-radius: 7px;
  background: #fff;
}
.log-filter-btn {
  height: 26px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--text-2);
  font: inherit;
  font-size: 12px;
  font-weight: 650;
  padding: 0 10px;
  cursor: pointer;
}
.log-filter-btn:hover { background: var(--surface-2); color: var(--text); }
.log-filter-btn.active.all { background: #e2e8f0; color: #0f172a; }
.log-filter-btn.active.info { background: #dbeafe; color: #1d4ed8; }
.log-filter-btn.active.warn { background: #fef3c7; color: #92400e; }
.log-filter-btn.active.error { background: #fee2e2; color: #991b1b; }
.log-follow-toggle {
  height: 32px;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: 7px;
  background: #fff;
  color: var(--text-2);
  font-size: 12px;
  font-weight: 650;
  cursor: pointer;
}
.log-follow-toggle input {
  width: 14px;
  height: 14px;
  accent-color: #2563eb;
}
.log-toolbar-note {
  margin-left: auto;
  color: var(--text-2);
  font-size: 12px;
}
.log-body {
  min-height: 0;
  background: #0b1220;
  color: #dbeafe;
  overflow: auto;
}
.log-table-wrap {
  min-height: 100%;
  min-width: 980px;
}
.log-table {
  width: 100%;
  border-collapse: collapse;
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.45;
  table-layout: auto;
}
.log-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  height: 34px;
  padding: 0 10px;
  border-bottom: 1px solid #1e293b;
  background: #111827;
  color: #94a3b8;
  font-size: 11px;
  font-weight: 700;
  text-align: left;
  white-space: nowrap;
}
.log-table td {
  height: 30px;
  padding: 5px 10px;
  border-bottom: 1px solid rgba(30, 41, 59, .72);
  white-space: nowrap;
  vertical-align: middle;
}
.log-table tbody tr:hover td {
  background: rgba(30, 41, 59, .72);
}
.log-col-time { width: 174px; color: #93c5fd; }
.log-col-file { width: 96px; color: #c4b5fd; }
.log-col-level { width: 72px; }
.log-col-message {
  min-width: 620px;
  color: #dbeafe;
}
.log-level {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 46px;
  height: 22px;
  border-radius: 999px;
  font-family: var(--font);
  font-size: 11px;
  font-weight: 700;
}
.log-table tr.info .log-level {
  background: rgba(37, 99, 235, .16);
  color: #93c5fd;
}
.log-table tr.warn .log-level {
  background: rgba(245, 158, 11, .16);
  color: #fcd34d;
}
.log-table tr.error .log-level {
  background: rgba(239, 68, 68, .18);
  color: #fca5a5;
}
.log-table tr.warn .log-col-message { color: #fde68a; }
.log-table tr.error .log-col-message { color: #fecaca; }
.log-table tr.warn .log-col-time,
.log-table tr.warn .log-col-file { color: #fbbf24; }
.log-table tr.error .log-col-time,
.log-table tr.error .log-col-file { color: #f87171; }
.log-body ::selection {
  background: rgba(59, 130, 246, .42);
  color: #fff;
}
.log-empty {
  min-height: 100%;
  display: grid;
  place-items: center;
  padding: 28px;
  color: #94a3b8;
  font-size: 13px;
}
.log-empty.error { color: #fecaca; }
.log-modal-fade-enter-active,
.log-modal-fade-leave-active {
  transition: opacity .15s ease;
}
.log-modal-fade-enter-from,
.log-modal-fade-leave-to {
  opacity: 0;
}
@media (max-width: 1480px) {
  .dashboard-summary {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
@media (max-width: 1200px) {
  .dashboard-page { height: auto; grid-template-rows: auto auto; overflow: visible; padding-bottom: 0; }
  .dashboard-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .dashboard-main { grid-template-columns: 1fr; }
  .trend-chart { flex: none; height: 300px; }
  .top5-scroll { height: auto; max-height: 260px; }
}
@media (max-width: 640px) {
  .dashboard-summary { grid-template-columns: 1fr; }
  .service-body,
  .cert-body {
    gap: 18px;
  }
  .top5-scroll {
    overflow-x: auto;
  }
  .top5-table {
    min-width: 620px;
  }
  .page-actions { flex-wrap: wrap; justify-content: flex-end; }
  .log-modal-mask { padding: 12px; }
  .log-modal { height: calc(100vh - 24px); }
  .log-modal-head { align-items: flex-start; flex-direction: column; }
  .log-toolbar { align-items: flex-start; flex-wrap: wrap; }
  .log-toolbar-note { margin-left: 0; width: 100%; }
}
</style>
