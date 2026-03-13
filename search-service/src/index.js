const express = require('express');
const cors = require('cors');
const https = require('https');
const http = require('http');

const app = express();
const PORT = 3003;

// Middleware
app.use(cors());
app.use(express.json());

// RAWG API 配置
const RAWG_API_URL = 'https://api.rawg.io/api';
const RAWG_API_KEY = '9b3e6bbc879b4684ab490b2d5b2a115e';

// 内存缓存 (简单实现)
const cache = new Map();
const CACHE_TTL = 5 * 60 * 1000; // 5分钟

// 搜索建议缓存
const suggestionCache = new Map();

// 带超时的 fetch
function fetchWithTimeout(url, options = {}, timeout = 10000) {
  return new Promise((resolve, reject) => {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), timeout);

    fetch(url, { ...options, signal: controller.signal })
      .then(resolve)
      .catch(reject)
      .finally(() => clearTimeout(timeoutId));
  });
}

// 获取游戏搜索建议
async function getSearchSuggestions(query, limit = 8) {
  if (!query || query.trim().length < 2) {
    return [];
  }

  const cacheKey = `suggest_${query.toLowerCase()}_${limit}`;

  // 检查缓存
  const cached = suggestionCache.get(cacheKey);
  if (cached && Date.now() - cached.timestamp < CACHE_TTL) {
    return cached.data;
  }

  try {
    const response = await fetchWithTimeout(
      `${RAWG_API_URL}/games?key=${RAWG_API_KEY}&search=${encodeURIComponent(query)}&page_size=${limit}&page=1`,
      { cache: 'no-store' },
      8000
    );

    if (!response.ok) {
      throw new Error(`RAWG API error: ${response.status}`);
    }

    const data = await response.json();

    // 格式化结果
    const suggestions = (data.results || []).map(game => ({
      id: game.id,
      name: game.name,
      slug: game.slug,
      released: game.released,
      background_image: game.background_image,
      metacritic: game.metacritic,
      genres: game.genres?.slice(0, 3).map(g => g.name) || [],
      platforms: game.platforms?.slice(0, 3).map(p => p.platform.name) || []
    }));

    // 存入缓存
    suggestionCache.set(cacheKey, {
      data: suggestions,
      timestamp: Date.now()
    });

    return suggestions;
  } catch (error) {
    console.error('Search suggestion error:', error);
    // 返回空数组而不是崩溃
    return [];
  }
}

// 热门搜索词（模拟数据）
const popularSearchTerms = [
  { term: 'Cyberpunk 2077', count: 1250 },
  { term: 'Elden Ring', count: 980 },
  { term: 'Baldur\'s Gate 3', count: 870 },
  { term: 'Red Dead Redemption 2', count: 760 },
  { term: 'The Witcher 3', count: 650 },
  { term: 'GTA V', count: 580 },
  { term: 'Hogwarts Legacy', count: 520 },
  { term: 'Spider-Man', count: 450 }
];

// 搜索历史（内存存储，生产环境应用数据库）
const searchHistory = new Map();

// API 路由

// 1. 搜索建议 API
app.get('/api/suggestions', async (req, res) => {
  const { q, limit = 8 } = req.query;

  if (!q || q.trim().length < 2) {
    return res.json({
      success: true,
      data: []
    });
  }

  const suggestions = await getSearchSuggestions(q, parseInt(limit));

  // 记录搜索历史
  const steamId = req.query.steam_id || 'anonymous';
  if (!searchHistory.has(steamId)) {
    searchHistory.set(steamId, []);
  }
  const history = searchHistory.get(steamId);
  history.unshift({ term: q, timestamp: Date.now() });
  // 保留最近20条
  if (history.length > 20) {
    history.pop();
  }

  res.json({
    success: true,
    data: suggestions
  });
});

// 2. 热门搜索词 API
app.get('/api/popular', (req, res) => {
  res.json({
    success: true,
    data: popularSearchTerms
  });
});

// 3. 搜索历史 API
app.get('/api/history', (req, res) => {
  const { steam_id } = req.query;
  const history = searchHistory.get(steam_id || 'anonymous') || [];

  res.json({
    success: true,
    data: history
  });
});

