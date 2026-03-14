"""
ULTIM 推荐算法 API
==================
基于 step10_ultimate_fusion_engine.py 的单用户推理版本
启动时会加载所有模型，之后接收 steam_id 返回推荐结果
"""
import os
import sys
import json
import time
import asyncio
import pickle
import numpy as np
import pandas as pd
import redis.asyncio as redis
from pathlib import Path
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from scipy.sparse import csr_matrix
from sklearn.preprocessing import normalize
import torch

# 尝试导入 gensim，如果失败则使用简化版本
try:
    from gensim.models import Word2Vec
    GENSIM_AVAILABLE = True
except ImportError:
    GENSIM_AVAILABLE = False
    print("WARNING: gensim not available, using simplified recommendation without semantic embeddings")

# ULTIM 模型路径
ULTIM_DIR = Path("E:/gs_steam_rcd/ULTIM")
STEP5_DIR = ULTIM_DIR / "Step5_Matrix"
STEP7_DIR = ULTIM_DIR / "Step7_Player_Profile"
STEP8_DIR = ULTIM_DIR / "Step8_Game_Profile"

# Redis 配置
REDIS_HOST = os.getenv("REDIS_HOST", "localhost")
REDIS_PORT = int(os.getenv("REDIS_PORT", "6379"))
REDIS_DB = int(os.getenv("REDIS_DB", "0"))

app = FastAPI(title="ULTIM Recommendation API")

# 全局模型变量
class ULTIMModels:
    def __init__(self):
        self.device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self.idx_to_player = None
        self.idx_to_game = None
        self.gid_to_gidx = None
        self.train_dict = None
        self.df_g = None
        self.cluster_labels = None
        self.max_c = None
        self.Pop_t = None
        self.w2v = None
        self.idf_w = None
        self.G_sem_t = None
        self.US = None
        self.Vt = None
        self.M_norm = None
        self.pid_to_cluster = None
        self.cluster_game_pop = None
        self.df_pur = None
        self.loaded = False

models = ULTIMModels()

def clean_tags(v):
    """清理标签"""
    return [i.strip().lower().replace(" ", "_") for i in str(v).replace('[','').replace(']','').replace("'","").split(',')]

