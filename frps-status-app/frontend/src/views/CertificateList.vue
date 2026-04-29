<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">证书列表</div>
        <div class="page-sub">共 {{ certs.length }} 张证书，{{ okCount }} 张正常</div>
      </div>
      <div class="flex-center">
        <button class="btn btn-outline btn-sm" :disabled="loading" @click="$emit('refresh')">
          <span v-if="loading" class="spinner"></span>
          <span v-else>↻</span> 刷新
        </button>
      </div>
    </div>

    <div class="page-body analytics-page">
      <section class="analytics-overview">
        <div>
          <div class="section-title">证书健康概览</div>
          <div class="text-muted text-sm">共 {{ certs.length }} 张证书，{{ warnCount }} 张临期，{{ failCount }} 张异常</div>
          <div class="analytics-overview-bar"><div class="analytics-overview-bar-inner cert-overview-fill"></div></div>
        </div>
        <div class="analytics-overview-metrics">
          <div><b>{{ okCount }}</b><span>/ {{ certs.length }}</span><small>正常证书</small></div>
          <div><b>{{ warnCount }}</b><small>15天内到期</small></div>
          <div><b>{{ minDaysLabel }}</b><small>最快过期</small></div>
        </div>
      </section>

      <section class="analytics-filters cert-filters-grid">
        <input class="form-input" v-model="search" placeholder="搜索域名、关联代理" />
        <select class="form-select" v-model="filterStatus">
          <option value="">全部状态</option>
          <option value="ok">有效</option>
          <option value="warn">临期</option>
          <option value="fail">异常</option>
        </select>
        <select class="form-select" v-model="filterDays">
          <option value="">全部剩余天数</option>
          <option value="lt7">少于 7 天</option>
          <option value="lt15">少于 15 天</option>
          <option value="ge15">15 天及以上</option>
          <option value="unknown">未知</option>
        </select>
        <button class="btn btn-outline btn-sm" :class="{ 'btn-on': filterStatus === 'ok' }" @click="filterStatus = filterStatus === 'ok' ? '' : 'ok'">有效</button>
        <button class="btn btn-outline btn-sm" :class="{ 'btn-on warn': filterStatus === 'warn' }" @click="filterStatus = filterStatus === 'warn' ? '' : 'warn'">临期</button>
      </section>

      <div class="proxy-main analytics-main-2col cert-main-custom">
        <section class="proxy-table-wrap">
          <div class="section-head">
            <div class="section-title">证书清单</div>
            <div class="proxy-table-tools">
              <span class="text-muted text-sm">显示 {{ pageStart }}-{{ pageEnd }} / 共 {{ sortedFiltered.length }}</span>
              <label class="text-muted text-sm">每页
                <select class="form-select page-size" v-model.number="pageSize">
                  <option :value="10">10</option>
                  <option :value="20">20</option>
                  <option :value="30">30</option>
                </select>
              </label>
            </div>
          </div>

          <div class="table-wrap table-scroll">
            <table>
              <thead class="sticky-head">
                <tr>
                  <th class="col-name">域名</th>
                  <th class="col-status">状态</th>
                  <th class="col-num sortable" @click="toggleSort('days')">剩余天数 <span :class="sortClass('days')">↕</span></th>
                  <th>到期时间</th>
                  <th>关联代理</th>
                  <th class="col-action">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="!pagedRows.length"><td colspan="6" class="empty">暂无证书数据</td></tr>
                <tr
                  v-for="c in pagedRows"
                  :key="c.domain"
                  :class="{ selected: selected && selected.domain === c.domain }"
                  @click="selected = c"
                >
                  <td class="col-name"><code>{{ c.domain }}</code></td>
                  <td class="col-status"><span class="badge" :class="certBadge(c)">{{ certText(c) }}</span></td>
                  <td class="col-num" :style="{ color: certColor(c.days_left) }">{{ daysText(c.days_left) }}</td>
                  <td>{{ fmtDate(c.expires_at) }}</td>
                  <td><code>{{ c.relatedProxy || '-' }}</code></td>
                  <td class="col-action"><button class="btn btn-outline btn-sm" @click.stop>详情</button></td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="totalPages > 1" class="analytics-pager proxy-pager">
            <button class="btn btn-outline btn-sm" :disabled="page <= 1" @click="page--">上一页</button>
            <div class="page-num-list">
              <button v-for="n in pageNumbers" :key="n" class="btn btn-outline btn-sm page-num" :class="{ active: n === page }" @click="page = n">{{ n }}</button>
            </div>
            <span class="text-muted text-sm">{{ page }} / {{ totalPages }}</span>
            <button class="btn btn-outline btn-sm" :disabled="page >= totalPages" @click="page++">下一页</button>
          </div>
        </section>

        <aside class="proxy-detail">
          <div class="section-title">证书详情</div>
          <div class="text-muted text-sm" v-if="selected">{{ selected.domain }}</div>
          <div class="text-muted text-sm" v-else>未选择证书</div>
          <template v-if="selected">
            <div class="detail-row"><span>检查状态</span><b :style="{ color: certColor(selected.days_left) }">{{ certText(selected) }}</b></div>
            <div class="detail-row"><span>剩余天数</span><b :style="{ color: certColor(selected.days_left) }">{{ daysText(selected.days_left) }}</b></div>
            <div class="detail-row"><span>到期时间</span><b>{{ fmtDate(selected.expires_at) }}</b></div>
            <div class="detail-row"><span>关联代理</span><b>{{ selected.relatedProxy || '-' }}</b></div>
            <div class="detail-row"><span>证书存在</span><b>{{ selected.present ? '是' : '否' }}</b></div>
            <div class="detail-row"><span>错误信息</span><b>{{ selected.error || '-' }}</b></div>
          </template>
        </aside>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { fmtDate } from '../utils/format.js'

