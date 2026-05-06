<template>
  <div class="dashboard-shell">
    <div class="page-header">
      <div>
        <div class="page-title">数据看板</div>
        <div class="page-sub">{{ updatedAt ? '最后更新：' + updatedAt : '加载中…' }}</div>
      </div>
      <div class="page-actions">
        <button class="btn btn-outline btn-sm" @click="openLogModal"><span class="btn-doc-icon" aria-hidden="true"></span>日志</button>
        <button class="btn btn-outline btn-sm" :disabled="loading" @click="$emit('refresh')">
          <span v-if="loading" class="spinner"></span>
          <span v-else class="btn-refresh-icon" aria-hidden="true">↻</span>刷新
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
              <div class="summary-sub">{{ bindLatency }}ms · Dashboard {{ dashOk ? dashLatency + 'ms' : '离线' }}</div>
            </div>
          </div>
        </section>

        <section class="summary-card throughput-card">
          <div class="summary-title">本月流量吞吐</div>
          <div class="traffic-bars">
            <div class="traffic-line">
              <div class="traffic-label"><span>上行</span><b>{{ humanBytes(status?.month_totals?.in) }} ({{ inPct }}%)</b></div>
              <div class="thin-progress"><div class="blue" :style="{ width: inPct + '%' }"></div></div>
            </div>
            <div class="traffic-line">
              <div class="traffic-label"><span>下行</span><b>{{ humanBytes(status?.month_totals?.out) }} ({{ outPct }}%)</b></div>
              <div class="thin-progress"><div class="green" :style="{ width: outPct + '%' }"></div></div>
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
          <div class="wide-progress"><div :style="{ width: onlinePct + '%' }"></div></div>
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
  chart.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      formatter: params => {
        const d = params[0]?.axisValue || ''
        return `${d}<br/>${params.map(p => `${p.marker}${p.seriesName}: <b>${humanBytes(p.value)}</b>`).join('<br/>')}`
      }
    },
    legend: { data: ['上行', '下行'], top: 0, left: 'center', textStyle: { color: '#334155', fontSize: 13 }, itemWidth: 22, itemHeight: 10 },
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
      axisLabel: { color: '#64748b', fontSize: 12, formatter: v => humanBytes(v) },
      splitLine: { lineStyle: { color: '#dbe3ee', type: 'dashed' } },
      axisLine: { show: false },
      axisTick: { show: false }
    },
    series: [
      { name: '上行', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => byDay[d].in), itemStyle: { color: '#1f7ae0' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(31,122,224,.08)' } },
      { name: '下行', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => byDay[d].out), itemStyle: { color: '#12b76a' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(18,183,106,.15)' } }
    ]
  })
}

watch(() => props.daily, (d) => buildChart(d))
const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart?.resize()) : null

