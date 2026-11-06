#!/usr/bin/env python3
"""
Purpose: Check local documentation file and fragment links without network access.
Used by: Phase 1A evidence verification and the repository check script.
Responsibilities: Inspect tracked HTML/Markdown and validate relative destinations and anchors.
Important: Skip external URLs and ignored/generated files; never fetch remote links or credential sources.
"""
import html
from html.parser import HTMLParser
from pathlib import Path
import re
import subprocess
import sys
import tempfile
from urllib.parse import unquote, urlsplit


class Links(HTMLParser):
    def __init__(self):
        super().__init__()
        self.links = []
        self.anchors = set()

    def handle_starttag(self, tag, attrs):
        values = dict(attrs)
        self.anchors.update(value for key, value in attrs if key == 'id' or (tag == 'a' and key == 'name'))
        for key in ('href', 'src'):
            if values.get(key):
                self.links.append(values[key])


def contents(path):
    source = path.read_text()
    parsed = Links()
    parsed.feed(source)
    if path.suffix == '.md':
        # Ignore fenced examples, which may contain illustrative Markdown links.
        source = re.sub(r'```.*?```', '', source, flags=re.S)
        parsed.links.extend(re.findall(r'!?\[[^\]]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)', source))
        for heading in re.findall(r'^#{1,6}\s+(.+?)\s*#*$', source, flags=re.M):
            text = re.sub(r'<[^>]*>', '', heading).lower()
            parsed.anchors.add(re.sub(r'[^\w\- ]', '', text).replace(' ', '-'))
    return parsed


def errors(path):
    result = []
    for link in contents(path).links:
        url = urlsplit(html.unescape(link))
        if url.scheme or url.netloc:
            continue
        relative = unquote(url.path)
        if path.parent.name == 'frontend' and path.name == 'index.html':
            relative = relative.lstrip('/')  # Vite's source entry is relative to its frontend root.
        target = path.parent / relative if relative else path
        if not target.exists():
            result.append(f'{path}: missing {link}')
        elif url.fragment and target.is_file() and target.suffix in ('.html', '.md'):
            if unquote(url.fragment) not in contents(target).anchors:
                result.append(f'{path}: missing fragment {link}')
    return result


def self_test():
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        page = root / 'index.html'
        page.write_text('<a href="guide.md#good-heading">ok</a><a href="https://example.com/no">external</a>')
        (root / 'guide.md').write_text('# Good heading\n[home](index.html)\n')
        assert not errors(page)
        page.write_text('<a href="guide.md#missing">bad</a><img src="absent.png">')
        assert len(errors(page)) == 2
    print('Documentation checker self-test passed')


if __name__ == '__main__':
    if '--self-test' in sys.argv:
        self_test()
    else:
        root = Path(__file__).resolve().parent.parent
        tracked = subprocess.check_output(['git', 'ls-files', '-z', '*.md', '*.html'], cwd=root).decode().split('\0')
        files = [root / item for item in tracked if item and (root / item).is_file()]
        failures = [failure for path in files for failure in errors(path)]
        print('\n'.join(failures) if failures else f'Checked local links in {len(files)} tracked documents')
        sys.exit(bool(failures))