const props = defineProps({ status: Object, loading: Boolean })
defineEmits(['refresh'])

const search = ref('')
const filterStatus = ref('')
const filterDays = ref('')
const selected = ref(null)
const page = ref(1)
const pageSize = ref(10)
const sortKey = ref('days')
const sortDir = ref('asc')

const domainProxyMap = computed(() => {
  const m = {}
  for (const p of (props.status?.proxies ?? [])) {
    for (const d of (p.domains || [])) {
      if (!d) continue
      m[d] ??= []
      m[d].push(p.name)
    }
  }
  return m
})

const certs = computed(() => (props.status?.certificates ?? []).map(c => ({
  ...c,
  relatedProxy: (domainProxyMap.value[c.domain] || []).join(', ')
})))

const okCount = computed(() => certs.value.filter(c => c.ok && (c.days_left == null || c.days_left >= 15)).length)
const warnCount = computed(() => certs.value.filter(c => c.ok && c.days_left != null && c.days_left < 15).length)
const failCount = computed(() => certs.value.filter(c => !c.ok).length)
const minDays = computed(() => {
  const vals = certs.value.map(c => c.days_left).filter(v => v != null).map(Number)
  return vals.length ? Math.min(...vals) : null
})
const minDaysLabel = computed(() => minDays.value == null ? '-' : `${minDays.value} 天`)

function certText(c) {
  if (!c.present || !c.ok) return 'FAIL'
  if (c.days_left != null && c.days_left < 15) return 'WARN'
  return 'OK'
}
function certBadge(c) {
  if (!c.present || !c.ok) return 'badge-bad'
  if (c.days_left != null && c.days_left < 15) return 'badge-warn'
  return 'badge-ok'
}
function certColor(days) {
  if (days == null) return 'var(--text-2)'
  if (days < 7) return 'var(--danger)'
  if (days < 15) return 'var(--warning)'
  return 'var(--success)'
}
function daysText(days) {
  if (days == null) return '-'
  return `${days} 天`
}

const filtered = computed(() => certs.value.filter(c => {
  if (search.value) {
    const q = search.value.toLowerCase()
    if (!c.domain.toLowerCase().includes(q) && !(c.relatedProxy || '').toLowerCase().includes(q)) return false
  }
  if (filterStatus.value) {
    const t = certText(c).toLowerCase()
    if (filterStatus.value === 'ok' && t !== 'ok') return false
    if (filterStatus.value === 'warn' && t !== 'warn') return false
    if (filterStatus.value === 'fail' && t !== 'fail') return false
  }
  if (filterDays.value) {
    const d = c.days_left
    if (filterDays.value === 'unknown' && d != null) return false
    if (filterDays.value === 'lt7' && !(d != null && d < 7)) return false
    if (filterDays.value === 'lt15' && !(d != null && d < 15)) return false
    if (filterDays.value === 'ge15' && !(d != null && d >= 15)) return false
  }
  return true
}))