onMounted(async () => {
  await nextTick()
  chart = echarts.init(chartEl.value)
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
  gap: 16px;
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
  min-height: 98px;
  padding: 24px 28px;
}
.dashboard-shell :deep(.page-title) {
  font-size: 26px;
  line-height: 1.15;
  font-weight: 800;
}
.dashboard-shell :deep(.page-sub) {
  margin-top: 8px;
  color: #64748b;
  font-size: 15px;
}
.page-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.page-actions .btn {
  height: 38px;
  padding: 0 16px;
  gap: 8px;
  border-color: #d7e1ed;
  background: #fff;
  color: #0f172a;
  font-size: 14px;
  font-weight: 650;
  box-shadow: 0 4px 12px rgba(15, 23, 42, .04);
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
.btn-refresh-icon {
  display: inline-block;
  font-size: 18px;
  line-height: 1;
}
.dashboard-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}
.summary-card {
  position: relative;
  min-height: 150px;
  min-width: 0;
  padding: 18px 22px;
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
  font-size: 16px;
  font-weight: 750;
}
.summary-sub {
  color: #475569;
  font-size: 14px;
}
.summary-icon {
  width: 74px;
  height: 74px;
  border-radius: 50%;
  background: #e7f6eb;
  color: #166534;
  flex: 0 0 auto;
}
.service-body {
  display: flex;
  align-items: center;
  gap: 24px;
  margin-top: 34px;
}
.server-icon {
  display: grid;
  place-content: center;
  gap: 6px;
}
.server-icon span {
  display: block;
  width: 36px;
  height: 12px;
  border: 3px solid currentColor;
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
  margin-bottom: 10px;
  font-size: 24px;
  font-weight: 800;
  color: #0f172a;
}
.status-dot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  display: inline-block;
}
.status-dot.ok { background: #12b76a; }
.status-dot.bad { background: #ef4444; }
.traffic-bars {
  display: grid;
  gap: 22px;
  margin-top: 32px;
}
.traffic-line {
  display: grid;
  gap: 8px;
}
.traffic-label {
  display: flex;
  align-items: baseline;
  gap: 16px;
  color: #0f172a;
  font-size: 14px;
}
.traffic-label b {
  font-weight: 500;
}
.thin-progress,
.wide-progress {
  height: 8px;
  border-radius: 999px;
  background: #e5e7eb;
  overflow: hidden;
}
.thin-progress div,
.wide-progress div {
  height: 100%;
  border-radius: inherit;
}
.thin-progress .blue { background: linear-gradient(90deg, #1f7ae0, #248af0); }
.thin-progress .green,
.wide-progress div { background: linear-gradient(90deg, #12b76a, #16a34a); }
.online-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 14px;
}
.online-head b {
  font-size: 36px;
  line-height: 1;
  color: #0f172a;
}
.online-head span {
  margin-left: 7px;
  font-size: 24px;
  font-weight: 700;
  color: #0f172a;
}
.online-card .summary-sub {
  margin-top: 10px;
}
.online-card .wide-progress {
  margin: 12px 0 14px;
}
.chart-icon {
  width: 58px;
  height: 58px;
  border-radius: 50%;
  position: relative;
}
.chart-icon::before {
  content: "";
  position: absolute;
  inset: 18px 16px 16px;
  border: 3px solid currentColor;
  border-radius: 4px;
}
.chart-icon::after {
  content: "";
  position: absolute;
  left: 20px;
  top: 31px;
  width: 22px;
  height: 14px;
  border-left: 3px solid currentColor;
  border-bottom: 3px solid currentColor;
  transform: skewX(-22deg);
}
.cert-pill {
  position: absolute;
  top: 54px;
  right: 22px;
  min-width: 58px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 750;
}
.cert-pill.ok { background: #dcfce7; color: #16a34a; }
.cert-pill.warn { background: #fef3c7; color: #b45309; }
.cert-pill.bad { background: #fee2e2; color: #dc2626; }
.cert-body {
  display: flex;
  align-items: center;
  gap: 34px;
  margin-top: 34px;
}
.shield-icon {
  position: relative;
}
.shield-icon::before {
  content: "";
  position: absolute;
  left: 24px;
  top: 18px;
  width: 28px;
  height: 34px;
  border: 4px solid currentColor;
  border-radius: 8px 8px 14px 14px;
  clip-path: polygon(50% 0, 100% 18%, 100% 62%, 50% 100%, 0 62%, 0 18%);
}
.shield-icon::after {
  content: "";
  position: absolute;
  left: 32px;
  top: 34px;
  width: 15px;
  height: 8px;
  border-left: 4px solid currentColor;
  border-bottom: 4px solid currentColor;
  transform: rotate(-45deg);
}
.cert-days {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 6px;
  color: #0f172a;
}
.cert-days b {
  font-size: 36px;
  line-height: 1;
}
.cert-days span {
  font-size: 20px;
  font-weight: 750;
}
.dashboard-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(520px, .98fr);
  grid-template-rows: 1fr;
  gap: 16px;
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
  padding: 18px 22px 8px;
}
.card-head .section-title {
  font-size: 20px;
  font-weight: 800;
}
.trend-chart {
  flex: 1;
  min-height: 0;
  padding: 0 14px 18px;
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
  padding: 0 16px 16px;
}
.top5-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
.top5-table th,
.top5-table td {
  height: 54px;
  padding: 10px 8px;
  border-bottom: 1px solid #dfe7f1;
  font-size: 14px;
  text-align: left;
}
.top5-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  height: 42px;
  background: #fff;
  font-size: 14px;
  font-weight: 750;
  color: #334155;
}
.top5-table tbody tr:hover td {
  background: #f8fafc;
}
.col-rank { width: 54px; }
.col-type { width: 76px; }
.col-name { width: auto; }
.col-num { width: 104px; text-align: right !important; }
.top5-table .col-rank b,
.top5-table .col-num b {
  color: #0f172a;
  font-size: 15px;
}
.top5-table .col-name code {
  display: block;
  width: 136px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 4px 9px;
  border: 0;
  border-radius: 5px;
  background: linear-gradient(90deg, #f1f5f9, #eef2f7);
  color: #0f172a;
  font-size: 14px;
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
