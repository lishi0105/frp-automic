<template>
  <div class="cert-shell">
    <div class="page-header">
      <div>
        <div class="page-title">证书列表</div>
        <div class="page-sub">共 {{ certs.length }} 张证书，{{ okCount }} 张正常</div>
      </div>
      <div class="header-side">
        <div class="header-summary">
          <div>
            <b>{{ okCount }}</b><span>/ {{ certs.length }}</span>
            <small>正常证书</small>
          </div>
          <div>
            <b>{{ warnCount }}</b>
            <small>15天内到期</small>
          </div>
          <div>
            <b>{{ minDaysLabel }}</b>
            <small>最快过期</small>
          </div>
        </div>
        <div class="flex-center">
          <button class="btn btn-outline btn-sm" :disabled="loading || localLoading" @click="$emit('refresh')">
            <span v-if="loading || localLoading" class="spinner"></span>
            <span v-else>↻</span> 刷新
          </button>
        </div>
      </div>
    </div>

    <div class="page-body analytics-page cert-page">
      <div class="proxy-main analytics-main-2col cert-main-custom">
        <section class="proxy-table-wrap">
          <div class="section-head">
            <div class="section-title">证书清单</div>
            <div class="proxy-table-tools">
              <span class="text-muted text-sm">显示 {{ pageStart }}-{{ pageEnd }} / 共 {{ totalItems }}</span>
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
                  <th class="col-index">序号</th>
                  <th class="col-name">域名</th>
                  <th class="col-status">状态</th>
                  <th class="col-status">公网TLS</th>
                  <th class="col-num sortable" @click="toggleSort('days')">剩余天数 <span :class="sortClass('days')">↕</span></th>
                  <th>到期时间</th>
                  <th>关联代理</th>
                  <th class="col-action">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="!pagedRows.length"><td colspan="8" class="empty">暂无证书数据</td></tr>
                <tr
                  v-for="(c, index) in pagedRows"
                  :key="c.domain"
                  :class="{ selected: selected && selected.domain === c.domain }"
                  @click="selected = c"
                >
                  <td class="col-index">{{ rowIndex(index) }}</td>
                  <td class="col-name"><code>{{ c.domain }}</code></td>
                  <td class="col-status"><span class="badge" :class="certBadge(c)">{{ certText(c) }}</span></td>
                  <td class="col-status"><span class="badge" :class="tlsBadge(c)">{{ tlsText(c) }}</span></td>
                  <td class="col-num" :style="{ color: certColor(c.days_left) }">{{ daysText(c.days_left) }}</td>
                  <td>{{ fmtDate(c.expires_at) }}</td>
                  <td><code>{{ c.relatedProxy || '-' }}</code></td>
                  <td class="col-action"><button class="btn btn-outline btn-sm" @click.stop>详情</button></td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="analytics-pager proxy-pager">
            <button class="btn btn-outline btn-sm" :disabled="page <= 1" @click="page--">上一页</button>
            <div class="page-num-list">
              <button v-for="n in pageNumbers" :key="n" class="btn btn-outline btn-sm page-num" :class="{ active: n === page }" @click="page = n">{{ n }}</button>
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
          <div class="section-title">证书详情</div>
          <div class="text-muted text-sm" v-if="selected">{{ selected.domain }}</div>
          <div class="text-muted text-sm" v-else>未选择证书</div>
          <template v-if="selected">
            <div class="detail-row"><span>检查状态</span><b :style="{ color: certColor(selected.days_left) }">{{ certText(selected) }}</b></div>
            <div class="detail-row"><span>剩余天数</span><b :style="{ color: certColor(selected.days_left) }">{{ daysText(selected.days_left) }}</b></div>
            <div class="detail-row"><span>到期时间</span><b>{{ fmtDate(selected.expires_at) }}</b></div>
            <div class="detail-row"><span>公网TLS</span><b :style="{ color: selected.tls_ok ? 'var(--success)' : 'var(--danger)' }">{{ tlsText(selected) }}</b></div>
            <div class="detail-row"><span>TLS时延</span><b>{{ selected.tls_latency_ms != null ? (selected.tls_latency_ms + ' ms') : '-' }}</b></div>
            <div class="detail-row"><span>公网到期</span><b>{{ fmtDate(selected.tls_expires_at) }}</b></div>
            <div class="detail-row"><span>公网剩余天数</span><b>{{ daysText(selected.tls_days_left) }}</b></div>
            <div class="detail-row"><span>公网与本地一致</span><b>{{ selected.tls_has_local_cert ? (selected.tls_match_local ? '是' : '否') : '-' }}</b></div>
            <div class="detail-row"><span>关联代理</span><b>{{ selected.relatedProxy || '-' }}</b></div>
            <div class="detail-row"><span>证书存在</span><b>{{ selected.present ? '是' : '否' }}</b></div>
            <div class="detail-row"><span>错误信息</span><b>{{ selected.error || selected.tls_error || '-' }}</b></div>
          </template>
        </aside>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { api } from '../api/index.js'
import { fmtDate } from '../utils/format.js'

const props = defineProps({ status: Object, loading: Boolean })
defineEmits(['refresh'])

const selected = ref(null)
const page = ref(1)
const pageSize = ref(10)
const jumpPage = ref(1)
const sortKey = ref('days')
const sortDir = ref('asc')
const rows = ref([])
const totalItems = ref(0)
const totalPages = ref(1)
const localLoading = ref(false)

