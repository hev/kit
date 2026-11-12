"""Spot-check documented commands with --help; never execute their actions."""
from pathlib import Path
import subprocess
import sys

# Build the v0.6.0 CLI, then pass its path as the first argument.
binary = sys.argv[1] if len(sys.argv) > 1 else '/tmp/hev-site-check'
repo = Path(sys.argv[2]) if len(sys.argv) > 2 else Path(__file__).resolve().parents[2]
checks = {
 '': ['json'],
 'up': ['store','no-dashboard'],
 'dispatch': ['config','host','json'],
 'dispatch run': ['accept','base','harness','issue','model','name','reply','timeout'],
 'dispatch hosts': [], 'dispatch ls': ['all'], 'dispatch show': [],
 'dispatch logs': ['follow'], 'dispatch attach': [], 'dispatch say': [],
 'dispatch stop': [], 'dispatch rework': ['from-validation'],
 'dispatch validate': ['allow-same-model'], 'dispatch clean': [], 'dispatch amend': [],
 'orchestrate': [], 'orchestrate apply': ['file','dry-run','scope-only-owner'],
 'orchestrate start': ['goal','runs','name','reply'], 'orchestrate ls': ['json'],
 'orchestrate show': [], 'orchestrate logs': ['round'], 'orchestrate say': [],
 'orchestrate pause': [], 'orchestrate resume': [], 'orchestrate wake': [],
 'orchestrate stop': [], 'orchestrate handoff': ['to'],
 'peers': [], 'expedite': ['authority','every','host','max-lifetime','repo'],
 'expedite status': ['host'], 'expedite stop': ['host'], 'expedite round': ['evidence'],
 'board serve': ['read-only','addr','namespace'],
 'skills check': [], 'skills sync': ['force'],
 'daemon': ['peers-only'], 'daemon install': [], 'daemon start': [],
 'daemon status': [], 'daemon stop': [], 'daemon uninstall': [],
 'index': ['tier','dry-run','limit','force','summarize'],
 'traces': ['since','order'], 'traces ls': ['since','order'],
 'traces show': ['json'],
 'traces search': ['also','since','harness','workdir','plan','order','json'],
 'serve': ['port','bind','namespace'], 'config init': ['force'],
 'config path': [], 'config show': [], 'bucket list': [], 'bucket use': [],
 'init': [], 'join': [], 'leave': [], 'open': [], 'pro': [], 'pro trial': [],
 'eval put': [], 'backfill': [], 'outcomes': [], 'cost-price': [],
 'skills-preview': [], 'native-job': [], 'completion': [], 'browse': [],
 'd': [], 's': [], 'stop': [], 'down': [],
 'api-resources': [], 'apply': [], 'get': [], 'describe': [], 'logs': [],
 'watch': [], 'wait': [], 'controller': [], 'nodes': [], 'orders': [], 'runs': [],
}
for command, flags in checks.items():
 result = subprocess.run([binary,*command.split(),'--help'], capture_output=True,text=True)
 assert result.returncode == 0, (command,result.stderr)
 for flag in flags:
  assert '--'+flag in result.stdout,(command,flag)
# Native peers uses custom parsing; Cobra help does not enumerate its flags.
engine = (repo / 'internal/peers/engine.go').read_text()
for flag in ['everywhere','peek','pending','limit','json','id']:
 assert '"'+flag+'"' in engine,flag
chat = (repo / 'internal/peers/chat.go').read_text()
for action in ['new','ls','show','say']:
 assert '"'+action+'"' in chat,action
print(f'PASS: {len(checks)} command help calls, listed flags, native peers flags and chat actions checked against supplied binary (use v0.6.0)')
