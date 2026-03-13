import os
from PIL import Image
import numpy as np

def remove_green_screen(input_path, output_path):
    # 打开图像并确保是 RGBA 模式
    img = Image.open(input_path).convert("RGBA")
    data = np.array(img)
    
    # 获取 R, G, B, A 通道
    r, g, b, a = data[:,:,0], data[:,:,1], data[:,:,2], data[:,:,3]
    
    # 识别“绿幕”区域
    # 逻辑：G 通道明显大于 R 和 B，且超过一定阈值
    # 这里可以根据实际绿色的纯度调整阈值
    mask = (g > 100) & (g > r + 30) & (g > b + 30)
    
    # 将符合条件的像素透明度设为 0
    data[mask, 3] = 0
    
    # 重新生成图像
    result = Image.fromarray(data)
    result.save(output_path, "PNG")
    print(f"成功处理图片并保存至: {output_path}")

if __name__ == "__main__":
    target_file = r"E:\gamescience\gamesci\public\wukong-segmented.png"
    if os.path.exists(target_file):
        remove_green_screen(target_file, target_file)
    else:
        print(f"未找到文件: {target_file}")
