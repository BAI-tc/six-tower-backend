package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"ultim_api_go/database"
	"ultim_api_go/models"

	"github.com/gin-gonic/gin"
)

// stringToList 解析数据库中逗号分隔的字符串为数组
func stringToList(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// GetGameDetail 获取单个游戏的详情信息
func GetGameDetail(c *gin.Context) {
	appIDParam := c.Param("app_id")
	productID, err := strconv.Atoi(appIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid app_id format"})
		return
	}

	var game models.GameMetadata
	if err := database.DB.Where("product_id = ?", productID).First(&game).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Game not found"})
		return
	}

	title := game.Title
	if game.AppName != "" {
		title = game.AppName
	} else if title == "" {
		title = strconv.Itoa(game.ProductID)
	}
	appName := game.AppName
	if appName == "" {
		appName = title
	}

	discountPrice := 0.0
	discountPercent := 0
	if game.DiscountPrice != nil {
		discountPrice = *game.DiscountPrice
		if game.Price != nil && *game.Price > 0 {
			discountPercent = int((1 - discountPrice/(*game.Price)) * 100)
		}
	}

	price := 0.0
	if game.Price != nil {
		price = *game.Price
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"app_id":            strconv.Itoa(game.ProductID),
			"app_name":          appName,
			"description":       game.Description,
			"short_description": game.ShortDescription,
			"genres":            stringToList(game.Genres),
			"tags":              stringToList(game.Tags),
			"developer":         game.Developer,
			"publisher":         game.Publisher,
			"release_date":      game.ReleaseDate,
			"price":             price,
			"discount_price":    discountPrice,
			"discount_percent":  discountPercent,
			"specs":             stringToList(game.Specs),
			"store_url":         "https://store.steampowered.com/app/" + strconv.Itoa(game.ProductID),
			"reviews_url":       "https://store.steampowered.com/app/" + strconv.Itoa(game.ProductID) + "#reviews",
			"early_access":      game.EarlyAccess,
		},
	})
}

// GetGamesList 获取游戏列表（发现页）
func GetGamesList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit > 50 {
		limit = 50
	} else if limit < 1 {
		limit = 20
	}

	genre := c.Query("genre")
	tags := c.Query("tags")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "popular")
	priceMinStr := c.Query("price_min")
	priceMaxStr := c.Query("price_max")

	dbQuery := database.DB.Model(&models.GameMetadata{})

	// 筛选条件
	if genre != "" {
		dbQuery = dbQuery.Where("genres LIKE ?", "%"+genre+"%")
	}

	if tags != "" {
		tagList := stringToList(tags)
		if len(tagList) > 0 {
			tagQuery := database.DB
			for _, t := range tagList {
				tagQuery = tagQuery.Or("tags LIKE ?", "%"+t+"%")
			}
			dbQuery = dbQuery.Where(tagQuery)
		}
	}

	if search != "" {
		dbQuery = dbQuery.Where("app_name LIKE ? OR description LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if priceMinStr != "" {
		if pMin, err := strconv.ParseFloat(priceMinStr, 64); err == nil {
			dbQuery = dbQuery.Where("price >= ?", pMin)
		}
	}
	if priceMaxStr != "" {
		if pMax, err := strconv.ParseFloat(priceMaxStr, 64); err == nil {
			dbQuery = dbQuery.Where("price <= ?", pMax)
		}
	}

	var total int64
	dbQuery.Count(&total)

	// 排序
	switch sortBy {
	case "newest":
		dbQuery = dbQuery.Order("release_date DESC")
	case "price_asc":
		dbQuery = dbQuery.Order("price ASC")
	case "price_desc":
		dbQuery = dbQuery.Order("price DESC")
	case "rating":
		dbQuery = dbQuery.Order("metascore DESC NULLS LAST")
	default: // popular
		dbQuery = dbQuery.Order("product_id DESC")
	}

	offset := (page - 1) * limit
	var games []models.GameMetadata
	dbQuery.Offset(offset).Limit(limit).Find(&games)

	var gameItems []gin.H
	for _, game := range games {
		title := game.Title
		if game.AppName != "" {
			title = game.AppName
		} else if title == "" {
			title = strconv.Itoa(game.ProductID)
		}
		appName := game.AppName
		if appName == "" {
			appName = title
		}

		price := 0.0
		if game.Price != nil {
			price = *game.Price
		}
		discountPrice := 0.0
		if game.DiscountPrice != nil {
			discountPrice = *game.DiscountPrice
		}

		gameItems = append(gameItems, gin.H{
			"app_id":         strconv.Itoa(game.ProductID),
			"app_name":       appName,
			"genres":         stringToList(game.Genres),
			"tags":           stringToList(game.Tags),
			"price":          price,
			"discount_price": discountPrice,
			"developer":      game.Developer,
			"publisher":      game.Publisher,
			"release_date":   game.ReleaseDate,
			"specs":          stringToList(game.Specs),
			"early_access":   game.EarlyAccess,
		})
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	hasMore := page < totalPages

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"games": gameItems,
			"pagination": gin.H{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": totalPages,
				"has_more":    hasMore,
			},
			"filters_applied": gin.H{
				"genre":     genre,
				"tags":      tags,
				"search":    search,
				"sort_by":   sortBy,
				"price_min": priceMinStr,
				"price_max": priceMaxStr,
			},
		},
	})
}

// GetGenres 获取所有可用的游戏品类列表
func GetGenres(c *gin.Context) {
	// 略：为防止每次遍历全表慢，可以直接去缓存或者使用原生聚合 SQL 
	// 为了确保一致性及性能要求，在此实现基础聚合逻辑：
	type Result struct {
		Genres string
	}
	var results []Result
	database.DB.Model(&models.GameMetadata{}).Select("genres").Where("genres IS NOT NULL").Find(&results)

	genreCount := make(map[string]int)
	for _, r := range results {
		gList := stringToList(r.Genres)
		for _, g := range gList {
			genreCount[g]++
		}
	}

	var genreResp []gin.H
	for name, count := range genreCount {
		genreResp = append(genreResp, gin.H{
			"id":    strings.ToLower(strings.ReplaceAll(name, " ", "_")),
			"name":  name,
			"count": count,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"genres": genreResp,
		},
	})
}

// GetTags 获取所有可用的游戏标签列表
func GetTags(c *gin.Context) {
	type Result struct {
		Tags string
	}
	var results []Result
	database.DB.Model(&models.GameMetadata{}).Select("tags").Where("tags IS NOT NULL").Find(&results)

	tagCount := make(map[string]int)
	for _, r := range results {
		tList := stringToList(r.Tags)
		for _, t := range tList {
			tagCount[t]++
		}
	}

	var tagResp []gin.H
	for name, count := range tagCount {
		tagResp = append(tagResp, gin.H{
			"id":    strings.ToLower(strings.ReplaceAll(name, " ", "_")),
			"name":  name,
			"count": count,
		})
	}
	// 在生产环境中应对 tagCount 进行排序并取前 50，此为基础功能。

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"tags": tagResp, // TODO: Sort to top 50 in production
		},
	})
}
