package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"ultim_api_go/database"
	"ultim_api_go/models"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// GetUltimRecommendations 负责响应 gamesci 前端的推荐请求
func GetUltimRecommendations(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	// 1. O(1) 极速从 Redis 读取 Python 离线算好的推荐列表
	// Redis 数据结构预想： Key -> "ultim_recom:{user_id}", Value -> JSON string 数组 '["1091500", "281990", ...]'
	redisKey := "ultim_recom:" + userID
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()

	if err == redis.Nil {
		// Redis 命中为空 (比如这是一个纯冷启动新用户，Python 离线脚本还没跑到他)
		// 此时降级处理：直接从 PostgreSQL 掏出热门游戏榜单给他兜底
		c.JSON(http.StatusOK, gin.H{
			"algorithm":       "popular_fallback",
			"recommendations": getPopularGamesFallback(),
		})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Redis cache error"})
		return
	}

	// 2. 解析 Redis 取出的 App ID 数组
	var appIDs []string
	if err := json.Unmarshal([]byte(val), &appIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse cached data"})
		return
	}

	// 3. 去 PostgreSQL 取游戏元数据进行“补齐”，因为前端需要图片和标题！
	var games []models.GameMetadata
	var intAppIDs []int
	for _, idStr := range appIDs {
		if id, err := strconv.Atoi(idStr); err == nil {
			intAppIDs = append(intAppIDs, id)
		}
	}
	database.DB.Where("product_id IN ?", intAppIDs).Find(&games)

	// 将查询出来的游戏，按照 Redis 里评分顺序进行原地排序
	gameMap := make(map[string]models.GameMetadata)
	for _, g := range games {
		gameMap[strconv.Itoa(g.ProductID)] = g
	}

	// 构建原始游戏列表
	var orderedGames []gin.H
	for _, id := range appIDs {
		if game, exists := gameMap[id]; exists {
			orderedGames = append(orderedGames, gin.H{
				"id":              game.ProductID,
				"appid":           strconv.Itoa(game.ProductID),
				"name":            game.Title,
				"title":           game.Title,
				"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(game.ProductID) + "/header.jpg",
				"metacritic":      game.Metascore,
				"genres":          stringToList(game.Genres),
			})
		}
	}

	// 4. 规则重排：对 ULTIM 推荐结果进行二次排序
	rerankedGames := rerankRecommendations(userID, orderedGames)

	// 5. 打包返回给前端
	c.JSON(http.StatusOK, gin.H{
		"algorithm":       "ULTIM_Golang_V1_Reranked",
		"recommendations": rerankedGames,
	})
}

// rerankRecommendations 对推荐结果进行规则重排
// 策略：多样性重排 + 用户偏好加权 + 热门加权
func rerankRecommendations(userID string, games []gin.H) []gin.H {
	if len(games) == 0 {
		return games
	}

	// 1. 获取用户偏好类型
	genreKey := "user_genres:" + userID
	genreVal, err := database.RDB.Get(database.Ctx, genreKey).Result()
	var userGenres []string
	if err == nil {
		json.Unmarshal([]byte(genreVal), &userGenres)
	}

	// 2. 计算每个游戏的重排分数
	type scoredGame struct {
		index    int
		score    float64
		game     gin.H
	}

	var scoredGames []scoredGame
	for i, g := range games {
		score := 0.0

		// 基础分数：metacritic 归一化 (0-100 -> 0-1)
		if metacritic, ok := g["metacritic"].(int); ok {
			score += float64(metacritic) / 100.0 * 0.3
		}

		// 获取当前游戏的类型
		currentGenres := ""
		if gStr, ok := g["genres"].(string); ok {
			currentGenres = gStr
		}

		// 用户偏好加权：匹配用户喜欢的类型
		if currentGenres != "" && len(userGenres) > 0 {
			for _, ug := range userGenres {
				if strings.Contains(currentGenres, ug) {
					score += 0.4 // 偏好类型加权 40%
					break
				}
			}
		}

		// 多样性奖励：不同类型轮换
		// 已在前面的推荐中出现过的类型，减少分数
		for j := 0; j < i && j < 5; j++ {
			if prevGenres, ok := games[j]["genres"].(string); ok && currentGenres != "" {
				if strings.Contains(currentGenres, prevGenres) {
					score -= 0.1 // 同类型连续推荐，降低分数
				}
			}
		}

		scoredGames = append(scoredGames, scoredGame{
			index: i,
			score: score,
			game:  g,
		})
	}

	// 3. 按分数排序
	sort.Slice(scoredGames, func(i, j int) bool {
		return scoredGames[i].score > scoredGames[j].score
	})

	// 4. 返回重排后的结果
	var result []gin.H
	for _, sg := range scoredGames {
		result = append(result, sg.game)
	}

	return result
}

