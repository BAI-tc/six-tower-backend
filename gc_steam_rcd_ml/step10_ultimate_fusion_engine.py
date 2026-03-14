?"""
step10_ultimate_fusion_engine.py - Ultimate Six-Tower Fusion Recommender
========================================================================
Recall@20 Goal: 30.85%+ (Self-contained in ULTIM directory)
"""
import pandas as pd
import numpy as np
import json, ast, pickle, time
from pathlib import Path
from scipy.sparse import csr_matrix
from sklearn.preprocessing import normalize
import torch

# Base directory locked to ULTIM
BASE_DIR  = Path("h:/Work/Game/ULTIM")
STEP5_DIR = BASE_DIR / "Step5_Matrix"
STEP7_DIR = BASE_DIR / "Step7_Player_Profile"
STEP8_DIR = BASE_DIR / "Step8_Game_Profile"
RAW_DATA_DIR = BASE_DIR / "raw_data/steam"
DEVICE    = torch.device("cuda" if torch.cuda.is_available() else "cpu")

def run():
    t0 = time.time()
    print("----------------------------------------------------------------------")
    print("  ULTIMATE GAME RECOMMENDATION ENGINE - PRODUCTION MODE")
    print("----------------------------------------------------------------------")
    print(">>> [1/5] Loading Assets (SVD, Word2Vec, Clusters)...")

    with open(STEP5_DIR / "id_mappings.json") as f: id_maps = json.load(f)
    idx_to_player, idx_to_game = id_maps['idx_to_player'], id_maps['idx_to_game']
    n_games = len(idx_to_game)
    n_users_total = len(idx_to_player)
    gid_to_gidx = {gid: i for i, gid in enumerate([idx_to_game[str(i)] for i in range(n_games)])}

    val_truth  = pd.read_csv(STEP5_DIR / "val_indices.csv").groupby('user_idx')['item_idx'].apply(set).to_dict()
    train_dict = pd.read_csv(STEP5_DIR / "train_indices.csv").groupby('user_idx')['item_idx'].apply(set).to_dict()

    # -- Game Profile --
    df_g = pd.read_csv(STEP8_DIR / "game_dna_profile.csv", dtype={'gameid': str}, low_memory=False)
    svd_gids = [idx_to_game[str(i)] for i in range(n_games)]
    df_g = df_g.set_index('gameid').reindex(svd_gids).fillna(0)
    cluster_labels = df_g['game_cluster'].fillna(-1).astype(int).values
    max_c = int(cluster_labels.max() + 1)
    pop_raw = pd.to_numeric(df_g['real_purchase_cnt'], errors='coerce').fillna(1.0).values
    pop_norm = np.log1p(pop_raw); pop_norm /= (pop_norm.max() + 1e-9)
    Pop_t = torch.tensor(pop_norm, dtype=torch.float32, device=DEVICE).unsqueeze(0)

    from gensim.models import Word2Vec
    w2v = Word2Vec.load(str(STEP8_DIR / "tag_word2vec_v2.model"))
    def clean_tags(v): return [i.strip().lower().replace(" ","_") for i in str(v).replace('[','').replace(']','').replace("'","").split(',')]
    tag_counts = {}
    for _, row in df_g.iterrows():
        for t in set(clean_tags(row['genres']) + clean_tags(row['top_tags'])): tag_counts[t] = tag_counts.get(t, 0) + 1
    idf_w = {t: np.log(len(df_g)/(c+1)) for t, c in tag_counts.items()}
    G_sem = np.zeros((n_games, 64), dtype=np.float32)
    for i, (_, row) in enumerate(df_g.iterrows()):
        vs, ws = [], []
        for t in set(clean_tags(row['genres']) + clean_tags(row['top_tags'])):
            if t in w2v.wv: vs.append(w2v.wv[t]); ws.append(idf_w.get(t, 1.0))
        if vs: G_sem[i] = np.average(vs, axis=0, weights=ws)
    G_sem_t = torch.tensor(G_sem, device=DEVICE)
    G_sem_t /= (torch.norm(G_sem_t, dim=1, keepdim=True) + 1e-9)

    # -- SVD --
    with open(BASE_DIR / "svd_model_assets.pkl", 'rb') as f:
        assets = pickle.load(f)
        US = torch.tensor(assets['user_factors'], dtype=torch.float32, device=DEVICE)
        Vt = torch.tensor(assets['item_factors'].T, dtype=torch.float32, device=DEVICE)

    # -- Item-CF --
    print(">>> [2/5] Building Item-CF Sparse Matrix...")
    train_df = pd.read_csv(STEP5_DIR / "train_indices.csv")
    M = csr_matrix(
        (np.ones(len(train_df), dtype=np.float32),
         (train_df['item_idx'].values, train_df['user_idx'].values)),
        shape=(n_games, n_users_total)
    )
    M_norm = normalize(M, norm='l2', axis=1)

    # -- User Clusters & Pop --
    print(">>> [3/5] Building Cluster-based Popularity Tower...")
    df_p_v2 = pd.read_csv(STEP7_DIR / "player_dna_profile_v2.csv", dtype={'playerid': str})
    pid_to_cluster = df_p_v2.set_index('playerid')['auto_cluster_id'].to_dict()
    n_pclusters = int(df_p_v2['auto_cluster_id'].max() + 1)
    cluster_size_real = np.zeros(n_pclusters)
    cluster_game_pop  = np.zeros((n_pclusters, n_games), dtype=np.float32)
    for user_idx, item_idx in zip(train_df['user_idx'].values, train_df['item_idx'].values):
        pid = idx_to_player[str(user_idx)]
        if pid in pid_to_cluster and item_idx < n_games:
            cluster_game_pop[pid_to_cluster[pid], item_idx] += 1
    for pid, k in pid_to_cluster.items(): cluster_size_real[k] += 1
    for k in range(n_pclusters):
        if cluster_size_real[k] > 0:
            cluster_game_pop[k] = np.log1p(cluster_game_pop[k]) / (np.log(cluster_size_real[k]+1)+1e-9)

    # -- Sampling for Eval --
    df_pur = pd.read_csv(RAW_DATA_DIR / "purchased_games.csv", dtype={'playerid': str}, low_memory=False).set_index('playerid')
    val_u_list = [u for u in val_truth.keys()
                  if idx_to_player[str(u)] in df_pur.index
                  and idx_to_player[str(u)] in pid_to_cluster]
    np.random.seed(42)
    test_u = np.random.choice(val_u_list, 2000, replace=False)
    test_pids = [idx_to_player[str(u)] for u in test_u]

    print(">>> [4/5] Computing Scores on GPU...")
    P_sem = np.zeros((2000, 64),    dtype=np.float32)
    P_cls = np.zeros((2000, max_c), dtype=np.float32)
    ICF   = np.zeros((2000, n_games), dtype=np.float32)
    CluPop= np.zeros((2000, n_games), dtype=np.float32)

    for i, pid in enumerate(test_pids):
        try:
            lib = ast.literal_eval(df_pur.at[pid, 'library'])
            gidx_p = [gid_to_gidx[str(g)] for g in lib if str(g) in gid_to_gidx]
            if gidx_p:
                w_s = np.ones(len(gidx_p)); w_s[-min(len(gidx_p),20):] = 3.0
                P_sem[i] = np.average(G_sem[gidx_p], axis=0, weights=w_s)
                cc = np.bincount(cluster_labels[gidx_p], minlength=max_c)
                P_cls[i] = cc / (cc.sum() + 1e-9)
            owned_train = list(train_dict.get(test_u[i], []))
            if owned_train:
                qv = M_norm[owned_train].sum(axis=0)
                ICF[i] = np.asarray(M_norm.dot(qv.T)).flatten()
            CluPop[i] = cluster_game_pop[pid_to_cluster[pid]]
        except: pass

    P_sem_t = torch.tensor(P_sem, device=DEVICE); P_sem_t /= (torch.norm(P_sem_t, dim=1, keepdim=True) + 1e-9)

    with torch.no_grad():
        U_test = US[test_u]
        S_SVD = torch.matmul(U_test, Vt)
        S_SVD = (S_SVD - S_SVD.min(1,True).values) / (S_SVD.max(1,True).values - S_SVD.min(1,True).values + 1e-9)
        S_Sem = (torch.matmul(P_sem_t, G_sem_t.T) + 1.0) / 2.0
        S_Pop = Pop_t.repeat(2000, 1)
        S_Prof= torch.tensor(P_cls[:, cluster_labels], device=DEVICE)
        S_ICF = torch.tensor(ICF, device=DEVICE)
        S_ICF = (S_ICF - S_ICF.min(1,True).values) / (S_ICF.max(1,True).values - S_ICF.min(1,True).values + 1e-9)
        S_CP  = torch.tensor(CluPop, device=DEVICE)
        S_CP  = (S_CP - S_CP.min(1,True).values) / (S_CP.max(1,True).values - S_CP.min(1,True).values + 1e-9)

    # Golden Recipe: 1.2*SVD + 0.5*Sem + 1.5*Pop + 0.2*Prof + 0.1*ICF + 0.05*CP
    Final_Scores = 1.2 * S_SVD + 0.5 * S_Sem + 1.5 * S_Pop + 0.2 * S_Prof + 0.1 * S_ICF + 0.05 * S_CP

    print(">>> [5/5] Final Audit...")
    s_eval = Final_Scores.cpu().numpy(); h, r = 0, 0
    for i, u in enumerate(test_u):
        truth = val_truth[u]; r += len(truth)
        sc = s_eval[i].copy(); owned = list(train_dict.get(u, []))
        if owned: sc[owned] = -1e9
        h += len(set(np.argpartition(sc, -20)[-20:]) & truth)
    
    print("\n" + "="*50)
    print(f"  Final Recall@20: {h/r:.2%}")
    print(f"  Total Time: {time.time()-t0:.1f}s")
    print("==================================================\n")

if __name__ == "__main__":
    run()
