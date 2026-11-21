mod cache;
mod config;
mod data;
mod ui;
mod wall;

use cache::{Pane, Snapshot};
use config::{Config, DEFAULT, HARNESSES, cache_path, color, expand, valid_name};
use data::{Item, display_text, render};
use std::{
    collections::HashSet,
    env, fs,
    io::{self, IsTerminal, Write},
    os::unix::process::CommandExt,
    process::{Command, Stdio},
    time::Duration,
};
use ui::Choice;

type Result<T> = std::result::Result<T, Box<dyn std::error::Error>>;

fn tmux() -> Command {
    let mut command = Command::new("tmux");
    if let Some(socket) = env::var_os("HEV_TMUX_SOCKET").filter(|s| !s.is_empty()) {
        command.arg("-L").arg(socket);
    }
    command
}

fn run(command: &mut Command) -> Result<()> {
    let status = command.status()?;
    if status.success() {
        Ok(())
    } else {
        Err(format!("{:?} returned {status}", command.get_program()).into())
    }
}

fn attach(target: Option<&str>) -> Result<()> {
    let mut command = tmux();
    if env::var_os("TMUX").is_some() {
        command.arg("switch-client");
    } else {
        command.arg("attach-session");
    }
    if let Some(target) = target {
        command.args(["-t", target]);
    }
    Err(command.exec().into())
}

fn refresh_command() -> Command {
    let mut command = Command::new("hev");
    command
        .args(["pick", "refresh"])
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null());
    command
}

fn refresh() {
    if cache_path()
        .ok()
        .and_then(|p| p.metadata().ok())
        .and_then(|m| m.modified().ok())
        .and_then(|t| t.elapsed().ok())
        .is_none_or(|age| age >= Duration::from_secs(3))
    {
        let _ = refresh_command().status();
    }
}

fn queue_refresh() {
    // The wall can keep rendering the last snapshot while hevd is queried.
    std::thread::spawn(refresh);
}

fn label(icon: &str, name: &str) -> String {
    format!("{icon} {name}").trim().to_owned()
}

fn unique_sessions(snapshot: &Snapshot) -> impl Iterator<Item = &Pane> {
    let mut seen = HashSet::new();
    snapshot
        .panes
        .iter()
        .map(|p| {
            if p.active {
                p
            } else {
                snapshot
                    .panes
                    .iter()
                    .find(|q| q.id == p.id && q.active)
                    .unwrap_or(p)
            }
        })
        .filter(move |p| seen.insert(&p.id))
}

fn items(cfg: &Config, snapshot: &Snapshot, styled: bool) -> Vec<Item> {
    let current = &cfg.workspaces[&cfg.workspace];
    let paint = |item: Item, accent: &str| {
        if styled {
            item.color(color(accent))
        } else {
            item
        }
    };
    let mut rows = vec![paint(Item::new("new", "new session"), &current.color)];
    if cfg.workspace == DEFAULT {
        rows.push(Item::new("shell", "shell"));
        rows.extend(
            cfg.pick
                .frequents
                .keys()
                .filter(|n| n.as_str() != "shell" && valid_name(n))
                .map(|name| Item::new(&format!("frequent:{name}"), name)),
        );
    }
    rows.extend(current.projects.iter().map(|(name, p)| {
        paint(
            Item::new(&format!("place:{name}"), &label(&p.icon, name)),
            &p.color,
        )
    }));
    rows.push(Item::new("heading", "── clients ──"));
    for (name, w) in &cfg.workspaces {
        if !w.archived && name != DEFAULT && name != &cfg.workspace {
            let text = format!("{} · new session", label(&w.icon, name));
            rows.push(paint(Item::new(&format!("client:{name}"), &text), &w.color));
        }
    }
    rows.extend([
        Item::new("new-client", "new client"),
        Item::new("archive", "archive"),
    ]);
    if !cfg.pick.check_in.is_empty() {
        rows.push(Item::new("check-in", "check-in"));
    }
    rows.extend([
        Item::new("wall", "agent wall"),
        Item::new("resume", "resume"),
        Item::new("clear", "clear tmux"),
    ]);
    for p in unique_sessions(snapshot).filter(|p| p.in_context(cfg)) {
        let mut parts = vec![p.name.as_str()];
        if p.role == "foh" {
            parts.push("FOH");
        }
        parts.extend(
            [p.harness.as_str(), &p.title, &p.prompt]
                .into_iter()
                .filter(|s| !s.is_empty()),
        );
        let accent = cfg
            .project(current, &p.project, &p.name, &p.dir)
            .map_or(current.color.as_str(), |(_, project)| {
                project.color.as_str()
            });
        rows.push(paint(
            Item::new(&format!("session:{}", p.id), &parts.join(" · ")),
            accent,
        ));
    }
    rows
}