async def load_models():
    """加载所有 ULTIM 模型"""
    if models.loaded:
        return

    print(">>> [1/6] Loading ID mappings...")
    with open(STEP5_DIR / "id_mappings.json") as f:
        id_maps = json.load(f)
    models.idx_to_player = id_maps['idx_to_player']
    models.idx_to_game = id_maps['idx_to_game']
    n_games = len(models.idx_to_game)
    n_users = len(models.idx_to_player)
    models.gid_to_gidx = {gid: i for i, gid in enumerate([models.idx_to_game[str(i)] for i in range(n_games)])}

    print(">>> [2/6] Loading train data...")
    train_df = pd.read_csv(STEP5_DIR / "train_indices.csv")
    models.train_dict = train_df.groupby('user_idx')['item_idx'].apply(set).to_dict()

    print(">>> [3/6] Loading game profiles...")
    df_g = pd.read_csv(STEP8_DIR / "game_dna_profile.csv", dtype={'gameid': str}, low_memory=False)
    svd_gids = [models.idx_to_game[str(i)] for i in range(n_games)]
    df_g = df_g.set_index('gameid').reindex(svd_gids).fillna(0)
    models.df_g = df_g
    models.cluster_labels = df_g['game_cluster'].fillna(-1).astype(int).values
    models.max_c = int(models.cluster_labels.max() + 1)

    # 流行度
    pop_raw = pd.to_numeric(df_g['real_purchase_cnt'], errors='coerce').fillna(1.0).values
    pop_norm = np.log1p(pop_raw)
    pop_norm /= (pop_norm.max() + 1e-9)
    models.Pop_t = torch.tensor(pop_norm, dtype=torch.float32, device=models.device).unsqueeze(0)

    # Word2Vec 语义
    if GENSIM_AVAILABLE:
        print(">>> [4/6] Loading Word2Vec model...")
        try:
            models.w2v = Word2Vec.load(str(STEP8_DIR / "tag_word2vec_v2.model"))
            tag_counts = {}
            for _, row in df_g.iterrows():
                for t in set(clean_tags(row['genres']) + clean_tags(row['top_tags'])):
                    tag_counts[t] = tag_counts.get(t, 0) + 1
            models.idf_w = {t: np.log(len(df_g)/(c+1)) for t, c in tag_counts.items()}

            G_sem = np.zeros((n_games, 64), dtype=np.float32)
            for i, (_, row) in enumerate(df_g.iterrows()):
                vs, ws = [], []
                for t in set(clean_tags(row['genres']) + clean_tags(row['top_tags'])):
                    if t in models.w2v.wv:
                        vs.append(models.w2v.wv[t])
                        ws.append(models.idf_w.get(t, 1.0))
                if vs:
                    G_sem[i] = np.average(vs, axis=0, weights=ws)
            models.G_sem_t = torch.tensor(G_sem, device=models.device)
            models.G_sem_t /= (torch.norm(models.G_sem_t, dim=1, keepdim=True) + 1e-9)
        except Exception as e:
            print(f"WARNING: Failed to load Word2Vec: {e}")
            models.w2v = None
            models.G_sem_t = None
    else:
        print(">>> [4/6] Skipping Word2Vec (gensim not available)")
        models.w2v = None
        models.G_sem_t = None

    # SVD
    print(">>> [5/6] Loading SVD model...")
    with open(ULTIM_DIR / "svd_model_assets.pkl", 'rb') as f:
        assets = pickle.load(f)
    models.US = torch.tensor(assets['user_factors'], dtype=torch.float32, device=models.device)
    models.Vt = torch.tensor(assets['item_factors'].T, dtype=torch.float32, device=models.device)

    # Item-CF
    M = csr_matrix(
        (np.ones(len(train_df), dtype=np.float32),
         (train_df['item_idx'].values, train_df['user_idx'].values)),
        shape=(n_games, n_users)
    )
    models.M_norm = normalize(M, norm='l2', axis=1)

    # 用户聚类流行度
    print(">>> [6/6] Loading user clusters...")
    df_p_v2 = pd.read_csv(STEP7_DIR / "player_dna_profile_v2.csv", dtype={'playerid': str})
    models.pid_to_cluster = df_p_v2.set_index('playerid')['auto_cluster_id'].to_dict()
    n_pclusters = int(df_p_v2['auto_cluster_id'].max() + 1)
    cluster_size_real = np.zeros(n_pclusters)
    cluster_game_pop = np.zeros((n_pclusters, n_games), dtype=np.float32)
    for user_idx, item_idx in zip(train_df['user_idx'].values, train_df['item_idx'].values):
        pid = models.idx_to_player[str(user_idx)]
        if pid in models.pid_to_cluster and item_idx < n_games:
            cluster_game_pop[models.pid_to_cluster[pid], item_idx] += 1
    for pid, k in models.pid_to_cluster.items():
        cluster_size_real[k] += 1
    for k in range(n_pclusters):
        if cluster_size_real[k] > 0:
            cluster_game_pop[k] = np.log1p(cluster_game_pop[k]) / (np.log(cluster_size_real[k] + 1) + 1e-9)
    models.cluster_game_pop = cluster_game_pop

    # 用户游戏库
    models.df_pur = pd.read_csv(ULTIM_DIR / "raw_data/steam/purchased_games.csv",
                                 dtype={'playerid': str}, low_memory=False).set_index('playerid')

    models.loaded = True
    print(f">>> ULTIM models loaded successfully! Device: {models.device}")

async def get_redis():
    """获取 Redis 连接"""
    return redis.Redis(host=REDIS_HOST, port=REDIS_PORT, db=REDIS_DB, decode_responses=True)

class RecommendRequest(BaseModel):
    steam_id: str
    top_k: int = 20
    # 六塔模型权重 (可选)
    weight_svd: float = 1.2
    weight_sem: float = 0.5
    weight_pop: float = 1.5
    weight_prof: float = 0.2
    weight_icf: float = 0.1
    weight_cp: float = 0.05
    offset: int = 0

class RecommendResponse(BaseModel):
    steam_id: str
    recommendations: list
    algorithm: str = "ULTIM"
    cached: bool = False
    compute_time_ms: float = 0

@app.on_event("startup")
async def startup():
    """启动时加载模型"""
    await load_models()

@app.get("/health")
async def health():
    return {"status": "ok", "models_loaded": models.loaded}

@app.get("/elite-games")
async def get_elite_games():
    """获取 ULTIM 精选游戏名称列表"""
    if not models.loaded or models.df_g is None:
        return {"games": [], "total": 0}

    # 返回所有游戏名称（使用索引作为gameid）
    games = []
    for idx, row in models.df_g.iterrows():
        title = row.get('title', 0)
        if title and title != 0:
            # idx 就是 gameid (Steam appid)
            games.append({
                "name": str(title).strip(),
                "gameid": str(idx)
            })

    return {
        "games": games,
        "total": len(games)
    }

