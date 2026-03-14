"""
step8b_tag_word2vec_profiler.py ??????? (???+ V3)
============================================================
?????
1. ??????????????['brackets', "'quotes'"]?
2. ?genres (????? ?top_tags (??????? ?????
3. ?????64 ????????
"""

import pandas as pd
import numpy as np
import logging
import ast
from pathlib import Path
from gensim.models import Word2Vec
from sklearn.preprocessing import StandardScaler

logging.basicConfig(level=logging.INFO, format='%(asctime)s - [%(levelname)s] - %(message)s')
logger = logging.getLogger(__name__)

BASE_DIR  = Path("h:/Work/Game/ULTIM")
STEP8_DIR = BASE_DIR / "Step8_Game_Profile"

def clean_tags(tags_val):
    """
    ??????????
    1. "['Genre1', 'Genre2']" -> ['genre1', 'genre2']
    2. "Tag1, Tag2" -> ['tag1', 'tag2']
    3. NaN -> []
    """
    if pd.isna(tags_val) or not tags_val: return []
    
    t_str = str(tags_val).strip()
    if t_str.startswith('[') and t_str.endswith(']'):
        try:
            # ???????
            l = ast.literal_eval(t_str)
            return [str(x).lower().strip().replace(" ", "_") for x in l]
        except:
            # ?? eval ???????
            pass
            
    # ???????? (????????????)
    raw_list = t_str.split(',')
    cleaned = []
    for t in raw_list:
        c = t.replace('[', '').replace(']', '').replace("'", "").replace('"', '').strip().lower().replace(" ", "_")
        if c and c != 'nan':
            cleaned.append(c)
    return cleaned

def run_clean_word2vec_profiling(vector_size=64):
    logger.info(">>> [1/4] ?? 7.3w ?????????????V3 ??...")
    df = pd.read_csv(STEP8_DIR / "game_dna_profile.csv", dtype={'gameid': str}, low_memory=False)
    
    # ????????
    df['tag_list'] = df.apply(lambda r: 
        list(set(clean_tags(r.get('genres', '')) + clean_tags(r.get('top_tags', '')))), axis=1)
    
    df_valid = df[df['tag_list'].map(len) > 0].copy()
    sentences = df_valid['tag_list'].tolist()
    
    logger.info(f"   ????? {len(sentences):,}?????Word2Vec ??...")
    # ??????????????????
    w2v_model = Word2Vec(sentences, vector_size=vector_size, window=5, min_count=1, workers=4, epochs=20)
    
    # ?????? (????????????)
    def get_dense_vector(tag_list):
        vectors = [w2v_model.wv[tag] for tag in tag_list if tag in w2v_model.wv]
        if not vectors: return np.zeros(vector_size)
        return np.mean(vectors, axis=0)

    logger.info(">>> [2/4] ????????????..")
    game_vectors = np.array([get_dense_vector(tags) for tags in df_valid['tag_list']])
    
    # ???
    norms = np.linalg.norm(game_vectors, axis=1, keepdims=True)
    game_vectors_norm = np.divide(game_vectors, norms, out=np.zeros_like(game_vectors), where=norms!=0)
    
    # ?????
    vec_cols = [f'sem_vec_{i}' for i in range(vector_size)]
    df_vec = pd.DataFrame(game_vectors_norm, columns=vec_cols, index=df_valid.index)
    df_valid = pd.concat([df_valid, df_vec], axis=1)
    
    # ?? 150 ?? (???
    from sklearn.cluster import MiniBatchKMeans
    logger.info(">>> [3/4] ?? 150 ???? (MiniBatch)...")
    kmeans = MiniBatchKMeans(n_clusters=150, random_state=42, batch_size=2048)
    df_valid['semantic_theme_id'] = kmeans.fit_predict(game_vectors_norm)
    
    # ???? (????
    logger.info(">>> [4/4] ?? V3 ?????????..")
    df_final = df.merge(df_valid[['gameid', 'semantic_theme_id'] + vec_cols], on='gameid', how='left').fillna(0)
    
    out_path = STEP8_DIR / "game_dna_profile_v2.csv"
    df_final.to_csv(out_path, index=False)
    w2v_model.save(str(STEP8_DIR / "tag_word2vec_v2.model"))
    
    print("\n" + "="*60)
    print("?V3 ?????????????)
    print(f"1. ??????? {len(w2v_model.wv.key_to_index)}")
    print(f"2. ?20 ??????: {list(w2v_model.wv.index_to_key[:20])}")
    print("="*60)

if __name__ == "__main__":
    run_clean_word2vec_profiling()