fn footer(cfg: &Config, snapshot: &Snapshot) -> String {
    if cfg.pick.hide_boh || snapshot.hosts.is_empty() {
        String::new()
    } else {
        format!(
            "BOH  {}",
            snapshot
                .hosts
                .iter()
                .map(|h| h.badge())
                .collect::<Vec<_>>()
                .join("  │  ")
        )
    }
}

fn status_text(text: &str) -> String {
    display_text(text).replace('#', "##")
}

fn status(cfg: &Config, args: &[String]) -> Result<()> {
    let snapshot = Snapshot::read()?;
    match args {
        [side] if side == "right" => {
            let badge = footer(cfg, &snapshot);
            let suffix = if badge.is_empty() {
                String::new()
            } else {
                format!("  │  {}", status_text(&badge))
            };
            println!("peers {}{suffix}", snapshot.peers);
        }
        [side, id] if side == "left" => {
            if let Some(p) = unique_sessions(&snapshot).find(|p| p.id == *id || p.name == *id) {
                let scope = cfg.scope(&p.workspace, &p.name, &p.dir);
                let w = cfg
                    .workspaces
                    .get(&scope)
                    .unwrap_or(&cfg.workspaces[DEFAULT]);
                let accent = cfg
                    .project(w, &p.project, &p.name, &p.dir)
                    .map_or(&w.color, |(_, project)| &project.color);
                println!(
                    "#[bg={},fg=#171923,bold]  {}  #[default]",
                    color(accent),
                    status_text(&p.name)
                );
            }
        }
        _ => return Err("expected --status left SESSION_ID or --status right".into()),
    }
    Ok(())
}

fn launch(cfg: &Config, workspace: &str, project: &str, named: bool) -> Result<()> {
    let mut command = Command::new("hev");
    command.args(["foh", "--workspace", workspace, "--harness", &cfg.harness]);
    if !project.is_empty() {
        command.args(["--project", project]);
    }
    if named {
        let prefix = if project.is_empty() {
            workspace
        } else {
            project
        };
        let Some(name) = ui::input("Name: ", "session name", 50, &format!("foh-{prefix}-"))? else {
            return Ok(());
        };
        command.args(["--session", &name]);
    }
    run(&mut command)
}

fn plain(dir: &str, harness: Option<&str>, name: Option<&str>) -> Result<()> {
    let mut command = tmux();
    command
        .args(["new-session", "-d", "-P", "-F", "#{session_id}", "-c"])
        .arg(expand(dir));
    if let Some(name) = name {
        command.args(["-s", name]);
    }
    if let Some(harness) = harness {
        command.arg(harness);
    }
    let output = command.output()?;
    if !output.status.success() {
        return Err(String::from_utf8_lossy(&output.stderr).into_owned().into());
    }
    attach(Some(String::from_utf8(output.stdout)?.trim()))
}

