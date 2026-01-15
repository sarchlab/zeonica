import yaml
import graphviz
from typing import Dict, List, Optional
import argparse

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
    
    if data is None:
        print(f"Error: Failed to load YAML file from {yaml_path}")
        return
    # 创建有向图
    dot = graphviz.Digraph(comment='DFG Graph')
    dot.attr(rankdir='LR')  # 从左到右布局
    dot.attr('node', shape='box', style='rounded,filled')
    
    # 获取所有节点
    nodes = data.get('nodes', [])
    edges = data.get('edges', [])
    
    print(id_to_count)
    # 计算 count 的范围，用于颜色映射
    counts = [id_to_count.get(node['id'], 0) for node in nodes]
    max_count = max(counts) if counts else 1
    min_count = min(counts) if counts else 0
    
    def get_color(count: int) -> str:
        """根据 count 值返回颜色 (Blue <-> Orange/Yellow cycle per 50)"""
        if count == 0:
            return '#E8E8E8'  # 浅灰色（默认）
        
        # 1-10: Blue -> Orange
        # 11-20: Orange -> Blue
        cycle_len = 20
        cycle_pos = count % cycle_len
        
        # Define Colors (RGB)
        # Blue: #6495ED (100, 149, 237)
        # Orange/Yellow: #FFA500 (255, 165, 0)
        c1 = (100, 149, 237)
        c2 = (255, 165, 0)
        
        if cycle_pos <= 10:
            # Blue -> Orange
            progress = cycle_pos / 10.0
            start_c = c1
            end_c = c2
        else:
            # Orange -> Blue
            progress = (cycle_pos - 10) / 10.0
            start_c = c2
            end_c = c1
            
        r = int(start_c[0] + (end_c[0] - start_c[0]) * progress)
        g = int(start_c[1] + (end_c[1] - start_c[1]) * progress)
        b = int(start_c[2] + (end_c[2] - start_c[2]) * progress)
        
        return f'#{r:02X}{g:02X}{b:02X}'
    
    # 添加节点
    for node in nodes:
        node_id = node['id']
        opcode = node.get('opcode', '')
        tile_x = node.get('tile_x', '')
        tile_y = node.get('tile_y', '')
        time_step = node.get('time_step', '')
        
        count = id_to_count.get(int(node_id), 0)
        color = get_color(count)
        print(node_id, count, color)
        
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
    
    # 添加图例
    with dot.subgraph(name='cluster_legend') as legend:
        legend.attr(label='Count Color Legend')
        legend.attr(style='filled')
        legend.attr(color='lightgray')
        legend.attr(rankdir='LR')  # 图例从左到右排列
        
        # 创建图例节点，展示不同 count 值对应的颜色
        legend_items = []
        
        # count = 0 (灰色)
        legend_items.append((0, '#E8E8E8'))
        
        # Add keyframes for the cycle
        # 1 (Blue), 13 (Mid), 25 (Orange), 38 (Mid), 50 (Blue)
        keyframes = [1, 5, 10]
        
        for k in keyframes:
            legend_items.append((k, get_color(k)))
        
        # 添加图例节点，水平排列
        prev_legend_id = None
        for idx, (count_val, color) in enumerate(legend_items):
            legend_id = f"legend_{idx}"
            label = f"count: {count_val}"
            legend.node(legend_id, label, fillcolor=color, style='filled,rounded')
            
            # 连接图例节点，使其水平排列
            if prev_legend_id is not None:
                legend.edge(prev_legend_id, legend_id, style='invis')  # 不可见边，用于布局
            
            prev_legend_id = legend_id
    
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
    # add cli param
    parser = argparse.ArgumentParser()
    parser.add_argument("--yaml_path", type=str, default="debugger/fir-dfg-new.yaml")
    parser.add_argument("--output_path", type=str, default="dfg_output")
    parser.add_argument("--highlight", type=list, default=[6, 31, 32])
    args = parser.parse_args()
    
    draw_dfg_with_counts(example_counts, yaml_path=args.yaml_path, output_path=args.output_path, highlight=args.highlight)

