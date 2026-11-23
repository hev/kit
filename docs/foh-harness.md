# Native front-of-house harness

Run `hev kit` or `hev foh` to start the interactive native harness. The session
registers with [peers](peers.md) and checks its local inbox about once a second.
The lower status bar shows the latest sender and message preview for each
delivery mode.

- **Background** is the default. The harness handles messages in a
  separate agent turn. Receipt leaves the current conversation and input alone.
- **Interrupt** starts a turn when the harness is idle. During a turn it uses
  the same steering boundaries as typing a follow-up. If that turn ends before
  a steer can land, the message starts the next turn.

The native inbox works with or without tmux. Third-party harnesses keep their
existing tmux delivery. `hev foh --once` runs a single non-interactive turn and
has no peer presence.
