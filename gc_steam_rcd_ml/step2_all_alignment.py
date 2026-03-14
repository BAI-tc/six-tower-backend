import pandas as pd
import numpy as np
import logging
from pathlib import Path
from step1_standardize_and_audit import MasterDataEngine

# ===============================================================================================
# ???????????? (NEW Step 2 - Final Aligned)
# ?????9.8 ?????????Step 1 ???????????
# ===============================================================================================

logging.basicConfig(level=logging.INFO, format='%(asctime)s - [%(levelname)s] - %(message)s')
logger = logging.getLogger(__name__)

class UnifiedProjectMerge:
    def __init__(self, raw_base="h:/Work/Game/ULTIM/raw_data"):
        self.raw_base = Path(raw_base)
        self.engine = MasterDataEngine(raw_base)
        self.output_dir = Path("h:/Work/Game/ULTIM/Ultimate_Archive_v1")
        if not self.output_dir.exists(): self.output_dir.mkdir(parents=True)

    def run_full_alignment(self):
        # 1. ?? Step 1 ????????Steam ?? Base
        logger.info(">>> [1/3] ???? Step 1 ?????? Steam ?????..")
        res = self.engine.run_standardization_v2("steam")
        df_base = res['games']
        
        # 2. ?????????(???Step 1 ???????????????
        logger.info(f">>> [2/3] ??????????????????? {len(df_base)}")

        # 3. ?? RAWG ?????
        # ???Wikidata ?Web ??????????
        mapping_cache_path = Path("h:/Work/Game/ULTIM/rawg_steam_mapping_cache.json")
        steam_to_rawg = {}
        if mapping_cache_path.exists():
            import json
            with open(mapping_cache_path, 'r', encoding='utf-8') as f:
                steam_to_rawg = json.load(f)
            logger.info(f">>> [3/3] ????????????? {len(steam_to_rawg)} ??????)
        
        rawg_f = self.raw_base / "rawg" / "rawg-games-dataset.csv"
        if rawg_f.exists():
            logger.info(">>> [3/3] ????RAWG ??????...")
            
            # ???????????? (gameid -> rawg slug)
            df_base['rawg_id_mapped'] = df_base['gameid'].map(steam_to_rawg)
            
            # ???????? ID ????????????????
            df_base['title_norm'] = df_base['title'].str.lower().str.replace(r'[^a-z0-9]', '', regex=True)
            df_base['year'] = pd.to_datetime(df_base['release_date_dt'], errors='coerce').dt.year.fillna(0).astype(int)
            
            # ????RAWG ??????????????????
            rawg_all_cols = pd.read_csv(rawg_f, nrows=0).columns.tolist()
            exclude_rawg = ['developers', 'publishers', 'genres', 'tags', 'website']
            rawg_cols = [c for c in rawg_all_cols if c not in exclude_rawg]
            
            rawg_chunks = pd.read_csv(rawg_f, usecols=rawg_cols, chunksize=100000, dtype=str)
            
            collected_rawg = []
            for chunk in rawg_chunks:
                # ???RAWG ?????
                chunk['title_norm'] = chunk['name'].str.lower().str.replace(r'[^a-z0-9]', '', regex=True)
                chunk['year'] = pd.to_datetime(chunk['released'], errors='coerce').dt.year.fillna(0).astype(int)
                
                # ?????????????? ??slug ID ???
                mask_id = chunk['slug'].isin(df_base['rawg_id_mapped'].dropna())
                mask_fingerprint = chunk['title_norm'].isin(df_base['title_norm'])
                
                match = chunk[mask_id | mask_fingerprint].copy()
                if not match.empty:
                    collected_rawg.append(match)
            
            if collected_rawg:
                df_rawg = pd.concat(collected_rawg)
                # ?rawg ???????? slug ??????????
                # ???? RAWG ?????
                rawg_merge_cols = [c for c in df_rawg.columns if c not in df_base.columns or c in ['title_norm', 'year', 'slug']]
                
                # ?????????(1. ?ID; 2. ?????
                # ????????????df_rawg ?slug ??
                df_rawg_by_slug = df_rawg.drop_duplicates('slug')
                
                # ?? 1?? ID ??
                df_final = df_base.merge(
                    df_rawg_by_slug[rawg_merge_cols], 
                    left_on='rawg_id_mapped', right_on='slug', how='left', 
                    suffixes=('', '_rawg')
                )
                
                # ?????? 2???ID ??????????????????????????
                unmatched_mask = df_final['rating'].isna()
                if unmatched_mask.any():
                    df_rawg_unique_title = df_rawg.drop_duplicates(['title_norm', 'year'])
                    df_base_unmatched = df_final[unmatched_mask].copy()
                    
                    # ????+??
                    df_recovered = df_base_unmatched[['gameid', 'title_norm', 'year']].merge(
                        df_rawg_unique_title[rawg_merge_cols], 
                        on=['title_norm', 'year'], how='inner'
                    )
                    
                    if not df_recovered.empty:
                        df_final.set_index('gameid', inplace=True)
                        df_recovered.set_index('gameid', inplace=True)
                        df_final.update(df_recovered)
                        df_final.reset_index(inplace=True)

                    # ??????????????????? (?????????????)
                    unmatched_mask_v2 = df_final['rating'].isna()
                    df_rawg_absolute_unique = df_rawg.drop_duplicates('title_norm', keep=False)
                    df_base_unmatched_v2 = df_final[unmatched_mask_v2].copy()
                    
                    rawg_recover_cols = [c for c in rawg_merge_cols if c not in ('year', 'slug')]
                    # Ensure title_norm is uniquely present
                    if 'title_norm' not in rawg_recover_cols:
                        rawg_recover_cols.append('title_norm')
                        
                    df_recovered_v2 = df_base_unmatched_v2[['gameid', 'title_norm']].merge(
                        df_rawg_absolute_unique[rawg_recover_cols], 
                        on='title_norm', how='inner'
                    )
                    
                    if not df_recovered_v2.empty:
                        df_final.set_index('gameid', inplace=True)
                        df_recovered_v2.set_index('gameid', inplace=True)
                        df_final.update(df_recovered_v2)
                        df_final.reset_index(inplace=True)

                cols_to_drop = ['title_norm', 'year', 'slug', 'rawg_id_mapped']
                df_final.drop(columns=[c for c in cols_to_drop if c in df_final.columns], inplace=True)
                logger.info(f"   [Done] ?????????{len(df_final)} ????????)
            else:
                df_final = df_base
        else:
            df_final = df_base

        # 4. ??
        save_file = self.output_dir / "ultimate_all_games_full_aligned.csv"
        df_final.to_csv(save_file, index=False)
        logger.info(f">>> ????????????? {save_file}")
        
        return df_final

if __name__ == "__main__":
    merger = UnifiedProjectMerge()
    merger.run_full_alignment()
