"""
step11_offline_redis_sync.py
定期运行（如Cron定时任务），使用ULTIM模型批量推断结果，并将用户的Top K直接推入Redis。
Golang API 直接读取Redis返回给前端，剥离了Python模型厚重的在线响应时间。
"""
import json
import redis
import torch
import numpy as np

# 假设这里是载入 step10 的代码与计算得出的 Final_Scores 分数矩阵
# from step10_ultimate_fusion_engine import load_assets, compute_scores

def sync_to_redis():
    print(">>> 1. 执行 ULTIM 模型推理中...")
    # mock 测试数据：计算完成的 App ID
    # 真实应用时，这里用 numpy argpartition 将 Final_Scores 取出 top 20
    
    print(">>> 2. 连接 Redis 准备写入缓存...")
    r = redis.Redis(host='localhost', port=6379, db=0, decode_responses=True)
    
    # 模拟给测试用户 test_pids 的写入过程
    test_pids = ["76561198000000001", "76561198000000002"] # Steam IDs
    
    for pid in test_pids:
        # 这个列表需要从你的模型输出里拿到 (此处为 mock)
        top_appids = ["1091500", "281990", "1174180", "570", "730"]
        
        redis_key = f"ultim_recom:{pid}"
        
        # 将结构化的列表存为 JSON String
        r.set(redis_key, json.dumps(top_appids))
        
        # 设置过期时间 24小时 (86400秒) 以保证数据新鲜度
        r.expire(redis_key, 86400)
    
    print(">>> 同步完成! Golang 端现在能用 1 毫秒读取出这些库了！")

if __name__ == "__main__":
    sync_to_redis()
