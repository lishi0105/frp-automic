<template>
  <div class="proxy-shell">
    <div class="page-header">
      <div>
        <div class="page-title">代理列表</div>
        <div class="page-sub">共 {{ totalItems }} 个代理，{{ onlineCount }} 个在线</div>
      </div>
      <div class="header-side">
        <div class="header-summary">
          <div>
            <b>{{ onlineCount }}</b><span>/ {{ totalProxies }}</span>
            <small>在线代理</small>
          </div>
          <div>
            <b>{{ curConnsTotal }}</b>
            <small>当前连接数</small>
          </div>
          <div>
            <b>{{ humanBytes(monthTotal) }}</b>
            <small>本月总流量</small>
          </div>
        </div>
        <div class="flex-center">
          <button class="btn btn-outline btn-sm icon-btn" title="导出列表" aria-label="导出列表" @click="exportCsv">⇩</button>
          <button class="btn btn-outline btn-sm icon-btn" title="刷新" aria-label="刷新" :disabled="loading || localLoading" @click="$emit('refresh')">
            <span v-if="loading || localLoading" class="spinner"></span>
            <span v-else>↻</span>
          </button>
        </div>
      </div>
    </div>

    <div class="page-body analytics-page proxy-page">
      <div class="proxy-main analytics-main-2col proxy-main-custom">
        <section class="proxy-table-wrap">
          <div class="section-head">
            <div class="section-title">代理清单</div>
            <div class="proxy-table-tools">
              <span class="text-muted text-sm">{{ totalItems }} 条</span>
              <label class="text-muted text-sm">每页
                <select class="form-select page-size" v-model.number="pageSize">
                  <option :value="10">10</option>
                  <option :value="20">20</option>
                  <option :value="30">30</option>
                  <option :value="50">50</option>
                </select>
              </label>
            </div>
          </div>
          <div class="table-wrap table-scroll">
            <table>
              <thead class="sticky-head">
                <tr>
                  <th class="col-index">序号</th>
                  <th class="col-name">名称</th>
                  <th class="col-type">类型</th>
                  <th class="col-status">状态</th>
                  <th class="col-num sortable" @click="toggleSort('conn')">连接 <span :class="sortClass('conn')">↕</span></th>
                  <th class="col-num sortable" @click="toggleSort('in')">本月入站 <span :class="sortClass('in')">↕</span></th>
                  <th class="col-num sortable" @click="toggleSort('out')">本月出站 <span :class="sortClass('out')">↕</span></th>
                  <th class="col-num sortable" @click="toggleSort('total')">总流量 <span :class="sortClass('total')">↕</span></th>
                  <th class="col-action">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="!pagedRows.length"><td colspan="9" class="empty">暂无代理数据</td></tr>
                <tr
                  v-for="(p, index) in pagedRows"
                  :key="p.name + p.type"
                  :class="{ selected: selectedProxy && selectedProxy.name === p.name && selectedProxy.type === p.type }"
                  @click="selectedProxy = p"
                >
                  <td class="col-index">{{ rowIndex(index) }}</td>
                  <td class="col-name"><code>{{ p.name }}</code></td>
                  <td class="col-type"><span class="badge badge-ok">{{ p.type }}</span></td>
                  <td class="col-status"><span class="flex-center"><span class="dot" :class="p.online ? 'ok' : 'bad'"></span>{{ p.online ? '在线' : '离线' }}</span></td>
                  <td class="col-num">{{ p.cur_conns }}</td>
                  <td class="col-num">{{ humanBytes(p.month_in) }}</td>
                  <td class="col-num">{{ humanBytes(p.month_out) }}</td>
                  <td class="col-num"><b>{{ humanBytes(p.month_in + p.month_out) }}</b></td>
                  <td class="col-action">
                    <RouterLink :to="'/statistics/' + encodeURIComponent(p.name)" class="btn btn-outline btn-sm" @click.stop>
                      统计
                    </RouterLink>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="analytics-pager proxy-pager">
            <button class="btn btn-outline btn-sm" :disabled="page <= 1" @click="page--">上一页</button>
            <div class="page-num-list">
              <button
                v-for="n in pageNumbers"
                :key="`p-${n}`"
                class="btn btn-outline btn-sm page-num"
                :class="{ active: n === page }"
                @click="page = n"
              >{{ n }}</button>
            </div>
            <span v-if="showRightEllipsis" class="text-muted text-sm">...</span>
            <span class="text-muted text-sm">第 {{ page }} / {{ totalPages }} 页</span>
            <button class="btn btn-outline btn-sm" :disabled="page >= totalPages" @click="page++">下一页</button>
            <label class="text-muted text-sm quick-jump">跳转
              <input class="form-input jump-input" type="number" min="1" :max="totalPages" v-model.number="jumpPage" @keyup.enter="applyJump" />
            </label>
            <button class="btn btn-outline btn-sm" @click="applyJump">确定</button>
          </div>
        </section>

        <aside class="proxy-detail">
          <div class="section-title">代理详情</div>
          <div class="text-muted text-sm" v-if="selectedProxy">{{ selectedProxy.name }} · {{ selectedProxy.type }}</div>
          <div class="text-muted text-sm" v-else>未选择代理</div>
          <template v-if="selectedProxy">
            <div class="detail-pair">
              <div class="detail-row"><span>在线状态</span><b :class="selectedProxy.online ? 'ok-t' : 'bad-t'">{{ selectedProxy.online ? '在线' : '离线' }}</b></div>
              <div class="detail-row"><span>最近心跳</span><b>{{ selectedProxy.online ? '正常' : '中断' }}</b></div>
            </div>
            <div class="detail-pair">
              <div class="detail-row"><span>24h在线率</span><b>{{ (selectedProxy.health?.online_rate ?? 0) + '%' }}</b></div>
              <div class="detail-row"><span>24h波动</span><b>{{ selectedProxy.health?.flap_count_24h ?? 0 }} 次</b></div>
            </div>
            <div class="detail-row"><span>当前连接</span><b>{{ selectedProxy.cur_conns }}</b></div>
            <div class="detail-pair">
              <div class="detail-row"><span>入站</span><b>{{ humanBytes(selectedProxy.month_in) }}</b></div>
              <div class="detail-row"><span>出站</span><b>{{ humanBytes(selectedProxy.month_out) }}</b></div>
            </div>
            <div class="detail-row"><span>总流量</span><b>{{ humanBytes(selectedProxy.month_in + selectedProxy.month_out) }}</b></div>
            <div class="detail-pair">
              <div class="detail-row"><span>连续离线轮询</span><b>{{ selectedProxy.health?.consecutive_offline ?? 0 }}</b></div>
              <div class="detail-row"><span>本次离线时长</span><b>{{ selectedProxy.online ? '-' : offlineSecondsText(selectedProxy.health?.offline_seconds) }}</b></div>
            </div>
            <div class="detail-progress">
              <div class="in" :style="{ width: selectedTotal ? ((selectedProxy.month_in / selectedTotal) * 100).toFixed(1) + '%' : '0%' }"></div>
              <div class="out" :style="{ width: selectedTotal ? ((selectedProxy.month_out / selectedTotal) * 100).toFixed(1) + '%' : '0%' }"></div>
            </div>
            <div class="detail-legend">
              <span><i class="dot in"></i>入站 {{ humanBytes(selectedProxy.month_in) }}</span>
              <span><i class="dot out"></i>出站 {{ humanBytes(selectedProxy.month_out) }}</span>
            </div>
            <div class="detail-cert-title">证书状态</div>
            <div v-if="selectedCerts.length" class="detail-certs">
              <div class="cert-item" v-for="c in selectedCerts" :key="c.domain">
                <code>{{ c.domain }}</code>
                <b :style="{ color: certColor(c.days_left) }">{{ certText(c) }}</b>
              </div>
            </div>
            <div v-else class="text-muted text-sm">未匹配证书信息</div>
          </template>
          <RouterLink v-if="selectedProxy" :to="'/statistics/' + encodeURIComponent(selectedProxy.name)" class="btn btn-outline btn-sm detail-link">
            打开分项统计
          </RouterLink>
        </aside>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../api/index.js'
