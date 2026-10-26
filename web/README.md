# Mihomo React 控制台

本地框架升级：React 19 + TypeScript + HeroUI 2 + Tailwind 4 + Vite 7，共用 `@holosama/ui` 0.2.2。视觉来源为 MIT 开源 NextNapCatWebUI；保留原管理器和网页网关接口，替换旧 Vue/daisyUI 前端。来源与许可证见 `THIRD_PARTY_NOTICES.md` 和 vendor 包内 `PROVENANCE.json`。

```sh
npm ci
npm test
npm run build
npm run test:ui
```

浏览器测试只连接模拟 API。已安装 Chromium 可通过 `PLAYWRIGHT_CHROMIUM_EXECUTABLE` 指定可执行路径；缺省使用 Playwright 自带版本。开发使用 `npm run dev`，API 默认代理到独立测试端口 19080；不要把测试脚本指向真实配置写操作。

`MIHOMO_WEB_DIST=/absolute/path npm run build` 支持独立输出目录。服务器只运行 Go 网关与静态文件；原生构建缺少 Rollup/LightningCSS 二进制时，在受支持的开发机生成可移植 dist，再交由服务器测试过的 Go 网关提供服务，不替换宿主运行库。

五个页面：运行概览、订阅配置、代理节点、规则管理、运行日志。登录模式由后端提供；cookie/CSRF、写操作串行、结果未知提示与活动续期保留。端口、核心及 TUN 变更需页面确认。日志离开页面关闭连接、后台暂停、断线退避。订阅链接不写入浏览器持久存储。

流量使用 `/status.metrics` 聚合计数差值和实际观测时间计算，首次采样、长间隔、计数重置不绘制虚假速率。未采集字段显示“—”，流量数据仅存当前页面内存。前端不获取每条连接的目标地址。

公共库源码在服务器 `/home/server/services/ui-kit`。更换库版本需更新 `vendor/` 和 `package-lock.json`，然后跑两端的构建和 UI 回归。不能仅修改 node_modules。
