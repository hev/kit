//! Every live agent pane at once: a grid of read-only mirrors redrawn from
//! `capture-pane` each second. Mirrors rather than nested `tmux attach`
//! clients, because an attached client resizes the real session to its tile.
use crate::{
    Result,
    cache::Snapshot,
    config::Config,
    data::{RESET, display_text},
    tmux,
};
use std::collections::HashMap;
use std::{
    io::{self, Read, Write},
    os::unix::process::CommandExt,
    process::{Command, Stdio},
    time::{Duration, Instant},
};

// Printed between captures so one tmux call can carry every pane.
const MARK: &str = "@@tmux-picker-wall@@";
const TICK: Duration = Duration::from_secs(1);
// The process table is the slow part; panes come and go less often than they redraw.
const DISCOVER_EVERY: u32 = 3;
// The smallest tile worth reading: a header over four lines of transcript.
const MIN_WIDTH: usize = 24;
const MIN_HEIGHT: usize = 5;
// Cells are about twice as tall as they are wide, and agent output is prose,
// so a tile reads best at roughly three columns per row.
const ASPECT: f64 = 3.0;

const PINK: &str = "\x1b[38;2;245;194;231m";
const FOCUS: &str = "\x1b[1;38;2;30;30;46;48;2;245;194;231m";
const WORKING: &str = "\x1b[38;2;249;226;175m";
const WAITING: &str = "\x1b[38;2;166;227;161m";
const FAINT: &str = "\x1b[38;2;88;91;112m";
const BAR: &str = "\x1b[38;2;205;214;244;48;2;49;50;68m";
const MUTED: &str = "\x1b[38;2;127;132;156m";
// Closes any hyperlink a clipped line left open.
const END_LINE: &str = "\x1b[0m\x1b]8;;\x1b\\";

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Grid {
    pub cols: usize,
    pub rows: usize,
}

/// `8x6` → 8 columns by 6 rows.
pub fn parse_grid(value: &str) -> Option<Grid> {
    let (cols, rows) = value.split_once(['x', 'X'])?;
    let (cols, rows) = (cols.parse().ok()?, rows.parse().ok()?);
    ((1..=16).contains(&cols) && (1..=16).contains(&rows)).then_some(Grid { cols, rows })
}

#[derive(Debug)]
struct Tile {
    pane: String,
    label: String,
    agent: String,
    lines: Vec<String>,
    /// Where the conversation ends and the agent's own input box begins.
    transcript: usize,
    working: bool,
}

/// Like the picker's running list, the wall shows only the current client's
/// sessions, or with no client only the ones that belong to none.
fn discover(config: &Config) -> Result<Vec<Tile>> {
    let snapshot = Snapshot::read()?;
    let panes: Vec<_> = snapshot
        .panes
        .iter()
        .filter(|p| !p.harness.is_empty() && p.in_context(config))
        .collect();
    let mut counts = HashMap::new();
    for p in &panes {
        *counts.entry(&p.id).or_insert(0) += 1;
    }
    Ok(panes
        .iter()
        .map(|p| Tile {
            pane: p.pane.clone(),
            label: if counts[&p.id] > 1 {
                display_text(&format!("{}:{}", p.name, p.pane))
            } else {
                display_text(&p.name)
            },
            agent: display_text(&p.harness),
            lines: Vec::new(),
            transcript: 0,
            working: p.state == "working",
        })
        .collect())
}

/// One tmux call for every pane. Returns false when a pane had vanished,
/// which ends the rest of tmux's command list, so the caller rediscovers.
fn capture(tiles: &mut [Tile]) -> Result<bool> {
    if tiles.is_empty() {
        return Ok(true);
    }
    let mut command = tmux();
    for (index, tile) in tiles.iter().enumerate() {
        if index > 0 {
            command.arg(";");
        }
        command.args([
            "display-message",
            "-p",
            "-t",
            &tile.pane,
            &format!("{MARK}{index}"),
        ]);
        command.args([";", "capture-pane", "-e", "-p", "-t", &tile.pane]);
    }
    let output = command
        .stdin(Stdio::null())
        .stderr(Stdio::null())
        .output()?;
    Ok(apply_capture(
        tiles,
        &String::from_utf8_lossy(&output.stdout),
    ))
}

