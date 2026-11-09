#!/usr/bin/env python3
"""Parent-local Nitro controller. Install root-owned; invoke through sudo -n.

Configuration is /etc/kit-nitro.json, never supplied by the caller. Each slot
contains ssh, host_key and a Firecracker config with prepared tap networking.
The parent image supplies Firecracker, guest kernel/rootfs and network setup.
"""
import fcntl
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
import uuid

ROOT = Path('/var/lib/kit-nitro')
CONFIG = Path('/etc/kit-nitro.json')


def run(*args):
    return subprocess.run(args, check=True, stdout=subprocess.DEVNULL,
                          stderr=subprocess.DEVNULL)


def active(unit):
    return subprocess.run(['systemctl', 'is-active', '--quiet', unit],
                          stdout=subprocess.DEVNULL,
                          stderr=subprocess.DEVNULL).returncode == 0


def main():
    op = sys.argv[1]
    request = json.load(sys.stdin)
    ROOT.mkdir(mode=0o700, parents=True, exist_ok=True)
    with (ROOT / 'lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        config = json.loads(CONFIG.read_text())
        records = []
        for path in ROOT.glob('*/record.json'):
            record = json.loads(path.read_text())
            if not active(record['unit']):
                record['state'] = 'stopped'
            records.append(record)
        host = request.get('host', request.get('Host'))
        if not host:
            raise ValueError('host required')
        owned = [r for r in records if r['labels']['hev.dispatch'] == host]
        if op == 'list':
            return owned
        if op == 'create':
            used = {r['slot'] for r in records}  # stopped guests retain disks/slots
            slot_id = next(i for i in range(len(config['slots'])) if i not in used)
            slot = config['slots'][slot_id]
            ident = 'kit-nitro-' + uuid.uuid4().hex
            directory = ROOT / ident
            directory.mkdir(mode=0o700)
            try:
                vm = json.loads(json.dumps(slot['config']))
                # Published Nitro API: empty enclave selects confidential mode.
                # Reject ordinary KVM or direct-mode templates.
                if vm.get('enclave') != {}:
                    raise ValueError('confidential Nitro enclave config required')
                for index, drive in enumerate(vm.get('drives', [])):
                    target = directory / ('disk-' + str(index))
                    shutil.copyfile(drive['path_on_host'], target)
                    target.chmod(0o600)
                    drive['path_on_host'] = str(target)
                (directory / 'sandbox.json').write_text(json.dumps(vm))
                ttl = int(request.get('AutoStop', 0)) // 1000000000
                if ttl < 1:
                    raise ValueError('positive lifetime required')
                record = dict(id=ident, unit=ident, slot=slot_id,
                              name=request.get('Name', ident),
                              ssh=slot['ssh'], host_key=slot['host_key'],
                              state='running', expires=time.time() + ttl,
                              labels={'kit-managed': 'true', 'hev.dispatch': host})
                (directory / 'record.json').write_text(json.dumps(record))
                run('systemd-run', '--quiet', '--collect', '--unit=' + ident,
                    '--property=RuntimeMaxSec=' + str(ttl),
                    config['firecracker'], '--config-file', str(directory / 'sandbox.json'))
                return record
            except BaseException:
                subprocess.run(['systemctl', 'stop', ident], stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL)
                shutil.rmtree(directory)
                raise
        record = next(r for r in owned if r['id'] == request['id'])
        if op == 'delete':
            run('systemctl', 'stop', record['unit'])
            shutil.rmtree(ROOT / record['id'])
        elif op == 'stop':
            # A stopped Firecracker VM cannot resume its prior memory. Refuse
            # rather than silently replacing the guest during rework.
            raise ValueError('Nitro stop/resume unsupported; use delete/recreate')
        elif op == 'resume':
            raise ValueError('Nitro resume unsupported; use a new order')
        else:
            raise ValueError('unknown operation')
        return None


if __name__ == '__main__':
    try:
        print(json.dumps(main()))
    except BaseException:
        # Images, networking, caller data and subprocess output stay private.
        print('Nitro controller operation failed', file=sys.stderr)
        sys.exit(1)
