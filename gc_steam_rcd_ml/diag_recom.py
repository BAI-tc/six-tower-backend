import subprocess
import json
import sys

def run():
    cmd = ["python", r"e:\gs_steam_rcd\ULTIM\step10b_player_preference.py", "76561199805172305", "--json"]
    result = subprocess.run(cmd, capture_output=True, text=True, encoding='utf-8')
    
    stdout = result.stdout
    for line in stdout.split('\n'):
        if line.strip().startswith('{"playerid"'):
            data = json.loads(line)
            print("=== Account Analysis: 76561199805172305 ===")
            print(f"Owned Games: {data['owned_games_count']}")
            print("\n--- Recommendation Modules (Themes) ---")
            for theme, games in data['recommendations'].items():
                print(f"Module Name: {theme}")
                print(f"Game Count: {len(games)}")
                print(f"Top 3 Games: {', '.join([g['title'] for g in games[:3]])}")
                print("-" * 30)
            return

if __name__ == "__main__":
    run()