const sortedFiltered = computed(() => {
  const list = [...filtered.value]
  const sign = sortDir.value === 'asc' ? 1 : -1
  return list.sort((a, b) => {
    const da = a.days_left == null ? 99999 : Number(a.days_left)
    const db = b.days_left == null ? 99999 : Number(b.days_left)
    return (da - db) * sign
  })
})

const totalPages = computed(() => Math.max(1, Math.ceil(sortedFiltered.value.length / pageSize.value)))
const pagedRows = computed(() => sortedFiltered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const pageStart = computed(() => sortedFiltered.value.length ? (page.value - 1) * pageSize.value + 1 : 0)
const pageEnd = computed(() => Math.min(page.value * pageSize.value, sortedFiltered.value.length))
const pageNumbers = computed(() => {
  const t = totalPages.value
  const p = page.value
  if (t <= 5) return Array.from({ length: t }, (_, i) => i + 1)
  if (p <= 3) return [1, 2, 3, 4, 5]
  if (p >= t - 2) return [t - 4, t - 3, t - 2, t - 1, t]
  return [p - 2, p - 1, p, p + 1, p + 2]
})

function toggleSort(key) {
  if (sortKey.value === key) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = key
    sortDir.value = 'asc'
  }
}
function sortClass(key) {
  if (sortKey.value !== key) return 'sort-idle'
  return sortDir.value === 'asc' ? 'sort-asc' : 'sort-desc'
}

watch([sortedFiltered, pageSize], () => { page.value = 1 })
watch(sortedFiltered, (arr) => {
  if (!arr.length) {
    selected.value = null
    return
  }
  if (!selected.value || !arr.find(c => c.domain === selected.value.domain)) selected.value = arr[0]
}, { immediate: true })
</script>

<style scoped>
.cert-overview-fill { width: 75%; }
.cert-filters-grid {
  display: grid;
  grid-template-columns: minmax(240px, 1fr) 130px 160px auto auto;
  gap: 10px;
  align-items: center;
}
.cert-main-custom { grid-template-columns: 1fr 320px; }
.proxy-table-wrap, .proxy-detail {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r);
  box-shadow: var(--shadow);
  padding: 14px;
}
.proxy-table-tools { display: flex; align-items: center; gap: 12px; }
.page-size { margin-left: 6px; width: 74px; height: 28px; padding: 0 8px; }
.table-scroll { max-height: calc(100vh - 440px); overflow: auto; }
.sticky-head th { position: sticky; top: 0; z-index: 1; background: var(--surface); }
.table-wrap table { table-layout: fixed; }
.table-wrap th { font-size: 12px; font-weight: 650; color: var(--text-2); padding: 11px 10px; }
.table-wrap td { padding: 12px 10px; font-size: 13px; vertical-align: middle; }
.table-wrap tbody tr:hover td, .selected td { background: var(--surface-2); }
.btn-on { color: #1d4ed8; border-color: #bfdbfe; background: #eff6ff; }
.btn-on.warn { color: #92400e; border-color: #fde68a; background: #fffbeb; }
.proxy-detail { display: grid; align-content: start; gap: 10px; max-height: calc(100vh - 340px); overflow: auto; }
.detail-row { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--border); padding-bottom: 6px; }
.detail-row span { color: var(--text-2); font-size: 12px; }
.sortable { cursor: pointer; user-select: none; white-space: nowrap; }
.col-name { min-width: 220px; }
.col-status { width: 92px; }
.col-num { width: 116px; text-align: right; }
.col-action { width: 74px; text-align: right; }
.sort-idle { color: var(--text-2); }
.sort-asc, .sort-desc { color: #2563eb; font-weight: 700; }
.sort-asc::before { content: '↑'; }
.sort-desc::before { content: '↓'; }
.sort-asc, .sort-desc { font-size: 0; }
.proxy-pager { margin-top: 10px; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.page-num-list { display: flex; gap: 6px; }
.page-num { min-width: 34px; padding: 0 8px; }
.page-num.active { color: #1d4ed8; border-color: #bfdbfe; background: #eff6ff; }
@media (max-width: 1200px) {
  .cert-filters-grid { grid-template-columns: 1fr 1fr 1fr; }
  .cert-main-custom { grid-template-columns: 1fr; }
  .table-scroll { max-height: 480px; }
  .proxy-detail { max-height: 520px; }
}
</style>
