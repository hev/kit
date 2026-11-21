use crate::{
    Result,
    config::Config,
    data::{Item, render},
};
use std::{
    env,
    io::{self, Write},
    os::unix::fs::PermissionsExt,
    process::{Command, Stdio},
};

pub fn require_dependencies() -> Result<()> {
    let path = env::var_os("PATH").unwrap_or_default();
    let missing: Vec<_> = ["tmux", "fzf", "gum"]
        .into_iter()
        .filter(|name| {
            !env::split_paths(&path).any(|dir| {
                dir.join(name)
                    .metadata()
                    .is_ok_and(|m| m.is_file() && m.permissions().mode() & 0o111 != 0)
            })
        })
        .collect();
    if !missing.is_empty() {
        return Err(format!(
            "Missing required dependencies: {}\nRun: brew install {}",
            missing.join(" "),
            missing.join(" ")
        )
        .into());
    }
    Ok(())
}

pub fn clear() -> io::Result<()> {
    print!("\x1b[H\x1b[2J\x1b[3J");
    io::stdout().flush()
}

pub fn shell_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', "'\\''"))
}

/// What fzf accepted: the row's hidden key and, when one of the `--expect`
/// keys rather than Enter accepted it, that key.
pub struct Choice {
    pub key: String,
    pub pressed: Option<String>,
}

pub fn choose(
    items: &[Item],
    header: &str,
    allow_kill: bool,
    expect: &[&str],
) -> Result<Option<Choice>> {
    choose_inner(items, header, "", allow_kill, expect, false, None, "")
}

/// Like `choose`, with a fixed line under the list (not selectable).
/// The main list: like `choose`, with a fixed line under the list (not
/// selectable). Both are drawn from saved data, then refreshed in the
/// background once fzf is up: the footer's servers, and the list itself with
/// fresh session descriptions. The header and its rule take `accent`, the
/// context's color.
pub fn choose_with_footer(
    items: &[Item],
    header: &str,
    footer: &str,
    allow_kill: bool,
    expect: &[&str],
    accent: &str,
    config: &Config,
) -> Result<Option<Choice>> {
    choose_inner(
        items,
        header,
        footer,
        allow_kill,
        expect,
        false,
        Some(config),
        accent,
    )
}

pub fn choose_wrapped(
    items: &[Item],
    header: &str,
    allow_kill: bool,
    expect: &[&str],
) -> Result<Option<Choice>> {
    choose_inner(items, header, "", allow_kill, expect, true, None, "")
}

