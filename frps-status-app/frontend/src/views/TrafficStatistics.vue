<template>
  <div class="stats-shell">
    <div class="page-header">
      <div>
        <div class="page-title">流量统计</div>
        <div class="page-sub">公网IP：{{ currentPublicIP || '-' }} · 网卡：{{ currentIface || '-' }}</div>
      </div>
      <div class="flex-center">
        <button class="btn btn-outline btn-sm" :disabled="loadingData" @click="fetchRows">↻ 刷新</button>
      </div>
    </div>

    <div class="page-body analytics-page stats-page">
      <section class="analytics-overview">
        <div>
          <div class="section-title">所选时间范围</div>
          <div class="text-muted text-sm">{{ startDate }} 至 {{ endDate }} · {{ currentPublicIP || '-' }} / {{ currentIface || '-' }}</div>
        </div>
        <div class="analytics-overview-metrics">
          <div><b>{{ humanBytesKB(totalRxKB) }}</b><small>上行</small></div>
          <div><b>{{ humanBytesKB(totalTxKB) }}</b><small>下行</small></div>
          <div><b>{{ humanBytesKB(totalKB) }}</b><small>总流量</small></div>
        </div>
      </section>

      <section class="analytics-filters">
        <div class="analytics-filter-grid traffic-filter-grid">
          <input class="form-input" type="date" v-model="startDate" />
          <input class="form-input" type="date" v-model="endDate" />
          <button class="btn btn-outline btn-sm" @click="quickThisMonth">本月</button>
          <button class="btn btn-outline btn-sm" @click="fetchRows">查询</button>
        </div>
      </section>

      <div class="analytics-main-2col stats-main">
        <section class="card stats-chart-card">
          <div class="section-head">
            <div class="section-title">日流量趋势</div>
          </div>
          <div ref="chartEl" class="analytics-trend-chart"></div>
        </section>

        <section class="card stats-table-card">
          <div class="section-head">
            <div class="section-title">日流量明细</div>
            <span class="text-muted text-sm">{{ filteredRows.length }} 条</span>
          </div>
          <div class="table-wrap stats-table-scroll">
            <table class="stats-detail-table">
              <thead>
                <tr><th class="col-date">日期</th><th>公网IP</th><th>网卡</th><th class="col-num">上行</th><th class="col-num">下行</th><th class="col-num">总流量</th></tr>
              </thead>
              <tbody>
                <tr v-if="!pagedRows.length"><td colspan="6" class="empty">暂无数据</td></tr>
                <tr v-for="r in pagedRows" :key="`${r.day}:${r.public_ip}:${r.iface}`">
                  <td class="col-date">{{ r.day }}</td>
                  <td>{{ r.public_ip }}</td>
                  <td><span class="badge badge-ok">{{ r.iface }}</span></td>
                  <td class="col-num">{{ humanBytesKB(r.rx_kb) }}</td>
                  <td class="col-num">{{ humanBytesKB(r.tx_kb) }}</td>
                  <td class="col-num"><b>{{ humanBytesKB(r.rx_kb + r.tx_kb) }}</b></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="analytics-pager">
            <button class="btn btn-outline btn-sm" :disabled="page <= 1" @click="page--">上一页</button>
            <div class="page-num-list">
              <button v-for="n in pageNumbers" :key="'iface-page-'+n" class="btn btn-outline btn-sm page-num" :class="{ active: n === page }" @click="page = n">{{ n }}</button>
            </div>
            <span class="text-muted text-sm">第 {{ page }} / {{ totalPages }} 页</span>
            <button class="btn btn-outline btn-sm" :disabled="page >= totalPages" @click="page++">下一页</button>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import * as echarts from 'echarts'
import { api } from '../api/index.js'

const chartEl = ref(null)
let chart = null
const loadingData = ref(false)
const rows = ref([])
const page = ref(1)
const PAGE_SIZE = 15
const hostPublicIP = ref('')
const hostIface = ref('')

const startDate = ref('')
const endDate = ref('')

function formatDate(d) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function quickThisMonth() {
  const now = new Date()
  startDate.value = formatDate(new Date(now.getFullYear(), now.getMonth(), 1))
  endDate.value = formatDate(now)
}

