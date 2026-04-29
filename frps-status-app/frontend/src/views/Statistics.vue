<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">历史统计</div>
        <div class="page-sub">全部代理汇总视角</div>
      </div>
      <div class="flex-center">
        <a class="btn btn-outline btn-sm" :href="exportUrl">⬇ 导出 CSV</a>
        <button class="btn btn-outline btn-sm" :disabled="loading" @click="$emit('refresh')">
          <span v-if="loading" class="spinner"></span>
          <span v-else>↻</span> 刷新
        </button>
      </div>
    </div>

    <div class="page-body analytics-page">
      <section class="analytics-overview">
        <div>
          <div class="section-title">所选时间范围</div>
          <div class="text-muted text-sm">{{ startDate || minDay || '-' }} 至 {{ endDate || maxDay || '-' }} · 全部代理汇总</div>
          <div class="analytics-overview-bar"><div class="analytics-overview-bar-inner stats-overview-fill"></div></div>
        </div>
        <div class="analytics-overview-metrics">
          <div><b>{{ humanBytes(totalTraffic) }}</b><small>总流量</small></div>
          <div><b>{{ humanBytes(totalIn) }}</b><small>上行</small></div>
          <div><b>{{ humanBytes(totalOut) }}</b><small>下行</small></div>
        </div>
      </section>

      <section class="analytics-filters">
        <div class="analytics-filter-grid stats-filter-grid">
          <input class="form-input" type="date" v-model="startDate" />
          <input class="form-input" type="date" v-model="endDate" />
          <input class="form-input" value="全局汇总，不筛代理" readonly />
          <button class="btn btn-outline btn-sm" @click="quickRange(30)">最近30天</button>
          <button class="btn btn-outline btn-sm" @click="quickThisMonth">本月</button>
          <button class="btn btn-outline btn-sm" @click="resetFilter">重置</button>
        </div>
      </section>

      <div class="analytics-main-2col">
        <section class="card">
          <div class="section-head">
            <div class="section-title">流量趋势</div>
          </div>
          <div ref="chartEl" class="analytics-trend-chart"></div>
        </section>

        <section class="card">
          <div class="section-head">
            <div class="section-title">代理详情入口</div>
          </div>
          <div class="entry-list-bars">
            <button v-for="p in topContrib" :key="p.name + p.type" class="entry-item" @click="goProxyStats(p.name)">
              <div class="entry-row">
                <code>{{ p.name }}</code>
                <span>{{ humanBytes(p.total) }}</span>
              </div>
              <div class="entry-bar">
                <div class="entry-bar-inner" :style="{ width: topTotal ? ((p.total / topTotal) * 100).toFixed(1) + '%' : '0%' }"></div>
              </div>
            </button>
          </div>
        </section>
      </div>

      <section class="card">
        <div class="section-head">
          <div class="section-title">代理统计子页</div>
          <span class="text-muted text-sm">{{ proxyPages.length }} 个动态页面</span>
        </div>
        <div class="proxy-page-grid">
          <button v-if="!proxyPages.length" class="entry-item" disabled>暂无代理</button>
          <button
            v-for="p in proxyPages"
            :key="p.name + p.type"
            class="entry-item proxy-page-entry"
            @click="goProxyStats(p.name)"
          >
            <div class="entry-row">
              <code>{{ p.name }}</code>
              <span class="badge badge-ok">{{ p.type }}</span>
            </div>
            <div class="text-muted text-sm">/statistics/{{ encodeURIComponent(p.name) }}</div>
          </button>
        </div>
      </section>

      <section class="card">
        <div class="section-head">
          <div class="section-title">每日明细（全局聚合）</div>
          <span class="text-muted text-sm">{{ filteredRows.length }} 条记录</span>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>日期</th><th>代理数</th><th>连接峰值</th><th>上行</th><th>下行</th><th>合计</th></tr></thead>
            <tbody>
              <tr v-if="!pagedRows.length"><td colspan="6" class="empty">暂无数据</td></tr>
              <tr v-for="r in pagedRows" :key="r.day">
                <td>{{ r.day }}</td>
                <td>{{ r.proxy_count }}</td>
                <td>{{ r.peak_conns }}</td>
                <td>{{ humanBytes(r.in) }}</td>
                <td>{{ humanBytes(r.out) }}</td>
                <td><b>{{ humanBytes(r.in + r.out) }}</b></td>
              </tr>
            </tbody>
          </table>
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
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import * as echarts from 'echarts'
import { humanBytes } from '../utils/format.js'
import { api } from '../api/index.js'

const props = defineProps({ status: Object, daily: Array, loading: Boolean })
defineEmits(['refresh'])
const router = useRouter()

const exportUrl = api.exportCSVUrl
const chartEl = ref(null)
let chart = null
const startDate = ref('')
const endDate = ref('')
const page = ref(1)
const PAGE_SIZE = 15

