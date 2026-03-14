import pandas as pd
import numpy as np
import json, ast
import requests
import sys
import io
import warnings
from pathlib import Path

# 抑制 pandas DtypeWarning 警告
warnings.filterwarnings('ignore', category=pd.errors.DtypeWarning)

# 设置 stdout 编码为 UTF-8，解决 Windows 下的 GBK 编码问题
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')
sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')

# 配置路径 - 自动适配当前项目根目录
BASE_DIR = Path(__file__).resolve().parent.parent / "ULTIM"
STEP8_DIR = BASE_DIR / "Step8_Game_Profile"
RAW_DATA_DIR = BASE_DIR / "raw_data" / "steam"
OUTPUT_PATH = BASE_DIR / "player_semantic_preference.csv"

# Steam API 配置
STEAM_API_KEY = "A11C381F817AB411C131C8AC2F60CB5F"  # 项目中的API Key
STEAM_API_URL = "https://api.steampowered.com"


def fetch_player_games_from_steam(steam_id, verbose=False):
    """
    从Steam API获取玩家的游戏列表
    """
    url = f"{STEAM_API_URL}/IPlayerService/GetOwnedGames/v0001/"
    params = {
        "key": STEAM_API_KEY,
        "steamid": steam_id,
        "format": "json",
        "include_appinfo": "false",  # 不需要appinfo，后续用本地库匹配
        "include_played_free_games": "true"
    }

    try:
        response = requests.get(url, params=params, timeout=30)
        if response.status_code == 200:
            data = response.json()
            if "response" in data and "games" in data["response"]:
                games = data["response"]["games"]
                game_ids = [g["appid"] for g in games]
                if verbose:
                    print(f"    [Steam API] 成功获取 {len(game_ids)} 款游戏")
                return game_ids
            else:
                if verbose:
                    print(f"    [Steam API] 玩家游戏库为空或未公开")
                return None
        elif response.status_code == 401:
            if verbose:
                print(f"    [Steam API] API Key无效")
            return None
        else:
            if verbose:
                print(f"    [Steam API] 请求失败: {response.status_code}")
            return None
    except Exception as e:
        if verbose:
            print(f"    [Steam API] 请求异常: {e}")
        return None

class RuleReranker:
    """
    业务规则重排引擎 - 实现用户提出的三大规则
    """
    def __init__(self, owned_game_ids):
        # 规则 3: 已消费集合 (Inventory)
        self.owned_ids = set(str(gid) for gid in owned_game_ids)
        # 规则 2: 全局页面占位符 (Cross-Module Seen Set)
        self.global_seen_ids = set()
    
    def apply(self, candidates, top_n=5, module_type="discovery"):
        """
        candidates: List of game dicts with 'gameid', 'title', 'recommendations', 'metacritic'
        module_type: 'discovery' (猜你喜欢/主特征), 'hot' (热门), 'replay' (重温)
        """
        final_list = []
        
        # 规则 1 & 排序逻辑: 
        # 我们在这里模拟 Tower Fusion 得分: FinalScore = HeatNorm * 0.7 + MetaNorm * 0.3
        # 如果是多塔，这里应该是各塔分数的累加
        for g in candidates:
            heat = float(g.get('recommendations', 0))
            meta = float(g.get('metacritic', 0)) if g.get('metacritic') else 75.0 # 默认 75 分
            
            # 基础分：利用对数平滑处理热度，防止长尾问题
            heat_score = np.log1p(heat) / 15.0 # 归一化参考值
            meta_score = meta / 100.0
            
            # 融合得分 (Golden Recipe 简化版)
            g['fusion_score'] = (heat_score * 0.7) + (meta_score * 0.3)
            
        # 按融合分从大到小排序
        candidates.sort(key=lambda x: x.get('fusion_score', 0), reverse=True)
        
        for game in candidates:
            gid = str(game['gameid'])
            
            # 规则 3: 已消费去重 (Strong Filtering)
            if module_type in ["discovery", "hot"]:
                if gid in self.owned_ids:
                    continue
            elif module_type == "replay":
                if gid not in self.owned_ids:
                    continue
                    
            # 规则 2: 跨模块去重 (Seen Logic)
            if gid in self.global_seen_ids:
                continue
            
            final_list.append(game)
            self.global_seen_ids.add(gid) # 占位
            
            if len(final_list) >= top_n:
                break
                
        return final_list