fn frequent(cfg: &Config, key: &str, named: bool) -> Result<()> {
    let directory = cfg.pick.frequents.get(key).map_or("~", String::as_str);
    let name = if named {
        let Some(name) = ui::input("Session name: ", "", 50, &format!("{key}-"))? else {
            return Ok(());
        };
        if !valid_name(&name) {
            return Err("invalid session name".into());
        }
        name
    } else {
        let output = tmux()
            .args(["list-sessions", "-F", "#{session_name}"])
            .output()?;
        let names = String::from_utf8_lossy(&output.stdout);
        std::iter::once(key.to_owned())
            .chain((2..).map(|i| format!("{key}-{i}")))
            .find(|candidate| !names.lines().any(|name| name == candidate))
            .unwrap()
    };
    let harness = (key != "shell").then_some(cfg.harness.as_str());
    plain(directory, harness, Some(&name))
}

fn close(cfg: &Config, target: Option<&str>) -> Result<()> {
    if target.is_none() && !ui::confirm("Close every session in this workspace?", "Close")? {
        return Ok(());
    }
    let snapshot = Snapshot::read()?;
    let mut errors = Vec::new();
    for p in unique_sessions(&snapshot)
        .filter(|p| p.in_context(cfg) && target.is_none_or(|id| id == p.id))
    {
        if let Err(err) = close_one(cfg, p) {
            errors.push(format!("{}: {err}", p.name));
        }
    }
    refresh_command().status().ok();
    if !errors.is_empty() {
        return Err(errors.join("\n").into());
    }
    Ok(())
}

fn close_one(cfg: &Config, p: &Pane) -> Result<()> {
    let live = tmux().args(["display-message", "-p", "-t", &p.pane,
        "#{session_id}\t#{session_name}\t#{@hev-workspace}\t#{pane_current_path}\t#{@peers-role}\t#{@hev-project}"]).output()?;
    let live = String::from_utf8_lossy(&live.stdout);
    let fields: Vec<_> = live.trim_end_matches('\n').split('\t').collect();
    let [id, name, workspace, dir, role, project] = fields.as_slice() else {
        return Ok(());
    };
    if *id != p.id || *name != p.name || cfg.scope(workspace, name, dir) != cfg.workspace {
        return Ok(());
    }
    let route = cfg
        .pick
        .close_routes
        .get(*project)
        .or_else(|| cfg.pick.close_routes.get(&cfg.workspace));
    if let Some(route) = route.filter(|_| *role == "foh") {
        let screen = tmux()
            .args(["capture-pane", "-p", "-t", &p.pane, "-S", "-30"])
            .output()?;
        if !screen.status.success() {
            return Err("could not capture the last screen for the close notice".into());
        }
        let message = format!(
            "Owner closed {} from the picker. Continue managing its runs.\n\n{}",
            p.name,
            String::from_utf8_lossy(&screen.stdout)
        );
        run(Command::new("hev").args(["peers", "send", route, &message, "--from", &p.name]))?;
    }
    run(tmux().args(["kill-session", "-t", &p.id]))
}

fn hosts(snapshot: &Snapshot) -> Result<()> {
    let rows: Vec<_> = snapshot
        .hosts
        .iter()
        .map(|h| Item::new(&h.name, &h.badge()))
        .collect();
    if let Some(choice) = ui::choose(&rows, "BOH hosts · enter details · esc back", false, &[])?
        && let Some(host) = snapshot.hosts.iter().find(|h| h.name == choice.key)
    {
        let rows = [
            Item::new("os", &format!("OS  {}", host.os)),
            Item::new("status", &format!("Status  {}", host.state)),
            Item::new("activity", &format!("Activity  {}", host.activity)),
        ];
        ui::choose(&rows, &host.name, false, &[])?;
    }
    Ok(())
}