fn apply_capture(tiles: &mut [Tile], output: &str) -> bool {
    let mut sections: Vec<Option<Vec<String>>> = tiles.iter().map(|_| None).collect();
    let mut current = None;
    for line in output.lines() {
        if let Some(index) = line
            .strip_prefix(MARK)
            .and_then(|i| i.parse::<usize>().ok())
        {
            current = sections.get_mut(index).map(|s| s.insert(Vec::new()));
            continue;
        }
        if let Some(section) = current.as_mut() {
            section.push(line.to_owned());
        }
    }
    let mut complete = true;
    for (tile, section) in tiles.iter_mut().zip(sections) {
        // A missing capture keeps the last frame rather than blanking the tile.
        let Some(mut lines) = section else {
            complete = false;
            continue;
        };
        while lines.last().is_some_and(|l| is_blank(&visible(l))) {
            lines.pop();
        }
        // The status line sits just above the prompt box, under at most a tip.
        let start = lines.len().saturating_sub(12);
        tile.working = lines[start..].iter().any(|l| is_working(&visible(l)));
        tile.transcript = if tile.agent == "pi" {
            pi_transcript_end(&lines)
        } else {
            transcript_end(&lines)
        };
        tile.lines = lines;
    }
    complete
}

// Codex fills the space around its input box with a braille shimmer.
fn is_blank(text: &str) -> bool {
    text.chars()
        .all(|c| c.is_whitespace() || ('\u{2800}'..='\u{28ff}').contains(&c))
}

fn is_rule(text: &str) -> bool {
    let text = text.trim();
    !text.is_empty() && text.chars().all(|c| c == '─')
}

/// A tile is too small for the agent's own chrome, so it stops above the
/// input box: Claude's `❯` between two rules (a permission prompt's `❯ 1. Yes`
/// has no rule above it), or Codex's `›` in the last few lines.
fn transcript_end(lines: &[String]) -> usize {
    let texts: Vec<_> = lines.iter().map(|l| visible(l)).collect();
    let mut end = texts.len();
    for i in (texts.len().saturating_sub(15)..texts.len()).rev() {
        let text = texts[i].trim_start();
        if text.starts_with('❯') && i > 0 && is_rule(&texts[i - 1]) {
            end = i - 1;
            break;
        }
        if let Some(rest) = text.strip_prefix('›')
            && texts.len() - i <= 4
            && !rest.trim_start().starts_with(|c: char| c.is_ascii_digit())
        {
            end = i;
            break;
        }
    }
    while end > 0 && is_blank(&texts[end - 1]) {
        end -= 1;
    }
    end
}

/// PI's input box sits between two rules above its footer; while a turn runs
/// the top rule carries the spinner (`── ⠋ Working… ───`).
fn pi_transcript_end(lines: &[String]) -> usize {
    let texts: Vec<_> = lines.iter().map(|l| visible(l)).collect();
    let rules: Vec<_> = (texts.len().saturating_sub(15)..texts.len())
        .filter(|&i| is_rule(&texts[i]) || is_pi_spinner(&texts[i]))
        .collect();
    let mut end = match rules.as_slice() {
        [.., top, _] => *top,
        _ => texts.len(),
    };
    while end > 0 && is_blank(&texts[end - 1]) {
        end -= 1;
    }
    end
}

/// PI's working indicator embedded in the input box's top rule.
fn is_pi_spinner(line: &str) -> bool {
    line.trim_start()
        .strip_prefix("── ")
        .and_then(|rest| rest.chars().next())
        .is_some_and(|c| "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏".contains(c))
}

/// The grid that shows `count` tiles, as many as fit at `MIN_WIDTH` by
/// `MIN_HEIGHT`, at the largest size for their shape.
fn layout(count: usize, width: usize, height: usize) -> Grid {
    let max_cols = (width / MIN_WIDTH).max(1);
    let max_rows = (height / MIN_HEIGHT).max(1);
    let count = count.clamp(1, max_cols * max_rows);
    let mut best = (Grid { cols: 1, rows: 1 }, f64::MIN);
    for cols in 1..=count.min(max_cols) {
        let rows = count.div_ceil(cols);
        if rows > max_rows {
            continue;
        }
        let size = (width as f64 / cols as f64 / ASPECT).min(height as f64 / rows as f64);
        if size > best.1 {
            best = (Grid { cols, rows }, size);
        }
    }
    best.0
}

