import { createApp } from "vue";
import { createRouter, createWebHistory } from "vue-router";
import App from "./App.vue";
import Overview from "./pages/Overview.vue";
import Profiles from "./pages/Profiles.vue";
import Proxies from "./pages/Proxies.vue";
import Rules from "./pages/Rules.vue";
import Logs from "./pages/Logs.vue";
import "./style.css";
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", redirect: "/overview" },
    { path: "/login", component: Overview },
    { path: "/overview", component: Overview },
    { path: "/profiles", component: Profiles },
    { path: "/proxies", component: Proxies },
    { path: "/rules", component: Rules },
    { path: "/logs", component: Logs },
    { path: "/:pathMatch(.*)*", redirect: "/overview" },
  ],
});
createApp(App).use(router).mount("#app");