fn resume(cfg: &Config, snapshot: &Snapshot) -> Result<()> {
    let history: Vec<_> = snapshot
        .history
        .iter()
        .filter(|s| !s.dir.is_empty() && cfg.scope("", "", &s.dir) == cfg.workspace)
        .collect();
    let rows: Vec<_> = history
        .iter()
        .enumerate()
        .map(|(i, s)| {
            Item::new(
                &i.to_string(),
                &format!("{} · {} · {} · {}", s.harness, s.dir, s.title, s.prompt),
            )
        })
        .collect();
    let Some(choice) = ui::choose_wrapped(
        &rows,
        "Recent conversations · enter details · esc back",
        false,
        &[],
    )?
    else {
        return Ok(());
    };
    let Some(row) = choice
        .key
        .parse::<usize>()
        .ok()
        .and_then(|i| history.get(i))
    else {
        return Ok(());
    };
    let command = match row.harness.as_str() {
        "claude" => format!("claude --resume {}", ui::shell_quote(&row.id)),
        "codex" => format!("codex resume {}", ui::shell_quote(&row.id)),
        "opencode" => format!("opencode --session {}", ui::shell_quote(&row.id)),
        _ => String::new(),
    };
    let w = &cfg.workspaces[&cfg.workspace];
    let running: Vec<_> = unique_sessions(snapshot)
        .filter(|p| p.in_context(cfg) && p.harness == row.harness && p.trace == row.id)
        .collect();
    let can_resume = running.is_empty()
        && !command.is_empty()
        && expand(&row.dir).is_dir()
        && (row.harness != "claude" || w.login != "own");
    let mut details = vec![
        Item::new("detail", &row.dir),
        Item::new("detail", &row.title),
        Item::new("detail", &row.prompt),
    ];
    if running.len() == 1 {
        details.insert(0, Item::new("attach", "attach to running conversation"));
    } else if can_resume {
        details.insert(0, Item::new("resume", "resume conversation"));
    }
    if let Some(choice) =
        ui::choose_wrapped(&details, "Conversation details · esc back", false, &[])?
    {
        match choice.key.as_str() {
            "attach" => attach(Some(&running[0].id))?,
            "resume" => plain(&row.dir, Some(&command), None)?,
            _ => (),
        }
    }
    Ok(())
}

fn new_client(cfg: &Config) -> Result<()> {
    let Some(name) = ui::input("Client: ", "short workspace name", 50, "")? else {
        return Ok(());
    };
    if !valid_name(&name) || cfg.workspaces.contains_key(&name) {
        return Err(
            "choose a new workspace name containing letters, digits, underscores or hyphens".into(),
        );
    }
    let dir = expand(&cfg.workspaces[DEFAULT].dir)
        .join("clients")
        .join(&name);
    fs::create_dir_all(&dir)?;
    if !cfg.pick.new_client_brief.is_empty() {
        let guide = dir.join("AGENTS.md");
        let brief = fs::read_to_string(expand(&cfg.pick.new_client_brief))?;
        match fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&guide)
        {
            Ok(mut file) => file.write_all(brief.replace("{{NAME}}", &name).as_bytes())?,
            Err(err) if err.kind() == io::ErrorKind::AlreadyExists => (),
            Err(err) => return Err(err.into()),
        }
        match std::os::unix::fs::symlink("AGENTS.md", dir.join("CLAUDE.md")) {
            Ok(()) => (),
            Err(err) if err.kind() == io::ErrorKind::AlreadyExists => (),
            Err(err) => return Err(err.into()),
        }
    }
    run(Command::new("hev")
        .args(["workspace", "add", &name, "--dir"])
        .arg(&dir))?;
    launch(cfg, &name, "", false)
}

