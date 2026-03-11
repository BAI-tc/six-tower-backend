package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"ultim_api_go/config"
	"ultim_api_go/database"
	"ultim_api_go/models"

	"github.com/gin-gonic/gin"
)

// SteamHandler handles Steam authentication and API requests
type SteamHandler struct {
	config *config.Config
}

// NewSteamHandler creates a new Steam handler
func NewSteamHandler(cfg *config.Config) *SteamHandler {
	return &SteamHandler{config: cfg}
}

// SteamLoginURLResponse represents the response for getting Steam login URL
type SteamLoginURLResponse struct {
	URL string `json:"url"`
}

// SteamUserInfo represents Steam user information
type SteamUserInfo struct {
	SteamID       string `json:"steam_id"`
	PersonaName   string `json:"personaname"`
	ProfileURL    string `json:"profileurl"`
	AvatarFull   string `json:"avatarfull"`
	Avatar        string `json:"avatar"`
	AvatarMedium  string `json:"avatarmedium"`
	LocCountryCode string `json:"loccountrycode,omitempty"`
	PersonaState  int    `json:"personastate,omitempty"`
}

// SteamGamesResponse represents the response for getting user's games
type SteamGamesResponse struct {
	GamesCount int         `json:"games_count"`
	Games      interface{} `json:"games"`
}

// GetSteamLoginURL handles GET /steam/url
// Returns the Steam OpenID login URL
func (h *SteamHandler) GetSteamLoginURL(c *gin.Context) {
	// Get the return URL from query parameter or use default
	returnURL := c.Query("return_url")
	if returnURL == "" {
		returnURL = h.config.FrontendURL + "/auth/steam/callback"
	} else {
		// Decode the return URL if it's URL-encoded
		decoded, err := url.QueryUnescape(returnURL)
		if err == nil {
			returnURL = decoded
		}
	}

	// Build Steam OpenID login URL
	steamLoginURL := fmt.Sprintf(
		"https://steamcommunity.com/openid/login?openid.ns=%s&openid.mode=%s&openid.return_to=%s&openid.realm=%s&openid.identity=%s&openid.claimed_id=%s",
		url.QueryEscape("http://specs.openid.net/auth/2.0"),
		"checkid_setup",
		url.QueryEscape(returnURL),
		url.QueryEscape(strings.TrimSuffix(returnURL, "/")),
		url.QueryEscape("http://specs.openid.net/auth/2.0/identifier_select"),
		url.QueryEscape("http://specs.openid.net/auth/2.0/identifier_select"),
	)

	log.Printf("Generated Steam login URL, return_to: %s", returnURL)

	c.JSON(http.StatusOK, SteamLoginURLResponse{URL: steamLoginURL})
}

// SteamCallback handles GET /steam/callback
// Verifies the OpenID response and redirects to frontend with user info
func (h *SteamHandler) SteamCallback(c *gin.Context) {
	// Get all query parameters
	claimedID := c.Query("openid.claimed_id")
	identity := c.Query("openid.identity")
	mode := c.Query("openid.mode")

	log.Printf("Received OpenID callback: claimed_id=%s, identity=%s, mode=%s", claimedID, identity, mode)

	// Verify the OpenID response
	if mode != "id_res" {
		log.Printf("OpenID mode is not id_res: %s", mode)
		c.Redirect(http.StatusTemporaryRedirect, h.config.FrontendURL+"/login?error=verification_failed")
		return
	}

	// Extract Steam ID from claimed_id
	// Format: https://steamcommunity.com/openid/id/76561198028184818
	steamID := extractSteamID(claimedID)
	if steamID == "" {
		log.Printf("Failed to extract Steam ID from claimed_id: %s", claimedID)
		c.Redirect(http.StatusTemporaryRedirect, h.config.FrontendURL+"/login?error=invalid_steam_id")
		return
	}

	log.Printf("Verified Steam ID: %s", steamID)

	// Get player information from Steam API
	playerInfo, err := h.getPlayerSummaries(steamID)
	if err != nil {
		log.Printf("Failed to get player info: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, h.config.FrontendURL+"/login?error=get_player_failed")
		return
	}

	// Redirect to frontend with user info
	redirectURL := fmt.Sprintf("%s/auth/steam/callback?steamId=%s&username=%s&avatar=%s",
		h.config.FrontendURL,
		steamID,
		url.QueryEscape(playerInfo.PersonaName),
		url.QueryEscape(playerInfo.AvatarFull),
	)

	log.Printf("Redirecting to: %s", redirectURL)
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

// GetSteamUser handles GET /steam/user/:steam_id
// Returns user information for the given Steam ID
func (h *SteamHandler) GetSteamUser(c *gin.Context) {
	steamID := c.Param("steam_id")

	if steamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "steam_id is required"})
		return
	}

	// Validate Steam ID format (should be numeric)
	if _, err := strconv.ParseInt(steamID, 10, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid steam_id format"})
		return
	}

	playerInfo, err := h.getPlayerSummaries(steamID)
	if err != nil {
		log.Printf("Failed to get player info for %s: %v", steamID, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Steam user not found"})
		return
	}

	c.JSON(http.StatusOK, playerInfo)
}

