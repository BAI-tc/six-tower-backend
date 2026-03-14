import pandas as pd
import numpy as np
import logging
from pathlib import Path

# ===============================================================================================
# ???????? (NEW Step 3 - Elite Game Filter)
# ===============================================================================================

logging.basicConfig(level=logging.INFO, format='%(asctime)s - [%(levelname)s] - %(message)s')
logger = logging.getLogger(__name__)

class EliteGameFilter:
    def __init__(self):
        self.input_file = Path("h:/Work/Game/ULTIM/Ultimate_Archive_v1/ultimate_all_games_full_aligned.csv")
        self.output_dir = Path("h:/Work/Game/ULTIM/Step3_Elite_Archive")
        if not self.output_dir.exists(): self.output_dir.mkdir(parents=True)

    def run_filter(self):
        logger.info(f">>> [1/3] ????????: {self.input_file}")
        df = pd.read_csv(self.input_file, dtype=str)
        
        logger.info(f"   ????? {len(df)} ???)
        
        # ?????????
        # 1. CCU (??????
        if 'peak_ccu' in df.columns:
            df['peak_ccu_num'] = pd.to_numeric(df['peak_ccu'], errors='coerce').fillna(0)
        else:
            df['peak_ccu_num'] = 0
            
        # 2. ??????
        if 'owners_median' in df.columns:
            df['owners_num'] = pd.to_numeric(df['owners_median'], errors='coerce').fillna(0)
        else:
            df['owners_num'] = 0
            
        # 3. ????
        if 'release_date_dt' in df.columns:
            df['release_year'] = pd.to_datetime(df['release_date_dt'], errors='coerce').dt.year.fillna(0)
        elif 'released' in df.columns: # rawg ??
            df['release_year'] = pd.to_datetime(df['released'], errors='coerce').dt.year.fillna(0)
        else:
            df['release_year'] = 0
            
        # =======================================================
        # ??????????????
        # =======================================================
        # =======================================================
        # 1. ???? (?? revenue_est = total_sales * price_usd)
        # ??????????? owners_median * price_float ?????? revenue_est
        if 'price_float' in df.columns:
            df['price_float_num'] = pd.to_numeric(df['price_float'], errors='coerce').fillna(0)
            df['revenue_est'] = df['owners_num'] * df['price_float_num']
        else:
            df['price_float_num'] = pd.to_numeric(df['price'] if 'price' in df.columns else 0, errors='coerce').fillna(0)
            df['revenue_est'] = df['owners_num'] * df['price_float_num']

        # 2. ???? (?? community_authority = helpful + awards * 5)
        # ???????? 'recommendations' ??????'num_reviews_total' ????????
        if 'recommendations' in df.columns:
            df['community_authority'] = pd.to_numeric(df['recommendations'], errors='coerce').fillna(0)
        elif 'num_reviews_total' in df.columns:
            df['community_authority'] = pd.to_numeric(df['num_reviews_total'], errors='coerce').fillna(0)
        else:
            df['community_authority'] = 0

        # =======================================================
        # ???????????(Vitality Score)
        # =======================================================
        logger.info(f">>> [2/3] ???????????????? (Vitality Scoring)...")
        
        # ???????????(???
        if 'achievements' in df.columns:
            df['ach_total'] = pd.to_numeric(df['achievements'], errors='coerce').fillna(0)
        elif 'achievements_count' in df.columns:
            df['ach_total'] = pd.to_numeric(df['achievements_count'], errors='coerce').fillna(0)
        else:
            df['ach_total'] = 0

        df['vitality_score'] = 0
        
        # --- ?????????---
        # 1. ???(CCU > 20) -> +50
        df.loc[df['peak_ccu_num'] > 20, 'vitality_score'] += 50
        
        # 2. ???? (Revenue > 200) -> +50 
        # (?????? revenue_est ?????*??????????????????????????????????????????0?
        df.loc[df['revenue_est'] > 200, 'vitality_score'] += 50
        
        # 3. ?????? (Achievements > 5) -> +30
        df.loc[df['ach_total'] > 5, 'vitality_score'] += 30
        
        # 4. ???? (Release >= 2024) -> +20
        df.loc[df['release_year'] >= 2024, 'vitality_score'] += 20
        
        # 5. ???? (MC?RAWG????? -> +10
        # ??????????????
        mc_col = next((c for c in ['metacritic', 'mc_score', 'metacritic_score'] if c in df.columns), None)
        rawg_col = next((c for c in ['rating', 'rawg_rating', 'mc_score_rawg'] if c in df.columns), None)
        
        if mc_col:
            df['_mc_num'] = pd.to_numeric(df[mc_col], errors='coerce').fillna(0)
            df.loc[df['_mc_num'] > 0, 'vitality_score'] += 10
        if rawg_col and rawg_col != mc_col:
            df['_rawg_num'] = pd.to_numeric(df[rawg_col], errors='coerce').fillna(0)
            df.loc[df['_rawg_num'] > 0, 'vitality_score'] += 5  # RAWG????
        
        # 6. ???? (????) -> +10
        df.loc[df['community_authority'] > 10, 'vitality_score'] += 10

        # ???? (???? 35?
        survival_threshold = 35
        survival_mask = df['vitality_score'] >= survival_threshold
        
        df_elite = df[survival_mask].copy()
        df_zombies = df[~survival_mask].copy()
        
        logger.info(f"   === ?????? ===")
        logger.info(f"   ??? {len(df)}")
        logger.info(f"   ? ????: {len(df_elite)} ({len(df_elite)/len(df)*100:.2f}%)")
        logger.info(f"   ? ????: {len(df_zombies)} ({len(df_zombies)/len(df)*100:.2f}%)")
        
        # ?????????? (???????????????)
        
        save_file = self.output_dir / "elite_games_filtered.csv"
        df_elite.to_csv(save_file, index=False)
        logger.info(f">>> [3/3] ??????????: {save_file}")
        
if __name__ == "__main__":
    filter_engine = EliteGameFilter()
    filter_engine.run_filter()
