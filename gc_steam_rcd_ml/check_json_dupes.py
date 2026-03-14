import json
from collections import Counter

def check_duplicates():
    with open(r'e:\gs_steam_rcd\ULTIM\76561199805172305_result_utf8.json', 'r', encoding='utf-8') as f:
        # Skip potential warning lines
        for line in f:
            if line.strip().startswith('{"playerid"'):
                data = json.loads(line)
                break
        else:
            print("No valid JSON found.")
            return
        
    all_recom_ids = []
    for theme, games in data['recommendations'].items():
        for g in games:
            all_recom_ids.append((g['gameid'], g['title']))
            
    counts = Counter(all_recom_ids)
    duplicates = {k: v for k, v in counts.items() if v > 1}
    
    if duplicates:
        print("Found duplicates in JSON recommendations:")
        for (gid, title), count in duplicates.items():
            print(f"  - {title} (ID: {gid}): {count} times")
    else:
        print("No duplicates found in JSON recommendations.")

if __name__ == "__main__":
    check_duplicates()
