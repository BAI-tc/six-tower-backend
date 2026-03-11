# 🚀 完整重构：从 Python FastAPI 到 Golang Gin (API 迁移指南)

您提出的大胆方案可行且很有前瞻性！与其保持“双引擎”导致部署和维护成本（两套环境、两个进程）加倍，不如**干脆基于 Golang 彻底重构整个后端 API（Auth、Games、Library、Steam Oauth 等）**。

下面我将梳理出一套**全量替换 `steam_recommend-main`** 的工程改造计划。这会是一次中大型的重构升级，但由于你的表结构 (PostgreSQL) 已经在 Python 版设计好了，Go 语言可以直接利用 GORM 无缝对接现存字段，无需迁移数据。

---

## 🏗️ 整体重构架构概览

在 `E:\gamescience\ultim_api_go` 项目中，我们将根据你之前 Python 版的目录进行一对一的路由复刻：

*   `/api/v1/auth/*` -> JWT 用户注册、登录、Token 刷新
*   `/api/v1/user/*` -> 用户偏好、统计数据获取
*   `/api/v1/steam/*` -> Steam OpenID 第三方登录回调与授权提取
*   `/api/v1/games/*` -> 游戏发现页、类型查询、分类查询
*   `/api/v1/library/*` -> 用户收藏操作、库状态同步
*   `/api/v1/recommendations/*` -> 对接 Redis 中的 ULTIM 离线推荐及 Postgres 的热门兜底

---

## 🛠️ 第一阶段：复刻必要的关系型表 (GORM Models)

首先，我们要用 Go 的结构体(struct)把你在 PostgreSQL 里的表全映射出来。在你现有的 `models/game.go` 旁边，我们需要补齐：

**`models/user.go`**:
```go
package models
import "time"

type User struct {
    UserID       int       `gorm:"primaryKey;column:user_id"`
    Username     string    `gorm:"column:username"`
    Email        string    `gorm:"column:email"`
    PasswordHash string    `gorm:"column:password_hash"`
    SteamID      string    `gorm:"column:steam_id"`
    CreatedAt    time.Time `gorm:"column:created_at"`
}
```

**`models/interaction.go`**:
```go
package models
import "time"

type Interaction struct {
    InteractionID int       `gorm:"primaryKey;column:interaction_id"`
    UserID        int       `gorm:"column:user_id"`
    ProductID     string    `gorm:"column:product_id"`
    Type          string    `gorm:"column:interaction_type"` // e.g., 'like', 'play'
    PlayHours     float64   `gorm:"column:play_hours"`
    Timestamp     time.Time `gorm:"column:timestamp"`
}
```

---

## 🔐 第二阶段：复刻 JWT 认证系统 (Auth)

Python 的 FastAPI 使用了 OAuth2Bearer，我们在 Golang 中使用 `golang-jwt/jwt/v4` 来接管生成 Token 的逻辑：

```go
// utils/jwt.go
package utils

import (
    "time"
    "github.com/golang-jwt/jwt/v4"
)

var SecretKey = []byte("your-python-old-secret-key-here-to-keep-sessions-valid")

func GenerateToken(userID int) (string, error) {
    claims := jwt.MapClaims{
        "sub": userID,
        "exp": time.Now().Add(time.Hour * 72).Unix(),
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(SecretKey)
}
```

编写 Auth Middleware 对拦截未登录的请求：
```go
// middleware/auth.go
func AuthRequired() gin.HandlerFunc {
    return func(c *gin.Context) {
        tokenString := c.GetHeader("Authorization")
        if tokenString == "" {
            c.AbortWithStatusJSON(401, gin.H{"error": "Unauthorized"})
            return
        }
        // ... JWT 解析校验逻辑
        c.Set("userID", parsedUserID)
        c.Next()
    }
}
```

---

## 🎮 第三阶段：核心业务逻辑端点的改写

我们需要把 python 的 `endpoints/games.py` 翻译为 Go：

