# server-pc 容器部署适配

本目录保存 services/mihomo 既有部署的固定 systemctl 适配与 Compose 结构模板；不创建新服务、不改变生产配置。运行事实和权限边界见服务器 `/home/server/services/mihomo/README.md`。此前本地部署快照中的 React 前端及指标提交已合回本常驻仓库。

`systemctl` 只映射 mihomo.service、mihomo-web.service 的固定 Docker 操作及只读状态；部署时位于 services/mihomo/bin/systemctl。模板的环境值全部由本机提供；源码、bin/static 构建产物、核心及 data 分开，运行配置和订阅不提交。不要从模板直接覆盖现场 Compose，先核对现有挂载与最小权限；本次仅同步源码，未部署。