const certs = computed(() => props.status?.certificates ?? [])
const pagedRows = computed(() => rows.value)
const sortedFiltered = computed(() => rows.value)

const okCount = computed(() => certs.value.filter(c => c.ok && (c.days_left == null || c.days_left >= 15)).length)
const warnCount = computed(() => certs.value.filter(c => c.ok && c.days_left != null && c.days_left < 15).length)
const failCount = computed(() => certs.value.filter(c => !c.ok).length)
const minDays = computed(() => {
  const vals = certs.value.map(c => c.days_left).filter(v => v != null).map(Number)
  return vals.length ? Math.min(...vals) : null
})
const minDaysLabel = computed(() => minDays.value == null ? '-' : `${minDays.value} 天`)

const pageStart = computed(() => totalItems.value ? (page.value - 1) * pageSize.value + 1 : 0)
const pageEnd = computed(() => Math.min(page.value * pageSize.value, totalItems.value))
const pageNumbers = computed(() => {
  const t = totalPages.value
  const p = page.value
  if (t <= 5) return Array.from({ length: t }, (_, i) => i + 1)
  if (p <= 3) return [1, 2, 3, 4, 5]
  if (p >= t - 2) return [t - 4, t - 3, t - 2, t - 1, t]
  return [p - 2, p - 1, p, p + 1, p + 2]
})
const showRightEllipsis = computed(() => pageNumbers.value[pageNumbers.value.length - 1] < totalPages.value)

function certText(c) {
  if (!c.tls_ok) return 'FAIL'
  if (c.tls_has_local_cert && !c.tls_match_local) return 'FAIL'
  if (!c.present || !c.ok) return 'FAIL'
  if (c.days_left != null && c.days_left < 15) return 'WARN'
  return 'OK'
}
function certBadge(c) {
  if (!c.tls_ok || (c.tls_has_local_cert && !c.tls_match_local)) return 'badge-bad'
  if (!c.present || !c.ok) return 'badge-bad'
  if (c.days_left != null && c.days_left < 15) return 'badge-warn'
  return 'badge-ok'
}
function tlsText(c) {
  if (!c.tls_ok) return 'FAIL'
  if (c.tls_has_local_cert && !c.tls_match_local) return 'MISMATCH'
  return 'OK'
}
function tlsBadge(c) {
  if (!c.tls_ok || (c.tls_has_local_cert && !c.tls_match_local)) return 'badge-bad'
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

async function fetchPage() {
  localLoading.value = true
  try {
    const res = await api.getCertificates({
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

function toggleSort(key) {
  if (sortKey.value === key) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = key
    sortDir.value = 'asc'
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
    selected.value = null
    return
  }
  if (!selected.value || !arr.find(c => c.domain === selected.value.domain)) selected.value = arr[0]
}, { immediate: true })
watch(() => props.status?.generated_at, () => {
  fetchPage()
})
onMounted(() => {
  fetchPage()
})
</script>

<style scoped>
.cert-shell {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}
.cert-page {
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
  font-size: 18px;
  line-height: 1;
}
.header-summary span {
  color: var(--text-2);
  font-size: 12px;
  margin-left: 3px;
}
.header-summary small {
  color: var(--text-3);
  font-size: 11px;
  white-space: nowrap;
}
.cert-main-custom {
  flex: 1;
  grid-template-columns: 1fr 320px;
  min-height: 0;
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
.proxy-table-tools { display: flex; align-items: center; gap: 12px; }
.page-size { margin-left: 6px; width: 74px; height: 28px; padding: 0 8px; }
.quick-jump { display: inline-flex; align-items: center; gap: 6px; }
.jump-input { width: 72px; height: 30px; padding: 0 8px; }
.table-scroll { min-height: 0; overflow: auto; }
.sticky-head th { position: sticky; top: 0; z-index: 1; background: var(--surface); }
.table-wrap table { table-layout: fixed; }
.table-wrap th { font-size: 12px; font-weight: 650; color: var(--text-2); padding: 11px 10px; }
.table-wrap td { padding: 12px 10px; font-size: 13px; vertical-align: middle; }
.table-wrap tbody tr:hover td, .selected td { background: var(--surface-2); }
.proxy-detail { display: grid; align-content: start; gap: 10px; min-height: 0; overflow: auto; }
.detail-row { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--border); padding-bottom: 6px; }
.detail-row span { color: var(--text-2); font-size: 12px; }
.sortable { cursor: pointer; user-select: none; white-space: nowrap; }
.col-index { width: 54px; text-align: center; color: var(--text-2); }
.col-name { width: 180px; }
.col-name code {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: bottom;
  white-space: nowrap;
}
.col-status { width: 92px; }
.col-num { width: 116px; text-align: right; }
.col-action { width: 74px; text-align: right; }
.sort-idle { color: var(--text-2); }
.sort-asc, .sort-desc { color: #60a5fa; font-weight: 700; }
.sort-asc::before { content: '↑'; }
.sort-desc::before { content: '↓'; }
.sort-asc, .sort-desc { font-size: 0; }
.proxy-pager { margin-top: 10px; display: flex; align-items: center; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
.page-num-list { display: flex; gap: 6px; }
.page-num { min-width: 34px; padding: 0 8px; }
.page-num.active { color: #bfdbfe; border-color: #1d4ed8; background: #172554; }
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
  .cert-page { flex: none; min-height: auto; }
  .cert-main-custom { grid-template-columns: 1fr; }
  .table-scroll { max-height: 480px; }
  .proxy-detail { max-height: 520px; }
}
</style>
