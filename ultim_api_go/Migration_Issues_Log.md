# 🐛 迁移与重构架构的过程中遇到的问题与解决方案 (Issue Log)

在彻底摒弃慢速的 Python FastAPI，将前端流量与业务接口平滑迁移至 Golang 的 `ultim_api_go` 的过程中，目前已经发现及将面临的典型工程问题汇总如下：

## 1. 结构化数据库对象(O/RM)命名空间不一致
**问题**：
Python 原本使用 SQLAlchemy，数据库表中有许多字段例如 `product_id`，而如果在 Go 的 GORM 中定义了 `ProductID`，GORM 会按蛇形命名法尝试寻找 `product_id`，但某些如 `playtime_hours` 这种命名可能会默认找错，从而查表失败。

**解决方案**：
在重构所有 Go 结构体时，强制给所有的表加上 `gorm:"column:字段原名"` 的 Tag，例如：
```go
type UserLibrary struct {
    LibraryID       int       `gorm:"primaryKey;column:library_id"`
    UserID          int       `gorm:"column:user_id"`
    ProductID       int       `gorm:"column:product_id"`
    PlaytimeHours   float64   `gorm:"column:playtime_hours"`
}
```
这样 Golang 能够完全兼容并且直接吸纳原来的 PostgreSQL 库内的百万级数据，**一行老数据都不需要动。**

## 2. JWT 生成算法与 Python 服务不统一
**问题**：
Python 端的用户在前端留存了旧的 `Bearer Token`，当路由全局切换到了 Golang（端口 `8080`），如果 Golang 解析不了老 Token，所有的“自动登录态”会掉线，用户体验割裂。

**解决方案**：
在 Go 端使用 `github.com/golang-jwt/jwt/v4`，并且将密钥字符串（Secret Key）硬编码复制使用与 Python 端的 `.env` 配置中的相同密钥字符串（`SECRET_KEY`）。保证老 Token 直接通用。同时解析的 `claims` 结构保持和原来一致（如包含 `sub` = `user_id`）。

## 3. Steam OpenID 登录回调的重定向差异
**问题**：
Python端之前的写法依赖于 `Request` 的 URL Params 的点解析（例如 `openid.claimed_id`），并且在解析后使用了 FastAPI 的 `RedirectResponse`。在 Golang 中，解析这种非常规名称的 query string 时需要不同的 API。

**解决方案**：
使用 Gin 框架下的 `c.Query("openid.identity")` 即可，只要解析成功，利用 `c.Redirect(http.StatusFound, redirectURL)`，这个行为和 FastAPI 是等价的，可以直接把含有 `steamId={id}` 的参数丢回前端 `http://localhost:3000/auth/steam/callback`。

## 4. 模型计算阻塞在线服务、显存溢出 (OOM)
**问题**：
原来 Python 将推荐算法塞入在线进程（`interactions.py`, `recommendations.py`），用户多的时候会导致主服务被计算任务占满，造成卡死或者爆显存崩溃。此时连刷新游戏目录的操作都会 502。

**解决方案（架构核心价值）**：
这也是我们迁移的核心动因。在新的“Lambda架构”下彻底断开。Golang 版负责 `GET /recommendations/ultim` 接口，它仅仅是利用 `go-redis` 去拿一把 `appID`。而由于运算是在离线 Python 脚本跑的，无论模型多少参数量，都不会有一丝一毫去挤占 Golang 服务的存留时间，保证在线服务延迟始终 < 5ms。

## 5. 跨域问题 (CORS) 的彻底处理
**问题**：
换了端口之后，前端跨域经常会被浏览器卡掉 (Preflight Options 失败)。

**解决方案**：
在 Golang 中必须实现一个专门的 CORS Middleware，放行特定的 Headers（比如 `Authorization`，并且 `Allow-Origin` 对 `3000` 端口放开），只有这样，在本地调试期间前端请求才不会报网络错误。

---

*这些宝贵的采坑记录将保证你的全量 Golang 并发大考过程极其丝滑。*
