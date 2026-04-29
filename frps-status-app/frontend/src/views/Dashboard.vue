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

    <div class="page-body dashboard-page">
      <div class="dashboard-kpis">
        <section class="kpi-card kpi-service">
          <div class="kpi-head">
            <div class="kpi-title">FRPS 服务</div>
            <span class="status-pill" :class="bindOk ? 'ok' : 'bad'">{{ bindOk ? '在线' : '离线' }}</span>
          </div>
          <div class="kpi-value">{{ bindLatency }} <span>ms</span></div>
          <div class="kpi-sub">服务端口 {{ status?.frps?.bind_port }} · Dashboard {{ dashOk ? dashLatency + 'ms' : '离线' }}</div>
          <div class="kpi-bar">
            <div class="kpi-bar-inner" :style="{ width: bindOk ? '30%' : '100%' }"></div>
          </div>
        </section>

        <section class="kpi-card">
          <div class="kpi-title">本月上行</div>
          <div class="kpi-value">{{ humanBytes(status?.month_totals?.in) }}</div>
          <div class="kpi-sub">阈值 {{ alertInGB || '未设置' }} GB · 已用 {{ inPct }}%</div>
          <div class="kpi-bar">
            <div class="kpi-bar-inner" :class="pctClass(inPct)" :style="{ width: inPct + '%' }"></div>
          </div>
        </section>

        <section class="kpi-card">
          <div class="kpi-title">本月下行</div>
          <div class="kpi-value">{{ humanBytes(status?.month_totals?.out) }}</div>
          <div class="kpi-sub">阈值 {{ alertOutGB || '未设置' }} GB · 已用 {{ outPct }}%</div>
          <div class="kpi-bar">
            <div class="kpi-bar-inner" :class="pctClass(outPct)" :style="{ width: outPct + '%' }"></div>
          </div>
        </section>

        <section class="kpi-card">
          <div class="kpi-title">代理在线</div>
          <div class="kpi-value">{{ onlineProxies }} <span>/ {{ totalProxies }}</span></div>
          <div class="kpi-sub">TCP {{ proxyTypeMap.tcp || 0 }} · HTTP {{ proxyTypeMap.http || 0 }} · HTTPS {{ proxyTypeMap.https || 0 }}</div>
          <div class="status-compact">{{ totalProxies ? Math.round((onlineProxies / totalProxies) * 100) : 0 }}%</div>
        </section>
      </div>

      <div class="dashboard-middle">
        <section class="card-area chart-block">
          <div class="card-head">
            <div class="section-title">本月流量趋势</div>
          </div>
          <div ref="chartEl" class="trend-chart"></div>
        </section>

        <section class="card-area cert-risk-block">
          <div class="card-head">
            <div class="section-title">证书风险</div>
          </div>
          <div class="risk-ring-wrap">
            <div class="risk-ring">
              <div class="risk-ring-core">
                <div class="risk-label">最快过期</div>
                <div class="risk-days" :style="{ color: certColor(certSummary.min_days_left) }">
                  {{ certSummary.min_days_left ?? '-' }}<span v-if="certSummary.min_days_left != null"> 天</span>
                </div>
              </div>
            </div>
          </div>
          <div class="risk-row">
            <span class="badge badge-warn">WARN {{ certSummary.warn || 0 }}</span>
            <span class="badge badge-ok">OK {{ certSummary.ok || 0 }}</span>
            <span class="badge badge-bad">FAIL {{ certSummary.fail || 0 }}</span>
          </div>
          <div class="risk-domain mono">{{ certSummary.min_domain || '未配置证书域名' }}</div>
        </section>
      </div>

      <div class="dashboard-bottom">
        <section class="card-area top5-block">
          <div class="card-head">
            <div class="section-title">本月总流量 Top 5</div>
            <span class="chip">表内滚动</span>
          </div>
          <div class="top5-scroll">
            <table class="top5-table">
              <thead>
                <tr>
                  <th class="col-rank">排名</th>
                  <th class="col-name">代理名称</th>
                  <th class="col-type">类型</th>
                  <th class="col-num">上行</th>
                  <th class="col-num">下行</th>
                  <th class="col-num">总流量</th>
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

        <section class="card-area cert-list-block">
          <div class="card-head">
            <div class="section-title">证书列表</div>
            <span class="chip">{{ certs.length }} 条</span>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>域名</th><th>状态</th><th class="col-num">剩余</th><th>到期时间</th></tr></thead>
              <tbody>
                <tr v-if="!pagedCerts.length"><td colspan="4" class="empty">暂无证书</td></tr>
                <tr v-for="c in pagedCerts" :key="c.domain">
                  <td><code>{{ c.domain }}</code></td>
                  <td>
                    <span class="badge" :class="certBadge(c)">
                      {{ certBadgeText(c) }}
                    </span>
                  </td>
                  <td class="col-num" :style="{ color: certColor(c.days_left) }">{{ c.days_left ?? '-' }}</td>
                  <td>{{ fmtDate(c.expires_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="certPages > 1" class="pager">
            <button class="btn btn-outline btn-sm" :disabled="certPage <= 1" @click="certPage--">上一页</button>
            <span class="text-muted text-sm">{{ certPage }} / {{ certPages }}</span>
            <button class="btn btn-outline btn-sm" :disabled="certPage >= certPages" @click="certPage++">下一页</button>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
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

const proxies = computed(() => props.status?.proxies ?? [])
const onlineProxies = computed(() => proxies.value.filter(p => p.online).length)
const totalProxies = computed(() => proxies.value.length)
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

const certPage = ref(1)
const CERT_PAGE_SIZE = 4
const certPages = computed(() => Math.max(1, Math.ceil(certs.value.length / CERT_PAGE_SIZE)))
const pagedCerts = computed(() => {
  const start = (certPage.value - 1) * CERT_PAGE_SIZE
  return certs.value.slice(start, start + CERT_PAGE_SIZE)
})
watch(certs, () => { certPage.value = 1 })

function certColor(days) {
  if (days == null) return 'var(--danger)'
  if (days < 7) return 'var(--danger)'
  if (days < 15) return 'var(--warning)'
  return 'var(--success)'
}

function certBadge(c) {
  if (!c.ok) return 'badge-bad'
  if (c.days_left != null && c.days_left < 15) return 'badge-warn'
  return 'badge-ok'
}

function certBadgeText(c) {
  if (!c.ok) return 'FAIL'
  if (c.days_left != null && c.days_left < 15) return 'WARN'
  return 'OK'
}

function pctClass(pct) {
  if (pct >= 100) return 'bad'
  if (pct >= 80) return 'warn'
  return 'ok'
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

<style scoped>
.dashboard-page {
  display: grid;
  gap: 14px;
}
.dashboard-kpis {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}
.kpi-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r);
  padding: 13px 14px;
  box-shadow: var(--shadow);
  position: relative;
  min-height: 142px;
}
.kpi-service {
  background: linear-gradient(130deg, #eff6ff, #ecfdf5);
}
.kpi-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.kpi-title {
  font-size: 13px;
  font-weight: 650;
  color: var(--text-2);
}
.status-pill {
  font-size: 11px;
  padding: 3px 8px;
  border-radius: 999px;
}
.status-pill.ok {
  color: #166534;
  background: #dcfce7;
}
.status-pill.bad {
  color: #991b1b;
  background: #fee2e2;
}
.kpi-value {
  margin-top: 7px;
  font-size: 24px;
  font-weight: 700;
  line-height: 1.1;
}
.kpi-value span {
  font-size: 14px;
  color: var(--text-2);
}
.kpi-sub {
  margin-top: 7px;
  color: var(--text-2);
  font-size: 12px;
}
.kpi-bar {
  margin-top: 10px;
  background: #e2e8f0;
  height: 8px;
  border-radius: 8px;
  overflow: hidden;
}
.kpi-bar-inner {
  height: 100%;
  border-radius: 8px;
  width: 0;
  background: var(--success);
}
.kpi-bar-inner.warn { background: var(--warning); }
.kpi-bar-inner.bad { background: var(--danger); }
.status-compact {
  position: absolute;
  right: 14px;
  bottom: 14px;
  font-size: 12px;
  background: #ecfdf5;
  color: #047857;
  border-radius: 999px;
  padding: 4px 10px;
}
.dashboard-middle {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 10px;
}
.card-area {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r);
  box-shadow: var(--shadow);
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
  height: 238px;
  padding: 0 10px 6px;
}
.cert-risk-block {
  padding: 0 14px 12px;
}
.risk-ring-wrap {
  display: flex;
  justify-content: center;
  margin-top: 8px;
}
.risk-ring {
  width: 156px;
  height: 156px;
  border-radius: 50%;
  border: 11px solid #e2e8f0;
  border-top-color: #f59e0b;
  border-right-color: #f59e0b;
  border-bottom-color: #16a34a;
  display: grid;
  place-items: center;
}
.risk-ring-core {
  text-align: center;
}
.risk-label {
  color: var(--text-2);
  font-size: 12px;
}
.risk-days {
  margin-top: 4px;
  font-size: 25px;
  font-weight: 750;
}
.risk-days span {
  font-size: 13px;
}
.risk-row {
  margin-top: 8px;
  display: flex;
  gap: 8px;
  justify-content: center;
}
.risk-domain {
  margin-top: 10px;
  text-align: center;
  font-size: 12px;
  color: var(--text-2);
}
.dashboard-bottom {
  display: grid;
  grid-template-columns: 1.35fr 1fr;
  gap: 10px;
}
.top5-block,
.cert-list-block {
  display: grid;
  grid-template-rows: auto 1fr auto;
  min-height: 296px;
}
.chip {
  font-size: 11px;
  color: var(--text-2);
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 999px;
  padding: 2px 8px;
}
.top5-scroll {
  max-height: 230px;
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
  padding: 10px 8px;
  border-bottom: 1px solid var(--border);
  font-size: 13px;
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
.table-wrap table {
  table-layout: fixed;
}
.table-wrap th,
.table-wrap td {
  padding: 10px 8px;
  border-bottom: 1px solid var(--border);
  font-size: 13px;
}
.table-wrap th {
  font-size: 12px;
  font-weight: 650;
  color: var(--text-2);
}
.table-wrap thead th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--surface);
}
.table-wrap tbody tr:hover td,
.top5-table tbody tr:hover td {
  background: var(--surface-2);
}
.col-rank { width: 60px; }
.col-type { width: 78px; }
.col-name { min-width: 150px; }
.col-num { width: 112px; text-align: right !important; }
.pager {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 8px;
  padding: 10px 0 12px;
}
@media (max-width: 1200px) {
  .dashboard-kpis { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .dashboard-middle,
  .dashboard-bottom { grid-template-columns: 1fr; }
}
</style>