function humanBytesKB(kb) {
  const n = Number(kb || 0) * 1024
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let idx = 0
  while (v >= 1024 && idx < units.length - 1) {
    v /= 1024
    idx++
  }
  return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[idx]}`
}

async function fetchRows() {
  loadingData.value = true
  try {
    rows.value = await api.getDailyInterface({ from: startDate.value, to: endDate.value })
    page.value = 1
  } finally {
    loadingData.value = false
  }
}

async function fetchHostNetwork() {
  try {
    const info = await api.getHostNetwork()
    hostPublicIP.value = info?.public_ip || ''
    hostIface.value = info?.iface || ''
  } catch {
    hostPublicIP.value = ''
    hostIface.value = ''
  }
}

const currentPublicIP = computed(() => hostPublicIP.value || rows.value[0]?.public_ip || '')
const currentIface = computed(() => hostIface.value || rows.value[0]?.iface || '')
const filteredRows = computed(() =>
  rows.value
    .slice()
    .sort((a, b) => (a.day === b.day ? String(a.iface).localeCompare(String(b.iface)) : b.day.localeCompare(a.day))))

const totalRxKB = computed(() => filteredRows.value.reduce((s, r) => s + Number(r.rx_kb || 0), 0))
const totalTxKB = computed(() => filteredRows.value.reduce((s, r) => s + Number(r.tx_kb || 0), 0))
const totalKB = computed(() => totalRxKB.value + totalTxKB.value)

const totalPages = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / PAGE_SIZE)))
const pagedRows = computed(() => filteredRows.value.slice((page.value - 1) * PAGE_SIZE, page.value * PAGE_SIZE))
const pageNumbers = computed(() => {
  const t = totalPages.value
  const p = page.value
  if (t <= 5) return Array.from({ length: t }, (_, i) => i + 1)
  if (p <= 3) return [1, 2, 3, 4, 5]
  if (p >= t - 2) return [t - 4, t - 3, t - 2, t - 1, t]
  return [p - 2, p - 1, p, p + 1, p + 2]
})

const trendRows = computed(() => {
  const m = new Map()
  for (const r of filteredRows.value) {
    const key = r.day
    if (!m.has(key)) m.set(key, { day: key, rx_kb: 0, tx_kb: 0 })
    const row = m.get(key)
    row.rx_kb += Number(r.rx_kb || 0)
    row.tx_kb += Number(r.tx_kb || 0)
  }
  return [...m.values()].sort((a, b) => a.day.localeCompare(b.day))
})

function buildChart() {
  if (!chart) return
  const days = trendRows.value
  if (!days.length) {
    chart.clear()
    return
  }
  chart.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      formatter: params => {
        const d = params[0]?.axisValue || ''
        return `${d}<br/>${params.map(p => `${p.marker}${p.seriesName}: <b>${humanBytesKB(p.value)}</b>`).join('<br/>')}`
      }
    },
    legend: { data: ['上行', '下行'], top: 0, left: 'center', textStyle: { color: '#334155', fontSize: 13 } },
    grid: { left: 74, right: 22, top: 48, bottom: 42 },
    xAxis: { type: 'category', data: days.map(d => d.day), boundaryGap: false, axisLabel: { color: '#64748b', fontSize: 12 }, axisLine: { lineStyle: { color: '#475569' } }, axisTick: { show: false } },
    yAxis: { type: 'value', axisLabel: { color: '#64748b', fontSize: 12, formatter: v => humanBytesKB(v) }, splitLine: { lineStyle: { color: '#dbe3ee', type: 'dashed' } }, axisLine: { show: false }, axisTick: { show: false } },
    series: [
      { name: '上行', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => d.rx_kb), itemStyle: { color: '#1f7ae0' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(31,122,224,.08)' } },
      { name: '下行', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => d.tx_kb), itemStyle: { color: '#12b76a' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(18,183,106,.15)' } }
    ]
  })
}

watch(filteredRows, () => { page.value = 1; buildChart() })
const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart?.resize()) : null

onMounted(async () => {
  quickThisMonth()
  await fetchHostNetwork()
  await fetchRows()
  await nextTick()
  chart = echarts.init(chartEl.value)
  buildChart()
  ro?.observe(chartEl.value)
})
onUnmounted(() => {
  ro?.disconnect()
  chart?.dispose()
})
</script>

<style scoped>
.stats-shell {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}
.stats-page {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}
.traffic-filter-grid { grid-template-columns: 150px 150px auto auto; }
.stats-main {
  flex: 1;
  grid-template-columns: 3fr 2fr;
  grid-template-rows: 1fr;
  min-height: 0;
  overflow: hidden;
}
.stats-chart-card {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.stats-table-card {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  min-height: 0;
}
.stats-chart-card .analytics-trend-chart {
  flex: 1;
  min-height: 0;
  height: auto;
}
.stats-table-scroll {
  min-height: 0;
  overflow: auto;
}
.stats-detail-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
.stats-detail-table th,
.stats-detail-table td {
  border-bottom: 1px solid var(--border);
  padding: 8px;
  text-align: left;
}
.stats-detail-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--surface);
}
.stats-detail-table .col-date { width: 92px; }
.stats-detail-table .col-num { width: 90px; text-align: right; }
.analytics-pager { display: flex; align-items: center; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
.page-num-list { display: flex; gap: 6px; }
.page-num { min-width: 34px; padding: 0 8px; }
.page-num.active { color: #bfdbfe; border-color: #1d4ed8; background: #172554; }
@media (max-width: 1200px) {
  .stats-page { flex: none; min-height: auto; overflow: visible; }
  .stats-main { grid-template-columns: 1fr; overflow: visible; }
  .stats-chart-card .analytics-trend-chart { flex: none; height: 300px; }
  .stats-table-scroll { max-height: 480px; }
  .traffic-filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