@app.post("/recommend", response_model=RecommendResponse)
async def recommend(request: RecommendRequest):
    """获取 ULTIM 推荐"""
    t0 = time.time()
    steam_id = request.steam_id
    top_k = request.top_k

    # 提取权重参数
    weights = {
        'svd': request.weight_svd,
        'sem': request.weight_sem,
        'pop': request.weight_pop,
        'prof': request.weight_prof,
        'icf': request.weight_icf,
        'cp': request.weight_cp
    }

    # 检查是否使用自定义权重
    using_custom_weights = not (
        request.weight_svd == 1.2 and
        request.weight_sem == 0.5 and
        request.weight_pop == 1.5 and
        request.weight_prof == 0.2 and
        request.weight_icf == 0.1 and
        request.weight_cp == 0.05
    )

    # 1. 只有不使用自定义权重时才走 Redis 缓存
    if not using_custom_weights:
        r = await get_redis()
        cache_key = f"ultim_recom:{steam_id}"
        cached = await r.get(cache_key)

        if cached:
            recommendations = json.loads(cached)
            compute_time = (time.time() - t0) * 1000
            return RecommendResponse(
                steam_id=steam_id,
                recommendations=recommendations[:top_k],
                algorithm="ULTIM",
                cached=True,
                compute_time_ms=compute_time
            )

    # 2. 计算推荐 (传入权重和 Offset)
    recommendations = await compute_recommendations(steam_id, top_k, weights, offset=request.offset)

    # 3. 只有默认权重时才缓存
    if not using_custom_weights and recommendations:
        await r.setex(cache_key, 86400, json.dumps(recommendations))

    compute_time = (time.time() - t0) * 1000
    return RecommendResponse(
        steam_id=steam_id,
        recommendations=recommendations[:top_k],
        algorithm="ULTIM",
        cached=False,
        compute_time_ms=compute_time
    )

