package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"ultim_api_go/database"

	"github.com/gin-gonic/gin"
)

const (
	STEAM_CDN_HEADER      = "https://steamcdn-a.akamaihd.net/steam/apps/%s/header.jpg"
	RAWG_IMAGE_CACHE_KEY  = "rawg:image:"      // Redis key prefix for image cache: rawg:image:{appid}
	POPULAR_GAMES_KEY     = "popular_games"    // Redis key for popular games list
)

// Popular game app IDs for preloading (top 100 most popular Steam games)
var popularSteamAppIDs = []int{
	730, 578080, 271590, 218620, 1091500, 552520, 814380, 1174180, 1551360, 1888930,
	252490, 294100, 440, 36120, 505460, 55230, 570, 250900, 230410, 2870,
	582010, 1604030, 1086940, 1593500, 1462040, 1332010, 1158310, 1817070, 1947500, 756800,
	1313140, 1817190, 990080, 1519490, 1599760, 1497950, 1328670, 1057090, 1468810, 1506830,
	1313140, 1332010, 1551360, 1593500, 1599760, 1627720, 1659520, 1682960, 1729740, 1739080,
	1760300, 1778820, 1782210, 1794680, 1806570, 1817070, 1829710, 1835810, 1841410, 1846440,
	1850570, 1863830, 1875930, 1888930, 1897110, 1905840, 1919590, 1938090, 1948210, 1953450,
	1963520, 1973610, 1978420, 1984330, 1996240, 2010730, 2027510, 2036800, 2050650, 2060140,
	2073850, 2083180, 2087580, 2096570, 2100540, 2108330, 2124120, 2136750, 2144890, 2146760,
	2153350, 2161040, 2167760, 2172690, 2185100, 218620, 2195250, 2202430, 2213800, 2222930,
}

// RAWGImageCache manages RAWG image caching
type RAWGImageCache struct {
	mu           sync.RWMutex
	imageCache   map[string]string // appid -> image URL
	steamToRAWG  map[string]int   // steam appid -> RAWG game id
}

// Global image cache instance
var imageCache = &RAWGImageCache{
	imageCache:  make(map[string]string),
	steamToRAWG: make(map[string]int),
}

// InitImageCache initializes the image cache by preloading popular games
func InitImageCache() {
	log.Println("[ImageCache] Starting image cache initialization...")

	// First, try to load from Redis
	if err := loadCacheFromRedis(); err != nil {
		log.Printf("[ImageCache] Failed to load from Redis: %v, will fetch from RAWG", err)
		// If Redis load fails, fetch from RAWG
		preloadPopularGames()
	}

	// Also start background refresh
	go startBackgroundRefresh()
}

// loadCacheFromRedis loads cached images from Redis
func loadCacheFromRedis() error {
	ctx := database.Ctx
	redisKey := "rawg:image:index" // Store all cached appids in a set

	// Get all cached appids
	appIDs, err := database.RDB.SMembers(ctx, redisKey).Result()
	if err != nil {
		return err
	}

	if len(appIDs) == 0 {
		return fmt.Errorf("no cached images found")
	}

	log.Printf("[ImageCache] Found %d cached images in Redis", len(appIDs))

	// Load each image URL
	for _, appID := range appIDs {
		key := fmt.Sprintf("%s%s", RAWG_IMAGE_CACHE_KEY, appID)
		imgURL, err := database.RDB.Get(ctx, key).Result()
		if err == nil && imgURL != "" {
			imageCache.mu.Lock()
			imageCache.imageCache[appID] = imgURL
			imageCache.mu.Unlock()
		}
	}

	log.Printf("[ImageCache] Loaded %d images into memory cache", len(imageCache.imageCache))
	return nil
}

