# 网页来源与依赖

本项目网页基于现有管理器接口独立实现。布局参考 Zashboard 的代理组、节点卡片、延迟颜色与手机布局；没有直接复制其组件源码、字体或图片，也没有将其 Clash API 客户端嵌入本项目。

- 参考仓库：https://github.com/Zephyruso/zashboard
- 核对提交：64b945828c2b4fbae498dc21f8582b60945ad0ed
- 核对文件：package.json、LICENSE、src/components/proxies/ProxyGroupPanel.vue、src/assets/styles/theme/tokens.css
- 上游许可证：MIT，Copyright 2024 Zephyruso；完整原文：https://github.com/Zephyruso/zashboard/blob/64b945828c2b4fbae498dc21f8582b60945ad0ed/LICENSE

当前 Vue、Vue Router、Vite、TypeScript、Tailwind CSS、daisyUI 及测试工具通过 pnpm 获取，精确版本和依赖锁见 package.json、pnpm-lock.yaml，各自许可证随依赖分发。为服务器原生构建选择已验证的 Vite 6、Tailwind 3 与 daisyUI 4，不机械复制参考项目最新的原生构建链。

采用系统字体；无云字体、图片 CDN 或第三方运行期脚本。Node 24.20.0 linux-loong64 取自 https://unofficial-builds.nodejs.org/download/release/v24.20.0/，压缩包 SHA256 为 ce808f2b812df963cf262af1f260c0d54a117fe145c71c804055eaced6a17515，与该发布的 SHASUMS256.txt 核对。Node 工具链和依赖目录不提交仓库。
