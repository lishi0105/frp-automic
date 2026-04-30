<template>
  <div class="dashboard-shell">
    <div class="page-header">
      <div>
        <div class="page-title">数据看板</div>
        <div class="page-sub">{{ updatedAt ? '最后更新：' + updatedAt : '加载中…' }}</div>
      </div>
      <div class="page-actions">
        <button class="btn btn-outline btn-sm" @click="openLogModal">日志</button>
        <button class="btn btn-outline btn-sm" :disabled="loading" @click="$emit('refresh')">
          <span v-if="loading" class="spinner"></span>
          <span v-else>↻</span> 刷新
        </button>
      </div>
    </div>

    <div class="page-body dashboard-page">
      <div class="dashboard-rings">
        <section class="ring-card">
          <div class="ring-title">FRPS 服务</div>
          <div class="ring-wrap">
            <div class="metric-ring" :style="{ '--ring': servicePct + '%' }"><div class="ring-center">{{ bindOk ? '在线' : '离线' }}</div></div>
          </div>
          <div class="ring-sub">{{ bindLatency }}ms · Dashboard {{ dashOk ? dashLatency + 'ms' : '离线' }}</div>
        </section>
        <section class="ring-card">
          <div class="ring-title">本月上行</div>
          <div class="ring-wrap">
            <div class="metric-ring blue" :style="{ '--ring': inPct + '%' }"><div class="ring-center">{{ inPct }}%</div></div>
          </div>
          <div class="ring-sub">{{ humanBytes(status?.month_totals?.in) }}</div>
        </section>
        <section class="ring-card">
          <div class="ring-title">本月下行</div>
          <div class="ring-wrap">
            <div class="metric-ring green" :style="{ '--ring': outPct + '%' }"><div class="ring-center">{{ outPct }}%</div></div>
          </div>
          <div class="ring-sub">{{ humanBytes(status?.month_totals?.out) }}</div>
        </section>
        <section class="ring-card">
          <div class="ring-title">代理在线</div>
          <div class="ring-wrap">
            <div class="metric-ring teal" :style="{ '--ring': onlinePct + '%' }"><div class="ring-center">{{ onlineProxies }}/{{ totalProxies }}</div></div>
          </div>
          <div class="ring-sub">TCP {{ proxyTypeMap.tcp || 0 }} · HTTP {{ proxyTypeMap.http || 0 }} · HTTPS {{ proxyTypeMap.https || 0 }}</div>
        </section>
        <section class="ring-card">
          <div class="ring-title">证书风险</div>
          <div class="ring-wrap">
            <div class="metric-ring amber" :style="{ '--ring': certRiskPct + '%' }"><div class="ring-center">{{ certSummary.min_days_left ?? '-' }}天</div></div>
          </div>
          <div class="ring-sub">WARN {{ certSummary.warn || 0 }} · FAIL {{ certSummary.fail || 0 }}</div>
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
                  <th class="col-rank">排名</th><th class="col-name">代理名称</th><th class="col-type">类型</th><th class="col-num">上行</th><th class="col-num">下行</th><th class="col-num">总流量</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="!topProxies.length"><td colspan="6" class="empty">暂无数据</td></tr>
                <tr v-for="(p, i) in topProxies" :key="p.name + p.type">
                  <td class="col-rank"><b>#{{ i + 1 }}</b></td>
                  <td class="col-name"><code>{{ p.name }}</code></td>
                  <td class="col-type"><span class="badge badge-ok">{{ p.type }}</span></td>
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
                  <span v-if="logLoading" class="spinner"></span>
                  <span v-else>↻</span> 刷新
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

const updatedAt = computed(() => props.status
  ? new Date(props.status.generated_at).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })
  : '')

const bindOk = computed(() => props.status?.frps?.bind?.ok ?? false)
const bindLatency = computed(() => props.status?.frps?.bind?.latency_ms ?? '-')
const dashOk = computed(() => props.status?.frps?.dashboard?.ok ?? false)
const dashLatency = computed(() => props.status?.frps?.dashboard?.latency_ms ?? '-')

const alertInGB = computed(() => props.status?.settings?.alert_in_gb || 0)
const alertOutGB = computed(() => props.status?.settings?.alert_out_gb || 0)
const inPct = computed(() => percent(props.status?.month_totals?.in, alertInGB.value))
const outPct = computed(() => percent(props.status?.month_totals?.out, alertOutGB.value))
const servicePct = computed(() => bindOk.value ? 100 : 0)

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

const certSummary = computed(() => props.status?.dashboard?.certificate || {
  total: certs.value.length,
  ok: certs.value.filter(c => c.ok && (c.days_left == null || c.days_left >= 15)).length,
  warn: certs.value.filter(c => c.ok && c.days_left != null && c.days_left < 15).length,
  fail: certs.value.filter(c => !c.ok).length,
  min_domain: certs.value[0]?.domain || '',
  min_days_left: certs.value[0]?.days_left ?? null
})

const certRiskPct = computed(() => {
  const total = certSummary.value.total || 0
  if (!total) return 0
  const risk = (certSummary.value.warn || 0) + (certSummary.value.fail || 0)
  return Math.round((risk / total) * 100)
})

function certColor(days) {
  if (days == null) return 'var(--danger)'
  if (days < 7) return 'var(--danger)'
  if (days < 15) return 'var(--warning)'
  return 'var(--success)'
}