// 4. 清空搜索历史 API
app.delete('/api/history', (req, res) => {
  const { steam_id } = req.body;
  searchHistory.set(steam_id || 'anonymous', []);

  res.json({
    success: true,
    message: 'History cleared'
  });
});

// 5. 完整搜索 API（返回更多结果）
app.get('/api/search', async (req, res) => {
  const { q, page = 1, page_size = 20 } = req.query;

  if (!q || q.trim().length < 2) {
    return res.json({
      success: true,
      data: [],
      pagination: {}
    });
  }

  try {
    const response = await fetch(
      `${RAWG_API_URL}/games?key=${RAWG_API_KEY}&search=${encodeURIComponent(q)}&page=${page}&page_size=${page_size}`,
      { cache: 'no-store' }
    );

    if (!response.ok) {
      throw new Error(`RAWG API error: ${response.status}`);
    }

    const data = await response.json();

    res.json({
      success: true,
      data: (data.results || []).map(game => ({
        id: game.id,
        name: game.name,
        slug: game.slug,
        released: game.released,
        background_image: game.background_image,
        metacritic: game.metacritic,
        rating: game.rating,
        genres: game.genres?.map(g => g.name) || [],
        platforms: game.platforms?.map(p => p.platform.name) || []
      })),
      pagination: {
        page: parseInt(page),
        page_size: parseInt(page_size),
        total: data.count || 0,
        has_more: !!data.next
      }
    });
  } catch (error) {
    console.error('Search error:', error);
    res.status(500).json({
      success: false,
      error: 'Search failed'
    });
  }
});

// ==================== 愿望单 API ====================

// 用户数据存储（内存）
// 结构: { steamId: { wishlist: [games] } }
const userDataStore = new Map();

// 获取或初始化用户数据
function getUserData(steamId) {
  if (!steamId) {
    steamId = 'anonymous';
  }
  if (!userDataStore.has(steamId)) {
    userDataStore.set(steamId, { wishlist: [] });
  }
  return userDataStore.get(steamId);
}

// 6. 获取用户愿望单
app.get('/api/wishlist', (req, res) => {
  const { steam_id } = req.query;
  const userData = getUserData(steam_id);

  res.json({
    success: true,
    data: userData.wishlist
  });
});

// 7. 添加到愿望单
app.post('/api/wishlist', (req, res) => {
  const { steam_id, game_id, game_data } = req.body;

  if (!game_id) {
    return res.status(400).json({
      success: false,
      error: 'game_id is required'
    });
  }

  const userData = getUserData(steam_id);

  // 检查是否已存在
  if (!userData.wishlist.some(g => g.game_id === game_id)) {
    userData.wishlist.unshift({
      game_id: game_id,
      game_data: game_data || null,
      added_at: Date.now()
    });
  }

  res.json({
    success: true,
    message: 'Added to wishlist',
    data: userData.wishlist
  });
});

// 8. 从愿望单移除
app.delete('/api/wishlist', (req, res) => {
  const { steam_id, game_id } = req.body;
  const userData = getUserData(steam_id);

  userData.wishlist = userData.wishlist.filter(g => g.game_id !== game_id);

  res.json({
    success: true,
    message: 'Removed from wishlist',
    data: userData.wishlist
  });
});

// 检查游戏状态（是否在愿望单中）
app.get('/api/game-status', (req, res) => {
  const { steam_id, game_id } = req.query;
  const userData = getUserData(steam_id);

  const inWishlist = userData.wishlist.some(g => g.game_id === game_id);

  res.json({
    success: true,
    data: {
      in_wishlist: inWishlist
    }
  });
});

// 健康检查
app.get('/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

// 启动服务器
app.listen(PORT, () => {
  console.log(`Search service running on http://localhost:${PORT}`);
  console.log(`- Suggestions: http://localhost:${PORT}/api/suggestions?q=game`);
  console.log(`- Popular: http://localhost:${PORT}/api/popular`);
  console.log(`- History: http://localhost:${PORT}/api/history?steam_id=xxx`);
});
