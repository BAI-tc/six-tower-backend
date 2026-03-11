# 🚀 GameScience: 下一代基于 Golang 的高并发推荐系统架构

本文档描述了将原来的 Python 后台（`steam_recommend-main`）全面升级并重构后的最终系统架构设计。

## 1. 架构演进与核心理念

原来的架构：前端 -> `Python FastAPI` -> PostgreSQL + 实时推荐模型计算（容易卡死，影响其余普通页面的性能）。

**重构后的新三体架构（Lambda Architecture）：**

1.  **用户前端边界 (`gamesci` / React/Next.js)**：
    *   作为单一的表现层，抛弃一切原有指向 Python 的请求。
    *   全局的 `API_BASE` 指向高性能的 Golang 聚合 API 服务（`127.0.0.1:8080`）。
2.  **核心实时网关与业务服务 (`ultim_api_go` / Golang)**：
    *   **唯一 24 小时常驻的主线服务器**。
    *   使用 `Gin` 框架承载上万并发连接。
    *   直连 PostgreSQL 读取游戏元数据、用户库、点赞记录和交互数据（完全复用原有表结构，GORM O/RM 映射）。
    *   处理 JWT 登入验证及 Steam OpenID 的 OAuth 分布式回调。
    *   当需要查询推荐列表时，**不进行任何复杂的矩阵浮点运算**，仅仅花 1 毫秒从 Redis 拿取数据，组装成结构体后丢回前端。
3.  **计算密集型离线运算层 (`ULTIM` 模型 / Python)**：
    *   **不需要常驻提供 Web 服务**。
    *   只需要成为一个定时任务（Cron Job/Airflow）。
    *   每天或每小时定期执行，加载笨重的 PyTorch, Word2Vec，提取 PostgreSQL 里的全量用户特征，进行离线矩阵推断（SVD + Embedding）。
    *   算出各个用户 “专属前 30 名” 的列表，批量刷入 Redis 缓存 (`List`或`Set`结构) 当中。执行完毕立刻释放显存/内存！

## 2. 模块级功能分布图

### API 端点分布 (Golang 承载所有在线流量)
*   **授权 (`/api/v1/auth`, `/api/v1/steam`)**: 
    接收 Steam OpenID Token -> 存库 -> 派发自身系统的 JWT Token。
*   **游戏目录 (`/api/v1/games`)**: 
    高并发查库，多维动态筛选（种类、Tag、价格等），详情。
*   **用户资产 (`/api/v1/library`, `/api/v1/user`)**: 
    获取已玩游戏、增删点赞收藏，获取 `Gamer DNA` 等画像。
*   **交互数据回收 (`/api/v1/interactions`)**: 
    前端发生的滑块动作、评价，写入 PostgreSQL。
*   **毫秒推荐出口 (`/api/v1/recommendations/ultim`)**: 
    直接基于 `user_id` 从 Redis 读预计算列表（如没读到走冷启动/热门排行兜底）。

## 3. 持久化存储规划 (The State)
*   **PostgreSQL**: Source of Truth 控制源。无论 Golang 怎么挂，Python 怎么挂，数据不可变模型均在 PG。
*   **Redis**: Temporary Acceleration 缓存层。仅用作 Python 推断计算后的 “分发暂存区” 及 JWT 热点白名单检查。即使 Redis 被清空，Golang 服务也有访问 Postgres 获取“热门游戏榜”作为保底显示的逻辑。

---

*随着后续代码的推演，此文档作为蓝图，指导着所有的 API 搬迁工作能够完美解耦计算密集场景。*


在这套将 Python（steam_recommend-main）全面升级为新三体架构（Lambda Architecture）的设计中，最终剥离出核心业务网关并使用 Go（Golang） 而不是 Rust，主要是基于这套系统的特定需求上下文、重构成本、以及 Web 开发的生态契合度来综合考量的。

结合 

New_System_Architecture.md
 和 

Golang_Backend_Rewrite_Guide.md
 的架构设计蓝图，采用 Go 而非 Rust 的核心原因可以归纳为以下几点：

1. 从 Python 平滑重构的开发效率 (Development Velocity)
1比1的路由和逻辑复刻：架构指南中提到要“根据 Python 版的目录进行一对一的路由复刻”。Go 的语法简洁，学习曲线平滑，从 Python (FastAPI) 迁移到 Go (Gin) 能够做到快速映射和重写。
心智负担：如果采用 Rust，其严格的借用检查器（Borrow Checker）、生命周期管理（Lifetimes）机制会导致极高的心智负担。对于一个从脚本语言（Python）重构过来的项目，前期需要极快地跑通业务逻辑（Auth、获取库、推荐兜底等），Go 能保障这种敏捷开发效率，而采用 Rust 会极大地拉长原本可以迅速完成的重构周期。
2. ORM 生态与数据库的无缝对接 (GORM vs Rust ORMs)
完美复用现有数据表：文档中强调：“由于表结构 (PostgreSQL) 已经在 Python 版设计好了，Go 语言可以直接利用 GORM 无缝对接现存字段，无需迁移数据。”
映射便利性：Go 的 GORM 只需几个简单的 Struct Tag（如 `gorm:"column:user_id"`）就能快速将 Python SQLAlchemy 等留下的旧表映射出来。相比之下，Rust 的主流行 ORM（如 Diesel 或 SeaORM）类型系统更为严苛，通常依赖宏（Macros）和强类型约束，对接既有且缺乏强约束的遗留表结构时，会遇到较多繁琐的适配问题。
3. 原生“极致并发的协程” (Goroutines vs Async/Await)
新架构指南中提到：迁移后将“拥抱了极致并发的协程时代”。Go 在语言底层原生提供轻量级协程（Goroutines）和调度器。
在面临高并发的网络 I/O 场景（如每秒成千上万次直连 PostgreSQL 或 1毫秒从 Redis 读取推荐缓存并组装结构体丢回前端），Go 只需要极其符合人类直觉的同步代码编写方式就能榨干 IO 性能。而 Rust 的 Async 生态（如 Tokio 运行时）虽然性能也是顶配，但编写异步代码的复杂度和处理状态跨线程传递的难度远大于 Go。
4. 性能需求已经完全溢出，且场景为 I/O 密集型
原架构的痛点是 "Python + 实时推荐模型计算容易卡死"。
重构后的 Lambda 架构已经将计算密集型任务（矩阵运算、PyTorch 排列）彻底抽离成了离线 Python 定时任务（Cron Job/Airflow）。
剥离后，剩下的常驻服务器 (ultim_api_go) 本质上变成了纯粹的 API 网关和数据搬运工（组装 JSON、高并发查库、读 Redis）。对于这种海量 I/O 数据流转的场景，Go 的性能已经完全足以胜任（毫秒级延迟），此时 Rust 在 CPU 密集型任务上的极限零开销抽象和极致内存控制显得“杀鸡用牛刀”，多出来的性能优势在网络 I/O 的瓶颈前体现不出来。
总结
选择 Go 是因为它是这个重构场景下的最优解：它能在保持接近 Python 开发速度和 Web 生态便利性（Gin + GORM）的同时，提供接近 C++ / Rust 级别的并发处理能力。而 Rust 更适合对内存极其敏感、必须绝对无 GC 停顿或需要极高 CPU 密集度计算的底层基础组件。