# 🚀 ULTIM 推荐系统工程化与前端接入全链路指南

本项目旨在将 `ULTIM` 目录中训练好并达到 30.85% 召回率的推荐算法模型，通过后端 API 封装工程化，并与 `gamesci` 前端的首页推荐模块（For You等）打通。本指南参考了 `steam_recommend-main` 的工程化设计思路，提供事无巨细的实施路径。

---

## 🏗️ 整体架构设计
`ULTIM` 模型当前是离线的 Python 批量推断脚本 (`step10_ultimate_fusion_engine.py`)。工程化的核心是将**单次/批量验证计算**改造成**支持低延迟在线计算（或近线计算）的 API 服务**。

**数据流转路径：**
1. 前端 (`gamesci`): 首页组件加载，调用 `src/api/recommendations.js` 发送带 `userId` 的 HTTP GET 请求。
2. 后端 API (`steam_recommend-main/backend` 或 新建立的 FastAPI 项目):
   - 接收 `userId`，获取该用户的游戏库 (Library) 数据。
   - 调用 ULTIM 模型引擎（已常驻内存）进行融合打分。
   - 将 Top K 游戏的游标 (App ID) 结合数据库或第三方缓存中的游戏元数据（封面、名称等）打包返回。
3. 前端接收渲染展示的 JSON。

---

## 🛠️ 第一阶段：改造 ULTIM 模型引擎 (近线/在线推断)

当前的 `step10_ultimate_fusion_engine.py` 中，所有数据都在 `run()` 时一次性被加载，且面向特定的 `test_u` 和 `test_pids` 进行批量预测并计算 Recall。我们需要将其解耦为两部分：**模型冷启动加载** 与 **单用户实时推断**。

### 1. 拆分文件：`ultim_inference.py`

创建一个封装好的模型服务类：

```python
import pandas as pd
import numpy as np
import json, ast, pickle
from pathlib import Path
from scipy.sparse import csr_matrix
from sklearn.preprocessing import normalize
import torch
from gensim.models import Word2Vec

class ULTIMRecommender:
    def __init__(self, base_dir="h:/Work/Game/ULTIM"):
        self.base_dir = Path(base_dir)
        self.device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self._load_assets()

    def _load_assets(self):
        # TODO: 从 step10.py 将所有资产加载逻辑迁移到这里
        # 包括：id_mappings.json、标签 Word2Vec 模型、svd_model_assets.pkl、
        # 用户和游戏的 DNA Profile、集群信息等。
        # 这些变量应作为类的属性 (self.US, self.Vt, self.G_sem 等) 挂载，仅在 API 启动时加载一次以节省时间。
        pass

    def recommend(self, user_steam_id, user_library_appids, top_k=20):
        """
        :param user_steam_id: String, 例如 "76561198xxxx"
        :param user_library_appids: List[String], 用户已拥有的且需要过滤的游戏 AppID 列表
        :return: List[Dict], 包含推荐的 GameID 以及对应权重分
        """
        # TODO:
        # 1. 计算单个用户的 S_SVD, S_Sem, S_Pop, S_Prof, S_ICF, S_CP
        # 2. 以 Golden Recipe 进行最终得分计算: 
        #    Score = 1.2*S_SVD + 0.5*S_Sem + 1.5*S_Pop + 0.2*S_Prof + 0.1*S_ICF + 0.05*S_CP
        # 3. 过滤掉 `user_library_appids` (权重设为 -1e9)
        # 4. argsort 提取 Top K
        # 5. 返回推荐的游戏 ID 列表
        pass
```

### 2. 编写 FastAPI 封装层 (后端服务)

参考 `steam_recommend-main\backend\api\v1` 中的设计，在后端项目中注入模型：

```python
# backend/main.py (新增或替换)
from fastapi import FastAPI
from backend.recommender.ultim_inference import ULTIMRecommender

app = FastAPI()
recommender = None

@app.on_event("startup")
async def startup_event():
    global recommender
    # 启动时先将几个GB的模型全部拉入内存 (或显存)
    recommender = ULTIMRecommender()

@app.get("/api/v1/recommendations/ultim")
async def get_ultim_recommendations(user_id: str, topk: int = 20):
    # 此处还需要一个工具函数来获取用户当前的 Steam 游戏库 (可以从数据库或调Steam API)
    user_library = await fetch_user_library_from_db_or_api(user_id) 
    
    # 实时计算打分
    recommendations_appids = recommender.recommend(user_id, user_library, topk=topk)
    
    # 根据 appid 查表，补充给前端展现所需的游戏名称 (title)、封面图 (cover) 等数据
    full_game_info = await batch_get_game_metadata(recommendations_appids)
    
    return {"algorithm": "ULTIM_V1", "recommendations": full_game_info}
```