function buildChart(daily) {
  if (!chart) return
  const rows = Array.isArray(daily) ? daily : []
  if (!rows.length) {
    chart.clear()
    return
  }
  const byDay = {}
  for (const r of rows) {
    byDay[r.day] ??= { in: 0, out: 0 }
    byDay[r.day].in += Number(r.in)
    byDay[r.day].out += Number(r.out)
  }
  const days = Object.keys(byDay).sort().slice(-30)
  const isDark = true
  chart.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      formatter: params => {
        const d = params[0]?.axisValue || ''
        return `${d}<br/>${params.map(p => `${p.marker}${p.seriesName}: <b>${humanBytes(p.value)}</b>`).join('<br/>')}`
      }
    },
    legend: { data: ['上行', '下行'], top: 0, textStyle: { color: isDark ? '#94a3b8' : '#475569' } },
    grid: { left: 56, right: 16, top: 38, bottom: 34 },
    xAxis: { type: 'category', data: days, axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11 }, axisLine: { lineStyle: { color: isDark ? '#334155' : '#e2e8f0' } } },
    yAxis: { type: 'value', axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11, formatter: v => humanBytes(v) }, splitLine: { lineStyle: { color: isDark ? '#1e293b' : '#f1f5f9' } } },
    series: [
      { name: '上行', type: 'line', smooth: true, data: days.map(d => byDay[d].in), itemStyle: { color: '#3b82f6' }, areaStyle: { color: 'rgba(59,130,246,.12)' } },
      { name: '下行', type: 'line', smooth: true, data: days.map(d => byDay[d].out), itemStyle: { color: '#10b981' }, areaStyle: { color: 'rgba(16,185,129,.12)' } }
    ]
  })
}

watch(() => props.daily, (d) => buildChart(d))
const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart?.resize()) : null

onMounted(async () => {
  await nextTick()
  const isDark = true
  chart = echarts.init(chartEl.value, isDark ? 'dark' : null)
  if (props.daily?.length) buildChart(props.daily)
  ro?.observe(chartEl.value)
})

onUnmounted(() => {
  stopLogAutoRefresh()
  ro?.disconnect()
  chart?.dispose()
})
</script>

<style scoped>
.dashboard-page {
  display: grid;
  gap: 10px;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  grid-template-rows: auto minmax(0, 1fr);
}
.dashboard-shell {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}
.page-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.dashboard-rings {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
}
.ring-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r);
  padding: 8px 10px;
  box-shadow: var(--shadow);
  min-width: 0;
  display: grid;
  align-content: start;
  gap: 6px;
}
.ring-title { font-size: 12px; font-weight: 650; color: var(--text-2); line-height: 1.2; }
.ring-wrap { display: grid; place-items: center; }
.metric-ring {
  --ring: 0%;
  width: 96px; height: 96px; border-radius: 50%;
  background: conic-gradient(#2563eb var(--ring), #334155 0);
  display: grid; place-items: center;
}
.metric-ring::before {
  content: '';
  width: 72px; height: 72px; border-radius: 50%;
  background: var(--surface);
}
.metric-ring.blue { background: conic-gradient(#2563eb var(--ring), #334155 0); }
.metric-ring.green { background: conic-gradient(#10b981 var(--ring), #334155 0); }
.metric-ring.teal { background: conic-gradient(#0ea5a4 var(--ring), #334155 0); }
.metric-ring.amber { background: conic-gradient(#f59e0b var(--ring), #334155 0); }
.ring-center {
  position: absolute;
  font-size: 12px;
  font-weight: 700;
  color: var(--text);
}
.ring-sub {
  font-size: 11px;
  color: var(--text-2);
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dashboard-main {
  display: grid;
  grid-template-columns: 3fr 2fr;
  grid-template-rows: 1fr;
  gap: 10px;
  min-height: 0;
  height: 100%;
  overflow: hidden;
}
.card-area {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r);
  box-shadow: var(--shadow);
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
  padding: 12px 14px 8px;
}
.card-head .section-title {
  font-size: 15px;
  font-weight: 700;
}
.trend-chart {
  flex: 1;
  min-height: 0;
  padding: 0 10px 6px;
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
  padding: 0 10px 8px;
}
.top5-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
.top5-table th,
.top5-table td {
  padding: 8px 8px;
  border-bottom: 1px solid var(--border);
  font-size: 12px;
  text-align: left;
}
.top5-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--surface);
  font-size: 12px;
  font-weight: 650;
  color: var(--text-2);
}
.top5-table tbody tr:hover td {
  background: var(--surface-2);
}
.col-rank { width: 48px; }
.col-type { width: 56px; }
.col-name { width: auto; }
.col-num { width: 86px; text-align: right !important; }
.top5-table .col-name code {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
@media (max-width: 1200px) {
  .dashboard-page { height: auto; grid-template-rows: auto auto; overflow: visible; padding-bottom: 0; }
  .dashboard-rings { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .dashboard-main { grid-template-columns: 1fr; }
  .trend-chart { flex: none; height: 300px; }
  .top5-scroll { height: auto; max-height: 260px; }
}
@media (max-width: 640px) {
  .page-actions { flex-wrap: wrap; justify-content: flex-end; }
  .log-modal-mask { padding: 12px; }
  .log-modal { height: calc(100vh - 24px); }
  .log-modal-head { align-items: flex-start; flex-direction: column; }
  .log-toolbar { align-items: flex-start; flex-wrap: wrap; }
  .log-toolbar-note { margin-left: 0; width: 100%; }
}
</style>
