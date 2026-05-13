import { createRouter, createWebHashHistory } from 'vue-router'
import Dashboard from '../views/Dashboard.vue'
import ProxyList from '../views/ProxyList.vue'
import CertificateList from '../views/CertificateList.vue'
import TrafficStatistics from '../views/TrafficStatistics.vue'
import StatisticsProxy from '../views/StatisticsProxy.vue'
import Speedtest from '../views/Speedtest.vue'
import Settings from '../views/Settings.vue'
import Login from '../views/Login.vue'
import { api } from '../api/index.js'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', component: Dashboard },
    { path: '/proxies', component: ProxyList },
    { path: '/certificates', component: CertificateList },
    { path: '/statistics', component: TrafficStatistics },
    { path: '/statistics/:proxyName', component: StatisticsProxy },
    { path: '/speedtest', component: Speedtest },
    { path: '/settings', component: Settings },
    { path: '/account', redirect: '/?account=1' },
    { path: '/login', component: Login, meta: { public: true } }
  ]
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  if (sessionStorage.getItem('frps_status_logged_in') !== '1') return '/login'
  try {
    const session = await api.getSession()
    return session.authenticated ? true : '/login'
  } catch {
    return '/login'
  }
})

export default router