fn choose_inner(
    items: &[Item],
    header: &str,
    footer: &str,
    allow_kill: bool,
    expect: &[&str],
    wrap: bool,
    config: Option<&Config>,
    accent: &str,
) -> Result<Option<Choice>> {
    let mut command = Command::new("fzf");
    if wrap {
        command.arg("--wrap=word");
    }
    command
        .args([
            "--ansi",
            // Fill the popup: "~" (shrink to fit) counts rows, not lines, and
            // cuts off the two-line session rows.
            "--height=100%",
            "--layout=reverse",
            "--no-sort",
            "--no-info",
            "--pointer=▸",
            "--color=pointer:#f5c2e7,hl+:#a6e3a1,fg+:#cdd6f4",
            "--header-first",
            "--bind=j:down,k:up",
            "--prompt=",
            "--read0",
            "--delimiter=\t",
            "--with-nth=2..",
        ])
        .arg(format!("--header={header}"));
    if !footer.is_empty() {
        command.arg(format!("--footer={footer}"));
    }
    if let Some(config) = config {
        // The main list is picked with the cursor, not typed into: no input
        // line (it only showed a blinking cursor). "/" brings it back to
        // filter, and then j/k type instead of moving.
        command.args(["--no-input", "--bind=/:show-input+unbind(j,k,/)"]);
        // Something under the help text, setting it apart from the rows.
        // With an accent the header is a color band, which is its own
        // divider: a line border would be filled with the band's color too
        // and show as an empty second row.
        if accent.is_empty() {
            command.arg("--header-border=line");
        } else {
            command.arg(format!("--color=header:#171923:bold,header-bg:{accent}"));
        }
        // Asynchronous reload keeps the cached first draw visible.
        let executable = shell_quote(&env::current_exe()?.to_string_lossy());
        let args = format!(
            "--workspace {} --harness {}",
            config.workspace, config.harness
        );
        let mut start = format!("--bind=start:bg-transform({executable} {args} --emit-reload)");
        if !config.pick.hide_boh {
            start.push_str(&format!(
                "+bg-transform-footer({executable} {args} --emit-footer)"
            ));
        }
        command.arg(start);
    }
    if let Some(config) = config.filter(|_| allow_kill) {
        let executable = shell_quote(&env::current_exe()?.to_string_lossy());
        let args = format!(
            "--workspace {} --harness {}",
            config.workspace, config.harness
        );
        let kill = format!(
            "execute({executable} {args} --kill-session {{1}})+reload({executable} {args} --emit-keyed)"
        );
        for key in ["ctrl-x", "delete"] {
            command.arg(format!("--bind={key}:{kill}"));
        }
        let template = kill.replace("{1}", "'%s'");
        command.arg(format!(
            r#"--bind=bspace:transform:[ -n "$FZF_QUERY" ] && echo backward-delete-char || printf {} {{1}}"#,
            shell_quote(&template)
        ));
    }
    if !expect.is_empty() {
        command.arg(format!("--expect={}", expect.join(",")));
    }
    let mut child = command
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::inherit())
        .spawn()?;
    let write = child
        .stdin
        .take()
        .expect("piped fzf stdin")
        .write_all(render(items, true).as_bytes());
    let output = child.wait_with_output()?;
    if matches!(output.status.code(), Some(1 | 130)) || output.status.code().is_none() {
        return Ok(None);
    }
    if !output.status.success() {
        return Err(format!("fzf exited with {}", output.status).into());
    }
    write?;
    let selected = String::from_utf8_lossy(&output.stdout);
    let mut lines = selected.lines();
    // With --expect the first line names the key that accepted, empty for Enter.
    let pressed = if expect.is_empty() {
        None
    } else {
        lines
            .next()
            .filter(|key| !key.is_empty())
            .map(str::to_owned)
    };
    Ok(lines
        .next()
        .and_then(|line| line.split_once('\t'))
        .map(|(key, _)| Choice {
            key: key.to_owned(),
            pressed,
        }))
}

fn gum() -> Command {
    let mut command = Command::new("gum");
    command
        .env("GUM_CHOOSE_CURSOR_FOREGROUND", "#f5c2e7")
        .env("GUM_CHOOSE_SELECTED_FOREGROUND", "#a6e3a1")
        .env("GUM_CHOOSE_HEADER_FOREGROUND", "#89b4fa")
        .env("GUM_INPUT_CURSOR_FOREGROUND", "#f5c2e7")
        .env("GUM_INPUT_PROMPT_FOREGROUND", "#cba6f7");
    command
}

/// `value` is text already in the field, for the user to finish or replace.
pub fn input(prompt: &str, placeholder: &str, width: u32, value: &str) -> Result<Option<String>> {
    println!();
    let mut command = gum();
    command.args([
        "input",
        &format!("--placeholder={placeholder}"),
        &format!("--prompt={prompt}"),
        &format!("--width={width}"),
    ]);
    if !value.is_empty() {
        command.arg(format!("--value={value}"));
    }
    let output = command
        .stdin(Stdio::inherit())
        .stderr(Stdio::inherit())
        .output()?;
    if !output.status.success() {
        return Ok(None);
    }
    let input = String::from_utf8(output.stdout)?
        .trim_end_matches(['\r', '\n'])
        .to_owned();
    Ok((!input.is_empty()).then_some(input))
}

pub fn confirm(prompt: &str, affirmative: &str) -> Result<bool> {
    Ok(gum()
        .args([
            "confirm",
            &format!("--affirmative={affirmative}"),
            "--negative=Cancel",
            "--prompt.foreground=#f38ba8",
            "--",
            prompt,
        ])
        .status()?
        .success())
}