```go
// handlers/games.go
func GetGameDetail(c *gin.Context) {
    appID := c.Param("app_id")
    var game models.Game
    
    // database.DB 是之前搭好的 Gorm PostgreSQL 连接
    if err := database.DB.Where("appid = ?", appID).First(&game).Error; err != nil {
        c.JSON(404, gin.H{"error": "Game not found"})
        return
    }
    
    // 把查出来的结构体发给 gamesci 前端
    c.JSON(200, gin.H{"code": 200, "message": "success", "data": game})
}

func GetGamesList(c *gin.Context) {
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
    offset := (page - 1) * limit
    
    // 根据 price, genres 进行 Where()
    // db.Where("genres LIKE ?", "%"+genre+"%").Offset(offset).Limit(limit).Find(&games)
    // 逻辑一比一复刻
}
```

## 🌐 第四阶段：重写 Steam OpenID 登录回调 (最复杂)

你们系统中最特殊的端点在于 `/api/v1/steam/callback`。Python 版通过 OpenID 把回调带到了前端。Go里面也很擅长处理重定向：

```go
// handlers/steam.go
func SteamCallback(c *gin.Context) {
    claimID := c.Query("openid.claimed_id")
    // ... 验证签名
    // 取出 steamID
    steamID := extractSteamID(claimID)

    // 重定向回你前端 gamesci 的页面
    redirectURL := fmt.Sprintf("http://localhost:3000/auth/steam/callback?steamId=%s", steamID)
    c.Redirect(http.StatusFound, redirectURL)
}
```

---

## ⚡ 第五阶段：将路由绑定进 `main.go` 替换 Python

把你之前建好的 `main.go` 扩展，囊括这所有一切。

```go
func main() {
    // 1. Init Config, Postgre, Redis
    // ...
    router := gin.Default()
    router.Use(corsMiddleware())
    
    v1 := router.Group("/api/v1")
    {
        // === 游戏查询模块 ===
        v1.GET("/games", handlers.GetGamesList)
        v1.GET("/games/:app_id", handlers.GetGameDetail)
        v1.GET("/genres", handlers.GetGenres)
        
        // === 认证模块 ===
        v1.POST("/auth/login", handlers.Login)
        v1.POST("/auth/register", handlers.Register)
        
        // === Steam 模块 ===
        v1.GET("/steam/url", handlers.GetSteamLoginURL)
        v1.GET("/steam/callback", handlers.SteamCallback)
        
        // === 你想要的 极速推荐模块 (已经写好了) ===
        v1.GET("/recommendations/ultim", handlers.GetUltimRecommendations)
        v1.GET("/recommendations/popular", handlers.GetPopularGames) // 兜底
        
        // 需要验证 Header Bearer JWT 的私有接口：
        private := v1.Group("")
        private.Use(middleware.AuthRequired())
        {
            private.GET("/user/profile", handlers.GetUserProfile)
            private.GET("/library", handlers.GetUserLibrary)
            private.POST("/library/toggle-favorite", handlers.ToggleFavorite)
        }
    }
    
    router.Run(":8080")
}
```

---

## 🎯 行动路径 (重构路线图)

如果你确定要进行**大改**，以下是我为你梳理的操作步骤顺序（建议一步步来，跑通一块再改前端）：

1. **迁移数据库层 (Gorm)**: 在 Go 工程里把所有的表结构 (Models) 一次性写对。
2. **移植只读接口 (Read-only)**: 像 `games.py` 中查游戏详情、搜游戏。这部分逻辑没有状态，迁移极快。
3. **攻坚认证与会话 (Auth/Steam)**: 引入 `golang-jwt` 处理普通登录，引入 `go-openid` 处理 Steam。并且把前端的请求端口彻底改为切到 `8080`。(此时你可以停掉 `steam_recommend-main` 的运行来验证)。
4. **重建写入接口 (Writes)**: `interactions.py`，点赞与评测，涉及到 Redis 的状态更新。在 Go 中做起来也更为轻松。
5. **正式切换**: 将 `gamesci/src/config.js` 的 `API_BASE` 全局替换为 `http://127.0.0.1:8080/api/v1`。此时你正式告别了笨重的 Python Web Server，拥抱了极致并发的协程时代！ 

你只需定期通过 AirFlow 开一下你的那几十个行的 `step11_offline_redis_sync.py` 更新推断库即可！
