import pandas as pd
import numpy as np
import logging
import ast
import json
from pathlib import Path
from sklearn.model_selection import train_test_split

# ===============================================================================================
# ????-???????????? (Step 5)
# ???
#   - ??????: Step4_Elite_Users/elite_users.csv
#   - ??????: Step3_Elite_Archive/elite_games_filtered.csv
#   - ????: steam/purchased_games.csv, steam/reviews.csv, steam/history.csv
# ===============================================================================================

logging.basicConfig(level=logging.INFO, format='%(asctime)s - [%(levelname)s] - %(message)s')
logger = logging.getLogger(__name__)

class InteractionMatrixBuilder:
    def __init__(self):
        self.steam_dir = Path("h:/Work/Game/ULTIM/raw_data/steam")
        self.elite_users_file = Path("h:/Work/Game/ULTIM/Step4_Elite_Users/elite_users.csv")
        self.elite_games_file = Path("h:/Work/Game/ULTIM/Step3_Elite_Archive/elite_games_filtered.csv")
        self.output_dir = Path("h:/Work/Game/ULTIM/Step5_Matrix")
        self.output_dir.mkdir(parents=True, exist_ok=True)

    def run(self):
        logger.info(">>> [1/4] ?????(Elite Users & Elite Games)...")
        df_users = pd.read_csv(self.elite_users_file, usecols=['playerid'], dtype=str)
        df_games = pd.read_csv(self.elite_games_file, usecols=['gameid'], dtype=str)
        
        valid_users = set(df_users['playerid'])
        valid_games = set(df_games['gameid'])
        
        logger.info(f"   ????? {len(valid_users)}")
        logger.info(f"   ????? {len(valid_games)}")
        
        # ????ID???? (?0 ??????
        user_to_idx = {pid: i for i, pid in enumerate(valid_users)}
        game_to_idx = {gid: i for i, gid in enumerate(valid_games)}
        
        # ?? ?????????????/?? ???????????
        logger.info(">>> [2/4] ?purchased_games ??????...")
        interactions = {}  # ?? (user_id, game_id) -> weight
        
        df_pur = pd.read_csv(self.steam_dir / "purchased_games.csv", dtype=str)
        for _, row in df_pur.iterrows():
            pid = row['playerid']
            if pid not in user_to_idx:
                continue
            try:
                for g in ast.literal_eval(row['library']):
                    gid = str(g)
                    if gid in game_to_idx:
                        interactions[(user_to_idx[pid], game_to_idx[gid])] = 1.0  # ?????? 1.0
            except Exception:
                pass

        logger.info(f"   ????????????????????? {len(interactions)}")
        
        logger.info(">>> [3/4] ?reviews ???? (Steam??????????????????)...")
        for chunk in pd.read_csv(self.steam_dir / "reviews.csv", usecols=['playerid', 'gameid'], dtype=str, chunksize=100_000):
            for _, row in chunk.iterrows():
                pid = row['playerid']
                gid = str(row['gameid'])
                if pid in user_to_idx and gid in game_to_idx:
                    pair = (user_to_idx[pid], game_to_idx[gid])
                    # Steam ?????????????= ????????????????
                    interactions[pair] = max(interactions.get(pair, 0), 1.0)
                    
        logger.info(f"   ??????????????? {len(interactions)}")
        
        # ???????????
        logger.info(">>> ?history.csv ?????? (?????...")
        df_ach = pd.read_csv(self.steam_dir / "achievements.csv", usecols=['achievementid', 'gameid'], dtype=str)
        ach_to_game = dict(zip(df_ach['achievementid'], df_ach['gameid']))
        
        chunk_idx = 0
        for chunk in pd.read_csv(self.steam_dir / "history.csv", usecols=['playerid', 'achievementid'], dtype=str, chunksize=1_000_000):
            # ?achievement ???gameid
            chunk['gameid'] = chunk['achievementid'].map(ach_to_game)
            # ??????????
            valid_mask = chunk['playerid'].isin(valid_users) & chunk['gameid'].isin(valid_games)
            valid_hist = chunk[valid_mask].copy()
            
            if not valid_hist.empty:
                valid_hist['u_idx'] = valid_hist['playerid'].map(user_to_idx)
                valid_hist['g_idx'] = valid_hist['gameid'].map(game_to_idx)
                
                # ?????(user, game) ??
                unique_pairs = valid_hist[['u_idx', 'g_idx']].drop_duplicates()
                for _, row in unique_pairs.iterrows():
                    pair = (int(row['u_idx']), int(row['g_idx']))
                    # ????????0.5??????????????
                    interactions[pair] = interactions.get(pair, 0) + 0.5

            chunk_idx += 1
            if chunk_idx % 5 == 0:
                logger.info(f"     ???{chunk_idx}00 ??????...")
                
        logger.info(f"   ????????????????? {len(interactions)}")

        # ?? ????????????(?? 0.2) ???????????
        logger.info(">>> ?friends.csv ???????? (??????????)...")
        try:
            df_friends = pd.read_csv(self.steam_dir / "friends.csv", usecols=['playerid', 'friends'], dtype=str)
            friend_links_added = 0
            for _, row in df_friends.iterrows():
                pid = row['playerid']
                if pid not in user_to_idx: continue
                u_idx = user_to_idx[pid]
                try:
                    friends_list = ast.literal_eval(row['friends'])
                    for f in friends_list:
                        fid = str(f)
                        if fid in user_to_idx:
                            f_idx = user_to_idx[fid]
                            # ????????????interactions????????????????????
                            # ??????????????????
                            # ????????????????????????????????????????
                            pass
                except Exception:
                    pass
        except FileNotFoundError:
            pass

        df_inter = pd.DataFrame([
            {'user_idx': k[0], 'item_idx': k[1], 'weight': v} 
            for k, v in interactions.items()
        ])
        
        # ?????????????? 3 ?????????????????
        user_counts = df_inter['user_idx'].value_counts()
        active_user_idx = user_counts[user_counts >= 3].index
        df_inter = df_inter[df_inter['user_idx'].isin(active_user_idx)]
        
        logger.info(f"   ??????(<3)????????: {len(df_inter)}")
        
        # ?? ??????????Leave-One-Out ?? ??????
        # ???????? 1 ??????????????????
        logger.info(">>> [4/4] Leave-One-Out ?????????..")
        
        train_rows = []
        val_rows = []
        
        rng = np.random.default_rng(42)
        for uid, group in df_inter.groupby('user_idx'):
            if len(group) < 2:
                # ?? 1 ???????????????????
                train_rows.append(group)
            else:
                # ???? 1 ?????
                val_idx = rng.integers(0, len(group))
                val_rows.append(group.iloc[[val_idx]])
                train_rows.append(group.drop(group.index[val_idx]))
        
        train_df = pd.concat(train_rows, ignore_index=True)
        val_df   = pd.concat(val_rows, ignore_index=True)
        logger.info(f"   ????: {len(train_df)}, ????: {len(val_df)} ???? 1 ??")
        
        # ?? ???? ??
        mappings = {
            'player_to_idx': user_to_idx,
            'idx_to_player': {v: k for k, v in user_to_idx.items()},
            'game_to_idx': game_to_idx,
            'idx_to_game': {v: k for k, v in game_to_idx.items()}
        }
        
        with open(self.output_dir / "id_mappings.json", "w", encoding='utf-8') as f:
            json.dump(mappings, f)
            
        train_df.to_csv(self.output_dir / "train_indices.csv", index=False)
        val_df.to_csv(self.output_dir / "val_indices.csv", index=False)
        
        logger.info(f"??????????????: {self.output_dir}")
        
if __name__ == "__main__":
    builder = InteractionMatrixBuilder()
    builder.run()