// GetSteamGames handles GET /steam/games/:steam_id
// Returns the list of games owned by the user
func (h *SteamHandler) GetSteamGames(c *gin.Context) {
	steamID := c.Param("steam_id")

	if steamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "steam_id is required"})
		return
	}

	// Validate Steam ID format
	if _, err := strconv.ParseInt(steamID, 10, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid steam_id format"})
		return
	}

	// Get include_appinfo parameter (default: true)
	includeAppInfo := c.DefaultQuery("include_appinfo", "true")

	games, err := h.getOwnedGames(steamID, includeAppInfo == "true")
	if err != nil {
		log.Printf("Failed to get owned games for %s: %v", steamID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get games"})
		return
	}

	if games == nil {
		c.JSON(http.StatusOK, SteamGamesResponse{
			GamesCount: 0,
			Games:      []interface{}{},
		})
		return
	}

	// Extract games from response
	gamesList, ok := games["games"].([]interface{})
	if !ok {
		gamesList = []interface{}{}
	}

	// 异步更新用户的库和类型偏好到 Redis
	go updateUserDataCache(steamID, gamesList)

	c.JSON(http.StatusOK, SteamGamesResponse{
		GamesCount: len(gamesList),
		Games:      gamesList,
	})
}

// GetSteamRecentGames handles GET /steam/recent/:steam_id
// Returns the list of recently played games
func (h *SteamHandler) GetSteamRecentGames(c *gin.Context) {
	steamID := c.Param("steam_id")

	if steamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "steam_id is required"})
		return
	}

	// Validate Steam ID format
	if _, err := strconv.ParseInt(steamID, 10, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid steam_id format"})
		return
	}

	// Get count parameter (default: 10, max: 100)
	count := c.DefaultQuery("count", "10")
	countInt, err := strconv.Atoi(count)
	if err != nil || countInt < 1 {
		countInt = 10
	}
	if countInt > 100 {
		countInt = 100
	}

	games, err := h.getRecentlyPlayedGames(steamID, countInt)
	if err != nil {
		log.Printf("Failed to get recent games for %s: %v", steamID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get recent games"})
		return
	}

	if games == nil {
		c.JSON(http.StatusOK, gin.H{
			"total_count": 0,
			"games":       []interface{}{},
		})
		return
	}

	totalCount, _ := games["total_count"].(float64)
	gamesList, ok := games["games"].([]interface{})
	if !ok {
		gamesList = []interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_count": int(totalCount),
		"games":       gamesList,
	})
}

// Helper functions

