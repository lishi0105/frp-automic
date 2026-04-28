<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">历史统计</div>
        <div class="page-sub">最近 30 天流量数据</div>
      </div>
      <div style="display:flex;gap:8px;align-items:center">
        <a class="btn btn-outline btn-sm" :href="exportUrl">⬇ 导出 CSV</a>
        <button class="btn btn-outline btn-sm" :disabled="loading" @click="$emit('refresh')">
          <span v-if="loading" class="spinner"></span>
          <span v-else>↻</span> 刷新
        </button>
      </div>
    </div>

    <div class="page-body">
      <!-- 筛选 -->
      <div class="card section" style="padding:16px">
        <div style="display:flex;gap:12px;flex-wrap:wrap;align-items:flex-end">
          <div class="form-item">
            <label class="form-label">代理名称</label>
            <select class="form-select" v-model="filterProxy">
              <option value="">全部</option>
              <option v-for="n in proxyNames" :key="n" :value="n">{{ n }}</option>
            </select>
          </div>
          <div class="form-item">
            <label class="form-label">开始日期</label>
            <input class="form-input" type="date" v-model="startDate" />
          </div>
          <div class="form-item">
            <label class="form-label">结束日期</label>
            <input class="form-input" type="date" v-model="endDate" />
          </div>
          <button class="btn btn-outline btn-sm" @click="resetFilter">重置</button>
        </div>
      </div>

      <!-- 本月 Top 5 -->
      <div class="section">
        <div class="section-head">
          <div class="section-title">本月流量 Top 5</div>
          <span class="text-muted text-sm">{{ currentMonth }}</span>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>排名</th><th>代理名称</th><th>本月上行</th><th>本月下行</th><th>合计</th></tr></thead>
            <tbody>
              <tr v-if="!top5.length"><td colspan="5" class="empty">暂无数据</td></tr>
              <tr v-for="(p, i) in top5" :key="p.name">
                <td>
                  <span class="badge" :class="i === 0 ? 'badge-bad' : i === 1 ? 'badge-warn' : 'badge-ok'">
                    #{{ i + 1 }}
                  </span>
                </td>
                <td><code>{{ p.name }}</code></td>
                <td>{{ humanBytes(p.in) }}</td>
                <td>{{ humanBytes(p.out) }}</td>
                <td><b>{{ humanBytes(p.in + p.out) }}</b></td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- 趋势图 -->
      <div class="chart-card section">
        <div class="chart-title">
          {{ filterProxy ? filterProxy + ' · ' : '' }}流量趋势
        </div>
        <div ref="chartEl" style="height:260px"></div>
      </div>

      <!-- 明细表格 -->
      <div class="section">
        <div class="section-head">
          <div class="section-title">每日明细</div>
          <span class="text-muted text-sm">{{ filteredRows.length }} 条记录</span>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>日期</th><th>代理名称</th><th>类型</th><th>上行</th><th>下行</th></tr></thead>
            <tbody>
              <tr v-if="!filteredRows.length"><td colspan="5" class="empty">暂无数据</td></tr>
              <tr v-for="(r, i) in pagedRows" :key="i">
                <td>{{ r.day }}</td>
                <td><code>{{ r.name }}</code></td>
                <td>{{ r.type }}</td>
                <td>{{ humanBytes(r.in) }}</td>
                <td>{{ humanBytes(r.out) }}</td>
              </tr>
            </tbody>
          </table>
          <div v-if="totalPages > 1" style="display:flex;gap:8px;justify-content:center;padding:12px">
            <button class="btn btn-outline btn-sm" :disabled="page <= 1" @click="page--">上一页</button>
            <span class="text-muted text-sm" style="line-height:28px">{{ page }} / {{ totalPages }}</span>
            <button class="btn btn-outline btn-sm" :disabled="page >= totalPages" @click="page++">下一页</button>
          </div>
        </div>
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
defineEmits(['refresh'])

const route = useRoute()
const filterProxy = ref(route.query.proxy || '')
const startDate = ref('')
const endDate = ref('')
const page = ref(1)
const PAGE_SIZE = 20

const exportUrl = api.exportCSVUrl
const currentMonth = new Date().toISOString().slice(0, 7)

const chartEl = ref(null)
let chart = null

const proxyNames = computed(() => [...new Set((props.daily ?? []).map(r => r.name))].sort())

const filteredRows = computed(() => {
  return (props.daily ?? []).filter(r => {
    if (filterProxy.value && r.name !== filterProxy.value) return false
    if (startDate.value && r.day < startDate.value) return false
    if (endDate.value && r.day > endDate.value) return false
    return true
  }).sort((a, b) => b.day.localeCompare(a.day) || a.name.localeCompare(b.name))
})

const totalPages = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / PAGE_SIZE)))
const pagedRows = computed(() => {
  const s = (page.value - 1) * PAGE_SIZE
  return filteredRows.value.slice(s, s + PAGE_SIZE)
})

watch(filteredRows, () => { page.value = 1 })

// Top 5 for current month
const top5 = computed(() => {
  const map = {}
  for (const r of props.daily ?? []) {
    if (!r.day.startsWith(currentMonth)) continue
    map[r.name] ??= { name: r.name, in: 0, out: 0 }
    map[r.name].in += Number(r.in)
    map[r.name].out += Number(r.out)
  }
  return Object.values(map).sort((a, b) => (b.in + b.out) - (a.in + a.out)).slice(0, 5)
})

function resetFilter() {
  filterProxy.value = ''
  startDate.value = ''
  endDate.value = ''
}

function buildChart(rows) {
  if (!chart) return
  const byDay = {}
  for (const r of rows) {
    byDay[r.day] ??= { in: 0, out: 0 }
    byDay[r.day].in += Number(r.in)
    byDay[r.day].out += Number(r.out)
  }
  const days = Object.keys(byDay).sort()
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
    grid: { left: 60, right: 20, top: 36, bottom: 50 },
    dataZoom: [{ type: 'inside' }, { type: 'slider', height: 20, bottom: 4 }],
    xAxis: { type: 'category', data: days, axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11 } },
    yAxis: { type: 'value', axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11, formatter: v => humanBytes(v) }, splitLine: { lineStyle: { color: isDark ? '#1e293b' : '#f1f5f9' } } },
    series: [
      { name: '上行', type: 'bar', stack: 'total', data: days.map(d => byDay[d].in), itemStyle: { color: '#3b82f6' } },
      { name: '下行', type: 'bar', stack: 'total', data: days.map(d => byDay[d].out), itemStyle: { color: '#10b981' } }
    ]
  })
}

watch(filteredRows, (rows) => buildChart(rows))

const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart?.resize()) : null

onMounted(async () => {
  await nextTick()
  const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  chart = echarts.init(chartEl.value, isDark ? 'dark' : null)
  if (filteredRows.value.length) buildChart(filteredRows.value)
  ro?.observe(chartEl.value)
})

onUnmounted(() => {
  ro?.disconnect()
  chart?.dispose()
})
</script>