/// Claude's live spinner, `✶ Cerebrating… (5m 2s · ↓ 30k tokens)`, which
/// becomes `✻ Churned for 10s` when the turn ends; older Claude and Codex
/// say "esc to interrupt" while running.
fn is_working(line: &str) -> bool {
    if line.to_lowercase().contains("esc to interrupt") || is_pi_spinner(line) {
        return true;
    }
    let mut chars = line.trim_start().chars();
    let Some(glyph) = chars.next() else {
        return false;
    };
    let rest = chars.as_str();
    let Some((verb, after)) = rest.split_once("… (") else {
        return false;
    };
    !glyph.is_alphanumeric()
        && verb.starts_with(' ')
        && verb[1..].chars().all(char::is_alphabetic)
        && !verb[1..].is_empty()
        && after
            .trim_start_matches(|c: char| c.is_ascii_digit())
            .starts_with(['s', 'm', 'h'])
        && after.starts_with(|c: char| c.is_ascii_digit())
}

/// Byte index just past the escape sequence starting at `start`.
fn escape_end(line: &str, start: usize) -> usize {
    let bytes = line.as_bytes();
    let mut i = start + 1;
    match bytes.get(i) {
        Some(b'[') => {
            i += 1;
            while i < bytes.len() && !(0x40..=0x7e).contains(&bytes[i]) {
                i += 1;
            }
            (i + 1).min(bytes.len())
        }
        Some(b']') => {
            while i < bytes.len() {
                if bytes[i] == 0x07 {
                    return i + 1;
                }
                if bytes[i] == 0x1b && bytes.get(i + 1) == Some(&b'\\') {
                    return i + 2;
                }
                i += 1;
            }
            bytes.len()
        }
        _ => i + line[i..].chars().next().map_or(0, char::len_utf8),
    }
}

// Close enough for tiles: a miss shifts one line by a cell, and the next
// tile is drawn at an absolute position over any overflow.
fn char_width(c: char) -> usize {
    match c as u32 {
        0x0300..=0x036f | 0x200b..=0x200f | 0xfe00..=0xfe0f => 0,
        0x1100..=0x115f
        | 0x2e80..=0xa4cf
        | 0xac00..=0xd7a3
        | 0xf900..=0xfaff
        | 0xfe30..=0xfe4f
        | 0xff00..=0xff60
        | 0xffe0..=0xffe6
        | 0x1f300..=0x1f64f
        | 0x1f680..=0x1f6ff
        | 0x1f900..=0x1f9ff
        | 0x20000..=0x3fffd => 2,
        _ => 1,
    }
}

/// Keeps escape sequences, drops other control characters, and stops before
/// the first character that would pass `width` columns.
fn clip(line: &str, width: usize) -> String {
    let mut out = String::new();
    let mut used = 0;
    let mut index = 0;
    while let Some(c) = line[index..].chars().next() {
        if c == '\x1b' {
            let end = escape_end(line, index);
            out.push_str(&line[index..end]);
            index = end;
            continue;
        }
        index += c.len_utf8();
        if c.is_control() {
            continue;
        }
        used += char_width(c);
        if used > width {
            break;
        }
        out.push(c);
    }
    out
}

fn visible(line: &str) -> String {
    let mut out = String::new();
    let mut index = 0;
    while let Some(c) = line[index..].chars().next() {
        if c == '\x1b' {
            index = escape_end(line, index);
            continue;
        }
        index += c.len_utf8();
        if !c.is_control() {
            out.push(c);
        }
    }
    out
}

#[derive(Debug, PartialEq, Eq)]
enum Key {
    Next,
    Prev,
    Down,
    Up,
    NextWaiting,
    Zoom,
    Jump,
    Quit,
}

fn keys(input: &[u8]) -> Vec<Key> {
    let mut keys = Vec::new();
    let mut i = 0;
    while i < input.len() {
        let rest = &input[i..];
        let (key, len) = match rest {
            [0x1b, b'[', code, ..] => (
                match code {
                    b'C' => Some(Key::Next),
                    b'D' | b'Z' => Some(Key::Prev),
                    b'B' => Some(Key::Down),
                    b'A' => Some(Key::Up),
                    _ => None,
                },
                3,
            ),
            [0x1b, ..] => (Some(Key::Quit), 1),
            [b, ..] => (
                match b {
                    b'\t' | b'l' => Some(Key::Next),
                    b'h' => Some(Key::Prev),
                    b'j' => Some(Key::Down),
                    b'k' => Some(Key::Up),
                    b'n' => Some(Key::NextWaiting),
                    b'z' | b' ' => Some(Key::Zoom),
                    b'\r' | b'\n' => Some(Key::Jump),
                    b'q' | 0x03 => Some(Key::Quit),
                    _ => None,
                },
                1,
            ),
            [] => break,
        };
        keys.extend(key);
        i += len;
    }
    keys
}

