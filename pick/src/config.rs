use crate::Result;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::{
    collections::BTreeMap,
    env, fs, io,
    path::{Path, PathBuf},
};

pub const DEFAULT: &str = "hev";
pub const HARNESSES: &[&str] = &["claude", "codex", "pi", "agy", "opencode"];

#[derive(Default, Deserialize)]
#[serde(default)]
pub struct Workspace {
    pub dir: String,
    pub icon: String,
    pub color: String,
    pub archived: bool,
    pub login: String,
    pub hosts: Vec<String>,
    pub projects: BTreeMap<String, Workspace>,
}

#[derive(Default, Deserialize)]
#[serde(default)]
pub struct Picker {
    pub frequents: BTreeMap<String, String>,
    pub check_in: Vec<String>,
    pub new_client_brief: String,
    pub close_routes: BTreeMap<String, String>,
    pub project_tables: bool,
    pub hide_boh: bool,
}

#[derive(Default, Deserialize)]
#[serde(default)]
pub struct Config {
    pub workspaces: BTreeMap<String, Workspace>,
    pub pick: Picker,
    #[serde(skip)]
    pub workspace: String,
    #[serde(skip)]
    pub harness: String,
}

impl Config {
    pub fn read() -> Result<Self> {
        let path = config_path();
        let mut cfg: Self = match fs::read_to_string(path) {
            Ok(raw) => toml::from_str(&raw)?,
            Err(err) if err.kind() == io::ErrorKind::NotFound => Self::default(),
            Err(err) => return Err(err.into()),
        };
        cfg.workspaces
            .entry(DEFAULT.into())
            .or_insert_with(|| Workspace {
                dir: "~/workspace".into(),
                ..Workspace::default()
            });
        cfg.workspaces.retain(|name, w| {
            w.projects.retain(|project, _| valid_name(project));
            valid_name(name)
        });
        let state = fs::read_to_string(cache_path()?.with_extension("context")).unwrap_or_default();
        let mut fields = state.split_whitespace();
        cfg.set_workspace(fields.next().unwrap_or(DEFAULT));
        cfg.set_harness(fields.next().unwrap_or(HARNESSES[0]));
        Ok(cfg)
    }

    pub fn set_workspace(&mut self, name: &str) {
        self.workspace = if self.workspaces.get(name).is_some_and(|w| !w.archived) {
            name
        } else {
            DEFAULT
        }
        .into();
    }

    pub fn set_harness(&mut self, name: &str) {
        self.harness = if HARNESSES.contains(&name) {
            name
        } else {
            HARNESSES[0]
        }
        .into();
    }

    pub fn remember(&self) -> Result<()> {
        let path = cache_path()?.with_extension("context");
        fs::create_dir_all(path.parent().unwrap_or(Path::new(".")))?;
        let temp = path.with_extension(format!("context.{}", std::process::id()));
        fs::write(&temp, format!("{} {}\n", self.workspace, self.harness))?;
        fs::rename(temp, path)?;
        Ok(())
    }

    pub fn scope(&self, workspace: &str, name: &str, dir: &str) -> String {
        if !workspace.is_empty() {
            return workspace.to_owned();
        }
        self.workspaces
            .iter()
            .filter(|(key, _)| key.as_str() != DEFAULT)
            .filter(|(key, _)| belongs(name, key) || belongs(name, &format!("foh-{key}")))
            .max_by_key(|(key, _)| key.len())
            .or_else(|| {
                self.workspaces
                    .iter()
                    .filter(|(key, w)| {
                        key.as_str() != DEFAULT
                            && !w.dir.is_empty()
                            && Path::new(dir).starts_with(expand(&w.dir))
                    })
                    .max_by_key(|(_, w)| w.dir.len())
            })
            .map_or_else(|| DEFAULT.to_owned(), |(key, _)| key.clone())
    }

    pub fn project<'a>(
        &'a self,
        ws: &'a Workspace,
        project: &str,
        name: &str,
        dir: &str,
    ) -> Option<(&'a str, &'a Workspace)> {
        ws.projects
            .get_key_value(project)
            .or_else(|| {
                ws.projects
                    .iter()
                    .filter(|(key, p)| {
                        belongs(name, &format!("foh-{key}"))
                            || (!p.dir.is_empty() && Path::new(dir).starts_with(expand(&p.dir)))
                    })
                    .max_by_key(|(_, p)| p.dir.len())
            })
            .map(|(key, p)| (key.as_str(), p))
    }
}

pub fn valid_name(name: &str) -> bool {
    let mut bytes = name.bytes();
    bytes
        .next()
        .is_some_and(|c| c.is_ascii_alphanumeric() || c == b'_')
        && bytes.all(|c| c.is_ascii_alphanumeric() || c == b'_' || c == b'-')
}

fn belongs(name: &str, base: &str) -> bool {
    name == base
        || name
            .strip_prefix(base)
            .is_some_and(|tail| tail.starts_with('-'))
}

pub fn color(value: &str) -> &str {
    if value.len() == 7
        && value.starts_with('#')
        && value.bytes().skip(1).all(|c| c.is_ascii_hexdigit())
    {
        value
    } else {
        "#B6A0FF"
    }
}

pub fn expand(path: &str) -> PathBuf {
    let home = || PathBuf::from(env::var_os("HOME").unwrap_or_default());
    if path == "~" {
        home()
    } else if let Some(suffix) = path.strip_prefix("~/") {
        home().join(suffix)
    } else {
        PathBuf::from(path)
    }
}

fn setting(key: &str) -> Option<PathBuf> {
    env::var_os(key)
        .filter(|v| !v.is_empty())
        .map(PathBuf::from)
}

fn config_path() -> PathBuf {
    setting("HEV_CONFIG").unwrap_or_else(|| expand("~/.hev/config.toml"))
}

pub fn cache_path() -> Result<PathBuf> {
    if let Some(path) = setting("HEV_PICK_CACHE") {
        return Ok(path);
    }
    let socket = setting("HEV_TMUX_SOCKET")
        .or_else(|| {
            env::var("TMUX").ok().and_then(|v| {
                v.split(',')
                    .next()
                    .filter(|s| !s.is_empty())
                    .map(PathBuf::from)
            })
        })
        .unwrap_or_else(|| setting("TMUX_TMPDIR").unwrap_or_default().join("default"));
    let path = std::path::absolute(config_path())?;
    let mut digest = Sha256::new();
    digest.update(path.as_os_str().as_encoded_bytes());
    digest.update([0]);
    digest.update(socket.as_os_str().as_encoded_bytes());
    let key: String = digest.finalize()[..12]
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect();
    Ok(setting("XDG_CACHE_HOME")
        .unwrap_or_else(|| expand("~/.cache"))
        .join("hev/pick")
        .join(format!("{key}.json")))
}
