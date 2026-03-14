package config

import "os"

type Config struct {
	Port         string
	RedisURL     string
	PostgresURL  string
	SteamAPIKey  string
	SteamAPIURL  string
	FrontendURL  string
	PythonAPIURL string
	ProjectRoot  string
	PythonPath   string
}

func LoadConfig() Config {
	// 从环境变量获取配置，若无则使用开发环境默认值
	port := os.Getenv("PORT")
	if port == "" {
		port = "9953" // Go API 运行端口
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}

	pgURL := os.Getenv("POSTGRES_URL")
	if pgURL == "" {
		// 替换成你的 PostgreSQL 实际连接串
		pgURL = "postgres://postgres:123456@localhost:5432/metadata?sslmode=disable"
	}

	// Steam API 配置
	steamAPIKey := os.Getenv("STEAM_API_KEY")
	if steamAPIKey == "" {
		steamAPIKey = "A11C381F817AB411C131C8AC2F60CB5F"
	}

	steamAPIURL := os.Getenv("STEAM_API_URL")
	if steamAPIURL == "" {
		steamAPIURL = "http://api.steampowered.com"
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	pythonAPIURL := os.Getenv("PYTHON_API_URL")
	if pythonAPIURL == "" {
		pythonAPIURL = "http://localhost:8000"
	}

	projectRoot := os.Getenv("PROJECT_ROOT")
	if projectRoot == "" {
		projectRoot = "E:/gs_steam_rcd"
	}

	pythonPath := os.Getenv("PYTHON_PATH")
	if pythonPath == "" {
		pythonPath = "python"
	}

	return Config{
		Port:         port,
		RedisURL:     redisURL,
		PostgresURL:  pgURL,
		SteamAPIKey:  steamAPIKey,
		SteamAPIURL:  steamAPIURL,
		FrontendURL:  frontendURL,
		PythonAPIURL: pythonAPIURL,
		ProjectRoot:  projectRoot,
		PythonPath:   pythonPath,
	}
}
