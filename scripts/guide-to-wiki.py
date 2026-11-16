#!/usr/bin/env python3
"""Publishes docs/guide/ to the repository's GitHub wiki.

Each guide page becomes a wiki page named after its heading ("Your first
project" is Your-first-project.md, which the wiki shows as "Your first
project"), the guide's README becomes Home, and its contents list becomes the
sidebar. Links between guide pages (`tasks.md#subtasks`) become links between
wiki pages (`Tasks#subtasks`). The wiki is a git repository of its own; this
clones it, replaces every page with the guide's, and pushes, so the wiki is
always the guide as it is in this checkout. Pages made in the wiki by hand are
removed. Pictures (docs/guide/images, made by `make shots`) go along, and a
picture written in the guide as `![...](images/<name>-light.webp)` shows its
`-dark` twin to readers in dark mode. Needs Python 3 and git with access to the
repository.

    python3 scripts/guide-to-wiki.py            # publish
    python3 scripts/guide-to-wiki.py --dry-run  # write the pages to a folder, push nothing
"""

import argparse
import html
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
GUIDE = ROOT / "docs" / "guide"
WIKI = "https://github.com/locodarwin/mila.wiki.git"

IMAGES = GUIDE / "images"
# A picture alone on its line, in its light version.
PICTURE = re.compile(r"^!\[([^\]]*)\]\(images/([a-z0-9-]+)-light\.webp\)$", re.M)
LINK = re.compile(r"\]\(([a-z0-9-]+)\.md(#[^)]*)?\)")
OUT = re.compile(r"\]\(\.\./([^)\s]+)\)")
BLOB = "https://github.com/locodarwin/mila/blob/main"


def title_of(path: Path) -> str:
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.startswith("# "):
            return line[2:].strip()
    sys.exit(f"{path.name} has no '# ' heading to name its wiki page by")


def page_name(title: str) -> str:
    # The wiki shows hyphens as spaces; characters it cannot hold in a file
    # name go.
    return re.sub(r"[\\/:*?\"<>|#%,]", "", title).strip().replace(" ", "-")


def build(out: Path) -> list[str]:
    names = {}
    for path in sorted(GUIDE.glob("*.md")):
        names[path.stem] = "Home" if path.stem == "README" else page_name(title_of(path))
    if len(set(names.values())) != len(names):
        sys.exit("two guide pages have the same heading, so the same wiki page name")

    def relink(text: str) -> str:
        def to(m: re.Match) -> str:
            stem, anchor = m.group(1), m.group(2) or ""
            if stem not in names:
                sys.exit(f"a link to {stem}.md, which is not a guide page")
            return f"]({names[stem]}{anchor})"

        # A link out of the guide (../self-hosting.md) goes to the file on GitHub.
        text = OUT.sub(lambda m: f"]({BLOB}/docs/{m.group(1)})", text)
        return LINK.sub(to, PICTURE.sub(picture, text))

    def picture(m: re.Match) -> str:
        alt, name = html.escape(m.group(1)), m.group(2)
        for scheme in ("light", "dark"):
            if not (IMAGES / f"{name}-{scheme}.webp").exists():
                sys.exit(f"images/{name}-{scheme}.webp is named in the guide but not there; run make shots")
        return (
            f'<picture><source media="(prefers-color-scheme: dark)" srcset="images/{name}-dark.webp">'
            f'<img alt="{alt}" src="images/{name}-light.webp"></picture>'
        )

    written = []
    for stem, name in names.items():
        lines = (GUIDE / f"{stem}.md").read_text(encoding="utf-8").splitlines()
        # The wiki shows the page's name as its title, so the heading would
        # say it twice.
        if lines and lines[0].startswith("# "):
            lines = lines[1:]
            while lines and not lines[0].strip():
                lines = lines[1:]
        (out / f"{name}.md").write_text(relink("\n".join(lines)) + "\n", encoding="utf-8")
        written.append(name)

    if IMAGES.is_dir():
        shutil.copytree(IMAGES, out / "images", dirs_exist_ok=True)

    # The sidebar: the README's contents, from its "## Contents" heading on.
    readme = (GUIDE / "README.md").read_text(encoding="utf-8")
    at = readme.find("## Contents")
    if at < 0:
        sys.exit("README.md has no '## Contents' section for the sidebar")
    contents = readme[at:].split("\n", 1)[1]
    # Titles only: a numbered line "1. [Title](page.md): what it holds" is
    # "- [Title](page)" here, so the sidebar stays a short list.
    contents = re.sub(r"(?m)^\d+\. (\[[^\]]+\]\([^)]+\)).*$", r"- \1", contents)
    sidebar = "**[Using Mila](Home)**\n" + relink(contents)
    (out / "_Sidebar.md").write_text(sidebar.rstrip() + "\n", encoding="utf-8")
    (out / "_Footer.md").write_text(
        "This wiki is made from `docs/guide/` in the repository by `scripts/guide-to-wiki.py`. "
        "Change the guide there, not here: the next publish replaces every page.\n",
        encoding="utf-8",
    )
    return written


def git(*args: str, cwd: Path) -> str:
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True).stdout


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--dry-run", action="store_true", help="write the pages to a folder and push nothing")
    args = ap.parse_args()

    if args.dry_run:
        out = Path(tempfile.mkdtemp(prefix="mila-wiki-"))
        pages = build(out)
        print(f"{len(pages)} pages and the sidebar written to {out}")
        return

    with tempfile.TemporaryDirectory(prefix="mila-wiki-") as tmp:
        repo = Path(tmp) / "wiki"
        try:
            subprocess.run(["git", "clone", "-q", WIKI, str(repo)], check=True)
        except subprocess.CalledProcessError:
            sys.exit("Cannot clone the wiki. It exists only once its first page is made on GitHub.")
        for old in repo.glob("*.md"):
            old.unlink()
        shutil.rmtree(repo / "images", ignore_errors=True)
        pages = build(repo)
        commit = git("rev-parse", "--short", "HEAD", cwd=ROOT).strip()
        git("add", "-A", ".", cwd=repo)
        if not git("status", "--porcelain", cwd=repo).strip():
            print("The wiki is already the guide as it is here.")
            return
        dirty = git("status", "--porcelain", "docs/guide", cwd=ROOT).strip()
        said = f"The guide as of {commit}" + (", with changes not yet committed" if dirty else "")
        git("commit", "-q", "-m", said, cwd=repo)
        git("push", "-q", "origin", "HEAD", cwd=repo)
        print(f"Published {len(pages)} pages and the sidebar from {commit}.")


if __name__ == "__main__":
    main()
