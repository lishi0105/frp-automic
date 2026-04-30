<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">代理统计详情</div>
        <div class="page-sub">当前代理：{{ proxyName }}</div>
      </div>
      <div class="flex-center">
        <button class="btn btn-outline btn-sm" @click="router.push('/statistics')">返回总览</button>
        <button class="btn btn-outline btn-sm" :disabled="loading" @click="$emit('refresh')">
          <span v-if="loading" class="spinner"></span>
          <span v-else>↻</span> 刷新
        </button>
      </div>
    </div>

    <div class="page-body analytics-page">
      <section class="analytics-overview">
        <div>
          <div class="section-title">{{ proxyName }}</div>
          <div class="text-muted text-sm">{{ proxyType || '-' }} · {{ proxyOnline ? '在线' : '离线' }}</div>
          <div class="analytics-overview-bar"><div class="analytics-overview-bar-inner proxy-overview-fill"></div></div>
        </div>
        <div class="analytics-overview-metrics">
          <div><b>{{ humanBytes(totalTraffic) }}</b><small>总流量</small></div>
          <div><b>{{ humanBytes(totalIn) }}</b><small>上行</small></div>
          <div><b>{{ humanBytes(totalOut) }}</b><small>下行</small></div>
        </div>
      </section>

      <section class="analytics-filters">
        <div class="analytics-filter-grid proxy-filter-grid">
          <input class="form-input" type="date" v-model="startDate" />
          <input class="form-input" type="date" v-model="endDate" />
          <button class="btn btn-outline btn-sm" @click="quickRange(60)">最近60天</button>
          <button class="btn btn-outline btn-sm" @click="resetFilter">重置</button>
        </div>
      </section>

      <div class="analytics-main-2col">
        <section class="card">
          <div class="section-head"><div class="section-title">代理流量趋势</div></div>
          <div ref="chartEl" class="analytics-trend-chart"></div>
        </section>
        <section class="card analytics-side-kv">
          <div class="section-head"><div class="section-title">代理详情</div></div>
          <div class="kv"><span>当前连接</span><b>{{ proxyCurConns }}</b></div>   
          <div class="kv"><span>日峰值连接</span><b>{{ peakConns }}</b></div>
          <div class="kv"><span>上行占比</span><b>{{ ratioIn }}%</b></div>
          <div class="kv"><span>下行占比</span><b>{{ ratioOut }}%</b></div>
          <div class="kv"><span>关联证书</span><b>{{ certDomain || '-' }}</b></div>
          <div class="kv"><span>证书剩余</span><b :style="{ color: certColor(certDaysLeft) }">{{ certDaysLeft != null ? certDaysLeft + ' 天' : '-' }}</b></div>
        </section>
      </div>

      <section class="card">
        <div class="section-head">
          <div class="section-title">当前代理每日明细</div>
          <span class="text-muted text-sm">{{ filteredRows.length }} 条记录</span>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>日期</th><th>类型</th><th>连接峰值</th><th>上行</th><th>下行</th><th>合计</th></tr></thead>
            <tbody>
              <tr v-if="!pagedRows.length"><td colspan="6" class="empty">暂无数据</td></tr>
              <tr v-for="r in pagedRows" :key="r.day + r.type">
                <td>{{ r.day }}</td>
                <td><span class="badge badge-ok">{{ r.type }}</span></td>
                <td>{{ r.peak_conns || 0 }}</td>
                <td>{{ humanBytes(r.in) }}</td>
                <td>{{ humanBytes(r.out) }}</td>
                <td><b>{{ humanBytes(r.in + r.out) }}</b></td>
              </tr>
            </tbody>
          </table>
          <div v-if="totalPages > 1" class="analytics-pager">
            <button class="btn btn-outline btn-sm" :disabled="page <= 1" @click="page--">上一页</button>
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
const proxyCurConns = computed(() => proxyInfo.value?.cur_conns ?? 0)

const certByDomain = computed(() => {
  const certs = props.status?.certificates ?? []
  const domains = (proxyInfo.value?.domains ?? []).filter(Boolean)
  if (domains.length) {
    const exact = certs.find(c => domains.includes(c.domain))
    if (exact) return exact
  }
  return certs.find(c => c.domain === proxyName.value || c.domain?.includes(proxyName.value)) || null
})
const certDomain = computed(() => certByDomain.value?.domain || '')
const certDaysLeft = computed(() => certByDomain.value?.days_left ?? null)

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
const peakConns = computed(() => filteredRows.value.reduce((m, r) => Math.max(m, Number(r.peak_conns || 0)), 0))
const ratioIn = computed(() => totalTraffic.value ? ((totalIn.value / totalTraffic.value) * 100).toFixed(1) : '0.0')
const ratioOut = computed(() => totalTraffic.value ? ((totalOut.value / totalTraffic.value) * 100).toFixed(1) : '0.0')

const totalPages = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / PAGE_SIZE)))
const pagedRows = computed(() => {
  const s = (page.value - 1) * PAGE_SIZE
  return filteredRows.value.slice(s, s + PAGE_SIZE)
})
watch(filteredRows, () => { page.value = 1 })

function certColor(days) {
  if (days == null) return 'var(--text-2)'
  if (days < 7) return 'var(--danger)'
  if (days < 15) return 'var(--warning)'
  return 'var(--success)'
}

function quickRange(days) {
  const end = new Date()
  const start = new Date(Date.now() - (days - 1) * 86400000)
  endDate.value = end.toISOString().slice(0, 10)
  startDate.value = start.toISOString().slice(0, 10)
}
function resetFilter() {
  startDate.value = ''
  endDate.value = ''
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
  const isDark = true
  chart.setOption({
    backgroundColor: 'transparent',
    tooltip: { trigger: 'axis', formatter: params => {
      const d = params[0]?.axisValue || ''
      return `${d}<br/>${params.map(p => `${p.marker}${p.seriesName}: <b>${humanBytes(p.value)}</b>`).join('<br/>')}`
    }},
    legend: { data: ['上行', '下行'], top: 0, textStyle: { color: isDark ? '#94a3b8' : '#475569' } },
    grid: { left: 56, right: 20, top: 38, bottom: 38 },
    xAxis: { type: 'category', data: days.map(d => d.day), axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11 } },
    yAxis: { type: 'value', axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11, formatter: v => humanBytes(v) }, splitLine: { lineStyle: { color: isDark ? '#1e293b' : '#f1f5f9' } } },
    series: [
      { name: '上行', type: 'line', smooth: true, data: days.map(d => Number(d.in || 0)), itemStyle: { color: '#3b82f6' }, areaStyle: { color: 'rgba(59,130,246,.1)' } },
      { name: '下行', type: 'line', smooth: true, data: days.map(d => Number(d.out || 0)), itemStyle: { color: '#10b981' }, areaStyle: { color: 'rgba(16,185,129,.1)' } }
    ]
  })
}

watch(filteredRows, buildChart)
const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart?.resize()) : null
onMounted(async () => {
  await nextTick()
  chart = echarts.init(chartEl.value, 'dark')
  buildChart()
  ro?.observe(chartEl.value)
})
onUnmounted(() => {
  ro?.disconnect()
  chart?.dispose()
})
</script>

<style scoped>
.proxy-overview-fill { width: 70%; }
.proxy-filter-grid { grid-template-columns: repeat(4, minmax(0, auto)); }
@media (max-width: 1200px) {
  .proxy-filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
