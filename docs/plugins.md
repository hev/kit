# Exec commands

An unknown top-level command, `hev NAME args...`, runs `hev-NAME` from
PATH with the remaining arguments. Standard input, output, error, and the
process exit code pass through. Built-in names and aliases always win.
On Windows, exec commands may be `hev-NAME.exe` or `hev-NAME.cmd`.

`hev help` and `hev --help` list discovered names under **Exec commands**.
Discovery follows PATH precedence and removes duplicates. Exec commands are
ordinary executables. Install only programs you trust.
Private commands can ship separately without adding their implementation here.

For example, put an executable named `hev-hello` on PATH:

```sh
#!/bin/sh
printf 'Hello %s\n' "$*"
```

Then run `hev hello world`.

On Unix, an exec command replaces the `hev` process, preserving terminal signals and
exit status. On Windows, console interrupts reach the exec command and `hev` waits
for its exit. Batch exec commands use `cmd.exe /d /s /c`; paths with spaces are
supported. For safety, `.cmd` paths and arguments containing shell syntax
(`"`, `^`, `&`, `|`, `<`, `>`, `(`, `)`, `%`, `!`, or newlines) are refused.
Use an `.exe` exec command when arbitrary arguments are required.