struct Wall {
    /// A grid from `--wall COLSxROWS`; otherwise it follows the terminal.
    fixed: Option<Grid>,
    /// Whose agents these are: the current client, else yours.
    context: String,
    grid: Grid,
    tiles: Vec<Tile>,
    focus: usize,
    zoom: bool,
}

impl Wall {
    fn focused(&self) -> Option<&Tile> {
        self.tiles.get(self.focus)
    }

    fn refocus(&mut self, pane: Option<String>) {
        self.focus = pane
            .and_then(|pane| self.tiles.iter().position(|t| t.pane == pane))
            .unwrap_or(self.focus)
            .min(self.tiles.len().saturating_sub(1));
    }

    fn step(&mut self, key: &Key) {
        let count = self.tiles.len();
        if count == 0 {
            return;
        }
        let cols = self.grid.cols;
        self.focus = match key {
            Key::Next => (self.focus + 1) % count,
            Key::Prev => (self.focus + count - 1) % count,
            Key::Down if self.focus + cols < count => self.focus + cols,
            Key::Up if self.focus >= cols => self.focus - cols,
            Key::NextWaiting => (1..=count)
                .map(|offset| (self.focus + offset) % count)
                .find(|&i| !self.tiles[i].working)
                .unwrap_or(self.focus),
            _ => self.focus,
        };
    }

    fn fit(&mut self, width: usize, height: usize) {
        self.grid = self.fixed.unwrap_or_else(|| {
            layout(
                self.tiles.len(),
                width.saturating_sub(1),
                height.saturating_sub(1),
            )
        });
    }

    // Every header is a bar, so rows stay apart; the focused one is pink.
    fn header(&self, index: usize, width: usize) -> String {
        let tile = &self.tiles[index];
        let text = clip(
            &format!(" {} · {}", tile.label, tile.agent),
            width.saturating_sub(2),
        );
        let pad = " ".repeat(width.saturating_sub(1 + visible(&text).chars().count()));
        if index == self.focus {
            format!("{FOCUS}●{text}{pad}{RESET}")
        } else {
            let dot = if tile.working { WORKING } else { WAITING };
            format!("{BAR}{dot}●\x1b[38;2;205;214;244m{text}{pad}{RESET}")
        }
    }

    fn footer(&self, width: usize) -> String {
        let working = self.tiles.iter().filter(|t| t.working).count();
        let per_page = self.grid.cols * self.grid.rows;
        let pages = self.tiles.len().div_ceil(per_page).max(1);
        let page = if pages > 1 {
            format!(" · page {}/{pages}", self.focus / per_page + 1)
        } else {
            String::new()
        };
        let text = format!(
            " ▦ {} · {} agents · {WORKING}●{FAINT} {working} working · {WAITING}●{FAINT} {} waiting{page}   tab/hjkl move · n next waiting · z zoom · enter jump · q quit",
            self.context,
            self.tiles.len(),
            self.tiles.len() - working,
        );
        format!("{FAINT}{}", clip(&text, width))
    }

