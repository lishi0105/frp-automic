<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">数据看板</div>
        <div class="page-sub">{{ updatedAt ? '最后更新：' + updatedAt : '加载中…' }}</div>
      </div>
      <button class="btn btn-outline btn-sm" :disabled="loading" @click="$emit('refresh')">
        <span v-if="loading" class="spinner"></span>
        <span v-else>↻</span> 刷新
      </button>
    </div>

    <div class="page-body">
      <!-- 状态卡片 -->
      <div class="stat-grid">
        <!-- FRPS 服务状态 -->
        <div class="stat-card">
          <div class="card-title">FRPS 服务</div>
          <div class="flex-center" style="gap:8px;margin-top:8px">
            <span class="dot" :class="bindOk ? 'ok' : 'bad'"></span>
            <span class="stat-value" style="font-size:18px">{{ bindOk ? '在线' : '离线' }}</span>
          </div>
          <div class="stat-sub">
            服务端口 {{ status?.frps?.bind_port }} · 延迟 {{ bindLatency }}ms
          </div>
          <div class="stat-sub" style="margin-top:3px">
            Dashboard
            <span class="dot" :class="dashOk ? 'ok' : 'bad'" style="margin:0 4px"></span>
            {{ dashOk ? dashLatency + 'ms' : '离线' }}
          </div>
        </div>

        <!-- 月度流量 -->
        <div class="stat-card">
          <div class="card-title">本月流量</div>
          <div style="margin-top:6px">
            <div class="flex-between">
              <span class="text-muted text-sm">上行</span>
              <span style="font-weight:600">{{ humanBytes(status?.month_totals?.in) }}</span>
            </div>
            <div class="progress mt-2">
              <div class="progress-bar" :class="inPct >= 100 ? 'bad' : inPct >= 80 ? 'warn' : 'ok'" :style="{ width: inPct + '%' }"></div>
            </div>
            <div class="text-sm text-muted mt-2" style="margin-top:10px">
              <div class="flex-between">
                <span>下行</span>
                <span style="font-weight:600">{{ humanBytes(status?.month_totals?.out) }}</span>
              </div>
              <div class="progress mt-2">
                <div class="progress-bar" :class="outPct >= 100 ? 'bad' : outPct >= 80 ? 'warn' : 'ok'" :style="{ width: outPct + '%' }"></div>
              </div>
            </div>
            <div class="stat-sub">阈值 {{ alertInGB || '未设置' }} GB / {{ alertOutGB || '未设置' }} GB</div>
          </div>
        </div>

        <!-- 证书预警 -->
        <div class="stat-card">
          <div class="card-title">证书预警</div>
          <template v-if="minCert">
            <div class="stat-value" style="font-size:22px;margin-top:8px">
              <span :style="{ color: certColor(minCert.days_left) }">{{ minCert.days_left }} 天</span>
            </div>
            <div class="stat-sub">{{ minCert.domain }} 最快过期</div>
            <div class="stat-sub">共 {{ certs.length }} 个证书</div>
          </template>
          <div v-else class="stat-sub" style="margin-top:8px">无证书配置</div>
          <RouterLink to="/proxies" class="text-sm" style="display:block;margin-top:8px">查看代理列表 →</RouterLink>
        </div>

        <!-- 代理统计 -->
        <div class="stat-card">
          <div class="card-title">代理统计</div>
          <div class="stat-value" style="margin-top:8px">
            {{ onlineProxies }} <span style="font-size:16px;color:var(--text-2)">/ {{ totalProxies }}</span>
          </div>
          <div class="stat-sub">在线 / 总计</div>
          <div style="margin-top:10px;display:flex;gap:6px;flex-wrap:wrap">
            <span v-for="(count, type) in proxyTypeMap" :key="type" class="badge badge-ok">{{ type }} ×{{ count }}</span>
          </div>
        </div>
      </div>

      <!-- 流量趋势图 -->
      <div class="chart-card section">
        <div class="chart-title">最近 30 天流量趋势</div>
        <div ref="chartEl" style="height:280px"></div>
      </div>

      <!-- 证书列表 -->
      <div class="section" v-if="certs.length">
        <div class="section-head">
          <div class="section-title">SSL 证书</div>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>域名</th><th>状态</th><th>剩余天数</th><th>到期时间</th></tr></thead>
            <tbody>
              <tr v-for="c in certs" :key="c.domain">
                <td><code>{{ c.domain }}</code></td>
                <td>
                  <span class="badge" :class="c.ok ? (c.days_left < 15 ? 'badge-warn' : 'badge-ok') : 'badge-bad'">
                    {{ c.ok ? (c.days_left < 15 ? 'WARN' : 'OK') : 'FAIL' }}
                  </span>
                </td>
                <td :style="{ color: certColor(c.days_left) }">{{ c.days_left ?? '-' }}</td>
                <td>{{ fmtDate(c.expires_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { RouterLink } from 'vue-router'
import * as echarts from 'echarts'
import { humanBytes, percent, fmtDate } from '../utils/format.js'

const props = defineProps({ status: Object, daily: Array, loading: Boolean })
defineEmits(['refresh'])

const chartEl = ref(null)
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

const certs = computed(() => props.status?.certificates ?? [])
const minCert = computed(() => {
  const valid = certs.value.filter(c => c.days_left != null)
  if (!valid.length) return null
  return valid.reduce((a, b) => a.days_left < b.days_left ? a : b)
})

const proxies = computed(() => props.status?.proxies ?? [])
const onlineProxies = computed(() => proxies.value.filter(p => p.online).length)
const totalProxies = computed(() => proxies.value.length)
const proxyTypeMap = computed(() => {
  const m = {}
  for (const p of proxies.value) m[p.type] = (m[p.type] || 0) + 1
  return m
})

function certColor(days) {
  if (days == null) return 'var(--danger)'
  if (days < 7) return 'var(--danger)'
  if (days < 15) return 'var(--warning)'
  return 'var(--success)'
}

function buildChart(daily) {
  if (!chart || !daily?.length) return
  const byDay = {}
  for (const r of daily) {
    byDay[r.day] ??= { in: 0, out: 0 }
    byDay[r.day].in += Number(r.in)
    byDay[r.day].out += Number(r.out)
  }
  const days = Object.keys(byDay).sort().slice(-30)
  const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  chart.setOption({
    backgroundColor: 'transparent',
    textStyle: { color: isDark ? '#94a3b8' : '#475569' },
    tooltip: {
      trigger: 'axis',
      formatter: params => {
        const d = params[0]?.axisValue || ''
        return `${d}<br/>${params.map(p => `${p.marker}${p.seriesName}: <b>${humanBytes(p.value)}</b>`).join('<br/>')}`
      }
    },
    legend: { data: ['上行', '下行'], top: 0, textStyle: { color: isDark ? '#94a3b8' : '#475569' } },
    grid: { left: 60, right: 20, top: 36, bottom: 50 },
    dataZoom: [{ type: 'inside', start: 0, end: 100 }, { type: 'slider', height: 20, bottom: 4 }],
    xAxis: { type: 'category', data: days, axisLine: { lineStyle: { color: isDark ? '#334155' : '#e2e8f0' } }, axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11 } },
    yAxis: { type: 'value', axisLabel: { color: isDark ? '#94a3b8' : '#475569', fontSize: 11, formatter: v => humanBytes(v) }, splitLine: { lineStyle: { color: isDark ? '#1e293b' : '#f1f5f9' } } },
    series: [
      { name: '上行', type: 'line', smooth: true, data: days.map(d => byDay[d].in), itemStyle: { color: '#3b82f6' }, areaStyle: { color: 'rgba(59,130,246,.1)' } },
      { name: '下行', type: 'line', smooth: true, data: days.map(d => byDay[d].out), itemStyle: { color: '#10b981' }, areaStyle: { color: 'rgba(16,185,129,.1)' } }
    ]
  })
}

watch(() => props.daily, (d) => buildChart(d))

const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => chart?.resize()) : null

onMounted(async () => {
  await nextTick()
  const isDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  chart = echarts.init(chartEl.value, isDark ? 'dark' : null)
  if (props.daily?.length) buildChart(props.daily)
  ro?.observe(chartEl.value)
})

onUnmounted(() => {
  ro?.disconnect()
  chart?.dispose()
})
</script>
