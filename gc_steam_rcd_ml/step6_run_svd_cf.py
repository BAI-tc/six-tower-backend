import pandas as pd
import numpy as np
import json
import logging
import time
from scipy.sparse import csr_matrix
from sklearn.decomposition import TruncatedSVD
from pathlib import Path

# ????
logging.basicConfig(level=logging.INFO, format='%(asctime)s - [%(levelname)s] - %(message)s')
logger = logging.getLogger(__name__)

class GameRecommender:
    def __init__(self, n_components=64):
        self.n_components = n_components
        self.user_factors = None
        self.item_factors = None
        self.id_mappings = None
        self.train_matrix = None
        self.n_users = 0
        self.n_items = 0
        self.data_dir = Path("h:/Work/Game/ULTIM/Step5_Matrix")

    def load_data(self):
        start_time = time.time()
        logger.info("????????????...")
        
        # ?????dtypes ?????
        train_df = pd.read_csv(self.data_dir / "train_indices.csv", dtype={'user_idx': np.int32, 'item_idx': np.int32})
        with open(self.data_dir / "id_mappings.json", "r", encoding='utf-8') as f:
            self.id_mappings = json.load(f)
        
        self.n_users = len(self.id_mappings['player_to_idx'])
        self.n_items = len(self.id_mappings['game_to_idx'])
        
        logger.info(f"????????({self.n_users:,} ?? x {self.n_items:,} ??)...")
        # ??????
        self.train_matrix = csr_matrix(
            (np.ones(len(train_df), dtype=np.int8), (train_df['user_idx'], train_df['item_idx'])),
            shape=(self.n_users, self.n_items)
        )
        
        logger.info(f"?????????: {time.time() - start_time:.2f}s")
        return self

    def train(self):
        start_time = time.time()
        logger.info(f"?? SVD ?? (??={self.n_components})... ????????...")
        
        svd = TruncatedSVD(n_components=self.n_components, random_state=42)
        
        # ???????? (n_users, n_components)
        self.user_factors = svd.fit_transform(self.train_matrix)
        # ???????? (n_items, n_components)
        self.item_factors = svd.components_.T
        
        logger.info(f"?????????: {time.time() - start_time:.2f}s")
        return self

    def evaluate(self, top_k=10):
        logger.info(f"???????? (Recall@{top_k})...")
        val_df = pd.read_csv(self.data_dir / "val_indices.csv")
        
        # ???????????????(Ground Truth)
        val_truth = val_df.groupby('user_idx')['item_idx'].apply(set).to_dict()
        val_user_indices = list(val_truth.keys())
        
        hits = 0
        total_relevant = 0
        
        start_eval = time.time()
        BATCH = 2000  # ???? 2000 ???

        # ???????????? "??????" ?????????????????
        logger.info("   ?????????..")
        owned_dict = {}  # u_idx -> set of item_idx
        for u_idx in val_user_indices:
            owned_dict[u_idx] = set(self.train_matrix[u_idx].indices)
        logger.info("   ????????????..")

        for batch_start in range(0, len(val_user_indices), BATCH):
            batch_indices = val_user_indices[batch_start: batch_start + BATCH]
            users_batch = self.user_factors[batch_indices]        # (B, K)
            scores_matrix = users_batch.dot(self.item_factors.T) # (B, n_items)
            
            for i, u_idx in enumerate(batch_indices):
                scores = scores_matrix[i].copy()
                
                # ???????
                owned = list(owned_dict[u_idx])
                if owned:
                    scores[owned] = -np.inf
                
                # ?argpartition ?? top-k?? argsort ????
                part = np.argpartition(scores, -top_k)[-top_k:]
                top_indices = part[np.argsort(scores[part])[::-1]]
                
                true_items = val_truth[u_idx]
                hits += len(set(top_indices) & true_items)
                total_relevant += len(true_items)

            done = batch_start + len(batch_indices)
            if done % 20000 < BATCH:
                elapsed = time.time() - start_eval
                logger.info(f"   ???{done}/{len(val_user_indices)}??? {elapsed:.1f}s")

        recall = hits / total_relevant if total_relevant > 0 else 0
        logger.info(f"???????: {time.time() - start_eval:.2f}s")
        logger.info(f"Result: Recall@{top_k} = {recall:.4%}")
        return recall

    def get_recommendations(self, user_idx_raw, top_k=5):
        """?????????""
        if str(user_idx_raw) in self.id_mappings['player_to_idx']:
            user_idx = self.id_mappings['player_to_idx'][str(user_idx_raw)]
        else:
            user_idx = user_idx_raw
            
        user_vector = self.user_factors[user_idx].reshape(1, -1)
        scores = user_vector.dot(self.item_factors.T).flatten()
        
        owned_items = self.train_matrix[user_idx].indices
        scores[owned_items] = -np.inf
        
        top_indices = np.argsort(scores)[-top_k:][::-1]
        
        df_games = pd.read_csv("h:/Work/Game/ULTIM/Step3_Elite_Archive/elite_games_filtered.csv", usecols=['gameid', 'title'], dtype=str)
        title_map = dict(zip(df_games['gameid'], df_games['title']))
        
        results = []
        for idx in top_indices:
            raw_game_id = self.id_mappings['idx_to_game'][str(idx)]
            game_title = title_map.get(raw_game_id, "????")
            results.append({"game_id": raw_game_id, "title": game_title, "score": float(scores[idx])})
            
        return results

if __name__ == "__main__":
    # ??
    engine = GameRecommender(n_components=64)
    engine.load_data()
    engine.train()
    
    # ?????????
    engine.evaluate(top_k=20)

    # ?? ?????? (?Step 10 ?????? ?????????????????
    model_out = Path("h:/Work/Game/ULTIM/svd_model_assets.pkl")
    import pickle
    with open(model_out, 'wb') as f:
        pickle.dump({
            'user_factors': engine.user_factors,
            'item_factors': engine.item_factors
        }, f)
    logger.info(f"SVD ??????? {model_out}")
    
    # ????????????????
    val_df = pd.read_csv(Path("h:/Work/Game/ULTIM/Step5_Matrix/val_indices.csv"))
    active_user = val_df['user_idx'].value_counts().index[0]  # ????????????
    
    recs = engine.get_recommendations(active_user, top_k=10)
    
    logger.info(f"\n[????] ?????(????={active_user}) ???Top-10 ???)
    for i, r in enumerate(recs):
        logger.info(f"  {i+1}. ??: {r['title']} (AppID: {r['game_id']}) ??? {r['score']:.4f}")