---

## 🧩 第二阶段：对齐 `gamesci` 前端结构

在 `gamesci` 中，目前的 `home/page.js` 中是这样请求数据的：
它高度依赖 `src/api/recommendations.js`

### 1. 修改 `gamesci\src\api\recommendations.js`

我们需要将原本的普通推荐 API 路由切换至 `ULTIM` 后端 API。

打开 `E:\gamescience\gamesci\src\api\recommendations.js`：

```javascript
export async function fetchPersonalizedRecommendations(userId, topk = 20, algorithm = 'ultim', rankingStrategy = 'default') {
  try {
    // 将此处改写为指向 ULTIM 对应的后端接口，可以通过调整 algorithm 等参数实现路由切换
    const response = await fetch(
      `${API_BASE}/recommendations/ultim?user_id=${userId}&topk=${topk}`,
      { cache: 'no-store' }
    );
    if (response.ok) {
      return await response.json();
    }
  } catch (error) {
    console.error('Error fetching personalized recommendations:', error);
  }
  return { recommendations: [], algorithm: 'error' };
}
```

### 2. 适应首页渲染 (home/page.js)

在 `E:\gamescience\gamesci\src\app\home\page.js` 中，首页实际上使用了多个基于推荐的 Section：
```javascript
// 基于用户的 ForYou 推荐 (由 fetchPersonalizedRecommendations 返回)
<PersonalizedSteamSection 
   games={forYouRecommendations?.recommendations} 
   title="For You" 
   reason="Based on your library and preferences" 
/>
```

**对接防坑点（元数据不齐）：**
ULTIM 模型跑出来的是游戏的 ID 集（通常形如 `appId`: `1091500`）。
你的前端组件 `<NetflixCard>`, `<PersonalizedSteamSection>` 需要用到的字段包括：
`game.product_id` 或 `game.id` 或 `game.appid` , `game.title`, `game.background_image`, `game.metacritic`等。
如果后端的 ULTIM 接口没有补齐这些元数据字段直接发给前端，会导致前端变成黑屏占位图或报错。请在 FastAPI 的 `batch_get_game_metadata` 函数中确保和 Rawg 数据 或 Steam 数据库里的格式对齐！

---

## ⚡ 第三阶段：性能与工程化进阶

如果你希望在未来承载高并发的 C 端流量（而不是自己测试用），完全在线推理上述矩阵可能存在时耗，你可以做如下改造：

**“近线” + “在线” 相结合：**
1. 每日在定时任务（如 Apache Airflow 或 Celery 任务）中跑一次 `step10_ultimate_fusion_engine.py`，遍历库活跃用户的 ID。
2. 将结果存写至 Redis（Key: `ultim_recom:{user_id}`, Value: `[appid1, appid2, ...]`）。
3. 当用户通过 `gamesci` 前端发起访问时，FastAPI 接口只需 `redis.get(f"ultim_recom:{user_id}")`，实现了 `O(1)` 的百毫秒级急速响应，而不必每次调用冗长且包含模型计算的过程。
4. （实时补丁模式）：如果用户库中突增某款大作，也可通过消息队列重新触发单用户的轻量在线推荐打分以覆盖历史离线推荐。

## 🎯 行动清单 (To-Do List)

1. [ ] 在 `ULTIM` 根下新开一个文件 `ultim_inference.py`，把 `step10` 的全局计算逻辑改为类封装的单点运算结构。
2. [ ] 在 `steam_recommend-main` (或者对应被 API_BASE 关联的新后端代码) 增加 ULTIM 模型实例化。
3. [ ] 新增相关的 API 接口 (如 `/recommendations/ultim`)，并在内部处理掉所有 AppID 到 详细信息的 Mappings。
4. [ ] 在 `gamesci/src/api/recommendations.js` 中把接口路径对上。
5. [ ] (可选) 增加容灾处理。当 ULTIM 因为用户冷启动(无库信息或获取不到该用户信息)时，返回热门兜底(如 `fetchPopularGames`) 的数据。
