import { createRouter, createWebHashHistory } from 'vue-router'
import Dashboard from '../views/Dashboard.vue'
import ProxyList from '../views/ProxyList.vue'
import Statistics from '../views/Statistics.vue'
import Settings from '../views/Settings.vue'

export default createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', component: Dashboard },
    { path: '/proxies', component: ProxyList },
    { path: '/statistics', component: Statistics },
    { path: '/settings', component: Settings }
  ]
})
