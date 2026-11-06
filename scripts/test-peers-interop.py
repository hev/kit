#!/usr/bin/env python3
"""Scratch-only CLI interoperability: HEV executable and Python reference path."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile


def main():
    hev, reference = map(lambda p: str(Path(p).resolve()), sys.argv[1:])
    tmux = shutil.which('tmux')
    if not tmux:
        raise SystemExit('tmux required')
    with tempfile.TemporaryDirectory(prefix='peers-interop-') as scratch:
        home = Path(scratch)
        bindir = home / '.local/bin'
        bindir.mkdir(parents=True)
        sock = str(home / 'tmux.sock')
        # subprocess argument forwarding, without interpreting fixture arguments.
        wrapper = bindir / 'tmux'
        wrapper.write_text('#!/usr/bin/env python3\nimport os,sys\nos.execv(' + repr(tmux) + ', [' + repr(tmux) + ", '-S', " + repr(sock) + '] + sys.argv[1:])\n')
        wrapper.chmod(0o755)
        (bindir / 'peers').symlink_to(hev)
        state = home / '.local/state'
        state.mkdir(parents=True)
        (state / 'peers').write_text('scratch legacy writer seal\n')
        active = state / 'peers-v2'
        active.mkdir()
        (active / 'cutover-ready.json').write_text(json.dumps({'version': 2, 'legacy_consumers_retired': True}))
        env = {k: v for k, v in os.environ.items() if not k.startswith(('PEERS_', 'HEV_PEERS_', 'TMUX'))}
        env.update(HOME=scratch, PEERS_SELF='fixture', PEERS_HOSTS='', PATH=str(bindir) + os.pathsep + os.environ['PATH'], HEV_CONFIG=str(home / 'absent.toml'), HEV_DISPATCH_ROOT=str(home / 'dispatch'))

        def run(args, pane=None):
            childenv = dict(env)
            if pane:
                childenv['TMUX_PANE'] = pane
            result = subprocess.run(args, env=childenv, text=True, capture_output=True, check=True)
            return result.stdout

        try:
            panes = {}
            for name in ('one', 'two'):
                run(['tmux', '-f', '/dev/null', 'new-session', '-d', '-s', name, "bash -c 'exec -a claude sleep 120'"])
                panes[name] = run(['tmux', 'display-message', '-p', '-t', name, '#{pane_id}']).strip()
                run([hev, 'peers', 'inbox', 'on', name])
            run([hev, 'peers', 'send', 'two', 'native message', '--from', 'one', '--id', 'native'])
            received = json.loads(run([str(bindir / 'peers'), 'inbox', '--pending', '--json'], panes['two']))
            assert [m['id'] for m in received['messages']] == ['native'], received
            # Python must see the same incarnation and receipts as the native CLI.
            replay = json.loads(run(['python3', reference, 'inbox', '--pending', '--json'], panes['two']))
            assert [m['id'] for m in replay['messages']] == ['native'], replay
            run(['python3', reference, 'inbox', 'ack', 'native'], panes['two'])
            run(['python3', reference, 'send', 'one', 'python message', '--from', 'two', '--id', 'python'])
            received = json.loads(run([hev, 'peers', 'inbox', '--pending', '--json'], panes['one']))
            assert [m['id'] for m in received['messages']] == ['python'], received
            run([str(bindir / 'peers'), 'inbox', 'ack', 'python'], panes['one'])
            for name in panes:
                snapshot = json.loads(run(['python3', reference, 'inbox', '--peek', '--pending', '--json'], panes[name]))
                assert snapshot['unacknowledged'] == 0, snapshot
            run([str(bindir / 'peers'), 'send', 'two', 'shim message', '--from', 'one', '--id', 'shim'])
            received = json.loads(run([hev, 'peers', 'inbox', '--pending', '--json'], panes['two']))
            assert [m['id'] for m in received['messages']] == ['shim'], received
            run([hev, 'peers', 'inbox', 'ack', 'shim'], panes['two'])
            print('PASS: native/shim exchange and Python/Go consume, replay and ACK in scratch HOME')
        finally:
            subprocess.run([tmux, '-S', sock, 'kill-server'], env=env, capture_output=True)


if __name__ == '__main__':
    main()
