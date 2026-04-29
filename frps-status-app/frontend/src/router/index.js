import { createRouter, createWebHashHistory } from 'vue-router'
import Dashboard from '../views/Dashboard.vue'
import ProxyList from '../views/ProxyList.vue'
import CertificateList from '../views/CertificateList.vue'
import Statistics from '../views/Statistics.vue'
import StatisticsProxy from '../views/StatisticsProxy.vue'
import Settings from '../views/Settings.vue'
import Login from '../views/Login.vue'
import { api } from '../api/index.js'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', component: Dashboard },
    { path: '/proxies', component: ProxyList },
    { path: '/certificates', component: CertificateList },
    { path: '/statistics', component: Statistics },
    { path: '/statistics/:proxyName', component: StatisticsProxy },
    { path: '/settings', component: Settings },
    { path: '/login', component: Login, meta: { public: true } }
  ]
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  try {
    const session = await api.getSession()
    return session.authenticated ? true : '/login'
  } catch {
    return '/login'
  }
})

export default router