const rows = computed(() => props.daily ?? [])
const minDay = computed(() => rows.value.length ? [...rows.value].sort((a, b) => a.day.localeCompare(b.day))[0].day : '')
const maxDay = computed(() => rows.value.length ? [...rows.value].sort((a, b) => b.day.localeCompare(a.day))[0].day : '')

const filteredRows = computed(() => {
  const byDay = {}
  for (const r of rows.value) {
    if (startDate.value && r.day < startDate.value) continue
    if (endDate.value && r.day > endDate.value) continue
    byDay[r.day] ??= { day: r.day, in: 0, out: 0, proxy_count: 0, peak_conns: 0, seen: new Set() }
    byDay[r.day].in += Number(r.in || 0)
    byDay[r.day].out += Number(r.out || 0)
    byDay[r.day].peak_conns += Number(r.peak_conns || 0)
    const key = `${r.name}:${r.type}`
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

const topContrib = computed(() => {
  const m = {}
  for (const r of rows.value) {
    if (startDate.value && r.day < startDate.value) continue
    if (endDate.value && r.day > endDate.value) continue
    const key = `${r.name}:${r.type}`
    m[key] ??= { name: r.name, type: r.type, total: 0 }
    m[key].total += Number(r.in || 0) + Number(r.out || 0)
  }
  return Object.values(m).sort((a, b) => b.total - a.total).slice(0, 8)
})
const topTotal = computed(() => topContrib.value[0]?.total || 0)
const proxyPages = computed(() => {
  const map = new Map()
  for (const p of (props.status?.proxies ?? [])) {
    const k = `${p.name}:${p.type}`
    if (!map.has(k)) map.set(k, { name: p.name, type: p.type })
  }
  return [...map.values()].sort((a, b) => a.name.localeCompare(b.name))
})

function goProxyStats(name) {
  router.push(`/statistics/${encodeURIComponent(name)}`)
}

function quickRange(days) {
  const end = new Date()
  const start = new Date(Date.now() - (days - 1) * 86400000)
  endDate.value = end.toISOString().slice(0, 10)
  startDate.value = start.toISOString().slice(0, 10)
}

function quickThisMonth() {
  const now = new Date()
  const start = new Date(now.getFullYear(), now.getMonth(), 1)
  startDate.value = start.toISOString().slice(0, 10)
  endDate.value = now.toISOString().slice(0, 10)
}

function resetFilter() {
  startDate.value = ''
  endDate.value = ''
}

function buildChart() {
  if (!chart) return
  const days = [...filteredRows.value].sort((a, b) => a.day.localeCompare(b.day))
  if (!days.length) {
    chart.clear()
    return
  }
  const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches
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
    grid: { left: 56, right: 20, top: 38, bottom: 38 },
    xAxis: { type: 'category', data: days.map(d => d.day), axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11 } },
    yAxis: { type: 'value', axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11, formatter: v => humanBytes(v) }, splitLine: { lineStyle: { color: isDark ? '#1e293b' : '#f1f5f9' } } },
    series: [
      { name: '上行', type: 'line', smooth: true, data: days.map(d => d.in), itemStyle: { color: '#3b82f6' }, areaStyle: { color: 'rgba(59,130,246,.1)' } },
      { name: '下行', type: 'line', smooth: true, data: days.map(d => d.out), itemStyle: { color: '#10b981' }, areaStyle: { color: 'rgba(16,185,129,.1)' } }
    ]
  })
}

watch(filteredRows, buildChart)
const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart?.resize()) : null
onMounted(async () => {
  await nextTick()
  chart = echarts.init(chartEl.value, window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : null)
  buildChart()
  ro?.observe(chartEl.value)
})
onUnmounted(() => {
  ro?.disconnect()
  chart?.dispose()
})
</script>

<style scoped>
.stats-overview-fill { width: 72%; }
.stats-filter-grid { grid-template-columns: 150px 150px minmax(180px, 1fr) auto auto auto; }
.entry-list-bars { margin-top: 8px; display: grid; gap: 10px; }
.entry-item {
  display: grid;
  gap: 8px;
  border: 1px solid var(--border); border-radius: var(--r-sm);
  background: var(--surface-2); padding: 8px 10px; cursor: pointer; color: var(--text);
  text-align: left;
}
.entry-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.entry-bar {
  height: 8px;
  border-radius: 999px;
  background: var(--border);
  overflow: hidden;
}
.entry-bar-inner {
  height: 100%;
  background: var(--primary);
  border-radius: 999px;
}
.entry-item:hover { border-color: var(--primary); }
.proxy-page-grid {
  margin-top: 8px;
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}
.proxy-page-entry {
  min-height: 66px;
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
.page-num.active {
  color: #1d4ed8;
  border-color: #bfdbfe;
  background: #eff6ff;
}
@media (max-width: 1200px) {
  .stats-filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .proxy-page-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 760px) {
  .proxy-page-grid { grid-template-columns: 1fr; }
}
</style>
