<template>
  <div class="stats-shell">
    <div class="page-header">
      <div>
        <div class="page-title">{{ selectedProxyName ? '历史统计 / ' + selectedProxyName : '历史统计' }}</div>
      </div>
      <div class="flex-center">
        <a class="btn btn-outline btn-sm icon-btn" title="导出 CSV" aria-label="导出 CSV" :href="exportUrl">⇩</a>
        <button class="btn btn-outline btn-sm icon-btn" title="刷新" aria-label="刷新" :disabled="loading || ifaceLoading" @click="refreshPage">
          <span v-if="loading" class="spinner"></span>
          <span v-else>↻</span>
        </button>
      </div>
    </div>

    <div class="page-body analytics-page stats-page">
      <section class="analytics-overview">
        <div>
          <div class="section-title">所选时间范围</div>
          <div class="text-muted text-sm">{{ startDate || minDay || '-' }} 至 {{ endDate || maxDay || '-' }} · {{ selectedProxyName || '网卡流量汇总' }}</div>
        </div>
        <div class="analytics-overview-metrics">
          <div><b>{{ humanBytes(totalTraffic) }}</b><small>总流量</small></div>
          <div><b>{{ humanBytes(totalIn) }}</b><small>入站</small></div>
          <div><b>{{ humanBytes(totalOut) }}</b><small>出站</small></div>
        </div>
      </section>

      <section class="analytics-filters">
        <div class="analytics-filter-grid stats-filter-grid">
          <input class="form-input" type="date" v-model="startDate" />
          <input class="form-input" type="date" v-model="endDate" />
          <span class="stats-filter-spacer" aria-hidden="true"></span>
          <button class="btn btn-outline btn-sm" @click="quickRange(60)">最近60天</button>
          <button class="btn btn-outline btn-sm" @click="quickThisMonth">本月</button>
          <button class="btn btn-outline btn-sm" @click="resetFilter">重置</button>
        </div>
      </section>

      <div class="analytics-main-2col stats-main">
        <section class="card stats-chart-card">
          <div class="section-head">
            <div class="section-title">{{ selectedProxyName ? '代理流量趋势' : '流量趋势' }}</div>
          </div>
          <div ref="chartEl" class="analytics-trend-chart"></div>
        </section>

        <section class="card stats-table-card">
          <div class="section-head">
            <div class="section-title">{{ selectedProxyName ? '当前代理每日明细' : '每日明细（网卡聚合）' }}</div>
            <span class="text-muted text-sm">{{ filteredRows.length }} 条</span>
          </div>
          <div class="table-wrap stats-table-scroll">
            <table class="stats-detail-table">
              <thead>
                <tr v-if="selectedProxyName"><th class="col-date">日期</th><th class="col-type">类型</th><th class="col-num">入站</th><th class="col-num">出站</th><th class="col-num">合计</th></tr>
                <tr v-else><th class="col-date">日期</th><th class="col-count">网卡记录</th><th class="col-num">入站</th><th class="col-num">出站</th><th class="col-num">合计</th></tr>
              </thead>
              <tbody>
                <tr v-if="!pagedRows.length"><td colspan="5" class="empty">暂无数据</td></tr>
                <tr v-for="r in pagedRows" :key="selectedProxyName ? r.day + r.type : r.day">
                  <td class="col-date">{{ r.day }}</td>
                  <td v-if="selectedProxyName" class="col-type"><span class="badge badge-ok">{{ r.type }}</span></td>
                  <td v-else class="col-count">{{ r.proxy_count }}</td>
                  <td class="col-num">{{ humanBytes(r.in) }}</td>
                  <td class="col-num">{{ humanBytes(r.out) }}</td>
                  <td class="col-num"><b>{{ humanBytes(r.in + r.out) }}</b></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="totalPages > 1" class="analytics-pager">
            <button class="btn btn-outline btn-sm" :disabled="page <= 1" @click="page--">上一页</button>
            <div class="page-num-list">
              <button
                v-for="n in pageNumbers"
                :key="'stats-page-' + n"
                class="btn btn-outline btn-sm page-num"
                :class="{ active: n === page }"
                @click="page = n"
              >{{ n }}</button>
            </div>
            <span class="text-muted text-sm">{{ page }} / {{ totalPages }}</span>
            <button class="btn btn-outline btn-sm" :disabled="page >= totalPages" @click="page++">下一页</button>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import * as echarts from 'echarts'
