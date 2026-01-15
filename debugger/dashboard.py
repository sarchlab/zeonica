import streamlit as st
import os
import sys
import glob
import importlib.util
import shutil

# Helper to import modules by path
def load_module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec and spec.loader:
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module
    return None

# Load debugger modules
# Get the directory where this script is located
current_dir = os.path.dirname(os.path.abspath(__file__))
try:
    yaml_drawer = load_module("yaml_drawer", os.path.join(current_dir, "yaml-drawer.py"))
    plot = load_module("plot", os.path.join(current_dir, "plot.py"))
except Exception as e:
    st.error(f"Error loading debugger modules: {e}")
    st.stop()

st.set_page_config(layout="wide", page_title="Debugger Dashboard")

st.title("Zeonica Debugger Dashboard")

# Inputs
st.sidebar.header("Configuration")
prog_yaml = st.sidebar.text_input("Program YAML Path", "test/test_gpred.yaml")
dfg_yaml = st.sidebar.text_input("DFG YAML Path", "debugger/test_gpred-dfg.yaml")
log_json = st.sidebar.text_input("Log JSON Path", "test_gpred.log")

if st.sidebar.button("Debug", type="primary"):
    if not os.path.exists(prog_yaml):
        st.error(f"Program YAML not found: {prog_yaml}")
    elif not os.path.exists(dfg_yaml):
        st.error(f"DFG YAML not found: {dfg_yaml}")
    elif not os.path.exists(log_json):
        st.error(f"Log JSON not found: {log_json}")
    else:
        with st.spinner("Generating visualizations..."):
            try:
                # Static Program Viz
                static_base_dir = "output/dashboard"
                # yaml_drawer.draw_yaml creates a subdirectory based on filename
                # We need to capture that.
                final_static_dir = yaml_drawer.draw_yaml(prog_yaml, static_base_dir)
                
                # Dynamic Viz
                dynamic_out_dir = "output/dashboard/dynamic"
                if os.path.exists(dynamic_out_dir):
                    shutil.rmtree(dynamic_out_dir)
                
                plot.process_log_and_draw(log_json, dfg_yaml, dynamic_out_dir)
                
                st.session_state['viz_ready'] = True
                st.session_state['static_dir'] = final_static_dir
                st.session_state['dynamic_dir'] = dynamic_out_dir
                st.success("Visualization generated successfully!")
            except Exception as e:
                st.error(f"Error during visualization: {e}")
                # Print traceback to terminal for debugging
                import traceback
                traceback.print_exc()

if st.session_state.get('viz_ready'):
    # Static Viz Section
    st.header("Static Program Schedule")
    static_dir = st.session_state['static_dir']
    
    if static_dir and os.path.exists(static_dir):
        static_images = sorted(glob.glob(os.path.join(static_dir, "*.png")))
        
        if static_images:
            if 'static_idx' not in st.session_state:
                st.session_state['static_idx'] = 0
                
            c1, c2, c3 = st.columns([1, 10, 1])
            with c1:
                if st.button("◀", key="s_prev"):
                    st.session_state['static_idx'] = (st.session_state['static_idx'] - 1) % len(static_images)
            with c3:
                if st.button("▶", key="s_next"):
                    st.session_state['static_idx'] = (st.session_state['static_idx'] + 1) % len(static_images)
            
            with c2:
                current_img = static_images[st.session_state['static_idx']]
                st.image(current_img, caption=os.path.basename(current_img))
                st.caption(f"Image {st.session_state['static_idx'] + 1} of {len(static_images)}")
        else:
            st.warning(f"No static images found in {static_dir}")
    else:
        st.warning("Static directory invalid.")

    st.divider()

    # Dynamic Viz Section
    st.header("Dynamic Execution Trace")
    dynamic_dir = st.session_state['dynamic_dir']
    mesh_dir = os.path.join(dynamic_dir, "mesh")
    dfg_dir = os.path.join(dynamic_dir, "dfg")
    
    mesh_images = sorted(glob.glob(os.path.join(mesh_dir, "*.png")))
    dfg_images = sorted(glob.glob(os.path.join(dfg_dir, "*.png")))
    
    if len(mesh_images) != len(dfg_images):
        st.error(f"Mismatch in image counts: Mesh={len(mesh_images)}, DFG={len(dfg_images)}")
    
    num_timesteps = min(len(mesh_images), len(dfg_images))
    
    if num_timesteps > 0:
        # Control for timestep
        col_ctrl1, col_ctrl2, col_ctrl3 = st.columns([1, 8, 1])
        
        if 'timestep' not in st.session_state:
            st.session_state['timestep'] = 0
            
        def dec_time():
            st.session_state['timestep'] = (st.session_state['timestep'] - 1) % num_timesteps
            
        def inc_time():
            st.session_state['timestep'] = (st.session_state['timestep'] + 1) % num_timesteps
            
        with col_ctrl1:
            st.button("◀", key="d_prev", on_click=dec_time)
            
        with col_ctrl3:
            st.button("▶", key="d_next", on_click=inc_time)
            
        with col_ctrl2:
            st.slider("Timestep", 0, num_timesteps - 1, key="timestep")
            
        timestep = st.session_state['timestep']
        
        c1, c2 = st.columns(2)
        with c1:
            st.subheader("Mesh")
            st.image(mesh_images[timestep], caption=os.path.basename(mesh_images[timestep]))
        with c2:
            st.subheader("DFG")
            st.image(dfg_images[timestep], caption=os.path.basename(dfg_images[timestep]))
    else:
        st.warning("No dynamic images found.")