    fn render(&mut self, width: usize, height: usize) -> String {
        self.fit(width, height);
        let mut out = String::from("\x1b[?2026h");
        let body_height = height.saturating_sub(1);
        let line = |out: &mut String, y: usize, x: usize, text: &str| {
            // Clear first: clearing after text that reaches the last column
            // erases that column, since the cursor stays on it.
            out.push_str(&format!("\x1b[{};{}H\x1b[K{text}{END_LINE}", y + 1, x + 1));
        };
        if self.tiles.is_empty() {
            for y in 0..body_height {
                let text = if y == body_height / 2 {
                    format!("{FAINT}  no claude or codex panes are running")
                } else {
                    String::new()
                };
                line(&mut out, y, 0, &text);
            }
        } else if self.zoom {
            let index = self.focus;
            line(&mut out, 0, 0, &self.header(index, width));
            let lines = &self.tiles[index].lines;
            let rows = body_height.saturating_sub(1);
            let start = lines.len().saturating_sub(rows);
            for y in 0..rows {
                let text = lines
                    .get(start + y)
                    .map_or(String::new(), |l| clip(l, width));
                line(&mut out, y + 1, 0, &text);
            }
        } else {
            let Grid { cols, rows } = self.grid;
            let per_page = cols * rows;
            let first = self.focus / per_page * per_page;
            // The last column is the right border.
            let span = width.saturating_sub(1);
            let x = |col: usize| col * span / cols;
            let top = |row: usize| row * body_height / rows;
            for y in 0..body_height {
                let row = (0..rows).rfind(|&r| top(r) <= y).unwrap_or(0);
                let (dy, tile_height) = (y - top(row), top(row + 1) - top(row));
                // Left to right, each clearing to the edge: every tile erases
                // whatever the one before it spilled past its border.
                for col in 0..=cols {
                    let index = first + row * cols + col;
                    // A border is pink on either side of the focused tile.
                    let lit = index == self.focus || (col > 0 && index - 1 == self.focus);
                    let gutter = if lit { PINK } else { FAINT };
                    let text = match self.tiles.get(index).filter(|_| col < cols) {
                        None => String::new(),
                        Some(_) if dy == 0 => self.header(index, x(col + 1) - x(col) - 1),
                        Some(tile) => {
                            let inner = x(col + 1) - x(col) - 1;
                            let lines = &tile.lines[..tile.transcript];
                            let start = lines.len().saturating_sub(tile_height - 1);
                            match lines.get(start + dy - 1) {
                                None => String::new(),
                                Some(l) if index == self.focus => clip(l, inner),
                                // Unfocused tiles are muted so the focused one stands out.
                                Some(l) => format!("{MUTED}{}", clip(&visible(l), inner)),
                            }
                        }
                    };
                    line(&mut out, y, x(col), &format!("{gutter}│{RESET}{text}"));
                }
            }
        }
        line(&mut out, body_height, 0, &self.footer(width));
        out.push_str("\x1b[?2026l");
        out
    }
}

fn stty(args: &[&str]) -> Result<String> {
    let output = Command::new("stty")
        .args(args)
        .stdin(Stdio::inherit())
        .stderr(Stdio::inherit())
        .output()?;
    if !output.status.success() {
        return Err("stty failed: the wall needs a terminal".into());
    }
    Ok(String::from_utf8_lossy(&output.stdout).trim().to_owned())
}

fn size() -> Result<(usize, usize)> {
    let size = stty(&["size"])?;
    let (rows, cols) = size.split_once(' ').ok_or("unreadable terminal size")?;
    Ok((cols.parse()?, rows.parse()?))
}

/// Raw keys, the alternate screen, and no autowrap for as long as it lives.
struct Screen {
    saved: String,
}

impl Screen {
    fn enter() -> Result<Self> {
        let saved = stty(&["-g"])?;
        // A read returns on the first key or after 0.2s, which paces the loop
        // without a reader thread competing for stdin.
        stty(&[
            "-icanon", "-echo", "-isig", "-ixon", "min", "0", "time", "2",
        ])?;
        print!("\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[2J");
        io::stdout().flush()?;
        Ok(Self { saved })
    }
}

impl Drop for Screen {
    fn drop(&mut self) {
        print!("\x1b[?7h\x1b[?25h\x1b[?1049l");
        let _ = io::stdout().flush();
        let _ = stty(&[&self.saved]);
    }
}