import { humanBytes } from '../utils/format.js'
import { api } from '../api/index.js'

const props = defineProps({ status: Object, daily: Array, loading: Boolean })
const emit = defineEmits(['refresh'])
const route = useRoute()

const exportUrl = api.exportCSVUrl
const chartEl = ref(null)
let chart = null
const startDate = ref('')
const endDate = ref('')
const page = ref(1)
const PAGE_SIZE = 15
const ifaceRows = ref([])
const ifaceLoading = ref(false)

const rows = computed(() => props.daily ?? [])
const selectedProxyName = computed(() => decodeURIComponent(route.params.proxyName || ''))
const globalRows = computed(() => ifaceRows.value.map(r => ({
  day: r.day,
  in: Number(r.rx_kb || 0) * 1024,
  out: Number(r.tx_kb || 0) * 1024,
  iface: r.iface || '',
  public_ip: r.public_ip || ''
})))
const proxyRows = computed(() => selectedProxyName.value ? rows.value.filter(r => r.name === selectedProxyName.value) : globalRows.value)
const proxyInfo = computed(() => (props.status?.proxies ?? []).find(p => p.name === selectedProxyName.value) || null)
const proxyType = computed(() => proxyInfo.value?.type || proxyRows.value[0]?.type || '')
const minDay = computed(() => proxyRows.value.length ? [...proxyRows.value].sort((a, b) => a.day.localeCompare(b.day))[0].day : '')
const maxDay = computed(() => proxyRows.value.length ? [...proxyRows.value].sort((a, b) => b.day.localeCompare(a.day))[0].day : '')

const filteredRows = computed(() => {
  const byDay = {}
  if (selectedProxyName.value) {
    const byDayType = {}
    for (const r of proxyRows.value) {
      if (startDate.value && r.day < startDate.value) continue
      if (endDate.value && r.day > endDate.value) continue
      const key = `${r.day}:${r.type || proxyType.value}`
      byDayType[key] ??= { day: r.day, type: r.type || proxyType.value, in: 0, out: 0, peak_conns: 0 }
      byDayType[key].in += Number(r.in || 0)
      byDayType[key].out += Number(r.out || 0)
      byDayType[key].peak_conns = Math.max(byDayType[key].peak_conns, Number(r.peak_conns || 0))
    }
    return Object.values(byDayType).sort((a, b) => b.day.localeCompare(a.day))
  }
  for (const r of globalRows.value) {
    if (startDate.value && r.day < startDate.value) continue
    if (endDate.value && r.day > endDate.value) continue
    byDay[r.day] ??= { day: r.day, in: 0, out: 0, proxy_count: 0, seen: new Set() }
    byDay[r.day].in += Number(r.in || 0)
    byDay[r.day].out += Number(r.out || 0)
    const key = `${r.iface || '-'}:${r.public_ip || '-'}`
    if (!byDay[r.day].seen.has(key)) {
      byDay[r.day].seen.add(key)
      byDay[r.day].proxy_count++
    }
  }
  return Object.values(byDay).sort((a, b) => b.day.localeCompare(a.day))
})

const totalIn = computed(() => filteredRows.value.reduce((s, r) => s + r.in, 0))
const totalOut = computed(() => filteredRows.value.reduce((s, r) => s + r.out, 0))
const totalTraffic = computed(() => totalIn.value + totalOut.value)