import { humanBytes } from '../utils/format.js'

const props = defineProps({ status: Object, loading: Boolean })
defineEmits(['refresh'])

const selectedProxy = ref(null)
const sortKey = ref('total')
const sortDir = ref('desc')
const page = ref(1)
const pageSize = ref(10)
const jumpPage = ref(1)
const rows = ref([])
const totalItems = ref(0)
const totalPages = ref(1)
const localLoading = ref(false)
const certs = computed(() => props.status?.certificates ?? [])

const proxies = computed(() => rows.value)
const onlineCount = computed(() => props.status?.proxies?.filter(p => p.online).length ?? 0)
const totalProxies = computed(() => props.status?.proxies?.length ?? totalItems.value)
const curConnsTotal = computed(() => props.status?.proxies?.reduce((s, p) => s + Number(p.cur_conns || 0), 0) ?? 0)
const monthTotal = computed(() => props.status?.proxies?.reduce((s, p) => s + Number(p.month_in || 0) + Number(p.month_out || 0), 0) ?? 0)
function proxyTotal(p) { return Number(p.month_in || 0) + Number(p.month_out || 0) }
function proxyCerts(p) {
  const ds = (p.domains || []).filter(Boolean)
  if (!ds.length) return []
  return certs.value.filter(c => ds.includes(c.domain))
}
const sortedFiltered = computed(() => rows.value)
const pagedRows = computed(() => rows.value)
const pageNumbers = computed(() => {
  const t = totalPages.value
  const p = page.value
  if (t <= 5) return Array.from({ length: t }, (_, i) => i + 1)
  if (p <= 3) return [1, 2, 3, 4, 5]
  if (p >= t - 2) return [t - 4, t - 3, t - 2, t - 1, t]
  return [p - 2, p - 1, p, p + 1, p + 2]
})
const showRightEllipsis = computed(() => pageNumbers.value[pageNumbers.value.length - 1] < totalPages.value)
const selectedTotal = computed(() => selectedProxy.value ? Number(selectedProxy.value.month_in || 0) + Number(selectedProxy.value.month_out || 0) : 0)
const selectedCerts = computed(() => selectedProxy.value ? proxyCerts(selectedProxy.value).slice(0, 3) : [])

