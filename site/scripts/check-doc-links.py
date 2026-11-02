"""Check every built /docs link, including fragment targets. Run after pnpm build."""
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlsplit


class Page(HTMLParser):
    def __init__(self, path):
        super().__init__()
        self.ids = set()
        self.links = []
        self.feed(path.read_text())

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if 'id' in attrs:
            self.ids.add(attrs['id'])
        if tag == 'a' and 'href' in attrs:
            self.links.append(attrs['href'])


root = Path(__file__).resolve().parents[1] / 'dist'
pages = {path: Page(path) for path in root.rglob('*.html')}
assert pages, 'No built pages; run pnpm build first'
errors = []
count = 0
for source, page in pages.items():
    for link in page.links:
        url = urlsplit(link)
        if url.scheme or url.netloc or not (url.path == '/docs' or url.path.startswith('/docs/')):
            continue
        count += 1
        target = root / unquote(url.path).lstrip('/')
        if target.suffix != '.html':
            target = target / 'index.html'
        if target not in pages:
            errors.append(f'{source.relative_to(root)}: missing {link}')
        elif url.fragment and unquote(url.fragment) not in pages[target].ids:
            errors.append(f'{source.relative_to(root)}: missing fragment {link}')
assert not errors, '\n'.join(errors)
print(f'Checked {count} /docs links and fragments across {len(pages)} built pages')