fn interactive(cfg: &mut Config) -> Result<()> {
    ui::require_dependencies()?;
    loop {
        ui::clear()?;
        let snapshot = Snapshot::read()?;
        let footer = footer(cfg, &snapshot);
        let allowed = &cfg.workspaces[&cfg.workspace].hosts;
        let mut header = format!(
            "enter open · ctrl-n named · ctrl-a harness: {} · ctrl-o workspace: {} · ctrl-s hosts · ctrl-w wall · ctrl-x/delete kill · ctrl-l clear · ctrl-r refresh · / search · esc quit",
            cfg.harness, cfg.workspace
        );
        if !allowed.is_empty() {
            header.push_str(&format!(
                " · workspace hosts: {}",
                display_text(&allowed.join(", "))
            ));
        }
        let keys = [
            "ctrl-n", "ctrl-a", "ctrl-o", "ctrl-s", "ctrl-w", "ctrl-l", "ctrl-r",
        ];
        let Some(Choice { key, pressed }) = ui::choose_with_footer(
            &items(cfg, &snapshot, true),
            &header,
            &footer,
            true,
            &keys,
            color(&cfg.workspaces[&cfg.workspace].color),
            cfg,
        )?
        else {
            return Ok(());
        };
        let named = pressed.as_deref() == Some("ctrl-n");
        match pressed.as_deref() {
            Some("ctrl-a") => {
                let i = HARNESSES
                    .iter()
                    .position(|n| *n == cfg.harness)
                    .unwrap_or_default();
                cfg.set_harness(HARNESSES[(i + 1) % HARNESSES.len()]);
                cfg.remember()?;
            }
            Some("ctrl-o") => {
                let mut active = vec![DEFAULT.to_owned()];
                active.extend(
                    cfg.workspaces
                        .iter()
                        .filter(|(k, w)| k.as_str() != DEFAULT && !w.archived)
                        .map(|(k, _)| k.clone()),
                );
                let i = active
                    .iter()
                    .position(|n| *n == cfg.workspace)
                    .unwrap_or_default();
                cfg.set_workspace(&active[(i + 1) % active.len()]);
                cfg.remember()?;
            }
            Some("ctrl-s") if !cfg.pick.hide_boh => hosts(&Snapshot::read()?)?,
            Some("ctrl-s") => (),
            Some("ctrl-w") => return wall::run(cfg, None),
            Some("ctrl-l") => close(cfg, None)?,
            Some("ctrl-r") => {
                let _ = refresh_command().status();
            }
            _ => {
                let (kind, target) = key.split_once(':').unwrap_or((&key, ""));
                match kind {
                    "new" => return launch(cfg, &cfg.workspace, "", named),
                    "client" => return launch(cfg, target, "", named),
                    "place" => return launch(cfg, &cfg.workspace, target, named),
                    "shell" => return frequent(cfg, "shell", named),
                    "frequent"
                        if cfg.pick.project_tables
                            && !named
                            && cfg.workspaces[DEFAULT].projects.contains_key(target) =>
                    {
                        return launch(cfg, DEFAULT, target, false);
                    }
                    "frequent" => return frequent(cfg, target, named),
                    "wall" => return wall::run(cfg, None),
                    "resume" => resume(cfg, &snapshot)?,
                    "clear" => close(cfg, None)?,
                    "session" => return attach(Some(target)),
                    "check-in" => {
                        return run(
                            Command::new(&cfg.pick.check_in[0]).args(&cfg.pick.check_in[1..])
                        );
                    }
                    "new-client" => new_client(cfg)?,
                    "archive" => {
                        let rows: Vec<_> = cfg
                            .workspaces
                            .iter()
                            .filter(|(_, w)| w.archived)
                            .map(|(name, w)| {
                                Item::new(name, &label(&w.icon, name)).color(color(&w.color))
                            })
                            .collect();
                        if let Some(choice) = ui::choose(
                            &rows,
                            "Archived workspaces · enter open · esc back",
                            false,
                            &[],
                        )? {
                            return launch(cfg, &choice.key, "", named);
                        }
                    }
                    _ => (),
                }
            }
        }
    }
}

