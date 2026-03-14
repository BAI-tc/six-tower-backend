import pandas as pd
import numpy as np
import ast
import re
import logging
import os
from pathlib import Path

# ===============================================================================================
# ???????????? (NEW Step 1)
# ???ID????ast?????Explode?????????
# ===============================================================================================

logging.basicConfig(level=logging.INFO, format='%(asctime)s - [%(levelname)s] - %(message)s')
logger = logging.getLogger(__name__)

class MasterDataEngine:
    def __init__(self, raw_base="h:/Work/Game/ULTIM/raw_data"):
        self.raw_base = Path(raw_base)
        self.clean_id_pattern = re.compile(r'\.0$')
        
        # --- ???????????????????(from NEW/raw_data/desc) ---
        desc_path = self.raw_base / "desc" / "games_march2025_cleaned.csv"
        if desc_path.exists():
            logger.info(f">>> ???????????????...")
            # ?????????????????00% ???Steam Master ????
            # (??????????? name, developers, publishers, genres, release_date, supported_languages)
            desc_usecols = [
                'appid', 'about_the_game', 'achievements', 'average_playtime_2weeks', 'average_playtime_forever', 
                'categories', 'detailed_description', 'discount', 'dlc_count', 'estimated_owners', 
                'full_audio_languages', 'header_image', 'linux', 'mac', 'median_playtime_2weeks', 
                'median_playtime_forever', 'metacritic_score', 'metacritic_url', 'movies', 'negative', 
                'notes', 'num_reviews_recent', 'num_reviews_total', 'packages', 'pct_pos_recent', 
                'pct_pos_total', 'peak_ccu', 'positive', 'price', 'recommendations', 'required_age', 
                'reviews', 'score_rank', 'screenshots', 'short_description', 'support_email', 
                'support_url', 'tags', 'user_score', 'website', 'windows'
            ]
            self.df_desc = pd.read_csv(desc_path, dtype=str, usecols=desc_usecols)
            self.df_desc['appid'] = self.df_desc['appid'].apply(self.clean_id)
        else:
            logger.warning(f"!!! ??????: {desc_path}?????????)
            self.df_desc = None

    def profile_report(self, df, name, platform):
        """????????????????ID???""
        logger.info(f"[{platform}] ????: {name} | ??: {df.shape}")
        missing = (df.isna().mean() * 100).sort_values(ascending=False).head(5)
        if missing.any():
            logger.info(f"    ???(Top 5): {missing.to_dict()}")
        
        # ??ID ??
        id_cols = [c for c in ['playerid', 'gameid', 'appid'] if c in df.columns]
        for ic in id_cols:
            dup_count = df[ic].duplicated().sum()
            if dup_count > 0:
                logger.warning(f"    ??? {ic} ?? {dup_count} ???????????..")
                df.drop_duplicates(subset=[ic], inplace=True)
        return df

    def safe_parse_list(self, x):
        """???????????????Python ??"""
        if pd.isna(x) or str(x).strip() in ("", "[]"): return []
        try:
            val = ast.literal_eval(str(x).strip())
            return val if isinstance(val, list) else []
        except:
            return [i.strip() for i in re.sub(r"[\[\]\'\"]", "", str(x)).split(',') if i.strip()]

    def clean_id(self, s):
        """?? ID ???????""
        if pd.isna(s) or s == "": return ""
        return self.clean_id_pattern.sub('', str(s).strip())

    # --- ?? EDA ???? (Inspired by steam-games-eda.ipynb) ---
    
    def get_owners_median(self, s):
        """?'1000 - 2000' ??????????""
        if pd.isna(s) or " - " not in str(s): return 0
        try:
            parts = str(s).split(" - ")
            return (int(parts[0]) + int(parts[1])) / 2
        except: return 0

    def get_price_group(self, p):
        """???????????(Free, <5$, <10$...)"""
        try:
            val = float(p)
            if val == 0: return "Free"
            for limit in [5, 10, 20, 30, 50, 80]:
                if val <= limit: return f"< {limit}$"
            return "> 80$"
        except: return "Unknown"

    def detect_sensitive_content(self, note):
        """?????????????(?????????)"""
        note = str(note).lower()
        if any(w in note for w in ['sex', 'naked', 'nudity', 'erotic', 'porn']): return 'Adult/Sexual'
        if any(w in note for w in ['violence', 'blood', 'gore', 'kill']): return 'Violent'
        if any(w in note for w in ['suicide', 'depression', 'drug']): return 'Sensitive/Dark'
        return 'General'

    def parse_raw_date(self, d):
        """?????????? 'Oct 2024' ?'Oct 21, 2024'"""
        if pd.isna(d) or str(d).strip() == "": return pd.NaT
        d_str = str(d).replace(',', '').strip()
        for fmt in ("%b %d %Y", "%b %Y", "%Y-%m-%d"):
            try: return pd.to_datetime(d_str, format=fmt)
            except: continue
        return pd.to_datetime(d_str, errors='coerce')

    # --- ???????????????? ---

    def clean_html_text(self, text):
        """?????HTML ??"""
        if pd.isna(text) or text == "": return ""
        clean = re.compile('<.*?>')
        text = re.sub(clean, '', str(text))
        text = re.sub(r'\s+', ' ', text).strip()
        return text

    def extract_top_tags(self, tags_str):
        """???????????? 10 ?????""
        if pd.isna(tags_str) or tags_str == "" or tags_str == "{}": return ""
        try:
            tags_dict = ast.literal_eval(str(tags_str))
            if isinstance(tags_dict, dict):
                sorted_tags = sorted(tags_dict.items(), key=lambda x: x[1], reverse=True)[:10]
                return ", ".join([t[0] for t in sorted_tags])
        except: pass
        return ""

    def run_standardization_v2(self, platform):
        """????????????"""
        p_dir = self.raw_base / platform
        if not p_dir.exists():
            logger.error(f"???? {platform}?????? {p_dir}")
            return None
        
        logger.info(f"--- ????????? {platform.upper()} (?? source ???NEW/raw_data) ---")
        
        # 1. ???????
        games_f = p_dir / "games.csv"
        df_games = pd.read_csv(games_f, dtype=str)
        df_games['gameid'] = df_games['gameid'].apply(self.clean_id)
        
        # 2. ???????? (????????????????????????????)
        if self.df_desc is not None:
            df_games = df_games.merge(self.df_desc, left_on='gameid', right_on='appid', how='left')
            if 'appid' in df_games.columns: df_games.drop(columns=['appid'], inplace=True)
        
        # 3. ?????????
        if 'detailed_description' in df_games.columns:
            df_games['detailed_description_clean'] = df_games['detailed_description'].apply(self.clean_html_text)
        if 'tags' in df_games.columns:
            df_games['top_tags'] = df_games['tags'].apply(self.extract_top_tags)
        if 'estimated_owners' in df_games.columns:
            df_games['owners_median'] = df_games['estimated_owners'].apply(self.get_owners_median)
        if 'price' in df_games.columns:
            df_games['price_group'] = df_games['price'].apply(self.get_price_group)
            df_games['price_float'] = pd.to_numeric(df_games['price'], errors='coerce').fillna(0)
        if 'notes' in df_games.columns:
            df_games['content_warning'] = df_games['notes'].apply(self.detect_sensitive_content)
        if 'release_date' in df_games.columns:
            df_games['release_date_dt'] = df_games['release_date'].apply(self.parse_raw_date)

        df_games = self.profile_report(df_games, "games", platform)
        
        # ????????????????????????
        # ???????Step 8b ?????? col ????? ast.literal_eval ??
        for col in ['developers', 'publishers', 'genres', 'supported_languages']:
            if col in df_games.columns:
                df_games[f'{col}_list'] = df_games[col].apply(self.safe_parse_list)
                # ????????????????? "['Action', 'Indie']" -> "Action, Indie"?
                df_games[col] = df_games[f'{col}_list'].apply(lambda lst: ", ".join(str(x) for x in lst) if isinstance(lst, list) else "")

        # 4. ???????
        players_f = p_dir / "players.csv"
        df_players = None
        if players_f.exists():
            df_players = pd.read_csv(players_f, dtype=str)
            df_players['playerid'] = df_players['playerid'].apply(self.clean_id)
            df_players = self.profile_report(df_players, "players", platform)

        # 5. ?????? (Explode)
        purch_f = p_dir / "purchased_games.csv"
        df_inter = None
        if purch_f.exists():
            df_purch = pd.read_csv(purch_f, dtype=str)
            df_purch['playerid'] = df_purch['playerid'].apply(self.clean_id)
            logger.info(f"    ???????? (Explode interaction library)...")
            df_purch['lib_list'] = df_purch['library'].apply(self.safe_parse_list)
            df_inter = df_purch.explode('lib_list')[['playerid', 'lib_list']].rename(columns={'lib_list': 'gameid'})
            df_inter['gameid'] = df_inter['gameid'].apply(self.clean_id)
            df_inter = df_inter.dropna(subset=['gameid'])

        # 6. ?????????
        return {
            "games": df_games,
            "players": df_players,
            "interactions": df_inter
        }

    def save_final_standardized_base(self, platform):
        """?????????Step 1 ???""
        results = self.run_standardization_v2(platform)
        if not results: return
        
        save_path = Path("h:/Work/Game/ULTIM/Step1_Cleaned_Base") / platform
        if not save_path.exists(): save_path.mkdir(parents=True)
        
        logger.info(f">>> ???Step 1 ????????: {save_path}")
        if results['games'] is not None:
            results['games'].to_csv(save_path / "games_cleaned.csv", index=False)
        if results['players'] is not None:
            results['players'].to_csv(save_path / "players_cleaned.csv", index=False)
        if results['interactions'] is not None:
            results['interactions'].to_csv(save_path / "interactions_exploded.csv", index=False)
        
        logger.info(f"<<< Step 1 ???????{platform.upper()} ??????)

if __name__ == "__main__":
    engine = MasterDataEngine()
    # ???? raw_data ????????????????
    platforms = [d.name for d in Path("h:/Work/Game/ULTIM/raw_data").iterdir() if d.is_dir() and d.name != "desc" and d.name != "rawg"]
    logger.info(f"???????? {platforms}")
    
    for p in platforms:
        engine.save_final_standardized_base(p)
    
    logger.info(">>> NEW ????????????????????????)
