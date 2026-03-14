package handlers

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"ultim_api_go/config"
	"ultim_api_go/database"

	"github.com/gin-gonic/gin"
)

const RAWG_API_URL = "https://api.rawg.io/api"
const RAWG_API_KEY = "6ca8bd255e02417fb90ce0b97c72a035"

// RawgProxy 代理 RAWG API 请求
func RawgProxy(c *gin.Context) {
	// 获取原始路径 (去掉 /api/v1/rawg 部分)
	path := c.Param("path")
	
	// 确保 path 以 / 开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// 构建目标 URL
	targetURL, err := url.Parse(RAWG_API_URL + path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
		return
	}

	// 确保注入 API Key 用于请求，但缓存 Key 不应包含 API Key
	query := c.Request.URL.Query()
	if query.Get("key") == "" {
		query.Set("key", RAWG_API_KEY)
	}
	targetURL.RawQuery = query.Encode()

	// --- 增加缓存逻辑 ---
	// 缓存 Key 排除 API Key，确保持久化和安全性
	cacheQuery := c.Request.URL.Query()
	cacheQuery.Del("key")
	cacheKeySource := path + "?" + cacheQuery.Encode()
	
	// 使用 MD5 缩短 key 长度
	hash := md5.Sum([]byte(cacheKeySource))
	redisKey := "rawg_cache:" + hex.EncodeToString(hash[:])

	// 尝试从 Redis 获取
	cachedData, err := database.RDB.Get(database.Ctx, redisKey).Result()
	if err == nil && cachedData != "" {
		// 命中缓存
		// 尝试获取 Content-Type (默认 json)
		contentType := database.RDB.Get(database.Ctx, redisKey+":ct").Val()
		if contentType == "" {
			contentType = "application/json"
		}
		c.Header("Content-Type", contentType)
		c.Header("X-Cache", "HIT")
		c.String(http.StatusOK, cachedData)
		return
	}

	// --- 结束缓存逻辑 ---

	// 创建代理请求
	req, err := http.NewRequest(c.Request.Method, targetURL.String(), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// 复制一些必要的 Header，但不复制 Origin/Referer 以免触发 RAWG 的域限制
	req.Header.Set("User-Agent", "SixTower-Backend/1.0")
	req.Header.Set("Accept", "application/json")

	// 发起请求 - 增加超时时间到 30 秒，并添加重试机制
	// 支持 HTTP 代理
	client := &http.Client{Timeout: 30 * time.Second}
	if cfg := config.LoadConfig(); cfg.HTTPProxy != "" {
		if proxyURL, err := url.Parse(cfg.HTTPProxy); err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
	}

	// 重试 3 次
	var httpResp *http.Response
	var httpErr error
	for retry := 0; retry < 3; retry++ {
		httpResp, httpErr = client.Do(req)
		if httpErr == nil {
			break
		}
		log.Printf("[RAWG Proxy] Retry %d/3 for %s: %v", retry+1, path, httpErr)
		time.Sleep(time.Duration(retry+1) * time.Second)
	}

	if httpErr != nil {
		// RAWG API 请求失败时，返回空数据而不是 502，让前端使用降级方案
		log.Printf("[RAWG Proxy] Failed to fetch after 3 retries: %v", httpErr)
		c.JSON(http.StatusOK, gin.H{"detail": "RAWG unavailable", "results": []interface{}{}})
		return
	}
	defer httpResp.Body.Close()

	// 读取响应体以便缓存
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read RAWG response"})
		return
	}

	// 只有成功响应才缓存 (200 OK)
	if httpResp.StatusCode == http.StatusOK {
		// 缓存 7 天 (游戏元数据和图片地址变动极小)
		ttl := 7 * 24 * time.Hour
		database.RDB.Set(database.Ctx, redisKey, string(respBody), ttl)

		contentType := httpResp.Header.Get("Content-Type")
		if contentType != "" {
			database.RDB.Set(database.Ctx, redisKey+":ct", contentType, ttl)
		}
		log.Printf("[RAWG Proxy] Cached: %s", cacheKeySource)
	}

	// 复制响应状态码
	c.Status(httpResp.StatusCode)

	// 复制响应头 (主要是 Content-Type)
	contentType := httpResp.Header.Get("Content-Type")
	if contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.Header("X-Cache", "MISS")

	// 写入响应体
	c.Writer.Write(respBody)
}
