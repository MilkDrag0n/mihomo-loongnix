# 网页来源与依赖

当前界面使用 `@holosama/ui`：从 [NextNapCatWebUI](https://github.com/bietiaop/NextNapCatWebUI) 的侧栏、背景、卡片、仪表提取视觉规则，重新实现为 React/HeroUI 公共组件。锁定修订 `b80f34893ec549ba9480e8300a52dd10ec9ded26`，MIT 许可证原文和对应源文件列在 vendor 包的 `licenses/`、`PROVENANCE.json`。没有复制 NapCat 的鉴权、QQ 业务或角色插画。

React、React DOM、React Router、HeroUI、Framer Motion、Lucide、Tailwind CSS、Vite、TypeScript 和测试依赖来自 npm，精确版本与完整依赖链见 package-lock.json，各许可证随依赖分发。系统字体，无图片 CDN 或第三方运行时脚本。

历史 Vue 版参考 Zashboard 的布局（提交 `64b945828c2b4fbae498dc21f8582b60945ad0ed`，MIT），没有复制其组件源码或资源；本次已移除 Vue、Vue Router、daisyUI 及其构建链。该说明仅保留来源历史。
