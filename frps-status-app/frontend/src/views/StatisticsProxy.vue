<template>
  <div class="proxy-stats-shell">
    <div class="page-header">
      <div>
        <div class="page-title">代理统计详情</div>
        <div class="page-sub">当前代理：{{ proxyName }} · {{ proxyType || '-' }} · {{ proxyOnline ? '在线' : '离线' }}</div>
      </div>
      <div class="flex-center">
        <button class="btn btn-outline btn-sm icon-btn" title="返回总览" aria-label="返回总览" @click="router.push('/statistics')">←</button>
        <button class="btn btn-outline btn-sm icon-btn" title="刷新" aria-label="刷新" :disabled="loading" @click="$emit('refresh')">
          <span class="refresh-glyph" :class="{ 'is-spinning': loading }" aria-hidden="true">↻</span>
        </button>
      </div>
    </div>

    <div class="page-body analytics-page proxy-stats-page">
      <section class="analytics-overview">
        <div>
          <div class="section-title">所选时间范围</div>
          <div class="text-muted text-sm">{{ startDate || minDay || '-' }} 至 {{ endDate || maxDay || '-' }} · {{ proxyName }}</div>
        </div>
        <div class="analytics-overview-metrics">
          <div><b>{{ humanBytes(totalTraffic) }}</b><small>总流量</small></div>
          <div><b>{{ humanBytes(totalIn) }}</b><small>入站</small></div>
          <div><b>{{ humanBytes(totalOut) }}</b><small>出站</small></div>
        </div>
      </section>

      <section class="analytics-filters">
        <div class="analytics-filter-grid proxy-filter-grid">
          <label class="date-filter-field">
            <span>开始日期</span>
            <input class="form-input" type="date" v-model="startDate" />
          </label>
          <label class="date-filter-field">
            <span>结束日期</span>
            <input class="form-input" type="date" v-model="endDate" />
          </label>
          <button class="btn btn-outline btn-sm" @click="quickRange(60)">最近60天</button>
          <button class="btn btn-outline btn-sm" @click="resetFilter">重置</button>
        </div>
      </section>

      <div class="analytics-main-2col proxy-main">
        <section class="card proxy-chart-card">
          <div class="section-head"><div class="section-title">代理流量趋势</div></div>
          <div ref="chartEl" class="analytics-trend-chart"></div>
        </section>
        <section class="card proxy-table-card">
          <div class="section-head">
            <div class="section-title">代理每日明细</div>
            <span class="text-muted text-sm">{{ filteredRows.length }} 条记录</span>
          </div>
          <div class="table-wrap proxy-table-scroll">
            <table class="proxy-detail-table">
              <thead><tr><th class="col-date">日期</th><th>类型</th><th class="col-num">连接峰值</th><th class="col-num">入站</th><th class="col-num">出站</th><th class="col-num">合计</th></tr></thead>
              <tbody>
                <tr v-if="!pagedRows.length"><td colspan="6" class="empty">暂无数据</td></tr>
                <tr v-for="r in pagedRows" :key="r.day + r.type">
                  <td class="col-date">{{ r.day }}</td>
                  <td><span class="badge badge-ok">{{ r.type }}</span></td>
                  <td class="col-num">{{ r.peak_conns || 0 }}</td>
                  <td class="col-num">{{ humanBytes(r.in) }}</td>
                  <td class="col-num">{{ humanBytes(r.out) }}</td>
                  <td class="col-num"><b>{{ humanBytes(r.in + r.out) }}</b></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="totalPages > 1" class="analytics-pager proxy-pager">
            <button class="btn btn-outline btn-sm" :disabled="page <= 1" @click="page--">上一页</button>
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
import { useRoute, useRouter } from 'vue-router'
import * as echarts from 'echarts'
import { humanBytes } from '../utils/format.js'

const props = defineProps({ status: Object, daily: Array, loading: Boolean })
defineEmits(['refresh'])
const route = useRoute()
const router = useRouter()

