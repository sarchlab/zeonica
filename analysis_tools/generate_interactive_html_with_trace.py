#!/usr/bin/env python3
"""
Enhanced Interactive CGRA Debugger with Data Flow Tracing
Wraps generate_interactive_html.py and adds backward tracing capability
"""

import json
import sys
import os
from collections import defaultdict

# Import the original function
from generate_interactive_html import generate_interactive_html as original_generate_html


def build_dataflow_graph(log_file):
    """
    Build a dataflow graph from log file for backward tracing.
    Returns a dict mapping (cycle, from_pe, direction, channel) -> (cycle, to_pe, ...)
    """
    graph = defaultdict(list)  # node -> [incoming nodes]
    reverse_graph = defaultdict(list)  # node -> [outgoing nodes]
    all_events = []
    
    print(f"[*] Building dataflow graph from {log_file}...")
    
    with open(log_file, 'r') as f:
        for line in f:
            if not line.strip():
                continue
            try:
                event = json.loads(line)
                all_events.append(event)
                
                # Look for DataFlow events
                if event.get('msg') == 'DataFlow':
                    time = event.get('cycle', event.get('Time'))
                    from_pe = event.get('From')
                    to_pe = event.get('To')
                    direction = event.get('Direction', '')
                    channel = event.get('Color', event.get('ColorIdx', ''))
                    data = event.get('Data', '')
                    behavior = event.get('Behavior', '')
                    
                    if from_pe and to_pe and time is not None:
                        # Create node keys
                        from_node = (int(time), from_pe, direction, channel)
                        to_node = (int(time), to_pe, direction, channel)
                        
                        # Store the event with full info
                        edge_info = {
                            'from_time': int(time),
                            'from_pe': from_pe,
                            'to_pe': to_pe,
                            'direction': direction,
                            'channel': channel,
                            'data': data,
                            'behavior': behavior,
                        }
                        
                        # Build graph: to_node receives from from_node
                        graph[to_node].append((from_node, edge_info))
                        reverse_graph[from_node].append((to_node, edge_info))
                        
            except json.JSONDecodeError:
                pass
    
    print(f"[+] Built graph with {len(graph)} sink nodes")
    return graph, reverse_graph, all_events


def trace_backward(graph, sink_node, max_depth=50):
    """
    Trace backward from a sink node to find the complete data path.
    sink_node format: (cycle, pe_string, direction, channel)
    Returns list of nodes from source to sink
    """
    path = [sink_node]
    visited = {sink_node}
    current_node = sink_node
    depth = 0
    
    while depth < max_depth:
        incoming = graph.get(current_node, [])
        if not incoming:
            break
        
        # Take the first (most direct) incoming edge
        prev_node, edge_info = incoming[0]
        
        if prev_node in visited:
            break
        
        visited.add(prev_node)
        path.insert(0, prev_node)
        current_node = prev_node
        depth += 1
    
    return path


def trace_forward(reverse_graph, source_node, max_depth=50):
    """
    Trace forward from a source node to find data destinations.
    """
    path = [source_node]
    visited = {source_node}
    queue = [source_node]
    depth = 0
    
    while queue and depth < max_depth:
        current_node = queue.pop(0)
        outgoing = reverse_graph.get(current_node, [])
        
        for next_node, edge_info in outgoing:
            if next_node not in visited:
                visited.add(next_node)
                path.append(next_node)
                queue.append(next_node)
        
        depth += 1
    
    return path


