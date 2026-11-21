use crate::{
    Result,
    config::{Config, cache_path},
    data::display_text,
};
use serde::Deserialize;
use std::fs;

#[derive(Default, Deserialize)]
#[serde(default, rename_all = "PascalCase")]
pub struct Snapshot {
    #[serde(deserialize_with = "null_default")]
    pub panes: Vec<Pane>,
    #[serde(deserialize_with = "null_default")]
    pub hosts: Vec<Host>,
    pub peers: usize,
    #[serde(deserialize_with = "null_default")]
    pub history: Vec<Conversation>,
}

#[derive(Default, Deserialize)]
#[serde(default, rename_all = "PascalCase")]
pub struct Pane {
    #[serde(rename = "ID")]
    pub id: String,
    pub name: String,
    pub dir: String,
    pub workspace: String,
    pub project: String,
    pub role: String,
    pub pane: String,
    pub harness: String,
    pub state: String,
    pub title: String,
    pub prompt: String,
    pub trace: String,
    pub active: bool,
}

#[derive(Default, Deserialize)]
#[serde(default, rename_all = "PascalCase")]
pub struct Host {
    pub name: String,
    #[serde(rename = "OS")]
    pub os: String,
    pub state: String,
    pub activity: String,
}

#[derive(Default, Deserialize)]
#[serde(default, rename_all = "PascalCase")]
pub struct Conversation {
    #[serde(rename = "ID")]
    pub id: String,
    pub harness: String,
    pub dir: String,
    pub title: String,
    pub prompt: String,
}

fn null_default<'de, D, T>(deserializer: D) -> std::result::Result<T, D::Error>
where
    D: serde::Deserializer<'de>,
    T: Deserialize<'de> + Default,
{
    Ok(Option::deserialize(deserializer)?.unwrap_or_default())
}

impl Snapshot {
    pub fn read() -> Result<Self> {
        let raw = fs::read(cache_path()?);
        Ok(raw
            .ok()
            .and_then(|raw| serde_json::from_slice(&raw).ok())
            .unwrap_or_default())
    }
}

impl Pane {
    pub fn in_context(&self, cfg: &Config) -> bool {
        cfg.scope(&self.workspace, &self.name, &self.dir) == cfg.workspace
    }
}

impl Host {
    pub fn badge(&self) -> String {
        let icon = match self.os.as_str() {
            "darwin" => " ",
            "linux" => "🐧 ",
            "windows" => "⊞ ",
            _ => "",
        };
        display_text(&format!(
            "{icon}{} {} {}",
            self.name, self.state, self.activity
        ))
    }
}