const totalPages = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / PAGE_SIZE)))
const pagedRows = computed(() => {
  const s = (page.value - 1) * PAGE_SIZE
  return filteredRows.value.slice(s, s + PAGE_SIZE)
})
const pageNumbers = computed(() => {
  const t = totalPages.value
  const p = page.value
  if (t <= 5) return Array.from({ length: t }, (_, i) => i + 1)
  if (p <= 3) return [1, 2, 3, 4, 5]
  if (p >= t - 2) return [t - 4, t - 3, t - 2, t - 1, t]
  return [p - 2, p - 1, p, p + 1, p + 2]
})
watch(filteredRows, () => { page.value = 1 })

function quickRange(days) {
  const end = new Date()
  const start = new Date(Date.now() - (days - 1) * 86400000)
  endDate.value = formatLocalDate(end)
  startDate.value = formatLocalDate(start)
}

function quickThisMonth() {
  const now = new Date()
  const start = new Date(now.getFullYear(), now.getMonth(), 1)
  startDate.value = formatLocalDate(start)
  endDate.value = formatLocalDate(now)
}

function resetFilter() {
  startDate.value = ''
  endDate.value = ''
}

async function loadIfaceRows() {
  ifaceLoading.value = true
  try {
    const data = await api.getDailyInterface()
    ifaceRows.value = Array.isArray(data) ? data : []
  } catch {
    ifaceRows.value = []
  } finally {
    ifaceLoading.value = false
  }
}

async function refreshPage() {
  await Promise.all([
    loadIfaceRows(),
    Promise.resolve(emit('refresh'))
  ])
}

function formatLocalDate(d) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function buildChart() {
  if (!chart) return
  const days = [...filteredRows.value].sort((a, b) => a.day.localeCompare(b.day))
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
        return `${d}<br/>${params.map(p => `${p.marker}${p.seriesName}: <b>${humanBytes(p.value)}</b>`).join('<br/>')}`
      }
    },
    legend: { data: ['入站', '出站'], top: 0, left: 'center', textStyle: { color: '#334155', fontSize: 13 }, itemWidth: 22, itemHeight: 10 },
    grid: { left: 74, right: 22, top: 48, bottom: 42 },
    xAxis: {
      type: 'category',
      data: days.map(d => d.day),
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
      { name: '入站', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => d.in), itemStyle: { color: '#1f7ae0' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(31,122,224,.08)' } },
      { name: '出站', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => d.out), itemStyle: { color: '#12b76a' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(18,183,106,.15)' } }
    ]
  })
}

watch(filteredRows, buildChart)
watch(() => props.status?.generated_at, () => {
  if (!selectedProxyName.value) loadIfaceRows()
})
const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart?.resize()) : null
onMounted(async () => {
  await nextTick()
  quickThisMonth()
  await loadIfaceRows()
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
.stats-filter-grid { grid-template-columns: 150px 150px 1fr auto auto auto; }
.stats-filter-spacer { min-width: 0; }
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
  padding: 8px 8px;
  text-align: left;
}
.stats-detail-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--surface);
}
.stats-detail-table .col-date { width: 92px; }
.stats-detail-table .col-count,
.stats-detail-table .col-type { width: 64px; }
.stats-detail-table .col-num { width: 86px; text-align: right; }
.stats-detail-table .badge {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}
.card :deep(table th),
.card :deep(table td) {
  font-size: 13px;
}
.card :deep(table th) {
  font-size: 12px;
  color: var(--text-2);
}
.analytics-pager {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.page-num-list {
  display: flex;
  gap: 6px;
}
.page-num {
  min-width: 34px;
  padding: 0 8px;
}
@media (max-width: 1200px) {
  .stats-page { flex: none; min-height: auto; overflow: visible; }
  .stats-main { grid-template-columns: 1fr; overflow: visible; }
  .stats-chart-card .analytics-trend-chart { flex: none; height: 300px; }
  .stats-table-scroll { max-height: 480px; }
  .stats-filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
