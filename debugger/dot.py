import yaml
import graphviz
from typing import Dict, List, Optional


def draw_dfg_with_counts(id_to_count: Dict[int, int], yaml_path: str = "debugger/fir-dfg-new.yaml", output_path: str = "dfg_output", highlight: Optional[List[int]] = None):
    """
    读取 YAML 文件并生成带颜色节点的 dot 图
    
    Args:
        id_to_count: 节点 id 到 count 次数的映射
        yaml_path: YAML 文件路径
        output_path: 输出文件路径（不含扩展名）
        highlight: 需要高亮的节点 id 列表，这些节点会用加粗红色边框显示
    """
    if highlight is None:
        highlight = []
    # 读取 YAML 文件
    with open(yaml_path, 'r') as f:
        data = yaml.safe_load(f)
    
    # 创建有向图
    dot = graphviz.Digraph(comment='DFG Graph')
    dot.attr(rankdir='LR')  # 从左到右布局
    dot.attr('node', shape='box', style='rounded,filled')
    
    # 获取所有节点
    nodes = data.get('nodes', [])
    edges = data.get('edges', [])
    
    # 计算 count 的范围，用于颜色映射
    counts = [id_to_count.get(node['id'], 0) for node in nodes]
    max_count = max(counts) if counts else 1
    min_count = min(counts) if counts else 0
    
    def get_color(count: int) -> str:
        """根据 count 值返回颜色"""
        if count == 0 or count not in id_to_count:
            return '#E8E8E8'  # 浅灰色（默认）
        
        # 归一化 count 到 0-1 范围
        if max_count == min_count:
            normalized = 0.5
        else:
            normalized = (count - min_count) / (max_count - min_count)
        
        # 使用颜色渐变：浅蓝 -> 蓝色 -> 深蓝 -> 红色
        if normalized < 0.25:
            # 浅蓝色系
            intensity = normalized / 0.25
            r = int(173 + (255 - 173) * intensity)
            g = int(216 + (255 - 216) * intensity)
            b = 255
        elif normalized < 0.5:
            # 蓝色系
            intensity = (normalized - 0.25) / 0.25
            r = int(100 + (173 - 100) * (1 - intensity))
            g = int(149 + (216 - 149) * (1 - intensity))
            b = int(237 + (255 - 237) * (1 - intensity))
        elif normalized < 0.75:
            # 深蓝色系
            intensity = (normalized - 0.5) / 0.25
            r = int(50 + (100 - 50) * (1 - intensity))
            g = int(50 + (149 - 50) * (1 - intensity))
            b = int(200 + (237 - 200) * (1 - intensity))
        else:
            # 红色系（高 count）
            intensity = (normalized - 0.75) / 0.25
            r = 255
            g = int(200 + (50 - 200) * intensity)
            b = int(200 + (50 - 200) * intensity)
        
        return f'#{r:02X}{g:02X}{b:02X}'
    
    # 添加节点
    for node in nodes:
        node_id = node['id']
        opcode = node.get('opcode', '')
        tile_x = node.get('tile_x', '')
        tile_y = node.get('tile_y', '')
        time_step = node.get('time_step', '')
        
        count = id_to_count.get(node_id, 0)
        color = get_color(count)
        
        # 节点标签：显示 id, opcode 和 count
        label = f"{node_id}\\n{opcode}\\ncount: {count}"
        if tile_x is not None and tile_y is not None:
            label += f"\\n({tile_x},{tile_y})"
        
        # 设置节点属性
        node_attrs = {'fillcolor': color}
        
        # 如果节点在 highlight 列表中，添加加粗红色边框
        if node_id in highlight:
            node_attrs['penwidth'] = '3'  # 加粗边框
            node_attrs['color'] = 'red'    # 红色边框
        
        dot.node(str(node_id), label, **node_attrs)
    
    # 添加边
    for edge in edges:
        from_id = edge['from']
        to_id = edge['to']
        dot.edge(str(from_id), str(to_id))
    
    # 渲染并保存
    dot.render(output_path, format='png', cleanup=True)
    print(f"Graph saved to {output_path}.png")
    
    return dot


# 示例使用
if __name__ == "__main__":
    # 示例：创建一个 id -> count 的映射
    example_counts = {
        0: 1,
        1: 2,
        6: 5,
        7: 3,
        12: 2,
        13: 1,
        14: 1,
        19: 4,
        20: 1,
        21: 1,
        26: 3,
        27: 2,
        31: 10,  # 高 count，会显示为红色
        32: 1,
        36: 1,
        37: 1,
        40: 1,
    }
    
    # 示例：高亮某些节点
    highlight_nodes = [6, 31, 32]  # 这些节点会用加粗红色边框显示
    
    draw_dfg_with_counts(example_counts, highlight=highlight_nodes)