// preloadPopularGames fetches images for popular games from RAWG API
func preloadPopularGames() {
	log.Println("[ImageCache] Preloading popular games images from RAWG...")

	// Use a subset for initial preload
	appIDs := popularSteamAppIDs
	if len(appIDs) > 50 {
		appIDs = appIDs[:50]
	}

	// Build comma-separated appids
	var idStrs []string
	for _, id := range appIDs {
		idStrs = append(idStrs, strconv.Itoa(id))
	}
	idsParam := strings.Join(idStrs, ",")

	url := fmt.Sprintf("%s/games?key=%s&steam_appids=%s&page_size=%d&fields=id,background_image,short_screenshots,steam_appid",
		RAWG_API_URL, RAWG_API_KEY, idsParam, len(appIDs))

	log.Printf("[ImageCache] Fetching from RAWG: %s...", url[:100])

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("[ImageCache] Failed to fetch from RAWG: %v", err)
		// Fallback: generate Steam CDN URLs
		generateSteamCDNFallback(appIDs)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[ImageCache] RAWG API returned status %d, using Steam CDN fallback", resp.StatusCode)
		generateSteamCDNFallback(appIDs)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[ImageCache] Failed to read RAWG response: %v", err)
		generateSteamCDNFallback(appIDs)
		return
	}

	var result struct {
		Count    int `json:"count"`
		Results []struct {
			ID              int    `json:"id"`
			BackgroundImage string `json:"background_image"`
			SteamAppID      int    `json:"steam_appid"`
			ShortScreenshots []struct {
				Image string `json:"image"`
			} `json:"short_screenshots"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[ImageCache] Failed to parse RAWG response: %v", err)
		generateSteamCDNFallback(appIDs)
		return
	}

	log.Printf("[ImageCache] RAWG returned %d games", len(result.Results))

	// Save to Redis and memory cache
	ctx := database.Ctx
	pipe := database.RDB.Pipeline()

	for _, game := range result.Results {
		if game.SteamAppID == 0 {
			continue
		}

		appID := strconv.Itoa(game.SteamAppID)
		var imgURL string

		// Prefer background_image, fallback to first screenshot
		if game.BackgroundImage != "" {
			imgURL = game.BackgroundImage
		} else if len(game.ShortScreenshots) > 0 {
			imgURL = game.ShortScreenshots[0].Image
		}

		if imgURL != "" {
			key := fmt.Sprintf("%s%s", RAWG_IMAGE_CACHE_KEY, appID)
			pipe.Set(ctx, key, imgURL, 7*24*time.Hour)

			imageCache.mu.Lock()
			imageCache.imageCache[appID] = imgURL
			imageCache.steamToRAWG[appID] = game.ID
			imageCache.mu.Unlock()
		}
	}

	// Save index for future loads
	appIDList := make([]string, 0, len(imageCache.imageCache))
	for appID := range imageCache.imageCache {
		appIDList = append(appIDList, appID)
	}
	if len(appIDList) > 0 {
		pipe.Del(ctx, "rawg:image:index")
		// Convert []string to []interface{}
		interfaceList := make([]interface{}, len(appIDList))
		for i, v := range appIDList {
			interfaceList[i] = v
		}
		pipe.SAdd(ctx, "rawg:image:index", interfaceList...)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		log.Printf("[ImageCache] Failed to save to Redis: %v", err)
	}

	log.Printf("[ImageCache] Preloaded %d game images", len(imageCache.imageCache))
}

// generateSteamCDNFallback generates Steam CDN URLs as fallback
func generateSteamCDNFallback(appIDs []int) {
	for _, appID := range appIDs {
		appIDStr := strconv.Itoa(appID)
		imgURL := fmt.Sprintf(STEAM_CDN_HEADER, appIDStr)

		imageCache.mu.Lock()
		imageCache.imageCache[appIDStr] = imgURL
		imageCache.mu.Unlock()

		// Also save to Redis
		key := fmt.Sprintf("%s%s", RAWG_IMAGE_CACHE_KEY, appIDStr)
		database.RDB.Set(database.Ctx, key, imgURL, 7*24*time.Hour)
	}

	log.Printf("[ImageCache] Generated %d Steam CDN fallback URLs", len(appIDs))
}

// startBackgroundRefresh runs periodic cache refresh
func startBackgroundRefresh() {
	ticker := time.NewTicker(24 * time.Hour)
	for range ticker.C {
		log.Println("[ImageCache] Running background image refresh...")
		preloadPopularGames()
	}
}

// GetGameImage gets the cached image URL for a Steam AppID
func GetGameImage(appID string) string {
	imageCache.mu.RLock()
	imgURL, exists := imageCache.imageCache[appID]
	imageCache.mu.RUnlock()

	if exists && imgURL != "" {
		return imgURL
	}

	// Try to fetch from Redis
	key := fmt.Sprintf("%s%s", RAWG_IMAGE_CACHE_KEY, appID)
	imgURL, err := database.RDB.Get(database.Ctx, key).Result()
	if err == nil && imgURL != "" {
		// Update memory cache
		imageCache.mu.Lock()
		imageCache.imageCache[appID] = imgURL
		imageCache.mu.Unlock()
		return imgURL
	}

	// Fallback to Steam CDN
	return fmt.Sprintf(STEAM_CDN_HEADER, appID)
}

// GetGameImagesBatch gets images for multiple appids
func GetGameImagesBatch(appIDs []string) map[string]string {
	result := make(map[string]string)
	for _, appID := range appIDs {
		result[appID] = GetGameImage(appID)
	}
	return result
}

// PreloadImagesForAppIDs preloads images for specific appids (called on login)
func PreloadImagesForAppIDs(appIDs []string) {
	if len(appIDs) == 0 {
		return
	}

	// Check which ones we don't have
	var missing []string
	for _, appID := range appIDs {
		imageCache.mu.RLock()
		_, exists := imageCache.imageCache[appID]
		imageCache.mu.RUnlock()
		if !exists {
			missing = append(missing, appID)
		}
	}

	if len(missing) == 0 {
		return
	}

	log.Printf("[ImageCache] Preloading images for %d missing appids", len(missing))

	// Fetch from RAWG
	var idStrs []string
	for _, id := range missing {
		idStrs = append(idStrs, id)
	}
	idsParam := strings.Join(idStrs, ",")

	url := fmt.Sprintf("%s/games?key=%s&steam_appids=%s&page_size=%d&fields=id,background_image,short_screenshots,steam_appid",
		RAWG_API_URL, RAWG_API_KEY, idsParam, len(missing))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		// Fallback to Steam CDN
		for _, appID := range missing {
			GetGameImage(appID) // This will populate the fallback
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		for _, appID := range missing {
			GetGameImage(appID)
		}
		return
	}

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Results []struct {
			ID              int    `json:"id"`
			BackgroundImage string `json:"background_image"`
			SteamAppID      int    `json:"steam_appid"`
			ShortScreenshots []struct {
				Image string `json:"image"`
			} `json:"short_screenshots"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return
	}

	ctx := database.Ctx

	for _, game := range result.Results {
		if game.SteamAppID == 0 {
			continue
		}

		appID := strconv.Itoa(game.SteamAppID)
		var imgURL string

		if game.BackgroundImage != "" {
			imgURL = game.BackgroundImage
		} else if len(game.ShortScreenshots) > 0 {
			imgURL = game.ShortScreenshots[0].Image
		}

		if imgURL != "" {
			key := fmt.Sprintf("%s%s", RAWG_IMAGE_CACHE_KEY, appID)
			database.RDB.Set(ctx, key, imgURL, 7*24*time.Hour)

			imageCache.mu.Lock()
			imageCache.imageCache[appID] = imgURL
			imageCache.steamToRAWG[appID] = game.ID
			imageCache.mu.Unlock()
		}
	}

	// Add any still missing to index
	if len(missing) > 0 {
		interfaceList := make([]interface{}, len(missing))
		for i, v := range missing {
			interfaceList[i] = v
		}
		database.RDB.SAdd(ctx, "rawg:image:index", interfaceList...)
	}
}

// API endpoint to manually trigger image cache refresh
func RefreshImageCache(c *gin.Context) {
	go func() {
		preloadPopularGames()
	}()
	c.JSON(200, gin.H{"message": "Image cache refresh started in background"})
}
