"""
step7_player_auto_cluster.py ??? DNA ?????? (?????
================================================================
?????
1. ?????????(???? 3??????
2. ?? 10 ????????
3. ????????? K (Elbow Method)?
4. ???????????
"""
import pandas as pd
import numpy as np
import logging
from pathlib import Path
from sklearn.preprocessing import StandardScaler
from sklearn.cluster import KMeans
import matplotlib.pyplot as plt
import joblib

logging.basicConfig(level=logging.INFO, format='%(asctime)s - [%(levelname)s] - %(message)s')
logger = logging.getLogger(__name__)

BASE_DIR  = Path("h:/Work/Game/ULTIM")
STEP7_DIR = BASE_DIR / "Step7_Player_Profile"
RAW_FILE  = STEP7_DIR / "player_dna_profile.csv" # ???? 10 ???

FEATS = ['lib_size', 'paid_game_cnt', 'avg_price', 'elite_review_cnt', 
         'helpful_sum', 'elite_ach_total', 'friend_count', 
         'elite_friend_count', 'friend_avg_lib', 'friend_influence']

def run_auto_clustering():
    logger.info(">>> [1/4] ??????????...")
    df = pd.read_csv(RAW_FILE, dtype={'playerid': str})
    X_raw = df[FEATS].fillna(0).values
    
    # ??????????
    scaler = StandardScaler()
    X_scaled = scaler.fit_transform(X_raw)
    
    # --- 2. ????K (Elbow Method) ---
    logger.info(">>> [2/4] ????????? K (2-14)...")
    inertias = []
    ks = range(2, 15)
    
    # ???????? 2w ???K ??
    X_sample = X_scaled[np.random.choice(len(X_scaled), min(20000, len(X_scaled)), replace=False)]
    
    for k in ks:
        km = KMeans(n_clusters=k, random_state=42, n_init=10)
        km.fit(X_sample)
        inertias.append(km.inertia_)
        
    # ?????????? (????)
    diffs = np.diff(inertias)
    diff_ratios = diffs[1:] / diffs[:-1]
    # ?? ratio ?????????
    best_k = ks[np.argmin(diff_ratios) + 1] 
    
    # ?????? best_k ????????????????
    best_k = max(best_k, 5)
    logger.info(f"   [????] ??????? K = {best_k}")

    # --- 3. ???????? ---
    logger.info(f">>> [3/4] ???? K-Means (K={best_k})...")
    kmeans = KMeans(n_clusters=best_k, random_state=42, n_init=10)
    df['auto_cluster_id'] = kmeans.fit_predict(X_scaled)
    
    # --- 4. ???????---
    logger.info(">>> [4/4] ???????????..")
    cluster_means = df.groupby('auto_cluster_id')[FEATS].mean()
    global_means = df[FEATS].mean()
    
    # ????????????????????????
    ratios = cluster_means / global_means
    
    print("\n" + "="*80)
    print(f"???DNA ???????? - ?? {best_k} ??)
    print("="*80)
    
    for cid in range(best_k):
        c_ratio = ratios.loc[cid]
        count = len(df[df['auto_cluster_id'] == cid])
        
        # ????????(Top 2)
        top_traits = c_ratio.sort_values(ascending=False).head(2)
        bottom_traits = c_ratio.sort_values(ascending=True).head(1)
        
        trait_str = " | ".join([f"{k}({v:.1f}x)" for k,v in top_traits.items()])
        weak_str = f"{bottom_traits.index[0]}({bottom_traits.values[0]:.1f}x)"
        
        print(f"  Cluster {cid:2d}:  {count:8,} | : [{trait_str:40}] | : {weak_str}")
        
    # ?????????
    df.to_csv(STEP7_DIR / "player_dna_profile_v2.csv", index=False)
    joblib.dump(kmeans, STEP7_DIR / "auto_kmeans_model.pkl")
    joblib.dump(scaler, STEP7_DIR / "auto_scaler.pkl")
    
    logger.info(f"\n???????????: player_dna_profile_v2.csv")

if __name__ == "__main__":
    run_auto_clustering()