def generate_interactive_html_with_trace(log_file, output_file="cgra_debug_interactive.html"):
    """
    Generate enhanced HTML with backward tracing capability
    """
    
    # Build the dataflow graph
    graph, reverse_graph, all_events = build_dataflow_graph(log_file)
    
    # Convert to JSON-serializable format
    graph_json = {}
    for node, incoming_list in graph.items():
        node_key = json.dumps(node)  # Use JSON string as key
        graph_json[node_key] = [
            {
                'from_node': list(from_node),
                'info': edge_info
            }
            for from_node, edge_info in incoming_list
        ]
    
    # Call original generator
    original_generate_html(log_file, output_file)
    
    # Read the generated HTML
    with open(output_file, 'r') as f:
        html_content = f.read()
    
    # Inject tracing JavaScript and data
    trace_js = f"""
    // Dataflow Graph for Backward Tracing
    const dataflowGraph = {json.dumps(graph_json)};
    
    // Trace backward from a node to find its source
    function traceBackward(sinkNodeTuple) {{
        const sinkKey = JSON.stringify(sinkNodeTuple);
        const path = [];
        const visited = new Set();
        let currentKey = sinkKey;
        let depth = 0;
        const maxDepth = 50;
        
        path.push(sinkNodeTuple);
        visited.add(sinkKey);
        
        while (depth < maxDepth) {{
            const incoming = dataflowGraph[currentKey] || [];
            if (incoming.length === 0) break;
            
            const prevEdge = incoming[0];
            const prevNode = prevEdge.from_node;
            const prevKey = JSON.stringify(prevNode);
            
            if (visited.has(prevKey)) break;
            
            visited.add(prevKey);
            path.unshift(prevNode);
            currentKey = prevKey;
            depth++;
        }}
        
        return path;
    }}
    
    // Trace forward from a node to find its destinations
    function traceForward(sourceNodeTuple) {{
        const path = [sourceNodeTuple];
        const visited = new Set();
        const queue = [sourceNodeTuple];
        visited.add(JSON.stringify(sourceNodeTuple));
        let depth = 0;
        const maxDepth = 50;
        
        while (queue.length > 0 && depth < maxDepth) {{
            const currentKey = JSON.stringify(queue.shift());
            
            // Find all outgoing edges (where this node is the source)
            for (const key in dataflowGraph) {{
                const incoming = dataflowGraph[key];
                for (const edge of incoming) {{
                    if (JSON.stringify(edge.from_node) === currentKey) {{
                        const nextNode = JSON.parse(key);
                        const nextKey = JSON.stringify(nextNode);
                        if (!visited.has(nextKey)) {{
                            visited.add(nextKey);
                            path.push(nextNode);
                            queue.push(nextNode);
                        }}
                    }}
                }}
            }}
            depth++;
        }}
        
        return path;
    }}
    
    // Display trace in a panel
    function displayTracePanel(tracePath, traceType = 'backward') {{
        const panel = document.getElementById('tracePanel');
        if (!panel) {{
            console.log('No trace panel found');
            return;
        }}
        
        let html = `<div style="padding: 10px; background-color: #252526; border-radius: 5px;">`;
        html += `<div style="font-weight: bold; color: #4ec9b0; margin-bottom: 10px;">`;
        html += traceType === 'backward' ? '📍 Backward Trace (Source → Sink)' : '📍 Forward Trace (Sink → Destination)';
        html += `</div>`;
        
        if (tracePath.length === 0) {{
            html += `<div style="color: #ff6b6b;">No trace found</div>`;
        }} else {{
            html += `<table style="width: 100%; font-size: 11px; border-collapse: collapse;">`;
            html += `<tr style="background-color: #1e1e1e; border-bottom: 1px solid #444;">`;
            html += `<th style="padding: 5px; text-align: left;">Hop</th>`;
            html += `<th style="padding: 5px; text-align: left;">Cycle</th>`;
            html += `<th style="padding: 5px; text-align: left;">PE</th>`;
            html += `<th style="padding: 5px; text-align: left;">Direction</th>`;
            html += `<th style="padding: 5px; text-align: left;">Channel</th>`;
            html += `<th style="padding: 5px; text-align: left;">Data</th>`;
            html += `</tr>`;
            
            tracePath.forEach((node, idx) => {{
                const [cycle, pe, direction, channel] = node;
                html += `<tr style="border-bottom: 1px solid #333; background-color: ${{idx % 2 === 0 ? '#1e1e1e' : '#252526'}};">`;
                html += `<td style="padding: 5px;">${{idx + 1}}</td>`;
                html += `<td style="padding: 5px; color: #4fc3f7;">${{cycle}}</td>`;
                html += `<td style="padding: 5px; color: #66bb6a;">${{pe}}</td>`;
                html += `<td style="padding: 5px; color: #ce9178;">${{direction}}</td>`;
                html += `<td style="padding: 5px; color: #dcdcaa;">${{channel}}</td>`;
                html += `<td style="padding: 5px; color: #c586c0;">-</td>`;
                html += `</tr>`;
            }});
            
            html += `</table>`;
        }}
        
        html += `</div>`;
        panel.innerHTML = html;
        
        // Highlight nodes on grid
        highlightTraceNodes(tracePath);
    }}
    
    // Highlight trace nodes on the grid
    function highlightTraceNodes(tracePath) {{
        // Clear previous highlights
        document.querySelectorAll('.trace-highlight').forEach(el => {{
            el.classList.remove('trace-highlight');
        }});
        
        // Highlight trace nodes
        tracePath.forEach((node, idx) => {{
            const [cycle, pe, direction, channel] = node;
            const cellId = `cell-${{cycle}}-${{pe}}`;
            const cell = document.getElementById(cellId);
            if (cell) {{
                cell.classList.add('trace-highlight');
                // Highlight with gradient based on position in trace
                const intensity = (idx / Math.max(1, tracePath.length - 1)) * 100;
                cell.style.boxShadow = `0 0 10px rgba(79, 195, 247, ${{0.3 + intensity * 0.007}}),
                                         inset 0 0 5px rgba(79, 195, 247, 0.2)`;
            }}
        }});
    }}
    
    // Add click handler to grid cells for tracing
    function attachTraceHandlers() {{
        document.querySelectorAll('[data-cycle][data-pe]').forEach(cell => {{
            cell.addEventListener('click', function(e) {{
                if (e.ctrlKey || e.metaKey) {{  // Ctrl/Cmd click for trace
                    const cycle = parseInt(this.dataset.cycle);
                    const pe = this.dataset.pe;
                    const direction = this.dataset.direction || '';
                    const channel = this.dataset.channel || '';
                    
                    const sinkNode = [cycle, pe, direction, channel];
                    const tracePath = traceBackward(sinkNode);
                    displayTracePanel(tracePath, 'backward');
                    
                    console.log('Traced backward:', tracePath);
                }}
            }});
        }});
    }}
    
    // Call after DOM is ready
    document.addEventListener('DOMContentLoaded', attachTraceHandlers);
    """
    
    # Inject the trace panel CSS and JS
    trace_css = """
    <style>
        #tracePanel {{
            position: fixed;
            right: 20px;
            top: 120px;
            width: 400px;
            max-height: 500px;
            background-color: #1e1e1e;
            border: 2px solid #4ec9b0;
            border-radius: 5px;
            padding: 10px;
            z-index: 100;
            overflow-y: auto;
            box-shadow: 0 0 20px rgba(78, 201, 176, 0.3);
            display: none;
        }
        
        #tracePanel.show {{
            display: block;
        }}
        
        .trace-highlight {{
            background-color: #1a3a3a !important;
            border: 2px solid #4fc3f7 !important;
        }}
    </style>
    """
    
    # Find the closing </head> tag and inject CSS before it
    html_content = html_content.replace('</head>', trace_css + '</head>', 1)
    
    # Find the closing </body> tag and inject JS before it
    trace_html_setup = """
    <div id="tracePanel" class="show">
        <div style="text-align: center; color: #858585; padding: 20px;">
            Ctrl+Click on any cell to trace data backward
        </div>
    </div>
    """
    
    html_content = html_content.replace('</body>', trace_html_setup + '</body>', 1)
    
    # Inject the trace JavaScript before the closing script tag
    # Find the last </script> and insert before it
    last_script_close = html_content.rfind('</script>')
    if last_script_close != -1:
        html_content = html_content[:last_script_close] + trace_js + html_content[last_script_close:]
    
    # Write the enhanced HTML
    with open(output_file, 'w') as f:
        f.write(html_content)
    
    print(f"[+] Enhanced HTML generated: {output_file}")
    print(f"[*] Backward tracing enabled - Ctrl+Click on any cell to trace")


if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python3 generate_interactive_html_with_trace.py <log_file> [output.html]")
        sys.exit(1)
    
    log_file = sys.argv[1]
    output = sys.argv[2] if len(sys.argv) > 2 else "cgra_debug_interactive_traced.html"
    
    generate_interactive_html_with_trace(log_file, output)
