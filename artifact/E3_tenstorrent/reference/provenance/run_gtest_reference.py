"""Run the 28 TT-Sim gtests (wh-cycle) with the original runner environment.
usage: run_gtest_reference.py <outroot> [dump|nodump] [kernel ...]
With 'dump', ZEONICA_TT_DUMP_DIR=<outroot>/<kernel>/dump so the patched gtests write inputs/outputs."""
from pathlib import Path
import json,os,re,subprocess,sys,hashlib
R=Path(__file__).resolve().parents[1];Y=R.parent;TT=Y/'tt-metal';B=TT/'build_ttsim_tests'
LIBS=[Y/'local-apt/usr/lib/x86_64-linux-gnu',Y/'miniconda3/envs/llvm-build/lib',Path('/usr/lib/x86_64-linux-gnu'),B/'lib',B/'tt_metal',B/'tt_metal/third_party/umd/lib',B/'tt_stl']
BIN={'legacy':B/'test/tt_metal/unit_tests_legacy','integration':B/'test/tt_metal/unit_tests_integration'}
def env(slow,dump):
 e=os.environ.copy();[e.pop(k) for k in list(e) if k.startswith('ZEONICA_') or k.startswith('TT_METAL')]
 e.update(TT_METAL_HOME=str(TT),TT_METAL_SIMULATOR=str(Y/'ttsim-run/wh-cycle/libttsim.so'),TT_METAL_CACHE=str(R/'build/tt_cache'),TT_METAL_DISABLE_SFPLOADMACRO='1',LD_LIBRARY_PATH=':'.join(map(str,LIBS))+(':'+os.environ['LD_LIBRARY_PATH'] if os.environ.get('LD_LIBRARY_PATH') else ''))
 if slow:e['TT_METAL_SLOW_DISPATCH_MODE']='1'
 if dump:e['ZEONICA_TT_DUMP_DIR']=str(dump);dump.mkdir(parents=True,exist_ok=True)
 return e
def main():
 outroot=Path(sys.argv[1]).resolve();mode=sys.argv[2];only=set(sys.argv[3:])
 parts=json.loads((R/'frozen/kernel_partition.json').read_text());res=[]
 for p in parts:
  k=p['kernel']
  if only and k not in only:continue
  d=outroot/k;d.mkdir(parents=True,exist_ok=True)
  e=env(p['slow_dispatch'],d/'dump' if mode=='dump' else None)
  cmd=[str(BIN[p['tt_binary']]),f"--gtest_filter={p['gtest_filter']}"]
  (d/'command.json').write_text(json.dumps({'argv':cmd,'cwd':str(TT),'env':{x:e.get(x) for x in e if x.startswith(('TT_METAL','ZEONICA','LD_LIB'))},'binary_sha256':hashlib.sha256(Path(cmd[0]).read_bytes()).hexdigest()},indent=1))
  q=subprocess.run(cmd,cwd=TT,env=e,capture_output=True,text=True,timeout=1800)
  (d/'stdout.log').write_text(q.stdout);(d/'stderr.log').write_text(q.stderr)
  t=q.stdout+'\n'+q.stderr;m=re.search(r"^\[(\d+)\]\s+([0-9.]+)\s+seconds\s+\(([0-9.]+)\s+KHz\)\s*$",t,re.M)
  ok=q.returncode==0 and ('[  PASSED  ] 1 test' in t) and '[  SKIPPED ]' not in t
  r=dict(kernel=k,role=p['role'],returncode=q.returncode,passed=ok,cycles=int(m.group(1)) if m else None)
  (d/'result.json').write_text(json.dumps(r));res.append(r);print(json.dumps(r),flush=True)
 return res
if __name__=='__main__':main()
