import pandas as pd
import numpy as np
import logging
import ast
from pathlib import Path

# ===============================================================================================
# ???????? v2 (Step 4 - Elite User Filter)
#
# ???
#   ????? >= 1 ??????? ??????????(???????
#   ??????????           ????????? (???? OR ????) ??
#   ?????????              ???
#
# ???????purchased_games.csv (UNION) reviews.csv
# ===============================================================================================

logging.basicConfig(level=logging.INFO, format='%(asctime)s - [%(levelname)s] - %(message)s')
logger = logging.getLogger(__name__)

class EliteUserFilter:
    def __init__(self):
        self.steam_dir   = Path("h:/Work/Game/ULTIM/raw_data/steam")
        self.elite_games = Path("h:/Work/Game/ULTIM/Step3_Elite_Archive/elite_games_filtered.csv")
        self.output_dir  = Path("h:/Work/Game/ULTIM/Step4_Elite_Users")
        self.output_dir.mkdir(parents=True, exist_ok=True)

    def run(self):
        # ?? 0. ?????????????vs ???? ????????????
        logger.info(">>> [0/4] ???????????/ ??...")
        df_elite = pd.read_csv(
            self.elite_games,
            usecols=['gameid', 'price_float', 'price'],
            dtype=str
        )
        df_elite['price_num'] = pd.to_numeric(df_elite['price_float'], errors='coerce').fillna(0)

        free_mask      = (df_elite['price_num'] == 0) | \
                         (df_elite['price'].str.lower().str.contains('free', na=False))
        elite_free_set = set(df_elite.loc[free_mask,  'gameid'].str.strip())
        elite_paid_set = set(df_elite.loc[~free_mask, 'gameid'].str.strip())
        elite_all_set  = elite_free_set | elite_paid_set

        logger.info(f"   ??????  : {len(elite_all_set)}")
        logger.info(f"   ?? ????  : {len(elite_free_set)}")
        logger.info(f"   ?? ????  : {len(elite_paid_set)}")

        # ?? 1. ???? ?????????????????????
        logger.info(">>> [1/4] ?? purchased_games.csv...")
        df_pur = pd.read_csv(self.steam_dir / "purchased_games.csv", dtype=str)

        rows = []
        for _, row in df_pur.iterrows():
            pid = row['playerid']
            try:
                library = ast.literal_eval(row['library'])
                owned = [str(g) for g in library]
                paid_elite = sum(1 for g in owned if g in elite_paid_set)
                free_elite = sum(1 for g in owned if g in elite_free_set)
                rows.append({
                    'playerid'         : pid,
                    'total_owned'      : len(owned),
                    'paid_elite_owned' : paid_elite,
                    'free_elite_owned' : free_elite,
                    'elite_owned'      : paid_elite + free_elite,
                })
            except Exception:
                continue

        df_ownership = pd.DataFrame(rows)
        logger.info(f"   ??????????????: {len(df_ownership)}")

        # ?? 2. ???? ?????????????????????? ??
        logger.info(">>> [2/4] ?? reviews.csv (??????...")
        review_agg = {}   # playerid -> bool (?????????)

        for chunk in pd.read_csv(
            self.steam_dir / "reviews.csv",
            usecols=['playerid', 'gameid'],
            dtype=str,
            chunksize=100_000
        ):
            for _, row in chunk.iterrows():
                pid = row['playerid']
                gid = str(row['gameid'])
                if pid not in review_agg:
                    review_agg[pid] = False      # ???????False
                if gid in elite_all_set:
                    review_agg[pid] = True        # ????????True

        df_reviews = pd.DataFrame(
            list(review_agg.items()), columns=['playerid', 'has_elite_review']
        )
        logger.info(f"   reviews ??????: {len(df_reviews)}, "
                    f"??????????: {df_reviews['has_elite_review'].sum()}")

        # ?? 3. ???? ????????????? ?????????????
        logger.info(">>> [3/4] ?? history.csv...")
        df_ach = pd.read_csv(self.steam_dir / "achievements.csv",
                             usecols=['achievementid', 'gameid'], dtype=str)
        ach_to_game = dict(zip(df_ach['achievementid'], df_ach['gameid']))

        df_hist = pd.read_csv(self.steam_dir / "history.csv",
                              usecols=['playerid', 'achievementid'], dtype=str)
        df_hist['gameid'] = df_hist['achievementid'].map(ach_to_game)
        players_with_ach = set(df_hist[df_hist['gameid'].isin(elite_all_set)]['playerid'].unique())
        logger.info(f"   ??????????????: {len(players_with_ach)}")

        # ?? 4. ??????outer join ????????????????????
        logger.info(">>> [4/4] ?????????..")

        # ???? = ?????? UNION ???? (outer join)
        df_users = df_ownership.merge(df_reviews, on='playerid', how='outer')

        # ???????????????????? 0
        for col in ['total_owned', 'paid_elite_owned', 'free_elite_owned', 'elite_owned']:
            if col in df_users.columns:
                df_users[col] = df_users[col].fillna(0).astype(int)
        df_users['has_elite_review'] = df_users['has_elite_review'].fillna(False)
        df_users['has_elite_ach']    = df_users['playerid'].isin(players_with_ach)

        logger.info(f"   ?????????: {len(df_users)}")

        # ?? ???????????????????????????????????????????????
        has_paid_elite  = df_users['paid_elite_owned'] >= 1
        only_free_elite = (df_users['free_elite_owned'] >= 1) & (df_users['paid_elite_owned'] == 0)
        no_purchase     = df_users['elite_owned'] == 0   # ?????????? purchased_games ??
        is_active       = df_users['has_elite_review'] | df_users['has_elite_ach']

        rule1 = has_paid_elite                  # ?????????????
        rule2 = only_free_elite & is_active     # ??????? + ?? ???
        rule3 = no_purchase & is_active         # ?????????????/???? ???

        elite_mask = rule1 | rule2 | rule3

        df_elite_users  = df_users[elite_mask].copy()
        df_zombie_users = df_users[~elite_mask].copy()

        # ??????
        df_elite_users['elite_type'] = ''
        df_elite_users.loc[rule1[elite_mask].values, 'elite_type'] = '????'
        df_elite_users.loc[(rule2 & ~rule1)[elite_mask].values, 'elite_type'] = '????(??)'
        df_elite_users.loc[(rule3 & ~rule1 & ~rule2)[elite_mask].values, 'elite_type'] = '???????

        # ?? ???? ?????????????????????????????????????????????
        total  = len(df_users)
        n_paid = rule1.sum()
        n_free = (rule2 & ~rule1).sum()
        n_nopurchase = (rule3 & ~rule1 & ~rule2).sum()

        logger.info(f"\n{'='*60}")
        logger.info(f"?????????v2?)
        logger.info(f"   ??????     : {total}")
        logger.info(f"   ? ??????  : {len(df_elite_users)} ({len(df_elite_users)/total*100:.2f}%)")
        logger.info(f"      ?? ????  : {n_paid} ({n_paid/total*100:.2f}%)")
        logger.info(f"      ?? ????  : {n_free} ({n_free/total*100:.2f}%)")
        logger.info(f"      ?? ????? {n_nopurchase} ({n_nopurchase/total*100:.2f}%)")
        logger.info(f"   ? ?????   : {len(df_zombie_users)} ({len(df_zombie_users)/total*100:.2f}%)")
        logger.info(f"\n   ????????????: {df_elite_users['elite_owned'].mean():.1f} ?)
        logger.info(f"{'='*60}")

        # ?? ?? ?????????????????????????????????????????????????
        out_elite = self.output_dir / "elite_users.csv"
        df_elite_users.to_csv(out_elite, index=False)
        logger.info(f">>> ??????? {out_elite}")

        out_whitelist = self.output_dir / "elite_user_ids.txt"
        df_elite_users['playerid'].to_csv(out_whitelist, index=False, header=False)
        logger.info(f">>> ???ID ??: {out_whitelist}")

        return df_elite_users


if __name__ == "__main__":
    engine = EliteUserFilter()
    engine.run()
