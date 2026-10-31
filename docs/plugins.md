# Plugins

An unknown top-level command, `hev NAME args...`, runs `hev-NAME` from
PATH with the remaining arguments. Standard input, output, error, and the
process exit code pass through. Built-in names and aliases always win.
On Windows, plugins may be `hev-NAME.exe` or `hev-NAME.cmd`.

`hev help` and `hev --help` list executable plugins in a separate Plugins
section, showing their names. Discovery follows PATH precedence and removes
duplicates. Plugins are ordinary executables; install only programs you trust.
Private commands can ship separately without adding their implementation here.

For example, put an executable named `hev-hello` on PATH:

```sh
#!/bin/sh
printf 'Hello %s\n' "$*"
```

Then run `hev hello world`.

On Unix, a plugin replaces the `hev` process, preserving terminal signals and
exit status. On Windows, console interrupts reach the plugin and `hev` waits
for its exit. Batch plugins use `cmd.exe /d /s /c`; paths with spaces are
supported. For safety, `.cmd` paths and arguments containing shell syntax
(`"`, `^`, `&`, `|`, `<`, `>`, `(`, `)`, `%`, `!`, or newlines) are refused.
Use an executable plugin when arbitrary arguments are required.
