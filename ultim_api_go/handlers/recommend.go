package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"ultim_api_go/config"
	"ultim_api_go/database"
	"ultim_api_go/models"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// getBackgroundImage 获取游戏背景图片，优先使用 RAWG 图片缓存
func getBackgroundImage(game models.GameMetadata) string {
	// 优先使用新的图片缓存系统
	appID := strconv.Itoa(game.ProductID)
	return GetGameImage(appID)
}

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
				"background_image": GetGameImage(id),
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
			"background_image": getBackgroundImage(game),
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
			"background_image": getBackgroundImage(game),
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
			"background_image": getBackgroundImage(game),
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
				"background_image": GetGameImage(strconv.Itoa(id)),
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
			"background_image": getBackgroundImage(game),
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
						"background_image": getBackgroundImage(g),
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
			"background_image": getBackgroundImage(game),
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
			"background_image": getBackgroundImage(game),
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
	if err == nil && val != "" {
		var ownedStr []string
		json.Unmarshal([]byte(val), &ownedStr)
		for _, idStr := range ownedStr {
			if id, err := strconv.Atoi(idStr); err == nil {
				ownedAppIDs = append(ownedAppIDs, id)
		}
		}
	}

	// 如果Redis缓存为空，从Steam API获取
	if len(ownedAppIDs) == 0 {
		log.Printf("⚠️ Redis cache empty for user %s, fetching from Steam API...", steamId)
		cfg := config.LoadConfig()
		steamHandler := NewSteamHandler(&cfg)
		if gamesData, err := steamHandler.getOwnedGames(steamId, false); err == nil {
			if games, ok := gamesData["games"].([]interface{}); ok {
				for _, g := range games {
					if gameMap, ok := g.(map[string]interface{}); ok {
						if appid, ok := gameMap["appid"].(float64); ok {
							ownedAppIDs = append(ownedAppIDs, int(appid))
						}
					}
				}
				log.Printf("✅ Fetched %d owned games from Steam API", len(ownedAppIDs))
			}
		} else {
			log.Printf("❌ Failed to fetch owned games from Steam API: %v", err)
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
			"background_image": getBackgroundImage(game),
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

// SixTowerWeights 六塔融合权重配置
type SixTowerWeights struct {
	S_SVD  float64 `json:"svd"`  // 协同过滤权重
	S_Sem  float64 `json:"sem"`  // 语义推荐权重
	S_Pop  float64 `json:"pop"`  // 热门推荐权重
	S_Prof float64 `json:"prof"` // 用户画像/聚类权重
	S_ICF  float64 `json:"icf"`  // 物品协同过滤权重
	S_CP   float64 `json:"cp"`   // 聚类热门权重
}

// DefaultSixTowerWeights 返回默认权重配置
func DefaultSixTowerWeights() SixTowerWeights {
	return SixTowerWeights{
		S_SVD:  1.2,
		S_Sem:  0.5,
		S_Pop:  1.5,
		S_Prof: 0.2,
		S_ICF:  0.1,
		S_CP:   0.05,
	}
}

// getUserOwnedGames 获取用户拥有的游戏列表
func getUserOwnedGames(userID string) map[int]bool {
	ownedMap := make(map[int]bool)

	if userID == "" {
		return ownedMap
	}

	redisKey := "user_games:" + userID
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()

	if err == nil {
		var ownedAppIDs []string
		if json.Unmarshal([]byte(val), &ownedAppIDs) == nil {
			for _, idStr := range ownedAppIDs {
				if id, err := strconv.Atoi(idStr); err == nil {
					ownedMap[id] = true
				}
			}
		}
	}

	return ownedMap
}

// callULTIMAPI 调用 Python 融合引擎 API
func callULTIMAPI(steamID string, weights *SixTowerWeights, topK int, offset int) []gin.H {
	pythonAPIURL := config.LoadConfig().PythonAPIURL

	url := fmt.Sprintf("%s/recommend", pythonAPIURL)

	var jsonBody string
	if weights != nil {
		jsonBody = fmt.Sprintf(`{
			"steam_id": "%s",
			"top_k": %d,
			"offset": %d,
			"weight_svd": %.2f,
			"weight_sem": %.2f,
			"weight_pop": %.2f,
			"weight_prof": %.2f,
			"weight_icf": %.2f,
			"weight_cp": %.2f
		}`, steamID, topK, offset, weights.S_SVD, weights.S_Sem, weights.S_Pop, weights.S_Prof, weights.S_ICF, weights.S_CP)
	} else {
		jsonBody = fmt.Sprintf(`{"steam_id": "%s", "top_k": %d, "offset": %d}`, steamID, topK, offset)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(jsonBody)))
	if err != nil {
		fmt.Printf("ULTIM API: Failed to create request: %v\n", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("ULTIM API: Failed to call Python API: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("ULTIM API: Python API returned status %d\n", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("ULTIM API: Failed to read response: %v\n", err)
		return nil
	}

	var result struct {
		Recommendations []string `json:"recommendations"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("ULTIM API: Failed to parse response: %v\n", err)
		return nil
	}

	if len(result.Recommendations) == 0 {
		return nil
	}

	var intAppIDs []int
	for _, idStr := range result.Recommendations {
		if id, err := strconv.Atoi(idStr); err == nil {
			intAppIDs = append(intAppIDs, id)
		}
	}

	if len(intAppIDs) == 0 {
		return nil
	}

	var games []models.GameMetadata
	database.DB.Where("product_id IN ?", intAppIDs).Find(&games)

	gameMap := make(map[int]models.GameMetadata)
	for _, g := range games {
		gameMap[g.ProductID] = g
	}

	var res []gin.H
	for _, appIDStr := range result.Recommendations {
		appID, err := strconv.Atoi(appIDStr)
		if err != nil {
			continue
		}
		if game, exists := gameMap[appID]; exists {
			res = append(res, gin.H{
				"id":               game.ProductID,
				"appid":            strconv.Itoa(game.ProductID),
				"name":             game.Title,
				"title":            game.Title,
				"background_image": GetGameImage(appIDStr),
				"metacritic":       game.Metascore,
				"genres":           stringToList(game.Genres),
			})
		}
	}

	return res
}

// GetWeightedRecommendations 处理带权重的推荐请求
// GET /api/v1/recommendations/weighted?user_id=xxx&topk=20&weight_svd=1.5&weight_sem=0.5&weight_pop=2.5...
func GetWeightedRecommendations(c *gin.Context) {
	userID := c.Query("user_id")
	topkStr := c.DefaultQuery("topk", "20")
	offsetStr := c.DefaultQuery("offset", "0")

	topk, _ := strconv.Atoi(topkStr)
	if topk <= 0 || topk > 100 {
		topk = 20
	}

	offset, _ := strconv.Atoi(offsetStr)
	if offset < 0 {
		offset = 0
	}

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	// 获取自定义权重
	weights := SixTowerWeights{}
	hasCustomWeights := false

	if svdStr := c.Query("weight_svd"); svdStr != "" {
		if svd, err := strconv.ParseFloat(svdStr, 64); err == nil {
			weights.S_SVD = svd
			hasCustomWeights = true
		}
	}
	if semStr := c.Query("weight_sem"); semStr != "" {
		if sem, err := strconv.ParseFloat(semStr, 64); err == nil {
			weights.S_Sem = sem
			hasCustomWeights = true
		}
	}
	if popStr := c.Query("weight_pop"); popStr != "" {
		if pop, err := strconv.ParseFloat(popStr, 64); err == nil {
			weights.S_Pop = pop
			hasCustomWeights = true
		}
	}
	if profStr := c.Query("weight_prof"); profStr != "" {
		if prof, err := strconv.ParseFloat(profStr, 64); err == nil {
			weights.S_Prof = prof
			hasCustomWeights = true
		}
	}
	if icfStr := c.Query("weight_icf"); icfStr != "" {
		if icf, err := strconv.ParseFloat(icfStr, 64); err == nil {
			weights.S_ICF = icf
			hasCustomWeights = true
		}
	}
	if cpStr := c.Query("weight_cp"); cpStr != "" {
		if cp, err := strconv.ParseFloat(cpStr, 64); err == nil {
			weights.S_CP = cp
			hasCustomWeights = true
		}
	}

	// 如果没有自定义权重，使用默认值
	if !hasCustomWeights {
		weights = DefaultSixTowerWeights()
	}

	// 检查用户游戏库数量
	ownedGames := getUserOwnedGames(userID)
	gameCount := len(ownedGames)

	// 冷启动用户调整权重
	if gameCount < 5 && gameCount > 0 {
		weights.S_Pop += 0.4
		weights.S_CP += 0.2
		weights.S_SVD *= 0.7
		fmt.Printf("冷启动用户 %s (游戏数: %d)，软化权重策略\n", userID, gameCount)
	} else if gameCount == 0 {
		weights.S_Pop += 0.8
		weights.S_CP += 0.4
		weights.S_SVD = 0.0
		fmt.Printf("新用户 %s (游戏数: 0)，增加召回候选以便多样化\n", userID)
	}

	// 加大缓冲区
	bufferTopK := topk * 5
	if gameCount == 0 && bufferTopK < 150 {
		bufferTopK = 150
	} else if bufferTopK < 100 {
		bufferTopK = 100
	}

	recommendations := callULTIMAPI(userID, &weights, bufferTopK, offset)

	if len(recommendations) == 0 {
		recommendations = getPopularGamesFallback()
	}

	// 规则重排
	if len(recommendations) > 0 {
		recommendations = rerankRecommendations(userID, recommendations)
	}

	c.JSON(http.StatusOK, gin.H{
		"algorithm":        "Six_Tower_Weighted",
		"weights":          weights,
		"source":           "custom",
		"recommendations":  recommendations,
	})
}

// PlayerPreferenceResult 玩家偏好结果
type PlayerPreferenceResult struct {
	PlayerID           string                   `json:"playerid"`
	OwnedGamesCount    int                      `json:"owned_games_count"`
	MatchedGamesCount  int                      `json:"matched_games_count"`
	TopPreferences    []string                 `json:"top_preferences"`
	Recommendations    map[string][]GameRecommend `json:"recommendations"`
}

// GameRecommend 推荐游戏
type GameRecommend struct {
	GameID     string  `json:"gameid"`
	Title      string  `json:"title"`
	Metacritic float64 `json:"metacritic"`
}

// GetPlayerPreference 获取玩家语义偏好并推荐游戏
// 前端调用: GET /api/v1/recommendations/player-preference?steam_id=xxx
func GetPlayerPreference(c *gin.Context) {
	steamID := c.Query("steam_id")
	if steamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "steam_id is required"})
		return
	}

	var result *PlayerPreferenceResult
	fromCache := false

	// 1. 尝试从 Redis 获取缓存
	redisKey := "player_preference:" + steamID
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()

	if err == nil {
		var cachedResult PlayerPreferenceResult
		if err := json.Unmarshal([]byte(val), &cachedResult); err == nil {
			result = &cachedResult
			fromCache = true
		}
	}

	if result == nil {
		result = callPlayerPreferencePython(steamID)
		if result == nil {
			// 返回空结果而不是500错误
			log.Printf("⚠️ Player preference Python script failed, returning empty result for steamID: %s", steamID)
			result = &PlayerPreferenceResult{}
		} else {
			jsonBytes, _ := json.Marshal(result)
			database.RDB.Set(database.Ctx, redisKey, string(jsonBytes), 24*time.Hour)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"algorithm":  "PlayerPreference",
		"data":       result,
		"from_cache": fromCache,
	})
}

// callPlayerPreferencePython 调用Python脚本获取玩家偏好
func callPlayerPreferencePython(steamID string) *PlayerPreferenceResult {
	cfg := config.LoadConfig()
	pythonScript := cfg.ProjectRoot + "/ULTIM/step10b_player_preference.py"

	// 设置超时 30 秒
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.PythonPath, pythonScript, steamID, "--json", "--topn", "30")
	cmd.Dir = cfg.ProjectRoot + "/ULTIM"

	stdout, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Printf("Python script timeout for steamID: %s\n", steamID)
		} else {
			fmt.Printf("Error running Python script: %v\nOutput: %s\n", err, string(stdout))
		}
		return nil
	}

	var result PlayerPreferenceResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		fmt.Printf("Error parsing JSON: %v\nOutput: %s\n", err, string(stdout))
		return nil
	}

	return &result
}

// ========== 缺失的推荐处理函数及类型 (从旧后端迁移) ==========

type GalaxyInfo struct {
	GalaxyID   int    `json:"galaxy_id"`
	GalaxyName string `json:"galaxy_name"`
	DNA        string `json:"dna"` // 游戏基因描述
}

type TribeInfo struct {
	Name        string `json:"name"`
	Badge       string `json:"badge"`
	Description string `json:"description"`
}

// GetRecommendations 通用推荐接口，支持所有算法类型
func GetRecommendations(c *gin.Context) {
	userID := c.Query("user_id")
	algorithm := c.DefaultQuery("algorithm", "ultim")
	topk := c.DefaultQuery("topk", "20")

	topkInt, _ := strconv.Atoi(topk)
	if topkInt <= 0 || topkInt > 50 {
		topkInt = 20
	}

	switch algorithm {
	case "ultim":
		if userID != "" {
			redisKey := "ultim_recom:" + userID
			val, err := database.RDB.Get(database.Ctx, redisKey).Result()
			if err == redis.Nil || err != nil {
				recommendations := callULTIMAPI(userID, nil, 50, 0)
				if recommendations != nil {
					c.JSON(http.StatusOK, gin.H{
						"recommendations": recommendations,
						"algorithm":       "ULTIM_Realtime",
					})
					return
				}
				recommendations = getPopularGamesFallback()
				c.JSON(http.StatusOK, gin.H{
					"recommendations": recommendations,
					"algorithm":       "popular_fallback",
				})
				return
			}
			var appIDs []string
			json.Unmarshal([]byte(val), &appIDs)
			recommendations := getGameDetails(appIDsToIDs(appIDs))
			c.JSON(http.StatusOK, gin.H{
				"recommendations": convertToGinH(recommendations),
				"algorithm":       "ULTIM_Golang_" + algorithm,
			})
			return
		}
		fallthrough
	default:
		games := getPopularGamesFallback()
		c.JSON(http.StatusOK, gin.H{
			"recommendations": games,
			"algorithm":       "popularity",
		})
	}
}

// GetNewReleases 获取最新发行的游戏
func GetNewReleases(c *gin.Context) {
	limit := c.DefaultQuery("limit", "20")
	limitInt, _ := strconv.Atoi(limit)

	var games []models.GameMetadata
	database.DB.Order("release_date DESC NULLS LAST").Limit(limitInt).Find(&games)

	c.JSON(http.StatusOK, gin.H{
		"games": convertToGinH(games),
		"total": len(games),
	})
}

// GetPopularByTheme 获取按主题分类的热门游戏
func GetPopularByTheme(c *gin.Context) {
	theme := c.Query("theme")
	limit := c.DefaultQuery("limit", "20")
	limitInt, _ := strconv.Atoi(limit)

	var games []models.GameMetadata
	database.DB.Where("tags LIKE ?", "%"+theme+"%").Order("metascore DESC NULLS LAST").Limit(limitInt).Find(&games)

	c.JSON(http.StatusOK, gin.H{
		"games": convertToGinH(games),
		"total": len(games),
		"theme": theme,
	})
}

// GetSixTowerWeights 获取当前六塔模型权重配置
func GetSixTowerWeights(c *gin.Context) {
	c.JSON(http.StatusOK, DefaultSixTowerWeights())
}

// SetSixTowerWeights 设置六塔模型权重配置
func SetSixTowerWeights(c *gin.Context) {
	var weights SixTowerWeights
	if err := c.ShouldBindJSON(&weights); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Weights updated locally for this request"})
}

// ResetSixTowerWeights 重置六塔模型权重配置
func ResetSixTowerWeights(c *gin.Context) {
	c.JSON(http.StatusOK, DefaultSixTowerWeights())
}

// GetSceneInfo 获取用户的场景信息
func GetSceneInfo(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	clusterID := getUserClusterID(userID)
	galaxyID := getUserGalaxyID(userID)
	tribeInfo := getClusterTribeInfo(clusterID)
	galaxyInfo := getGalaxyInfo(galaxyID)

	c.JSON(http.StatusOK, gin.H{
		"scene_id":      2,
		"player_cluster": clusterID,
		"galaxy_id":     galaxyID,
		"tribe_info":    tribeInfo,
		"galaxy_info":   galaxyInfo,
		"available_scenes": []gin.H{
			{"scene_id": 1, "name": "Because You Like", "name_zh": "因为你喜欢", "type": "personalized"},
			{"scene_id": 2, "name": "Popular in Your Genre", "name_zh": "同类型热门", "type": "galaxy"},
			{"scene_id": 3, "name": "Players Like You Also Play", "name_zh": "和你一样的玩家也在玩", "type": "tribe"},
		},
	})
}

// GetSceneRecommendation 根据场景ID返回推荐
func GetSceneRecommendation(c *gin.Context) {
	userID := c.Query("user_id")
	sceneID := c.DefaultQuery("scene_id", "2")
	topk := c.DefaultQuery("topk", "20")
	topkInt, _ := strconv.Atoi(topk)

	switch sceneID {
	case "2":
		GetGalaxyRecommendation(c)
	case "3":
		getTribeRecommendation(c, userID, topkInt)
	default:
		c.JSON(http.StatusOK, getPopularGamesFallback())
	}
}

// helper functions and supporting logic
func appIDsToIDs(appIDs []string) []int {
	var ids []int
	for _, idStr := range appIDs {
		if id, err := strconv.Atoi(idStr); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func convertToGinH(games []models.GameMetadata) []gin.H {
	var res []gin.H
	for _, g := range games {
		res = append(res, gin.H{
			"id":               g.ProductID,
			"appid":            strconv.Itoa(g.ProductID),
			"name":             g.Title,
			"background_image": getBackgroundImage(g),
			"metacritic":       g.Metascore,
		})
	}
	return res
}

func getUserClusterID(userID string) int {
	if userID == "" {
		return -1
	}
	redisKey := "user_cluster:" + userID
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()
	if err == nil {
		clusterID, _ := strconv.Atoi(val)
		return clusterID
	}
	hash := 0
	for _, c := range userID {
		hash = hash*31 + int(c)
	}
	return hash % 6
}

func getClusterTribeInfo(clusterID int) TribeInfo {
	tribeNames := map[int]TribeInfo{
		0: {"策略大师部落", "strategy_master", "热爱策略游戏的智者"},
		1: {"速通狂人部落", "speedrunner", "追求极致通关速度"},
		2: {"剧情探索者部落", "story_explorer", "沉浸在游戏剧情中"},
		3: {"多人竞技部落", "multiplayer_warrior", "热爱PVP对抗"},
		4: {"独立游戏部落", "indie_lover", "支持独立游戏开发"},
		5: {"休闲益智部落", "casual_puzzle", "轻松愉快的游戏体验"},
	}
	if info, ok := tribeNames[clusterID]; ok {
		return info
	}
	return TribeInfo{Name: "探索者部落", Badge: "explorer", Description: "游戏爱好者"}
}

func getClusterHotGames(clusterID int) []int {
	redisKey := "cluster_hot:" + strconv.Itoa(clusterID)
	val, err := database.RDB.Get(database.Ctx, redisKey).Result()
	if err == nil {
		var appIDs []int
		if json.Unmarshal([]byte(val), &appIDs) == nil {
			return appIDs
		}
	}
	return []int{}
}

func getClusterPlayerCount(clusterID int) int {
	return 2000 + (clusterID * 150)
}

func getItemCFRecommendations(userID string, clusterHotGames []int, ownedGames map[int]bool, topk int) []int {
	return []int{} // 简化版
}

func getUserLikedGames(userID string) []int {
	ownedGames := getUserOwnedGames(userID)
	var ownedList []int
	for appID := range ownedGames {
		ownedList = append(ownedList, appID)
	}
	if len(ownedList) > 10 {
		return ownedList[:10]
	}
	return ownedList
}

func getGameDetails(appIDs []int) []models.GameMetadata {
	if len(appIDs) == 0 {
		return []models.GameMetadata{}
	}
	var games []models.GameMetadata
	database.DB.Where("product_id IN ?", appIDs).Find(&games)
	gameMap := make(map[int]models.GameMetadata)
	for _, g := range games {
		gameMap[g.ProductID] = g
	}
	var ordered []models.GameMetadata
	for _, id := range appIDs {
		if game, exists := gameMap[id]; exists {
			ordered = append(ordered, game)
		}
	}
	return ordered
}

// GetGalaxyRecommendation 根据锚点游戏推荐星系游戏
func GetGalaxyRecommendation(c *gin.Context) {
	userID := c.Query("user_id")
	anchorGameID := c.Query("anchor_game_id")
	topkStr := c.DefaultQuery("topk", "20")
	topk, _ := strconv.Atoi(topkStr)

	var anchorID int
	if anchorGameID != "" {
		anchorID, _ = strconv.Atoi(anchorGameID)
	}
	if anchorID == 0 && userID != "" {
		anchorID = getUserFavoriteGame(userID)
	}
	if anchorID == 0 {
		c.JSON(http.StatusOK, gin.H{"recommendations": getPopularGamesFallback()})
		return
	}
	galaxyID := getGameGalaxyID(anchorID)
	galaxyInfo := getGalaxyInfo(galaxyID)
	candidateGames := getGalaxyGames(galaxyID)
	ownedGames := getUserOwnedGames(userID)
	var filteredGames []int
	for _, appID := range candidateGames {
		if !ownedGames[appID] {
			filteredGames = append(filteredGames, appID)
		}
		if len(filteredGames) >= topk {
			break
		}
	}
	games := getGameDetails(filteredGames)
	c.JSON(http.StatusOK, gin.H{
		"algorithm":       "galaxy_traverse",
		"scene_name_zh":   "同类型热门",
		"galaxy_info":     galaxyInfo,
		"recommendations": convertToGinH(games),
	})
}

func getUserFavoriteGame(userID string) int {
	return 0
}

func getGameGalaxyID(gameID int) int {
	var game models.GameMetadata
	if err := database.DB.Where("product_id = ?", gameID).First(&game).Error; err == nil {
		return hashToCluster(game.Tags, 9) + 1
	}
	return 1
}

func getGalaxyInfo(galaxyID int) GalaxyInfo {
	galaxyNames := map[int]GalaxyInfo{
		1: {GalaxyID: 1, GalaxyName: "高画质·硬核·开放世界", DNA: "高画质·硬核·开放世界"},
		2: {GalaxyID: 2, GalaxyName: "休闲·益智·独立游戏", DNA: "休闲·益智·独立游戏"},
		3: {GalaxyID: 3, GalaxyName: "竞技·FPS·多人对战", DNA: "竞技·FPS·多人对战"},
		4: {GalaxyID: 4, GalaxyName: "RPG·剧情·角色扮演", DNA: "RPG·剧情·角色扮演"},
		5: {GalaxyID: 5, GalaxyName: "策略·经营·模拟经营", DNA: "策略·经营·模拟经营"},
	}
	if info, ok := galaxyNames[galaxyID]; ok {
		return info
	}
	return GalaxyInfo{GalaxyID: galaxyID, GalaxyName: "未分类", DNA: "未分类"}
}

func getGalaxyGames(galaxyID int) []int {
	return []int{1091500, 281990, 1174180}
}

func getUserGalaxyID(userID string) int {
	return 1
}

func hashToCluster(tags string, numClusters int) int {
	if tags == "" {
		return 0
	}
	hash := 0
	for _, ch := range tags {
		hash = hash*31 + int(ch)
	}
	return ((hash % numClusters) + numClusters) % numClusters
}

func getTribeRecommendation(c *gin.Context, userID string, topk int) {
	clusterID := getUserClusterID(userID)
	tribeInfo := getClusterTribeInfo(clusterID)
	clusterHotGames := getClusterHotGames(clusterID)
	if len(clusterHotGames) == 0 {
		c.JSON(http.StatusOK, gin.H{"recommendations": getPopularGamesFallback(), "tribe_info": tribeInfo})
		return
	}
	ownedGames := getUserOwnedGames(userID)
	var filteredGames []int
	for _, appID := range clusterHotGames {
		if !ownedGames[appID] {
			filteredGames = append(filteredGames, appID)
		}
		if len(filteredGames) >= topk {
			break
		}
	}
	games := getGameDetails(filteredGames)
	c.JSON(http.StatusOK, gin.H{
		"algorithm":      "tribe_echo",
		"scene_name_zh": "和你一样的玩家也在玩",
		"tribe_info":    tribeInfo,
		"recommendations": convertToGinH(games),
	})
}
