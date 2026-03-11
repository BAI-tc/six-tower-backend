# 🎯 ULTIM 推荐系统与 Golang 后端全面完工倒计时（缺项盘点）

目前的重构工作已经为最难的高并发请求部分（`/recommendations/ultim`、跨域、部分基础查询）打通了道路。但如果要彻底丢弃原来的 Python (FastAPI, 端口 3001) 服务，让整个 `gamesci` 前端**完全依赖** `ultim_api_go` (端口 8080) 正常运转，我们不仅需要在 Go 工程里补齐大量基础业务接口，还需要将 Python 的算法彻底生产化。

以下是实现“闭环”所欠缺的核心拼图：

## 1. 🏗️ Golang API 后端 (ultim_api_go) 的缺失接口
目前 `main.go` 仅实现了 `/games`, `/genres`, `/tags` 和 `/recommendations/ultim` 几个端点。前端在请求点赞、登录、获取库等普通业务时依然指向了 3001 端口。需要将这些端点用 Go 重构并接入：

*   **1.1 用户认证与鉴权 (Auth & Steam)**：
    *   `/api/v1/auth/login`, `/api/v1/auth/register` (基础邮箱注册/登录分配 JWT)。
    *   `/api/v1/steam/url`, `/api/v1/steam/callback` (Steam OpenID 登录跳转、抓取 Steam_ID 并通过 URL 参数带回前端)。
*   **1.2 个人数据抓取与行为追踪 (Library & Interactions)**：
    *   `/api/v1/steam/recent/:steamId` (前端提取最近游玩使用)。
    *   `/api/v1/steam/games/:steamId` (前端提取整个游戏库进行展示使用)。
    *   `/api/v1/library/toggle-favorite` 等心愿单/点赞接口。这部分极其重要，这构成了用户的最新行为反馈 (User-Item Interaction)。
*   **1.3 其他推荐页与标签页回退端点 (Recommend Fallbacks)**：
    *   目前在 `/recommendations` 页面中，如果你拉到底部，系统会根据 `/recommendations/popular-not-owned/:steamId`, `/recommendations/similar/:productId`, `/recommendations/by-genre/:steamId` 等端点无限滚动。这些目前还在向旧服务器请求，需要使用 Go + PostgreSQL 聚合成高性能返回逻辑。
*   **1.4 GORM 结构体的完美映射**：
    *   为了承接 Python SQLAlchemy 遗留的数据表结构，还需要在 `models/` 下创建如 `user.go`, `interaction.go`, `steam_profile.go` 并设置对应的 `gorm:"column:xxx"` 映射关系。

## 2. 🧠 ULTIM 算法侧的生产化 (Python 模型层)
`E:\gamescience\ULTIM` 下的所有代码目前依然还是在本地执行的数据科学脚本。

*   **2.1 打通 `step11_offline_redis_sync.py`**：
    *   **现在的状态**：这是一个 Mock 脚本，只写死了两个 `test_pids`，并且返回硬编码的一串 AppID。
    *   **缺少的工作**：需要在这个文件里导入 `step10_ultimate_fusion_engine.py` 的计算核心，正式连上 PostgreSQL 读取所有全站注册的有效用户 (Users)，进行 S_SVD（矩阵分解）、S_Sem（词嵌入）、S_Pop 等算法指标的真实矩阵运算聚合，并在算出 **True Top K** 后正式写入 Redis。
*   **2.2 定时任务编排 (Cron / Airflow)**：
    *   随着 Go 接口记录越来越多用户的交互 (Interaction DB 落库)，每天（或者每几小时）需要有一套调度系统自动拉起这些 Python 脚本，用最新的 PostgreSQL 数据重训轻量级的 SVD 分解矩阵并将最新的推断库同步给 Redis，否则新用户的推荐永远是冷启动（Fallback 至热门榜单）。
    *   需要一套冷启动算法：在 Python 推断前如果用户刚注册，Go 要能在 Redis 取到不到值时，平滑降级（如目前 Go 中的 `getPopularGamesFallback`）。

## 3. 🖥️ 前端配置的最终切换
*   当前 `E:\gamescience\gamesci\src\config.js` 依然写着：
    ```javascript
    export const API_BASE = 'http://127.0.0.1:3001/api/v1';
    export const ULTIM_API_BASE = 'http://127.0.0.1:8080/api/v1';
    ```
*   在 Go 服务把以上 1 (Golang API的缺失接口) 全部写完之后，这个前端的终极改造就是**删掉 3001**，全局唯一指定到 Go 网关：
    ```javascript
    export const API_BASE = 'http://127.0.0.1:8080/api/v1';
    ```

---

**最终结语：**
现在这套系统处于**骨架刚搭好主血管，分支毛细血管仍在旧系统上**的阶段。需要先在 Go 里照葫芦画瓢把其他接口的 Gorm DTO 铺满，然后把 `ULTIM` Python 的 `step11` 和跑数流程自动化，这套架构就算大功告成了！
