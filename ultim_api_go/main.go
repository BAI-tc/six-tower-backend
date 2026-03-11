package main

import (
	"log"
	"ultim_api_go/config"
	"ultim_api_go/database"
	"ultim_api_go/handlers"

	"github.com/gin-gonic/gin"
)

func main() {
	// 1. 初始化配置
	cfg := config.LoadConfig()

	// 2. 初始化 Redis 和 PostgreSQL
	database.InitRedis(cfg.RedisURL)
	database.InitPostgres(cfg.PostgresURL)

	// 3. 设置 Gin 路由引擎
	router := gin.Default()

	// 配置跨域策略 (CORS)，允许 gamesci 前端访问
	router.Use(corsMiddleware())

	// 4. 定义 API 版本路由组
	api := router.Group("/api/v1")
	{
		// 核心推荐接口：对接首页的 fetchPersonalizedRecommendations
		api.GET("/recommendations/ultim", handlers.GetUltimRecommendations)
		api.GET("/recommendations/popular", handlers.GetPopularGames)
		api.GET("/recommendations/trending", handlers.GetTrendingGames)
		api.GET("/recommendations/similar/:productId", handlers.GetSimilarGames)
		api.GET("/recommendations/similar-to-owned/:steamId", handlers.GetSimilarToOwned)
		api.GET("/recommendations/by-genre/:steamId", handlers.GetByGenre)
		api.GET("/recommendations/popular-not-owned/:steamId", handlers.GetPopularNotOwned)
		api.GET("/recommendations/explanation", handlers.GetRecommendationExplanation)
		api.GET("/recommendations/stats", handlers.GetRecommendationStats)

		// === 游戏查询模块 ===
		api.GET("/games", handlers.GetGamesList)
		api.GET("/games/:app_id", handlers.GetGameDetail)
		api.GET("/genres", handlers.GetGenres)
		api.GET("/tags", handlers.GetTags)

		// === Steam 认证和用户数据 ===
		steamHandler := handlers.NewSteamHandler(&cfg)
		api.GET("/steam/url", steamHandler.GetSteamLoginURL)
		api.GET("/steam/callback", steamHandler.SteamCallback)
		api.GET("/steam/user/:steam_id", steamHandler.GetSteamUser)
		api.GET("/steam/games/:steam_id", steamHandler.GetSteamGames)
		api.GET("/steam/recent/:steam_id", steamHandler.GetSteamRecentGames)

		// === 认证模块 ===
		authHandler := handlers.NewAuthHandler()
		api.POST("/auth/login", authHandler.Login)
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/refresh", authHandler.Refresh)
		api.POST("/auth/logout", authHandler.Logout)
		api.GET("/auth/verify", authHandler.Verify)

		// === 愿望单/收藏模块 ===
		libraryHandler := handlers.NewLibraryHandler()
		api.GET("/library/wishlist", libraryHandler.GetWishlist)
		api.POST("/library/wishlist", libraryHandler.AddToWishlist)
		api.DELETE("/library/wishlist", libraryHandler.RemoveFromWishlist)
		api.GET("/library/game-status", libraryHandler.GetGameStatus)
		api.GET("/library/favorites", libraryHandler.GetFavorites)
		api.POST("/library/toggle-favorite", libraryHandler.ToggleFavorite)

		// === 交互行为模块 ===
		interactionsHandler := handlers.NewInteractionsHandler()
		api.POST("/interactions/interact", interactionsHandler.Interact)
		api.POST("/interactions/review", interactionsHandler.SubmitReview)
		api.GET("/interactions/review/:product_id", interactionsHandler.GetReviews)
		api.POST("/interactions/feedback", interactionsHandler.SubmitFeedback)
		api.GET("/interactions/history", interactionsHandler.GetHistory)
		api.DELETE("/interactions/history", interactionsHandler.DeleteHistory)
		api.GET("/interactions/stats", interactionsHandler.GetStats)

		// === 用户资料模块 ===
		usersHandler := handlers.NewUsersHandler()
		api.GET("/users/profile", usersHandler.GetProfile)
		api.PUT("/users/profile", usersHandler.UpdateProfile)
		api.GET("/users/interactions", usersHandler.GetInteractions)
		api.GET("/users/played-games", usersHandler.GetPlayedGames)
		api.GET("/users/preferences", usersHandler.GetPreferences)
		api.DELETE("/users/account", usersHandler.DeleteAccount)
		api.GET("/users/profile/complete", usersHandler.GetProfileComplete)
	}

	log.Printf("🚀 ULTIM Golang API Server running on port %s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// 跨域处理中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