function certColor(days) {
  if (days == null) return 'var(--text-2)'
  if (days < 7) return 'var(--danger)'
  if (days < 15) return 'var(--warning)'
  return 'var(--success)'
}
function certText(c) {
  if (c.days_left == null) return '未知'
  return `${c.days_left} 天`
}
function offlineSecondsText(sec) {
  const n = Number(sec || 0)
  if (!n) return '0 秒'
  if (n < 60) return `${n} 秒`
  if (n < 3600) return `${Math.floor(n / 60)} 分钟`
  return `${Math.floor(n / 3600)} 小时 ${Math.floor((n % 3600) / 60)} 分钟`
}
function exportCsv() {
  const rows = sortedFiltered.value
  const header = ['name', 'type', 'online', 'cur_conns', 'month_in', 'month_out', 'total', 'domains']
  const body = rows.map(p => [
    p.name,
    p.type,
    p.online ? 'online' : 'offline',
    p.cur_conns,
    p.month_in,
    p.month_out,
    proxyTotal(p),
    (p.domains || []).join('|')
  ])
  const text = [header, ...body].map(r => r.map(v => `"${String(v ?? '').replaceAll('"', '""')}"`).join(',')).join('\n')
  const blob = new Blob([text], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `proxy-list-${new Date().toISOString().slice(0, 10)}.csv`
  a.click()
  URL.revokeObjectURL(url)
}
function toggleSort(key) {
  if (sortKey.value === key) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = key
    sortDir.value = 'desc'
  }
  page.value = 1
  fetchPage()
}
function sortClass(key) {
  if (sortKey.value !== key) return 'sort-idle'
  return sortDir.value === 'asc' ? 'sort-asc' : 'sort-desc'
}
function rowIndex(index) {
  return (page.value - 1) * pageSize.value + index + 1
}
function applyJump() {
  const p = Number(jumpPage.value || 1)
  if (!Number.isFinite(p)) return
  page.value = Math.min(totalPages.value, Math.max(1, Math.floor(p)))
  fetchPage()
}

async function fetchPage() {
  localLoading.value = true
  try {
    const res = await api.getProxies({
      page: page.value,
      page_size: pageSize.value,
      sort: sortKey.value,
      order: sortDir.value
    })
    rows.value = res.items || []
    totalItems.value = res.meta?.total || 0
    totalPages.value = Math.max(1, res.meta?.total_pages || 1)
    page.value = Math.max(1, res.meta?.page || 1)
  } finally {
    localLoading.value = false
  }
}

watch(pageSize, () => {
  page.value = 1
  jumpPage.value = 1
  fetchPage()
})
watch(page, (p) => {
  jumpPage.value = p
  fetchPage()
})
watch(sortedFiltered, (arr) => {
  if (!arr.length) {
    selectedProxy.value = null
    return
  }
  if (!selectedProxy.value || !arr.find(p => p.name === selectedProxy.value.name && p.type === selectedProxy.value.type)) {
    selectedProxy.value = arr[0]
  }
}, { immediate: true })
watch(() => props.status?.generated_at, () => {
  fetchPage()
})
onMounted(() => {
  fetchPage()
})
</script>