pub fn run(config: &Config, fixed: Option<Grid>) -> Result<()> {
    let in_tmux = std::env::var_os("TMUX").is_some();
    let mut wall = Wall {
        fixed,
        context: config.workspace.clone(),
        grid: Grid { cols: 1, rows: 1 },
        tiles: discover(config)?,
        focus: 0,
        zoom: false,
    };
    // Start on the pane you came from, so Tab then Enter is "the next agent".
    if in_tmux {
        let current = tmux()
            .args(["display-message", "-p", "#{pane_id}"])
            .stderr(Stdio::null())
            .output()?;
        wall.refocus(Some(
            String::from_utf8_lossy(&current.stdout).trim().to_owned(),
        ));
    }
    let screen = Screen::enter()?;
    let mut stdin = io::stdin().lock();
    let mut buffer = [0u8; 64];
    let mut next_tick = Instant::now();
    let mut ticks = 0u32;
    let mut stale = false;
    crate::queue_refresh();
    loop {
        if Instant::now() >= next_tick {
            if stale || (ticks > 0 && ticks.is_multiple_of(DISCOVER_EVERY)) {
                crate::queue_refresh();
                let focused = wall.focused().map(|t| t.pane.clone());
                let mut tiles = discover(config)?;
                // Keep the last frame so a rediscovery never blanks the wall.
                for tile in &mut tiles {
                    if let Some(old) = wall.tiles.iter_mut().find(|t| t.pane == tile.pane) {
                        tile.lines = std::mem::take(&mut old.lines);
                        tile.transcript = old.transcript;
                        tile.working = old.working;
                    }
                }
                wall.tiles = tiles;
                wall.refocus(focused);
            }
            stale = !capture(&mut wall.tiles)?;
            ticks += 1;
            next_tick = Instant::now() + TICK;
            let (width, height) = size()?;
            print!("{}", wall.render(width, height));
            io::stdout().flush()?;
        }
        let read = stdin.read(&mut buffer)?;
        if read == 0 {
            continue;
        }
        for key in keys(&buffer[..read]) {
            match key {
                Key::Quit => return Ok(()),
                Key::Zoom => wall.zoom = !wall.zoom,
                Key::Jump => {
                    let Some(pane) = wall.focused().map(|t| t.pane.clone()) else {
                        continue;
                    };
                    drop(screen);
                    let mut command = tmux();
                    // A pane target also selects its window and pane.
                    if in_tmux {
                        command.args(["switch-client", "-t", &pane]).status()?;
                        return Ok(());
                    }
                    return Err(command.args(["attach-session", "-t", &pane]).exec().into());
                }
                key => wall.step(&key),
            }
        }
        let (width, height) = size()?;
        print!("{}", wall.render(width, height));
        io::stdout().flush()?;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn tile(pane: &str, working: bool) -> Tile {
        Tile {
            pane: pane.into(),
            label: pane.into(),
            agent: "claude".into(),
            lines: Vec::new(),
            transcript: 0,
            working,
        }
    }

    #[test]
    fn parses_grids_within_bounds() {
        assert_eq!(parse_grid("8x6"), Some(Grid { cols: 8, rows: 6 }));
        assert_eq!(parse_grid("3X2"), Some(Grid { cols: 3, rows: 2 }));
        for bad in ["0x6", "8x17", "8", "axb", ""] {
            assert_eq!(parse_grid(bad), None, "{bad}");
        }
    }

    #[test]
    fn splits_one_capture_per_pane_and_reads_working_state() {
        let mut tiles = vec![tile("%1", true), tile("%2", false), tile("%3", false)];
        tiles[2].lines = vec!["last frame".into()];
        let output = format!(
            "{MARK}0\n> prompt\n\x1b[2m? for shortcuts\x1b[0m\n\n   \n{MARK}1\n✻ Thinking… (3s · \x1b[1mesc\x1b[0m to interrupt)\n"
        );
        assert!(!apply_capture(&mut tiles, &output));
        assert_eq!(tiles[0].lines.len(), 2, "trailing blank lines trimmed");
        assert!(!tiles[0].working);
        assert!(tiles[1].working);
        assert_eq!(
            tiles[2].lines,
            ["last frame"],
            "missing capture keeps its frame"
        );
    }

    #[test]
    fn autosizes_to_fill_the_screen_and_pages_past_the_minimum_tile() {
        let grid = |cols, rows| Grid { cols, rows };
        assert_eq!(layout(1, 200, 40), grid(1, 1));
        assert_eq!(layout(2, 200, 40), grid(2, 1), "side by side, not stacked");
        assert_eq!(layout(27, 199, 34), grid(7, 4), "no empty rows");
        assert_eq!(layout(27, 239, 59), grid(6, 5));
        assert_eq!(
            layout(200, 96, 20),
            grid(4, 4),
            "capped at the smallest readable tile"
        );
        assert_eq!(layout(5, 10, 3), grid(1, 1));
    }

    #[test]
    fn tiles_stop_above_the_agents_input_box() {
        let lines = |text: &str| text.lines().map(String::from).collect::<Vec<_>>();
        let claude = lines(
            "  reply\n\n✻ Worked for 16s\n   new task? /clear\n────\n❯ merge it\n────\n  status\n  ⏵⏵ bypass",
        );
        assert_eq!(transcript_end(&claude), 4);
        let codex = lines("• done\n\n  ⠈   ⠁\n›⠁Ask Codex to do anything\n    ⠠\n  gpt-5 medium");
        assert_eq!(transcript_end(&codex), 1);
        let permission = lines("────\n Do you want to proceed?\n ❯ 1. Yes\n   2. No");
        assert_eq!(
            transcript_end(&permission),
            4,
            "a prompt the agent is waiting on stays"
        );
        let approval = lines("Run tests?\n› 1. Yes\n  2. No");
        assert_eq!(transcript_end(&approval), 3);
        let pi = lines("answer\n\n── ⠋ Working... ────\nnext ask\n──────────\n~/w (main)\n↑1k ↓2k");
        assert_eq!(pi_transcript_end(&pi), 1);
        let idle = lines("answer\n──────────\n\n──────────\n~/w (main)");
        assert_eq!(pi_transcript_end(&idle), 1);
    }

    #[test]
    fn spots_a_running_turn_but_not_a_finished_one_or_prose() {
        for working in [
            "✶ Cerebrating… (5m 2s · ↓ 30.5k tokens · thought for 5s)",
            "  · Sautéing… (12s)",
            "• Working (3s • esc to interrupt)",
            "── ⠹ Working... ──────────",
        ] {
            assert!(is_working(working), "{working}");
        }
        for idle in [
            "✻ Churned for 10s · done 3:34",
            "❯ wait… (maybe) later",
            "- Loading… (1 file) in two words",
            "a… (1)",
            "──────────",
            "── ↑ 3 more ──",
            "",
        ] {
            assert!(!is_working(idle), "{idle}");
        }
    }

    #[test]
    fn clips_by_visible_width_and_keeps_escapes() {
        assert_eq!(clip("\x1b[31mhello\x1b[0m world", 3), "\x1b[31mhel");
        assert_eq!(clip("ab\x07cd", 3), "abc");
        assert_eq!(clip("日本語", 5), "日本");
        assert_eq!(
            clip("\x1b]8;;https://x\x1b\\link\x1b]8;;\x1b\\", 9),
            "\x1b]8;;https://x\x1b\\link\x1b]8;;\x1b\\"
        );
        assert_eq!(visible("\x1b[1;32m● ok\x1b[0m"), "● ok");
    }

    #[test]
    fn reads_keys_and_escape_sequences_from_one_chunk() {
        assert_eq!(
            keys(b"\t\x1b[Zj\x1b[Aqz\r"),
            [
                Key::Next,
                Key::Prev,
                Key::Down,
                Key::Up,
                Key::Quit,
                Key::Zoom,
                Key::Jump
            ]
        );
        assert_eq!(keys(b"\x1b"), [Key::Quit]);
        assert_eq!(keys(b"x\x1b[H"), []);
    }

    #[test]
    fn focus_wraps_moves_by_rows_and_finds_waiting_agents() {
        let mut wall = Wall {
            fixed: None,
            context: "hev".into(),
            grid: Grid { cols: 2, rows: 2 },
            tiles: vec![
                tile("%1", true),
                tile("%2", true),
                tile("%3", false),
                tile("%4", true),
                tile("%5", true),
            ],
            focus: 0,
            zoom: false,
        };
        wall.step(&Key::Prev);
        assert_eq!(wall.focus, 4);
        wall.step(&Key::Down);
        assert_eq!(wall.focus, 4, "no row below");
        wall.step(&Key::Up);
        assert_eq!(wall.focus, 2);
        wall.step(&Key::NextWaiting);
        assert_eq!(wall.focus, 2, "the only waiting agent");
        wall.tiles.truncate(2);
        wall.refocus(Some("%9".into()));
        assert_eq!(wall.focus, 1, "a vanished focus clamps into range");
    }

    #[test]
    fn renders_later_pages_and_an_empty_wall() {
        let mut wall = Wall {
            fixed: Some(Grid { cols: 2, rows: 1 }),
            context: "hev".into(),
            grid: Grid { cols: 1, rows: 1 },
            tiles: vec![tile("%1", false), tile("%2", true), tile("%3", false)],
            focus: 2,
            zoom: false,
        };
        let frame = wall.render(80, 6);
        assert!(frame.contains("page 2/2"));
        assert!(frame.contains("hev · 3 agents"));
        assert!(frame.contains("%3 · claude"));
        assert!(!frame.contains("%1 · claude"));
        wall.tiles.clear();
        assert!(wall.render(80, 6).contains("no claude or codex panes"));
    }
}