const proxyName = computed(() => decodeURIComponent(route.params.proxyName || ''))
const allRows = computed(() => props.daily ?? [])
const proxyRows = computed(() => allRows.value.filter(r => r.name === proxyName.value))
const proxyInfo = computed(() => (props.status?.proxies ?? []).find(p => p.name === proxyName.value) || null)
const proxyType = computed(() => proxyInfo.value?.type || proxyRows.value[0]?.type || '')
const proxyOnline = computed(() => proxyInfo.value?.online ?? false)
const minDay = computed(() => proxyRows.value.length ? [...proxyRows.value].sort((a, b) => a.day.localeCompare(b.day))[0].day : '')
const maxDay = computed(() => proxyRows.value.length ? [...proxyRows.value].sort((a, b) => b.day.localeCompare(a.day))[0].day : '')

const startDate = ref('')
const endDate = ref('')
const page = ref(1)
const PAGE_SIZE = 15

const filteredRows = computed(() => proxyRows.value.filter(r => {
  if (startDate.value && r.day < startDate.value) return false
  if (endDate.value && r.day > endDate.value) return false
  return true
}).sort((a, b) => b.day.localeCompare(a.day)))

const totalIn = computed(() => filteredRows.value.reduce((s, r) => s + Number(r.in || 0), 0))
const totalOut = computed(() => filteredRows.value.reduce((s, r) => s + Number(r.out || 0), 0))
const totalTraffic = computed(() => totalIn.value + totalOut.value)

const totalPages = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / PAGE_SIZE)))
const pagedRows = computed(() => {
  const s = (page.value - 1) * PAGE_SIZE
  return filteredRows.value.slice(s, s + PAGE_SIZE)
})
watch(filteredRows, () => { page.value = 1 })
watch([proxyName, minDay, maxDay], ([currentName, min, max], [oldName] = []) => {
  const proxyChanged = oldName !== undefined && currentName !== oldName
  if (proxyChanged || !startDate.value) startDate.value = min || ''
  if (proxyChanged || !endDate.value) endDate.value = max || ''
}, { immediate: true })

function quickRange(days) {
  const end = new Date()
  const start = new Date(Date.now() - (days - 1) * 86400000)
  endDate.value = end.toISOString().slice(0, 10)
  startDate.value = start.toISOString().slice(0, 10)
}
function resetFilter() {
  startDate.value = minDay.value || ''
  endDate.value = maxDay.value || ''
}

const chartEl = ref(null)
let chart = null
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
      { name: '入站', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => Number(d.in || 0)), itemStyle: { color: '#1f7ae0' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(31,122,224,.08)' } },
      { name: '出站', type: 'line', smooth: false, symbolSize: 8, data: days.map(d => Number(d.out || 0)), itemStyle: { color: '#12b76a' }, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(18,183,106,.15)' } }
    ]
  })
}

watch(filteredRows, buildChart)
const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart?.resize()) : null
onMounted(async () => {
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
.proxy-stats-shell {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}
.proxy-stats-page {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}
.proxy-filter-grid { grid-template-columns: repeat(4, minmax(0, auto)); }
.date-filter-field {
  display: grid;
  gap: 5px;
  min-width: 0;
}
.date-filter-field span {
  color: var(--text-2);
  font-size: 12px;
  font-weight: 600;
}
.proxy-main {
  flex: 1;
  grid-template-columns: 3fr 2fr;
  grid-template-rows: 1fr;
  min-height: 0;
  overflow: hidden;
}
.proxy-chart-card {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.proxy-chart-card .analytics-trend-chart {
  flex: 1;
  min-height: 0;
  height: auto;
}
.proxy-table-card {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  min-height: 0;
}
.proxy-table-scroll {
  min-height: 0;
  overflow: auto;
}
.proxy-detail-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
.proxy-detail-table th,
.proxy-detail-table td {
  border-bottom: 1px solid var(--border);
  padding: 8px;
  text-align: left;
}
.proxy-detail-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--surface);
}
.proxy-detail-table .col-date {
  width: 92px;
}
.proxy-detail-table .col-num {
  width: 78px;
  text-align: right;
}
.proxy-pager {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
  padding-top: 10px;
}
@media (max-width: 1200px) {
  .proxy-stats-page {
    flex: none;
    min-height: auto;
    overflow: visible;
  }
  .proxy-main {
    grid-template-columns: 1fr;
    overflow: visible;
  }
  .proxy-chart-card .analytics-trend-chart {
    flex: none;
    height: 300px;
  }
  .proxy-table-scroll {
    max-height: 480px;
  }
  .proxy-filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