// 降级推荐：获取评分最高/热门的游戏
func getPopularGamesFallback() []gin.H {
	var games []models.GameMetadata
	database.DB.Order("metascore DESC NULLS LAST").Limit(20).Find(&games)

	var res []gin.H
	for _, game := range games {
		res = append(res, gin.H{
			"id":              game.ProductID,
			"appid":           strconv.Itoa(game.ProductID),
			"name":            game.Title,
			"title":           game.Title,
			"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(game.ProductID) + "/header.jpg",
			"metacritic":      game.Metascore,
			"genres":          stringToList(game.Genres),
		})
	}
	return res
}

// GetPopularGames handles GET /recommendations/popular
func GetPopularGames(c *gin.Context) {
	limit := c.DefaultQuery("limit", "20")
	genre := c.Query("genre")

	limitInt, _ := strconv.Atoi(limit)
	if limitInt <= 0 || limitInt > 100 {
		limitInt = 20
	}

	var games []models.GameMetadata
	query := database.DB.Order("metascore DESC NULLS LAST").Limit(limitInt)

	if genre != "" {
		query = query.Where("genres LIKE ?", "%"+genre+"%")
	}

	query.Find(&games)

	var result []gin.H
	for _, game := range games {
		result = append(result, gin.H{
			"id":              game.ProductID,
			"appid":           strconv.Itoa(game.ProductID),
			"name":            game.Title,
			"title":           game.Title,
			"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(game.ProductID) + "/header.jpg",
			"metacritic":      game.Metascore,
			"genres":          stringToList(game.Genres),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"games": result,
		"total": len(result),
	})
}

// GetTrendingGames handles GET /recommendations/trending
func GetTrendingGames(c *gin.Context) {
	limit := c.DefaultQuery("limit", "20")
	timeWindow := c.DefaultQuery("time_window", "weekly")

	limitInt, _ := strconv.Atoi(limit)
	if limitInt <= 0 || limitInt > 100 {
		limitInt = 20
	}

	// Try to get from Redis first
	redisKey := "trending:" + timeWindow
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()

	if err == nil {
		// Parse from Redis cache
		var appIDs []string
		json.Unmarshal([]byte(val), &appIDs)
		returnTrendingGames(c, appIDs, limitInt)
		return
	}

	// Fallback to PostgreSQL
	var games []models.GameMetadata
	database.DB.Order("metascore DESC NULLS LAST").Limit(limitInt).Find(&games)

	var result []gin.H
	for _, game := range games {
		result = append(result, gin.H{
			"id":              game.ProductID,
			"appid":           strconv.Itoa(game.ProductID),
			"name":            game.Title,
			"title":           game.Title,
			"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(game.ProductID) + "/header.jpg",
			"metacritic":      game.Metascore,
			"genres":          stringToList(game.Genres),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"games": result,
		"total": len(result),
		"time_window": timeWindow,
	})
}

func returnTrendingGames(c *gin.Context, appIDs []string, limit int) {
	if len(appIDs) > limit {
		appIDs = appIDs[:limit]
	}

	var intAppIDs []int
	for _, idStr := range appIDs {
		if id, err := strconv.Atoi(idStr); err == nil {
			intAppIDs = append(intAppIDs, id)
		}
	}

	var games []models.GameMetadata
	database.DB.Where("product_id IN ?", intAppIDs).Find(&games)

	gameMap := make(map[int]models.GameMetadata)
	for _, g := range games {
		gameMap[g.ProductID] = g
	}

	var result []gin.H
	for _, id := range intAppIDs {
		if game, exists := gameMap[id]; exists {
			result = append(result, gin.H{
				"id":              game.ProductID,
				"appid":           strconv.Itoa(game.ProductID),
				"name":            game.Title,
				"title":           game.Title,
				"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(game.ProductID) + "/header.jpg",
				"metacritic":      game.Metascore,
				"genres":          stringToList(game.Genres),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"games": result,
		"total": len(result),
	})
}

// GetSimilarGames handles GET /recommendations/similar/:productId
func GetSimilarGames(c *gin.Context) {
	productId := c.Param("productId")
	limit := c.DefaultQuery("limit", "10")

	limitInt, _ := strconv.Atoi(limit)
	if limitInt <= 0 || limitInt > 50 {
		limitInt = 10
	}

	productIDInt, err := strconv.Atoi(productId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid product_id"})
		return
	}

	// Get the source game
	var sourceGame models.GameMetadata
	if err := database.DB.Where("product_id = ?", productIDInt).First(&sourceGame).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}

	// Find similar games by genre
	var similarGames []models.GameMetadata
	database.DB.Where("genres LIKE ? AND product_id != ?", "%"+sourceGame.Genres+"%", productIDInt).
		Order("metascore DESC NULLS LAST").
		Limit(limitInt).
		Find(&similarGames)

	var result []gin.H
	for _, game := range similarGames {
		result = append(result, gin.H{
			"id":              game.ProductID,
			"appid":           strconv.Itoa(game.ProductID),
			"name":            game.Title,
			"title":           game.Title,
			"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(game.ProductID) + "/header.jpg",
			"metacritic":      game.Metascore,
			"genres":          stringToList(game.Genres),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"games": result,
		"total": len(result),
	})
}

// GetSimilarToOwned handles GET /recommendations/similar-to-owned/:steamId
func GetSimilarToOwned(c *gin.Context) {
	steamId := c.Param("steamId")
	topk := c.DefaultQuery("topk", "20")

	topkInt, _ := strconv.Atoi(topk)
	if topkInt <= 0 || topkInt > 50 {
		topkInt = 20
	}

	// Try to get user's owned games from Redis cache
	redisKey := "user_games:" + steamId
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()

	if err == nil {
		// User has cached game list
		var ownedAppIDs []string
		json.Unmarshal([]byte(val), &ownedAppIDs)

		// Get recommendations based on owned games
		var result []gin.H

		// For each owned game, find similar games
		seen := make(map[int]bool)
		for _, appIDStr := range ownedAppIDs {
			if len(result) >= topkInt {
				break
			}
			appID, _ := strconv.Atoi(appIDStr)
			seen[appID] = true

			var game models.GameMetadata
			if err := database.DB.Where("product_id = ?", appID).First(&game).Error; err != nil {
				continue
			}

			var similar []models.GameMetadata
			database.DB.Where("genres LIKE ? AND product_id NOT IN ?", "%"+game.Genres+"%", ownedAppIDs).
				Order("metascore DESC NULLS LAST").
				Limit(5).
				Find(&similar)

			for _, g := range similar {
				if !seen[g.ProductID] {
					seen[g.ProductID] = true
					result = append(result, gin.H{
						"id":               g.ProductID,
						"appid":            strconv.Itoa(g.ProductID),
						"name":             g.Title,
						"title":            g.Title,
						"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(g.ProductID) + "/header.jpg",
						"metacritic":       g.Metascore,
						"genres":           stringToList(g.Genres),
						"similar_to":       game.Title,
					})
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"games": result,
			"total": len(result),
		})
		return
	}

	// Fallback: return popular games
	var games []models.GameMetadata
	database.DB.Order("metascore DESC NULLS LAST").Limit(topkInt).Find(&games)

	var result []gin.H
	for _, game := range games {
		result = append(result, gin.H{
			"id":              game.ProductID,
			"appid":           strconv.Itoa(game.ProductID),
			"name":            game.Title,
			"title":           game.Title,
			"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(game.ProductID) + "/header.jpg",
			"metacritic":      game.Metascore,
			"genres":          stringToList(game.Genres),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"games": result,
		"total": len(result),
	})
}

// GetByGenre handles GET /recommendations/by-genre/:steamId
func GetByGenre(c *gin.Context) {
	steamId := c.Param("steamId")
	limit := c.DefaultQuery("limit", "20")

	limitInt, _ := strconv.Atoi(limit)
	if limitInt <= 0 || limitInt > 50 {
		limitInt = 20
	}

	// Try to get user's preferred genres from Redis
	redisKey := "user_genres:" + steamId
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()

	var genres []string
	if err == nil {
		json.Unmarshal([]byte(val), &genres)
	}

	// If no cached genres, use popular fallback
	if len(genres) == 0 {
		genres = []string{"Action", "Adventure", "RPG"}
	}

	var games []models.GameMetadata
	query := database.DB.Where("1 = 0")
	for _, genre := range genres {
		query = query.Or("genres LIKE ?", "%"+genre+"%")
	}

	query.Order("metascore DESC NULLS LAST").Limit(limitInt).Find(&games)

	var result []gin.H
	for _, game := range games {
		result = append(result, gin.H{
			"id":              game.ProductID,
			"appid":           strconv.Itoa(game.ProductID),
			"name":            game.Title,
			"title":           game.Title,
			"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(game.ProductID) + "/header.jpg",
			"metacritic":      game.Metascore,
			"genres":          stringToList(game.Genres),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"games": result,
		"total": len(result),
	})
}

// GetPopularNotOwned handles GET /recommendations/popular-not-owned/:steamId
func GetPopularNotOwned(c *gin.Context) {
	steamId := c.Param("steamId")
	limit := c.DefaultQuery("limit", "20")
	offset := c.DefaultQuery("offset", "0")
	genre := c.Query("genre")

	limitInt, _ := strconv.Atoi(limit)
	offsetInt, _ := strconv.Atoi(offset)
	if limitInt <= 0 || limitInt > 50 {
		limitInt = 20
	}

	// Get user's owned games
	redisKey := "user_games:" + steamId
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()

	var ownedAppIDs []int
	if err == nil {
		var ownedStr []string
		json.Unmarshal([]byte(val), &ownedStr)
		for _, idStr := range ownedStr {
			if id, err := strconv.Atoi(idStr); err == nil {
				ownedAppIDs = append(ownedAppIDs, id)
			}
		}
	}

	// Get popular games not owned
	var games []models.GameMetadata
	query := database.DB.Order("metascore DESC NULLS LAST")

	// Filter by genre if provided
	if genre != "" {
		query = query.Where("genres LIKE ?", "%"+genre+"%")
	}

	if len(ownedAppIDs) > 0 {
		query = query.Where("product_id NOT IN ?", ownedAppIDs)
	}

	query.Offset(offsetInt).Limit(limitInt).Find(&games)

	// Get total count (with genre filter)
	var total int64
	countQuery := database.DB.Model(&models.GameMetadata{})
	if genre != "" {
		countQuery = countQuery.Where("genres LIKE ?", "%"+genre+"%")
	}
	if len(ownedAppIDs) > 0 {
		countQuery = countQuery.Where("product_id NOT IN ?", ownedAppIDs)
	}
	countQuery.Count(&total)

	var result []gin.H
	for _, game := range games {
		result = append(result, gin.H{
			"id":              game.ProductID,
			"appid":           strconv.Itoa(game.ProductID),
			"name":            game.Title,
			"title":           game.Title,
			"background_image": "https://steamcdn-a.akamaihd.net/steam/apps/" + strconv.Itoa(game.ProductID) + "/header.jpg",
			"metacritic":      game.Metascore,
			"genres":          stringToList(game.Genres),
		})
	}

	hasMore := (offsetInt + limitInt) < int(total)

	c.JSON(http.StatusOK, gin.H{
		"games":     result,
		"total":     total,
		"has_more":  hasMore,
	})
}

// GetRecommendationExplanation handles GET /recommendations/explanation
func GetRecommendationExplanation(c *gin.Context) {
	productID := c.Query("product_id")
	userID := c.Query("user_id")

	// Dummy response
	c.JSON(http.StatusOK, gin.H{
		"product_id": productID,
		"user_id":    userID,
		"explanation": []string{
			"Because you played similar games",
			"High metacritic score",
			"Matches your preferred genres",
		},
	})
}

// GetRecommendationStats handles GET /recommendations/stats
func GetRecommendationStats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"total_recommendations_served": 15000,
		"popular_genres": []string{
			"Action", "RPG", "Strategy",
		},
		"algorithm_version": "ULTIM_Golang_V1",
	})
}
