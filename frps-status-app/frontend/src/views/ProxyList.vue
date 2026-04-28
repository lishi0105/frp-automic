<template>
  <div>
    <div class="page-header">
      <div>
        <div class="page-title">代理列表</div>
        <div class="page-sub">共 {{ proxies.length }} 个代理，{{ onlineCount }} 个在线</div>
      </div>
      <button class="btn btn-outline btn-sm" :disabled="loading" @click="$emit('refresh')">
        <span v-if="loading" class="spinner"></span>
        <span v-else>↻</span> 刷新
      </button>
    </div>

    <div class="page-body">
      <!-- 筛选 -->
      <div style="display:flex;gap:10px;margin-bottom:16px;flex-wrap:wrap">
        <input class="form-input" style="width:200px" v-model="search" placeholder="搜索代理名称…" />
        <select class="form-select" style="width:130px" v-model="filterType">
          <option value="">全部类型</option>
          <option v-for="t in types" :key="t" :value="t">{{ t }}</option>
        </select>
        <select class="form-select" style="width:120px" v-model="filterOnline">
          <option value="">全部状态</option>
          <option value="online">在线</option>
          <option value="offline">离线</option>
        </select>
      </div>

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>名称</th>
              <th>类型</th>
              <th>状态</th>
              <th>连接数</th>
              <th>本月上行</th>
              <th>本月下行</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!filtered.length">
              <td colspan="7" class="empty">暂无代理数据</td>
            </tr>
            <tr v-for="p in filtered" :key="p.name + p.type">
              <td><code>{{ p.name }}</code></td>
              <td><span class="badge badge-ok">{{ p.type }}</span></td>
              <td>
                <span class="flex-center">
                  <span class="dot" :class="p.online ? 'ok' : 'bad'"></span>
                  {{ p.online ? '在线' : '离线' }}
                </span>
              </td>
              <td>{{ p.cur_conns }}</td>
              <td>{{ humanBytes(p.month_in) }}</td>
              <td>{{ humanBytes(p.month_out) }}</td>
              <td>
                <RouterLink :to="'/statistics?proxy=' + p.name" class="btn btn-outline btn-sm">
                  统计
                </RouterLink>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 证书列表（如有） -->
      <div class="section mt-4" v-if="certs.length">
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
                <td :style="{ color: certColor(c.days_left) }"><b>{{ c.days_left ?? '-' }}</b></td>
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
import { ref, computed } from 'vue'
import { RouterLink } from 'vue-router'
import { humanBytes, fmtDate } from '../utils/format.js'

const props = defineProps({ status: Object, daily: Array, loading: Boolean })
defineEmits(['refresh'])

const search = ref('')
const filterType = ref('')
const filterOnline = ref('')

const proxies = computed(() => props.status?.proxies ?? [])
const certs = computed(() => props.status?.certificates ?? [])
const onlineCount = computed(() => proxies.value.filter(p => p.online).length)
const types = computed(() => [...new Set(proxies.value.map(p => p.type))].sort())

const filtered = computed(() => proxies.value.filter(p => {
  if (search.value && !p.name.toLowerCase().includes(search.value.toLowerCase())) return false
  if (filterType.value && p.type !== filterType.value) return false
  if (filterOnline.value === 'online' && !p.online) return false
  if (filterOnline.value === 'offline' && p.online) return false
  return true
}))

function certColor(days) {
  if (days == null) return 'var(--danger)'
  if (days < 7) return 'var(--danger)'
  if (days < 15) return 'var(--warning)'
  return 'var(--success)'
}
</script>
