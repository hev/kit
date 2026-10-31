#!/usr/bin/env bash
# Build a secret-free Boat snapshot from a Linux hev binary built from this checkout.
# Usage: BOAT_API_KEY=... scripts/boat-template.sh SNAPSHOT [LINUX_AMD64_BINARY]
set -euo pipefail
snapshot=${1:?usage: boat-template.sh SNAPSHOT [LINUX_AMD64_BINARY]}
binary=${2:-}
if [[ -z "$binary" ]]; then
  binary=$(mktemp)
  trap 'rm -f "$binary"' EXIT
  GOOS=linux GOARCH=amd64 go build -o "$binary" ./cmd/hev
fi
python3 - "$snapshot" "$binary" <<'PY'
import json, os, sys, time, urllib.request, urllib.error
name, binary = sys.argv[1:]
key = os.environ['BOAT_API_KEY']
base = 'https://boat.dev/api/v1'
def api(method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base+path, data=data, method=method, headers={'Authorization':'Bearer '+key,'Content-Type':'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=660) as r: return json.load(r)
    except urllib.error.HTTPError as e:
        raise RuntimeError('Boat HTTP '+str(e.code)+' at '+path) from None
def command(vm, text):
    r=api('POST','/sandboxes/'+vm+'/commands',{'command':text,'timeoutSeconds':600})
    if r.get('exitCode') != 0 or r.get('timedOut'): raise RuntimeError('Template command failed: '+r.get('stderr',''))
    return r.get('stdout','')
vm=None
try:
    # noEnv prevents credentials and account secrets entering the template disk.
    vm=api('POST','/sandboxes',{'name':'hev-template-'+name,'type':'small','noEnv':True,'ttlSeconds':1800,'labels':{'hev.template':name}})['sandbox']['id']
    print('Template VM:',vm,flush=True)
    deadline=time.monotonic()+600
    while api('GET','/sandboxes/'+vm)['sandbox']['state'] not in ('ready','idle','running'):
        if time.monotonic()>deadline: raise RuntimeError('Template readiness timeout')
        time.sleep(2)
    upload=api('POST','/sandboxes/'+vm+'/files/upload-url',{'path':'/tmp/hev-template-binary'})
    with open(binary,'rb') as f:
        req=urllib.request.Request(upload['url'],data=f.read(),method='PUT')
    with urllib.request.urlopen(req,timeout=300) as r: r.read()
    setup=r'''set -eu
sudo install -m 755 /tmp/hev-template-binary /usr/local/bin/hev
rm /tmp/hev-template-binary
command -v tmux
command -v codex
command -v claude
mkdir -p ~/.hev ~/dispatch
cat > ~/.hev/config.toml <<'CONFIG'
[layer]
endpoint = "https://gcp-us-central1.turbopuffer.com"
[capture]
scan_interval = "5s"
CONFIG
sudo tee /etc/systemd/system/hevd.service >/dev/null <<'SERVICE'
[Unit]
Description=hev trace capture
After=network-online.target
[Service]
User=user
Environment=HOME=/home/user
EnvironmentFile=/run/hev-capture.env
ExecStart=/usr/local/bin/hev daemon
Restart=on-failure
[Install]
WantedBy=multi-user.target
SERVICE
sudo systemctl daemon-reload
# Enabled now; the runtime environment file is created by dispatch, not baked.
sudo systemctl enable hevd.service
hev --version
'''
    print(command(vm,setup),flush=True)
    api('POST','/saved-snapshots',{'sandboxId':vm,'name':name})
    deadline=time.monotonic()+1200
    while True:
        r=api('GET','/saved-snapshots/'+name)
        status=r.get('savedSnapshot',r.get('snapshot',r)).get('status')
        if status=='ready': break
        if status=='failed' or time.monotonic()>deadline: raise RuntimeError('Snapshot failed or timed out')
        time.sleep(5)
    print('Saved secret-free snapshot:',name,flush=True)
finally:
    if vm:
        # Delete even on setup failure; report cleanup failure explicitly.
        api('DELETE','/sandboxes/'+vm)
        print('Deleted template VM:',vm,flush=True)
PY
