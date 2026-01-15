import yaml
import graphviz
from typing import Dict, List, Optional
import argparse

def draw_dfg_with_counts(id_to_count: Dict[int, int], yaml_path: str = "debugger/fir-dfg-new.yaml", output_path: str = "dfg_output", highlight: Optional[List[int]] = None):
    """
    Read YAML file and generate dot graph with colored nodes
    
    Args:
        id_to_count: Mapping from node id to count
        yaml_path: Path to YAML file
        output_path: Output file path (without extension)
        highlight: List of node ids to highlight, these nodes will be displayed with bold red border
    """
    if highlight is None:
        highlight = []
    # Read YAML file
    with open(yaml_path, 'r') as f:
        data = yaml.safe_load(f)
    
    if data is None:
        print(f"Error: Failed to load YAML file from {yaml_path}")
        return
    # Create directed graph
    dot = graphviz.Digraph(comment='DFG Graph')
    dot.attr(rankdir='LR')  # Left to right layout
    dot.attr('node', shape='box', style='rounded,filled')
    
    # Get all nodes
    nodes = data.get('nodes', [])
    edges = data.get('edges', [])
    
    print(id_to_count)
    # Calculate count range for color mapping
    counts = [id_to_count.get(node['id'], 0) for node in nodes]
    max_count = max(counts) if counts else 1
    min_count = min(counts) if counts else 0
    
    def get_color(count: int) -> str:
        """Return color based on count value (Blue <-> Orange/Yellow cycle per 50)"""
        if count == 0:
            return '#E8E8E8'  # Light gray (default)
        
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
    
    # Add nodes
    for node in nodes:
        node_id = node['id']
        opcode = node.get('opcode', '')
        tile_x = node.get('tile_x', '')
        tile_y = node.get('tile_y', '')
        time_step = node.get('time_step', '')
        
        count = id_to_count.get(int(node_id), 0)
        color = get_color(count)
        print(node_id, count, color)
        
        # Node label: display id, opcode and count
        label = f"{node_id}\\n{opcode}\\ncount: {count}"
        if tile_x is not None and tile_y is not None:
            label += f"\\n({tile_x},{tile_y})"
        
        # Set node attributes
        node_attrs = {'fillcolor': color}
        
        # If node is in highlight list, add bold red border
        if node_id in highlight:
            node_attrs['penwidth'] = '3'  # Bold border
            node_attrs['color'] = 'red'    # Red border
        
        dot.node(str(node_id), label, **node_attrs)
    
    # Add edges
    for edge in edges:
        from_id = edge['from']
        to_id = edge['to']
        dot.edge(str(from_id), str(to_id))
    
    # Add legend
    with dot.subgraph(name='cluster_legend') as legend:
        legend.attr(label='Count Color Legend')
        legend.attr(style='filled')
        legend.attr(color='lightgray')
        legend.attr(rankdir='LR')  # Legend arranged left to right
        
        # Create legend nodes to show colors for different count values
        legend_items = []
        
        # count = 0 (gray)
        legend_items.append((0, '#E8E8E8'))
        
        # Add keyframes for the cycle
        # 1 (Blue), 13 (Mid), 25 (Orange), 38 (Mid), 50 (Blue)
        keyframes = [1, 5, 10]
        
        for k in keyframes:
            legend_items.append((k, get_color(k)))
        
        # Add legend nodes, arranged horizontally
        prev_legend_id = None
        for idx, (count_val, color) in enumerate(legend_items):
            legend_id = f"legend_{idx}"
            label = f"count: {count_val}"
            legend.node(legend_id, label, fillcolor=color, style='filled,rounded')
            
            # Connect legend nodes to arrange them horizontally
            if prev_legend_id is not None:
                legend.edge(prev_legend_id, legend_id, style='invis')  # Invisible edge for layout
            
            prev_legend_id = legend_id
    
    # Render and save
    dot.render(output_path, format='png', cleanup=True)
    print(f"Graph saved to {output_path}.png")
    
    return dot


# Example usage
if __name__ == "__main__":
    # Example: create a mapping from id to count
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
        31: 10,  # High count, will be displayed in red
        32: 1,
        36: 1,
        37: 1,
        40: 1,
    }
    
    # Example: highlight certain nodes
    highlight_nodes = [6, 31, 32]  # These nodes will be displayed with bold red border
    # Add CLI parameters
    parser = argparse.ArgumentParser()
    parser.add_argument("--yaml_path", type=str, default="debugger/fir-dfg-new.yaml")
    parser.add_argument("--output_path", type=str, default="dfg_output")
    parser.add_argument("--highlight", type=list, default=[6, 31, 32])
    args = parser.parse_args()
    
    draw_dfg_with_counts(example_counts, yaml_path=args.yaml_path, output_path=args.output_path, highlight=args.highlight)