// extractSteamID extracts Steam ID from OpenID claimed_id
func extractSteamID(claimedID string) string {
	if claimedID == "" {
		return ""
	}

	// Pattern: https://steamcommunity.com/openid/id/76561198028184818
	re := regexp.MustCompile(`/id/(\d+)`)
	matches := re.FindStringSubmatch(claimedID)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// getPlayerSummaries fetches player information from Steam API
func (h *SteamHandler) getPlayerSummaries(steamID string) (*SteamUserInfo, error) {
	apiURL := fmt.Sprintf("%s/ISteamUser/GetPlayerSummaries/v0002/?key=%s&steamids=%s",
		h.config.SteamAPIURL, h.config.SteamAPIKey, steamID)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam API returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Response struct {
			Players []struct {
				SteamID           string `json:"steamid"`
				PersonaName       string `json:"personaname"`
				ProfileURL        string `json:"profileurl"`
				AvatarFull        string `json:"avatarfull"`
				Avatar            string `json:"avatar"`
				AvatarMedium      string `json:"avatarmedium"`
				LocCountryCode    string `json:"loccountrycode"`
				PersonaState      int    `json:"personastate"`
			} `json:"players"`
		} `json:"response"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Response.Players) == 0 {
		return nil, fmt.Errorf("no player found")
	}

	player := result.Response.Players[0]
	return &SteamUserInfo{
		SteamID:        player.SteamID,
		PersonaName:    player.PersonaName,
		ProfileURL:     player.ProfileURL,
		AvatarFull:    player.AvatarFull,
		Avatar:         player.Avatar,
		AvatarMedium:   player.AvatarMedium,
		LocCountryCode: player.LocCountryCode,
		PersonaState:   player.PersonaState,
	}, nil
}

// getOwnedGames fetches the list of games owned by the user
func (h *SteamHandler) getOwnedGames(steamID string, includeAppInfo bool) (map[string]interface{}, error) {
	includeAppInfoStr := "true"
	if !includeAppInfo {
		includeAppInfoStr = "false"
	}

	apiURL := fmt.Sprintf("%s/IPlayerService/GetOwnedGames/v0001/?key=%s&steamid=%s&include_appinfo=%s&include_played_free_games=true",
		h.config.SteamAPIURL, h.config.SteamAPIKey, steamID, includeAppInfoStr)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam API returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Response map[string]interface{} `json:"response"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Response, nil
}

// getRecentlyPlayedGames fetches the list of recently played games
func (h *SteamHandler) getRecentlyPlayedGames(steamID string, count int) (map[string]interface{}, error) {
	apiURL := fmt.Sprintf("%s/IPlayerService/GetRecentlyPlayedGames/v0001/?key=%s&steamid=%s&count=%d",
		h.config.SteamAPIURL, h.config.SteamAPIKey, steamID, count)

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam API returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Response map[string]interface{} `json:"response"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Response, nil
}

// updateUserDataCache 同步用户的游戏库和计算最偏好的类型
func updateUserDataCache(steamID string, gamesList []interface{}) {
	var appIDs []string
	var intAppIDs []int

	for _, gameInterface := range gamesList {
		if gameMap, ok := gameInterface.(map[string]interface{}); ok {
			if appIDFloat, ok := gameMap["appid"].(float64); ok {
				appID := int(appIDFloat)
				appIDs = append(appIDs, strconv.Itoa(appID))
				intAppIDs = append(intAppIDs, appID)
			}
		}
	}

	if len(appIDs) == 0 {
		return
	}

	// 1. 保存拥有游戏库到 Redis
	appIDsJSON, _ := json.Marshal(appIDs)
	database.RDB.Set(database.Ctx, "user_games:"+steamID, appIDsJSON, 0)

	// 2. 查询这些游戏对应的 genres
	var games []models.GameMetadata
	// 分批查询或者直接 in
	database.DB.Where("product_id IN ?", intAppIDs).Select("genres").Find(&games)

	genreCount := make(map[string]int)
	for _, g := range games {
		if g.Genres == "" {
			continue
		}
		parts := strings.Split(g.Genres, ",")
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				genreCount[trimmed]++
			}
		}
	}

	// 简单取出现次数最多的前3个Genre
	type genreFreq struct {
		name  string
		count int
	}
	var freqs []genreFreq
	for k, v := range genreCount {
		freqs = append(freqs, genreFreq{name: k, count: v})
	}

	// 冒泡排序
	for i := 0; i < len(freqs); i++ {
		for j := i + 1; j < len(freqs); j++ {
			if freqs[i].count < freqs[j].count {
				freqs[i], freqs[j] = freqs[j], freqs[i]
			}
		}
	}

	var topGenres []string
	limit := 5
	for i := 0; i < len(freqs) && i < limit; i++ {
		topGenres = append(topGenres, freqs[i].name)
	}

	if len(topGenres) > 0 {
		topGenresJSON, _ := json.Marshal(topGenres)
		database.RDB.Set(database.Ctx, "user_genres:"+steamID, topGenresJSON, 0)
	}
}