async def compute_recommendations(steam_id: str, top_k: int, weights: dict = None, offset: int = 0) -> list:
    """计算 ULTIM 推荐 (支持 Offset 翻页)"""
    n_games = len(models.idx_to_game)

    # 查找用户索引
    user_idx = None
    for idx, pid in models.idx_to_player.items():
        if pid == steam_id:
            user_idx = int(idx)
            break

    if user_idx is None:
        # 新用户，返回热门游戏
        print(f">>> User {steam_id} not found, returning popular games with offset {offset}")
        return await get_popular_games(top_k, offset=offset, steam_id=steam_id)

    # 获取用户游戏库
    try:
        lib = list(models.train_dict.get(user_idx, []))
    except:
        lib = []

    # 如果用户没有游戏库，尝试从原始数据获取
    if not lib and steam_id in models.df_pur.index:
        try:
            lib_str = models.df_pur.at[steam_id, 'library']
            if isinstance(lib_str, str):
                import ast
                lib = ast.literal_eval(lib_str)
                lib = [models.gid_to_gidx[str(g)] for g in lib if str(g) in models.gid_to_gidx]
        except:
            lib = []

    if not lib:
        # 用户没有游戏数据，返回热门
        return await get_popular_games(top_k, offset=offset, steam_id=steam_id)

    # ============ ULTIM 融合算法 ============
    gidx_p = [g for g in lib if g < n_games]
    if not gidx_p:
        return await get_popular_games(top_k, offset=offset, steam_id=steam_id)

    # 1. 语义嵌入 (可选)
    if models.G_sem_t is not None:
        w_s = np.ones(len(gidx_p))
        w_s[-min(len(gidx_p), 20):] = 3.0
        P_sem = np.average(models.G_sem_t.cpu().numpy()[gidx_p], axis=0, weights=w_s)
        P_sem_t = torch.tensor(P_sem, dtype=torch.float32, device=models.device).unsqueeze(0)
        P_sem_t /= (torch.norm(P_sem_t, dim=1, keepdim=True) + 1e-9)
    else:
        # 没有语义嵌入时使用零向量
        P_sem_t = None

    # 2. 用户聚类
    P_cls = np.zeros(models.max_c, dtype=np.float32)
    cc = np.bincount(models.cluster_labels[gidx_p], minlength=models.max_c)
    P_cls = cc / (cc.sum() + 1e-9)

    # 3. Item-CF
    ICF = np.zeros(n_games, dtype=np.float32)
    if lib:
        qv = models.M_norm[lib].sum(axis=0)
        ICF = np.asarray(models.M_norm.dot(qv.T)).flatten()

    # 4. 聚类流行度
    cluster_id = models.pid_to_cluster.get(steam_id, 0)
    CluPop = models.cluster_game_pop[cluster_id] if cluster_id < len(models.cluster_game_pop) else np.zeros(n_games)

    # 使用传入的权重，或使用默认值
    w_svd = weights.get('svd', 1.2) if weights else 1.2
    w_sem = weights.get('sem', 0.5) if weights else 0.5
    w_pop = weights.get('pop', 1.5) if weights else 1.5
    w_prof = weights.get('prof', 0.2) if weights else 0.2
    w_icf = weights.get('icf', 0.1) if weights else 0.1
    w_cp = weights.get('cp', 0.05) if weights else 0.05

    # 转换为 tensor
    with torch.no_grad():
        U_test = models.US[user_idx:user_idx+1]
        S_SVD = torch.matmul(U_test, models.Vt)
        S_SVD = (S_SVD - S_SVD.min(1, True).values) / (S_SVD.max(1, True).values - S_SVD.min(1, True).values + 1e-9)
        S_Pop = models.Pop_t.repeat(1, 1).cpu().numpy()
        S_Prof = np.tile(P_cls, (1, n_games))
        S_ICF = (ICF - ICF.min()) / (ICF.max() - ICF.min() + 1e-9) if ICF.max() > ICF.min() else ICF
        S_CP = (CluPop - CluPop.min()) / (CluPop.max() - CluPop.min() + 1e-9) if CluPop.max() > CluPop.min() else CluPop

    # 融合公式 (使用配置的权重)
    if P_sem_t is not None:
        S_Sem = (torch.matmul(P_sem_t, models.G_sem_t.T).cpu().numpy() + 1.0) / 2.0
        Final_Scores = w_svd * S_SVD.flatten() + w_sem * S_Sem.flatten() + w_pop * S_Pop.flatten() + \
                       w_prof * S_Prof.flatten() + w_icf * S_ICF.flatten() + w_cp * S_CP.flatten()
    else:
        # 没有语义嵌入时的简化公式
        print(">>> Using simplified recommendation (no semantic embeddings)")
        Final_Scores = w_svd * S_SVD.flatten() + w_pop * S_Pop.flatten() + \
                       w_prof * S_Prof.flatten() + w_icf * S_ICF.flatten() + w_cp * S_CP.flatten()

    # 增加细微随机性以提高多样性，防止不同权重组合出现完全一致的 topk
    # 对于未识别用户（fallback 到 popular 的情况），增加更大的随机性以帮助去重
    if lib is None or len(lib) == 0:
        noise_level = 0.05
    else:
        noise_level = 0.005
    Final_Scores += np.random.normal(0, noise_level, size=Final_Scores.shape)

    # 排除已拥有的游戏
    scores = Final_Scores.copy()
    scores[lib] = -1e9

    # 取 Top K + Paging (Offset)
    total_needed = top_k + offset
    if total_needed > n_games:
        total_needed = n_games

    top_indices = np.argpartition(scores, -total_needed)[-total_needed:]
    top_indices = top_indices[np.argsort(scores[top_indices])[::-1]]
    
    # 应用 Offset
    paged_indices = top_indices[offset : offset + top_k]

    # 转换为 appid
    recommendations = [models.idx_to_game[str(i)] for i in paged_indices]
    return recommendations

async def get_popular_games(top_k: int, offset: int = 0, steam_id: str = None) -> list:
    """获取热门游戏（支持 Offset 和 随机性）"""
    if models.df_g is not None:
        # 获取热度基础分
        pop_raw = pd.to_numeric(models.df_g['real_purchase_cnt'], errors='coerce').fillna(0).copy()
        
        # 增加基于 steam_id 的随机偏置，确保不同用户看到的排列略有不同
        if steam_id:
            import hashlib
            seed = int(hashlib.md5(steam_id.encode()).hexdigest(), 16) % 10000
            np.random.seed(seed)
            # 添加 1% 以内的随机波动
            noise = np.random.normal(1.0, 0.01, size=len(pop_raw))
            pop_raw = pop_raw * noise
            
        # 取 top_k + offset
        top_indices = pop_raw.nlargest(top_k + offset).index.tolist()
        # 返回 offset 之后的
        return top_indices[offset:]
    return []

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=3001)
