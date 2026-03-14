"""
step8_game_auto_cluster.py ??? DNA ???????????
=============================================================
?????
1. ???????????????????????????????
2. ????????? Tag ???????????????????
3. ??????????????????????????
"""
import pandas as pd
import numpy as np
import logging, json, pickle, ast, time
from pathlib import Path
from sklearn.preprocessing import StandardScaler
from sklearn.cluster import KMeans
import joblib
from tqdm import tqdm

logging.basicConfig(level=logging.INFO, format='%(asctime)s - [%(levelname)s] - %(message)s')
logger = logging.getLogger(__name__)

BASE_DIR  = Path("h:/Work/Game/ULTIM")
STEAM_DIR = BASE_DIR / "raw_data/steam"
STEP8_DIR = BASE_DIR / "Step8_Game_Profile"
STEP5_DIR = BASE_DIR / "Step5_Matrix"

def run_game_evolution():
    t0 = time.time()
    logger.info(">>> [1/4] ??????????????????..")
    
    # ?????
    df_g = pd.read_csv(BASE_DIR / "Step3_Elite_Archive/elite_games_filtered.csv", dtype={'gameid': str})
    elite_gids = set(df_g['gameid'])
    
    # ????????
    real_ownership = {gid: set() for gid in elite_gids}
    
    # ??1: ????
    df_pur = pd.read_csv(STEAM_DIR / "purchased_games.csv", dtype=str)
    for _, row in df_pur.iterrows():
        try:
            lib = ast.literal_eval(row['library'])
            for g in lib:
                gid = str(g)
                if gid in elite_gids: real_ownership[gid].add(row['playerid'])
        except: continue
        
    # ??2: ???? (?????
    for chunk in pd.read_csv(STEAM_DIR / "reviews.csv", usecols=['playerid', 'gameid'], dtype=str, chunksize=100000):
        valid = chunk[chunk['gameid'].isin(elite_gids)]
        for r in valid.itertuples():
            real_ownership[r.gameid].add(r.playerid)
            
    # ????????
    df_g['real_purchase_cnt'] = df_g['gameid'].map({k: len(v) for k, v in real_ownership.items()}).fillna(0)
    logger.info(f"   ????????????? {df_g['real_purchase_cnt'].max():.0f}")

    # --- 2. ?????? (??????) ---
    logger.info(">>> [2/4] ????????????..")
    
    # ???????
    feats = ['real_purchase_cnt', 'peak_ccu', 'price_float_num', 'metacritic', 'community_authority']
    for f in feats:
        if f in df_g.columns:
            df_g[f] = pd.to_numeric(df_g[f], errors='coerce').fillna(0)
    
    # ?????????(CCU / ??
    df_g['popularity_density'] = df_g['peak_ccu'] / (df_g['real_purchase_cnt'] + 10)
    # ????
    df_g['is_free'] = (df_g['price_float_num'] == 0).astype(int)
    
    # --- ???????????????????????? ---
    df_cluster = df_g.copy()
    for col in ['real_purchase_cnt', 'peak_ccu', 'popularity_density']:
        df_cluster[col] = np.log1p(df_cluster[col])

    cluster_feats = feats + ['popularity_density', 'is_free']
    X_raw = df_cluster[cluster_feats].values
    scaler = StandardScaler()
    X_scaled = scaler.fit_transform(X_raw)

    # --- 3. ?????? (2-15) ---
    logger.info(">>> [3/4] ????????????(K-Means Search)...")
    ks = range(2, 16)
    inertias = []
    # ????
    X_sample = X_scaled[np.random.choice(len(X_scaled), min(10000, len(X_scaled)), replace=False)]
    for k in ks:
        km = KMeans(n_clusters=k, random_state=42, n_init=5)
        km.fit(X_sample)
        inertias.append(km.inertia_)
    
    # ?????? (Elbow)
    diffs = np.diff(inertias)
    best_k = ks[np.argmin(diffs[1:] / diffs[:-1]) + 1]
    best_k = max(best_k, 7) # ?????????7 ?
    logger.info(f"   [????] ?????????{best_k} ???)

    kmeans = KMeans(n_clusters=best_k, random_state=42, n_init=10)
    df_g['game_cluster'] = kmeans.fit_predict(X_scaled)

    # --- 4. ???? ---
    logger.info(">>> [4/4] ???????Cluster)?????..")
    summary = df_g.groupby('game_cluster')[cluster_feats].mean()
    global_avg = df_g[cluster_feats].mean()
    ratios = summary / global_avg

    print("\n" + "="*90)
    print(f"???????????? - K={best_k}?)
    print("-" * 90)
    for cid in range(best_k):
        c_ratio = ratios.loc[cid]
        count = len(df_g[df_g['game_cluster'] == cid])
        top_traits = c_ratio.sort_values(ascending=False).head(2)
        
        trait_str = " | ".join([f"{k}({v:.1f}x)" for k,v in top_traits.items()])
        print(f"  Galaxy {cid:2d}:  {count:6,} | : [{trait_str:50}]")

    # ????????
    df_g.to_csv(STEP8_DIR / "game_dna_profile.csv", index=False)
    joblib.dump(kmeans, STEP8_DIR / "game_auto_kmeans_v2.pkl")
    joblib.dump(scaler, STEP8_DIR / "game_auto_scaler_v2.pkl")
    
    logger.info(f"\n??? DNA ???? (???: {time.time()-t0:.1f}s)")

if __name__ == "__main__":
    run_game_evolution()