def get_player_preference(target_player_id=None, show_recommendations=True, top_n_recommend=10, json_output=False):
    """
    分析玩家偏好并推荐游戏（集成规则重排版）
    """
    verbose = not json_output

    if verbose:
        print(">>> 启动重排推荐引擎...")
    
    # --- 加载数据资产 ---
    dna_path = STEP8_DIR / "game_dna_profile_v2.csv"
    if not dna_path.exists(): return
    df_g = pd.read_csv(dna_path, dtype={'gameid': str})
    
    game_to_theme = df_g.set_index('gameid')['semantic_theme_id'].to_dict()
    theme_to_games_dict = {}
    for theme_id, group in df_g.groupby('semantic_theme_id'):
        theme_to_games_dict[int(theme_id)] = group.to_dict('records')
    
    leaderboard_path = STEP8_DIR / "theme_leaderboard.csv"
    theme_id_to_name = pd.read_csv(leaderboard_path).set_index('theme_id')['theme_name'].to_dict() if leaderboard_path.exists() else {}
    
    num_themes = 150

    # --- 获取玩家库 ---
    purchased_path = RAW_DATA_DIR / "purchased_games.csv"
    df_pur = pd.read_csv(purchased_path, dtype={'playerid': str}).set_index('playerid')

    if target_player_id:
        if target_player_id not in df_pur.index:
            steam_games = fetch_player_games_from_steam(target_player_id, verbose)
            if steam_games:
                player_id_list = [target_player_id]
                external_lib = {target_player_id: str(steam_games)}
            else: return None
        else:
            player_id_list = [target_player_id]
            external_lib = None
    else:
        player_id_list = df_pur.index[:10].tolist()
        external_lib = None

    for pid in player_id_list:
        try:
            if external_lib and pid in external_lib:
                lib = ast.literal_eval(str(external_lib[pid]))
            else:
                lib_val = df_pur.at[pid, 'library']
                if isinstance(lib_val, pd.Series): lib_val = lib_val.iloc[0]
                lib = ast.literal_eval(str(lib_val))
            
            if not lib: continue

            # --- 召回层 (Recall Phase) ---
            themes_in_lib = [int(game_to_theme[str(gid)]) for gid in lib if str(gid) in game_to_theme]
            
            if not themes_in_lib:
                # 冷启动/新用户兜底：选取一些代表性的多样化主题
                # 比如：动作(主题1)、角色扮演(主题2)、策略(主题3)等，这里简化为常驻主题
                fallback_themes = [1, 5, 10, 20, 30] 
                pref_dist = np.zeros(num_themes)
                for t in fallback_themes:
                    pref_dist[t] = 0.2
                if verbose: print(f"    [Fallback] 新用户召回默认多样化主题")
            else:
                weights = np.ones(len(themes_in_lib))
                if len(weights) > 20: weights[-20:] = 3.0
                counts = np.bincount(themes_in_lib, weights=weights, minlength=num_themes)
                pref_dist = counts / (counts.sum() + 1e-9)

            top_indices = np.argsort(pref_dist)[-10:][::-1]
            top_preferences_list = []
            
            # --- 初始化重排引擎 (Rerank Engine) ---
            reranker = RuleReranker(owned_game_ids=lib)
            recommendations = {}

            for i in top_indices:
                if pref_dist[i] > 0:
                    name = theme_id_to_name.get(i, f"Cluster_{i}")
                    score = pref_dist[i]
                    top_preferences_list.append(f"{name} ({score:.1%})")

                    # 1. 召回候选集 (Recall Candidates)
                    if show_recommendations and i in theme_to_games_dict:
                        candidates = []
                        for g in theme_to_games_dict[i]:
                            candidates.append({
                                'gameid': str(g['gameid']),
                                'title': g['title'],
                                'recommendations': int(g.get('recommendations', 0)),
                                'metacritic': float(g.get('metacritic_score', 0)) if not pd.isna(g.get('metacritic_score')) else None
                            })
                        
                        # 2. 执行重排 (Rerank)
                        # 自动处理了规则 1(融合评分), 规则 2(跨主题去重), 规则 3(库过滤)
                        final_rec = reranker.apply(candidates, top_n=top_n_recommend, module_type="discovery")
                        
                        if final_rec:
                            recommendations[name] = final_rec

            result = {
                'playerid': pid,
                'owned_games_count': len(lib),
                'top_preferences': top_preferences_list,
                'recommendations': recommendations
            }

            if json_output:
                print(json.dumps(result, ensure_ascii=False))
                return result
            
            if verbose:
                print(f"\n玩家 {pid} 推荐结果 (已应用多塔融合与跨模块去重规则)")
                for theme, games in recommendations.items():
                    print(f"【{theme}】: {', '.join([g['title'] for g in games[:3]])}...")

        except Exception as e:
            if json_output: print(json.dumps({"error": str(e)}))
            else: print(f"Error: {e}")
            continue

if __name__ == "__main__":
    json_mode = "--json" in sys.argv
    target_id = None
    top_n = 20
    
    # 解析参数
    skip_next = False
    for i, arg in enumerate(sys.argv):
        if i == 0: continue
        if skip_next:
            skip_next = False
            continue
        if arg == "--json": continue
        if arg == "--topn" and i + 1 < len(sys.argv):
            try:
                top_n = int(sys.argv[i+1])
            except:
                pass
            skip_next = True
            continue
        if arg.startswith("-"): continue
        target_id = arg

    if target_id:
        get_player_preference(target_id, show_recommendations=True, top_n_recommend=top_n, json_output=json_mode)
    else:
        print("Usage: python step10b_player_preference.py <SteamID> [--json] [--topn 20]")
