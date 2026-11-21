pub const RESET: &str = "\x1b[0m";

pub struct Item {
    key: String,
    label: String,
}

impl Item {
    pub fn new(key: &str, label: &str) -> Self {
        Self {
            key: key.to_owned(),
            label: display_text(label),
        }
    }

    pub fn color(mut self, color: &str) -> Self {
        let [r, g, b] = [1, 3, 5].map(|i| u8::from_str_radix(&color[i..i + 2], 16).unwrap());
        self.label = format!("\x1b[38;2;{r};{g};{b}m{}{RESET}", self.label);
        self
    }
}

pub fn render(items: &[Item], keyed: bool) -> String {
    let mut output = String::new();
    for item in items {
        if keyed {
            output.push_str(&item.key);
            output.push('\t');
        }
        output.push_str(&item.label);
        output.push(if keyed { '\0' } else { '\n' });
    }
    output
}

pub fn display_text(value: &str) -> String {
    value
        .chars()
        .map(|c| if c.is_control() { ' ' } else { c })
        .collect()
}
