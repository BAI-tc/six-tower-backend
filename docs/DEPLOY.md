# SixTower 部署文档

## 架构

```
用户 → Netlify (前端) → Cloudflare Tunnel → 本地:9953 → Go后端 → Redis/PostgreSQL
                                                                   → Python API (3001)
```

## 前端部署 (Netlify)

1. **推送代码到 GitHub**
   ```bash
   cd gs_steam_rcd_frontend
   git add .
   git commit -m "update"
   git push origin master
   ```

2. **Netlify 自动部署**
   - 访问 https://app.netlify.com
   - 连接 GitHub 仓库 `BAI-tc/six-tower-frontend`
   - 自动部署完成

## 后端部署 (本地)

1. **启动 Go 后端** (端口 9953)
   ```bash
   cd gs_steam_rcd/bakend
   go run main.go
   ```

2. **启动 Python 推荐算法** (端口 3001)
   ```bash
   cd gs_steam_rcd/ULTIM
   python ultim_api.py
   ```

3. **启动 Cloudflare Tunnel**
   ```bash
   cloudflared.exe service install <your-token>
   ```
   - 隧道指向 `localhost:9953`
   - 域名: `tian.fourever.top`

## 环境要求

- Redis (本地)
- PostgreSQL (本地)
- Cloudflare 账号 + 域名

## 测试

- 前端: https://six-tower-frontend.netlify.app
- 后端: https://tian.fourever.top/api/v1