fn main_result() -> Result<()> {
    let mut args = env::args().skip(1).peekable();
    if args.peek().is_some_and(|a| a == "--version") {
        println!("hev-pick {}", env!("CARGO_PKG_VERSION"));
        return Ok(());
    }
    if args.peek().is_some_and(|a| a == "--help" || a == "-h") {
        println!(
            "Usage: hev-pick [--workspace NAME] [--harness NAME] [--wall [COLSxROWS] | --emit-items | --status left SESSION_ID | --status right]"
        );
        return Ok(());
    }
    let mut cfg = Config::read()?;
    let mut mode = Vec::new();
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--workspace" => cfg.set_workspace(&args.next().ok_or("--workspace requires a name")?),
            "--harness" => cfg.set_harness(&args.next().ok_or("--harness requires a name")?),
            _ => mode.push(arg),
        }
    }
    match mode.first().map(String::as_str) {
        None => interactive(&mut cfg),
        Some("--status") => status(&cfg, &mode[1..]),
        Some("--emit-items" | "--emit-reload" | "--emit-keyed") => {
            let keyed = mode[0] != "--emit-items";
            if mode[0] == "--emit-reload" {
                refresh();
                let exe = ui::shell_quote(&env::current_exe()?.to_string_lossy());
                println!(
                    "reload-sync({exe} --workspace {} --harness {} --emit-keyed)",
                    cfg.workspace, cfg.harness
                );
                return Ok(());
            }
            let snapshot = Snapshot::read()?;
            let mut output = render(&items(&cfg, &snapshot, keyed), keyed);
            if !keyed {
                let footer = footer(&cfg, &snapshot);
                if !footer.is_empty() {
                    output.push_str(&format!("{footer}\n"));
                }
            }
            io::stdout().lock().write_all(output.as_bytes())?;
            Ok(())
        }
        Some("--emit-footer") => {
            refresh();
            println!("{}", footer(&cfg, &Snapshot::read()?));
            Ok(())
        }
        Some("--wall") => {
            let fixed = mode
                .get(1)
                .map(|s| wall::parse_grid(s).ok_or("expected a wall grid from 1x1 to 16x16"))
                .transpose()?;
            wall::run(&cfg, fixed)
        }
        Some("--kill-session") => mode
            .get(1)
            .ok_or("missing session ID")?
            .strip_prefix("session:")
            .map_or(Ok(()), |id| close(&cfg, Some(id))),
        Some(arg) => Err(format!("unknown picker option {arg:?}").into()),
    }
}

fn main() {
    if let Err(err) = main_result() {
        if err
            .downcast_ref::<io::Error>()
            .is_some_and(|e| e.kind() == io::ErrorKind::BrokenPipe)
        {
            return;
        }
        eprintln!("hev-pick: {err}");
        if io::stdin().is_terminal() && io::stderr().is_terminal() {
            eprintln!("Press Enter to return.");
            let _ = io::stdin().read_line(&mut String::new());
        }
        std::process::exit(1);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn scopes_cached_rows_to_workspaces() {
        let mut cfg: Config = toml::from_str(
            r#"
            [workspaces.hev]
            dir = "/work"
            [workspaces.hev.projects.kit]
            dir = "/work/kit"
            [workspaces.bcc]
            dir = "/work/bcc"
            [workspaces.other]
            dir = "/work/other"
            [workspaces.past]
            archived = true
        "#,
        )
        .unwrap();
        let snapshot: Snapshot = serde_json::from_str(
            r#"{
          "Panes": [
            {"ID":"$1", "Name":"foh-kit", "Dir":"/work/kit", "Title":"Kit title"},
            {"ID":"$2", "Name":"client-agent", "Dir":"/work/bcc", "Title":"Client title"}
          ],
          "Hosts":null,
          "History":null
        }"#,
        )
        .unwrap();
        cfg.set_workspace("hev");
        let main = render(&items(&cfg, &snapshot, false), false);
        assert!(main.contains("shell\n") && main.contains("bcc · new session"));
        assert!(main.contains("Kit title") && !main.contains("Client title"));
        cfg.set_workspace("bcc");
        let client = render(&items(&cfg, &snapshot, false), false);
        assert!(client.starts_with("new session\n") && client.contains("other · new session"));
        assert!(client.contains("Client title") && !client.contains("Kit title"));
        assert!(client.contains("archive") && !client.contains("shell\n"));
        assert!(!client.contains("past · new session"));
    }
}