<style scoped>
.proxy-shell {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}
.proxy-page {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}
.header-side {
  display: flex;
  align-items: center;
  gap: 18px;
}
.header-summary {
  display: flex;
  align-items: center;
  gap: 18px;
}
.header-summary > div {
  display: grid;
  gap: 2px;
  min-width: 74px;
}
.header-summary b {
  font-size: var(--fs-page-title-sm);
  line-height: 1;
}
.header-summary span {
  color: var(--text-2);
  font-size: var(--fs-caption);
  margin-left: 3px;
}
.header-summary small {
  color: var(--text-3);
  font-size: var(--fs-table-head);
  white-space: nowrap;
}
.proxy-table-wrap, .proxy-detail {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r);
  box-shadow: var(--shadow);
  padding: 14px;
}
.proxy-table-wrap {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  min-height: 0;
}
.proxy-table-wrap .section-title,
.proxy-detail .section-title {
  font-size: var(--fs-section-title);
  font-weight: var(--fw-section);
}
.proxy-table-wrap .text-muted,
.proxy-detail .text-muted {
  font-size: var(--fs-caption);
}
.proxy-table-tools {
  display: flex;
  align-items: center;
  gap: 12px;
}
.page-size {
  margin-left: 6px;
  width: 74px;
  height: 28px;
  padding: 0 8px;
}
.table-scroll {
  min-height: 0;
  overflow: auto;
}
.sticky-head th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--surface);
}
.table-wrap table {
  table-layout: fixed;
}
.table-wrap th {
  font-size: 12px;
  font-weight: 650;
  color: var(--text-2);
  padding: 11px 10px;
}
.table-wrap td {
  padding: 12px 10px;
  font-size: 13px;
  line-height: 1.25;
  vertical-align: middle;
}
.table-wrap tbody tr {
  height: 44px;
}
.table-wrap tbody tr:hover td {
  background: var(--surface-2);
}
.btn-on {
  color: #bfdbfe;
  border-color: #1d4ed8;
  background: #172554;
}
.selected td { background: var(--surface-2); }
.proxy-detail { display: grid; align-content: start; gap: 8px; }
.proxy-detail {
  min-height: 0;
  overflow: visible;
}
.detail-pair {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  column-gap: 14px;
  border-bottom: 1px solid var(--border);
  padding-bottom: 6px;
}
.detail-pair .detail-row {
  border-bottom: 0;
  padding-bottom: 0;
}
.detail-row { display: flex; justify-content: space-between; align-items: center; gap: 8px; border-bottom: 1px solid var(--border); padding-bottom: 6px; }
.detail-row span { color: var(--text-2); font-size: 12px; }
.detail-row b { font-size: 13px; white-space: nowrap; }
.ok-t { color: var(--success); }
.bad-t { color: var(--danger); }
.detail-progress { display: flex; height: 8px; border-radius: 8px; overflow: hidden; background: var(--border); }
.detail-progress .in { background: #2563eb; }
.detail-progress .out { background: #10b981; }
.detail-legend { display: flex; gap: 14px; flex-wrap: wrap; color: var(--text-2); font-size: 12px; }
.detail-legend .dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; margin-right: 6px; }
.detail-legend .dot.in { background: #2563eb; }
.detail-legend .dot.out { background: #10b981; }
.detail-cert-title { margin-top: 2px; font-size: 13px; font-weight: 600; }
.detail-certs { display: grid; gap: 6px; }
.cert-item {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 7px 9px;
}
.cert-item code {
  font-size: 12px;
}
.cert-item b {
  font-size: 12px;
}
.detail-link { justify-self: start; }
.proxy-main-custom {
  flex: 1;
  grid-template-columns: 1fr 300px;
  min-height: 0;
}
.proxy-pager {
  margin-top: 10px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
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
.quick-jump {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.jump-input {
  width: 72px;
  height: 28px;
  padding: 0 8px;
}
.sortable { cursor: pointer; user-select: none; white-space: nowrap; }
.col-index { width: 54px; text-align: center; color: var(--text-2); }
.col-name { width: 150px; }
.col-name code {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: bottom;
  white-space: nowrap;
}
.col-type { width: 76px; }
.col-status { width: 92px; }
.col-num { width: 110px; text-align: right; }
.col-action { width: 74px; text-align: right; }
.sticky-head .col-num { text-align: right; }
.badge {
  font-size: 11px;
  line-height: 1;
  padding: 6px 9px;
  border-radius: 999px;
}
.dot {
  width: 8px;
  height: 8px;
}
.btn.btn-sm {
  height: 30px;
  padding: 0 10px;
  font-size: 12px;
}
.sort-idle { color: var(--text-2); }
.sort-asc, .sort-desc { font-weight: 700; }
.sort-asc::before { content: '↑'; }
.sort-desc::before { content: '↓'; }
.sort-asc, .sort-desc { font-size: 0; }
@media (max-width: 1200px) {
  .header-side {
    align-items: flex-start;
    flex-direction: column;
    gap: 10px;
  }
  .header-summary {
    flex-wrap: wrap;
    gap: 12px;
  }
  .proxy-page { flex: none; min-height: auto; }
  .proxy-main-custom { grid-template-columns: 1fr; }
  .table-scroll { max-height: 480px; }
  .proxy-detail { max-height: 520px; }
}
</style>
