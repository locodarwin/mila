#!/usr/bin/env python3
"""Writes Mila sample workspaces as JSON-export zips, ready for Settings,
Your data, Import.

    python scripts/sample-data.py OUT_DIR [--today 2026-11-02] [--seed 7]
                                  [--set household business enterprise edge-cases]

Each zip becomes one new workspace. By default it writes three: a family's
household, a small business and a large company with several launches under
way. --set picks others; edge-cases holds every length limit and the odd
shapes a zip can hold. Dates are placed around --today (default: the real
today), so "overdue", "today" and "this week" have something in them on the
day the zips are made. Needs only Python 3.

Rows keep the same IDs for the same --seed, and "Keep the original IDs" refuses
IDs already on the instance: use another --seed to import a set there again.

An import keeps one person, whoever imports it: the exporter here becomes them,
along with their assignments, My Day, reminders and running timer. Comments and
time logged by anyone else are credited to them too, and other people's
assignments and private rows are left out (syncer/restore.go). Mentions stay as
chips with the name they were written with, and notify nobody.

Either choice of IDs works when importing.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import random
import re
import struct
import uuid
import zipfile
import zlib
from pathlib import Path

# The made-up exporter. An import turns this ID into the importer, so
# assignees, "user" field values, comments and private rows all become
# theirs.
EXPORTER = "0190a000-0000-7000-8000-00000000e4e0"

# The database version an export of this app says it came from (the last
# migration in server/internal/db/migrations).
SCHEMA = 29

STARTER = [("Backlog", "backlog"), ("To do", "todo"), ("In progress", "in-progress"),
           ("Done", "done"), ("Canceled", "canceled")]
PALETTE = ["red", "orange", "amber", "yellow", "lime", "green", "teal", "cyan",
           "blue", "indigo", "purple", "pink"]
DIGITS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
# Mixed into a project's or workspace's ID to make its wiki's main page's
# (server/internal/syncer/pages.go).
MAIN_PAGE_SPACE = "2f5c6c0e-8a9b-4c7e-9d3a-6b1e4f2a7c58"
# What separates a page's title from a section of it, in a reference's label
# (web/src/lib/mentions.ts).
SECTION_MARK = " › "
WEEKDAYS = ["MO", "TU", "WE", "TH", "FR", "SA", "SU"]
UUID = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
# In notes, page bodies and comments: {t:Task title} and {p:Page title} become
# links to them, {p:Page title#Heading} a link to a section, {@:Name} a mention,
# and {img:file.png} an image of an attached file. They may name something made
# later; write() fills them in and fails on any that names nothing.
TOKEN = re.compile(r"\{(t|p|@|img):([^{}\n]+)\}")


def sort_key(n: int) -> str:
    """The nth key of a list, in order, as sortkey.ts would accept it."""
    if n < 62:
        return "a" + DIGITS[n]
    n -= 62
    return "b" + DIGITS[n // 62] + DIGITS[n % 62]


def hash_id(s: str) -> str:
    b = bytearray(hashlib.sha256(s.encode()).digest()[:16])
    b[6] = b[6] & 0x0F | 0x80
    b[8] = b[8] & 0x3F | 0x80
    h = b.hex()
    return f"{h[:8]}-{h[8:12]}-{h[12:16]}-{h[16:20]}-{h[20:]}"


def pair_id(a: str, b: str) -> str:
    """The ID of a pair's row (task and label, task and person...), as the app makes it."""
    return hash_id(a + "/" + b)


def xor_id(a: str, b: str) -> str:
    """The two UUIDs XORed: how a wiki's main page's ID is made from its project's or workspace's."""
    x = bytes(p ^ q for p, q in zip(uuid.UUID(a).bytes, uuid.UUID(b).bytes))
    return str(uuid.UUID(bytes=x))


def next_task_id(after: str) -> str:
    return hash_id(after + "/next")


def iso(t: dt.datetime) -> str:
    return t.astimezone(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")


def hlc(t: dt.datetime) -> str:
    """A moment as a row's clock, as the server writes one (serverHLC in syncer/history.go)."""
    return f"{int(t.timestamp() * 1000):013d}-0000-server"


def slug_of(heading: str) -> str:
    """A heading as it goes in an address, after the "#" (slugOf in mentions.ts)."""
    return re.sub(r"[\W_]+", "-", heading.lower()).strip("-")


def headings_in(markdown: str) -> list[str]:
    """The headings of a page's Markdown, leaving out code blocks (headingsIn in mentions.ts)."""
    out, fenced = [], False
    for line in markdown.split("\n"):
        if re.match(r"\s*(```|~~~)", line):
            fenced = not fenced
        m = None if fenced else re.match(r"#{1,6}\s+(.+?)\s*#*\s*$", line)
        if m:
            out.append(re.sub(r"[*_`~]", "", re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", m.group(1))))
    return out


# Small files for attachments.

def png(width: int, height: int, color: tuple[int, int, int],
        boxes: list[tuple[int, int, int, int, tuple[int, int, int]]] = ()) -> bytes:
    """A picture of one color, with colored boxes (x, y, width, height, color) drawn on it."""
    rows = [bytearray(bytes(color) * width) for _ in range(height)]
    for x, y, w, h, c in boxes:
        for row in rows[y:y + h]:
            row[x * 3:(x + w) * 3] = bytes(c) * (min(x + w, width) - x)
    raw = b"".join(b"\x00" + bytes(r) for r in rows)
    def chunk(kind: bytes, data: bytes) -> bytes:
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b""))


def bars(values: list[float], color: tuple[int, int, int], width: int = 480, height: int = 240) -> bytes:
    """A bar chart as a picture."""
    most = max(values) or 1
    step = (width - 20) // len(values)
    boxes = [(10, height - 12, width - 20, 2, (120, 120, 130))]
    for i, v in enumerate(values):
        h = int((height - 40) * v / most)
        boxes.append((14 + i * step, height - 12 - h, max(step - 8, 2), h, color))
    return png(width, height, (250, 250, 252), boxes)


def pdf(text: str) -> bytes:
    stream = f"BT /F1 18 Tf 72 720 Td ({text}) Tj ET".encode()
    objs = [b"<< /Type /Catalog /Pages 2 0 R >>",
            b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
            b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
            b"<< /Length %d >>\nstream\n" % len(stream) + stream + b"\nendstream",
            b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"]
    out = bytearray(b"%PDF-1.4\n")
    offsets = []
    for i, o in enumerate(objs, 1):
        offsets.append(len(out))
        out += b"%d 0 obj\n" % i + o + b"\nendobj\n"
    xref = len(out)
    out += b"xref\n0 %d\n0000000000 65535 f \n" % (len(objs) + 1)
    for off in offsets:
        out += b"%010d 00000 n \n" % off
    out += b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(objs) + 1, xref)
    return bytes(out)


def csv_file(*lines: str) -> tuple[str, bytes]:
    return "text/csv", ("\n".join(lines) + "\n").encode()


def tpl_task(tid: str, title: str, **kw) -> dict:
    """A task as a template's snapshot holds it (web/src/lib/templates.ts). Its title must be one a task can have."""
    if title != title.strip():
        raise ValueError(f"{title!r}: a task's title has no spaces around it")
    t = dict(id=tid, title=title, parent=None, notes="", priority="none", status=None, category=None,
             start=None, due=None, due_time=None, recurrence=None, recurrence_mode="schedule", estimate=None,
             labels=[], values={}, checklist=[], waits=[])
    t.update(kw)
    return t


class Workspace:
    def __init__(self, name: str, slug: str, today: dt.date, now: dt.datetime, rng: random.Random,
                 people: list[tuple[str, str]] = (("Sample data", "owner"),)):
        """people are (name, role); the first is the exporter, whom an import makes the importer."""
        self.name = name
        self.slug = slug
        self.today = today
        self.now = now
        self.rng = rng
        self.rows: list[dict] = []
        self.activity: list[dict] = []
        self.versions: list[dict] = []
        self.files: dict[str, tuple[str, bytes]] = {}
        self.keys: dict[object, int] = {}
        self.project_statuses: dict[str, dict[str, str]] = {}
        self.status_category: dict[str, str] = {}
        self.task_project: dict[str, str | None] = {}
        # What tokens name: titles and keys to IDs, and IDs to what a link needs.
        self.task_names: dict[str, str] = {}
        self.task_titles: dict[str, str] = {}
        self.page_names: dict[str, str] = {}
        self.page_info: dict[str, tuple[str, str | None, str]] = {}
        self.file_names: dict[str, str] = {}
        self.timers = 0
        self.people: list[dict] = []
        self.person_ids: dict[str, str] = {}
        for i, (person, role) in enumerate(people):
            pid = EXPORTER if i == 0 else hash_id("person/" + person)
            first = person.split()[0]
            self.people.append({"id": pid, "name": person, "email": f"{first.lower()}@example.com", "role": role})
            self.person_ids[person] = self.person_ids[first] = pid
        self.ws_id = self.id()

    # helpers

    def id(self) -> str:
        return str(uuid.UUID(int=self.rng.getrandbits(128), version=4))

    def next_key(self, scope: object) -> str:
        n = self.keys.get(scope, 0)
        self.keys[scope] = n + 1
        return sort_key(n)

    def day(self, offset: int | None) -> str | None:
        return None if offset is None else (self.today + dt.timedelta(days=offset)).isoformat()

    def ago(self, days: float, hours: float = 0) -> dt.datetime:
        return self.now - dt.timedelta(days=days, hours=hours)

    def on(self, weekday: str, weeks: int = 0) -> int:
        """Days from today to the next such weekday (today counts), and whole weeks after it."""
        return (WEEKDAYS.index(weekday) - self.today.weekday()) % 7 + weeks * 7

    def who(self, person: str | None) -> str:
        return EXPORTER if person is None else self.person_ids[person]

    def row(self, entity: str, id: str, at: dt.datetime | None = None, **fields) -> str:
        """A row. Its clocks say when it was written, which is what the app calls a comment's or a task's date; at is that moment, a month ago if not given."""
        clock = hlc(at or self.ago(30))
        self.rows.append({"entity": entity, "id": id, "fields": fields, "clocks": {f: clock for f in fields}})
        return id

    def touched(self, id: str, at: dt.datetime, *fields: str) -> None:
        """Says that these fields of the row made last were written later."""
        row = self.rows[-1]
        assert row["id"] == id
        row["clocks"].update({f: hlc(at) for f in fields})

    def history(self, task: str, entity: str, entity_id: str, kind: str, at: dt.datetime,
                actor: str = EXPORTER, **fields):
        self.activity.append({"id": self.id(), "actorId": actor, "actorName": "", "entity": entity,
                              "entityId": entity_id, "kind": kind, "fields": fields, "at": iso(at), "taskId": task})

    # rows

    def project(self, name: str, *, icon: str = "", color: str = "blue", description: str = "",
                statuses: list[tuple[str, str]] | None = STARTER, archived_days: float | None = None,
                deleted_days: float | None = None) -> str:
        pid = self.id()
        self.row("project", pid, name=name, icon=icon, color=color, description=description,
                 sort_key=self.next_key("project"),
                 archived_at=iso(self.ago(archived_days)) if archived_days is not None else None,
                 deleted_at=iso(self.ago(deleted_days)) if deleted_days is not None else None)
        self.project_statuses[pid] = {}
        for sname, category in statuses or []:
            self.status(pid, sname, category)
        return pid

    def status(self, project: str, name: str, category: str, *, deleted: bool = False) -> str:
        sid = self.id()
        self.row("status", sid, project_id=project, name=name, category=category,
                 sort_key=self.next_key(("status", project)),
                 deleted_at=iso(self.ago(2)) if deleted else None)
        if not deleted:
            self.project_statuses[project][name] = sid
        self.status_category[sid] = category
        return sid

    def label(self, name: str, color: str, *, deleted: bool = False) -> str:
        return self.row("label", self.id(), name=name, color=color, sort_key=self.next_key("label"),
                        deleted_at=iso(self.ago(3)) if deleted else None)

    def field(self, name: str, type: str, *, project: str | None = None,
              options: list[tuple[str, str, str]] | None = None, deleted: bool = False) -> str:
        return self.row("custom_field", self.id(), project_id=project, name=name, type=type,
                        options=[{"id": i, "name": n, "color": c} for i, n, c in (options or [])],
                        sort_key=self.next_key(("field", project)),
                        deleted_at=iso(self.ago(1)) if deleted else None)

    def choices(self, name: str, names: list[str], *, multi: bool = False, project: str | None = None) -> tuple[str, dict[str, str]]:
        """A select or multi_select field whose choices take the palette's colors in turn; gives its ID and its choices' IDs by name."""
        ids = {n: slug_of(n) for n in names}
        fid = self.field(name, "multi_select" if multi else "select", project=project,
                         options=[(ids[n], n, PALETTE[(i * 5 + len(name)) % 12]) for i, n in enumerate(names)])
        return fid, ids

    def automation(self, name: str, rule: dict, *, project: str | None = None) -> str:
        """A rule, as an export carries it. An import turns it off, to be looked at first."""
        return self.row("automation", self.id(), project_id=project, name=name, enabled=True,
                        enabled_at=iso(self.now), rule=rule, made_by=EXPORTER, paused_why=None,
                        sort_key=self.next_key("automation"), deleted_at=None)

    def view(self, name: str, type: str, *, filter: dict | None = None, display: dict | None = None,
             project: str | None = None, private: bool = False, deleted_days: float | None = None) -> str:
        return self.row("saved_view", self.id(), project_id=project, owner_id=EXPORTER if private else None,
                        name=name, type=type, config={"filter": filter or {}}, display=display,
                        sort_key=self.next_key("view"),
                        deleted_at=iso(self.ago(deleted_days)) if deleted_days is not None else None)

    def main_page(self, project: str | None = None, body: str = "", name: str = "") -> str:
        """A wiki's main page: the project's, or the workspace's for None. name is what tokens call it."""
        pid = self.row("page", xor_id(project or self.ws_id, MAIN_PAGE_SPACE), project_id=project,
                       parent_id=None, title="", body=body, sort_key="a0", deleted_at=None)
        if name:
            self.page_names[name] = pid
            self.page_info[pid] = (name, project, body)
        return pid

    def page(self, title: str, parent: str, *, project: str | None = None, body: str = "",
             deleted_days: float | None = None, versions: list[tuple[str, float, str | None]] = ()) -> str:
        """A subpage. versions are earlier bodies, oldest first: (body, days ago, who), who "" for the first, with no editor."""
        pid = self.row("page", self.id(), project_id=project, parent_id=parent, title=title, body=body,
                       sort_key=self.next_key(("page", parent)),
                       deleted_at=iso(self.ago(deleted_days)) if deleted_days is not None else None)
        self.page_names[title] = pid
        self.page_info[pid] = (title, project, body)
        for old, days, editor in versions:
            self.version(pid, old, days, editor)
        return pid

    def page_thread(self, page: str, quote: str, comments: list[tuple[str, float, str | None]],
                    *, resolved_days: float | None = None) -> str:
        """Comments pinned to some words of a page, which must be in its body; comments are (body, days ago, who)."""
        first = self.ago(comments[0][1])
        tid = self.row("page_thread", self.id(), at=first, page_id=page, created_by=self.who(comments[0][2]),
                       quote=quote, anchor="", resolved_at=iso(self.ago(resolved_days)) if resolved_days is not None else None)
        for body, days, who in comments:
            self.row("page_comment", self.id(), at=self.ago(days), page_id=page, thread_id=tid, author_id=self.who(who),
                     body=body, edited_at=None)
        return tid

    def version(self, page: str, body: str, days: float, editor: str | None = None):
        """An earlier version of a page's body, for its history; editor "" for one with none."""
        self.versions.append({"id": self.id(), "pageId": page, "actorId": "" if editor == "" else self.who(editor),
                              "actorName": "", "body": body, "at": iso(self.ago(days))})

    def image_href(self, attachment: str) -> str:
        """The address an image of an attached file has in a page or notes."""
        return f"/api/v1/workspaces/{self.ws_id}/attachments/{attachment}/file"

    def page_file(self, page: str, filename: str, mime: str, data: bytes, *, deleted_days: float | None = None) -> str:
        """A file attached to a wiki page."""
        aid = self.row("page_attachment", self.id(), page_id=page, filename=filename, mime=mime, size=len(data),
                       storage_key="sample", uploaded_by=EXPORTER,
                       deleted_at=iso(self.ago(deleted_days)) if deleted_days is not None else None)
        self.files[aid] = (filename, data)
        self.file_names[filename] = aid
        return aid

    def task(self, title: str, *, project: str | None = None, status: str | None = None,
             parent: str | None = None, notes: str = "", priority: str = "none",
             start: int | None = None, due: int | None = None, time: str | None = None,
             done_days: float | None = None, deleted_days: float | None = None,
             repeat: str | None = None, mode: str = "schedule", series: str | None = None,
             id: str | None = None, created_days: float = 14, key: str | None = None,
             labels: list[str] = (), assigned: bool = False,
             checklist: list[tuple[str, bool]] = (), comments: list[tuple] = (),
             values: dict[str, object] | None = None, my_day: bool = False, my_day_on: int = 0,
             reminders: list[dict] = (), files: list[tuple] = (),
             changes: list[tuple[float, dict]] = (), estimate: int | None = None,
             logged: list[tuple] = (), timer: tuple[int, str] | None = None,
             waits: list[str] = ()) -> str:
        """A task and everything that hangs off it. status names a status of
        the project; a done or canceled one completes the task. key is another
        name for tokens to call it, for titles that repeat. comments are (body,
        days ago, who), who left out for the exporter; a body may start
        "(edited) " or "(deleted) ". changes are (days ago, fields) for its
        history. estimate is in minutes; logged is time logged on it, (day
        offset, minutes, note, who); timer is a running timer, (minutes since
        it started, note). files are (filename, mime, data), with days ago as a
        fourth for one deleted. waits are tasks it waits for."""
        tid = id or self.id()
        status_id = None
        if project is not None:
            statuses = self.project_statuses[project]
            if status is None:
                status = next((n for n, s in statuses.items() if self.status_category[s] == ("done" if done_days is not None else "todo")), None)
                status = status or next(iter(statuses), None)
            status_id = statuses.get(status) if status else None
            if status and status_id is None:
                raise ValueError(f"{title}: no status {status!r}")
            if status_id and self.status_category[status_id] in ("done", "canceled") and done_days is None:
                done_days = 1
        # When it was made and first went in progress, for Reports' trends:
        # a task made at the default time is spread over the six weeks
        # before, a done one is made before it was done, and one started
        # (in progress now, or done in a project, three in four of them)
        # went in progress a while before it was done. Worked out from the
        # title, so the random rows stay the same for the same seed.
        h = int(hashlib.sha256(title.encode()).hexdigest()[:8], 16)
        if created_days == 14:
            created_days = 14 + h % 36
        if done_days is not None and created_days <= done_days:
            created_days = done_days + 1 + h % 5
        created = self.ago(created_days)
        category = self.status_category[status_id] if status_id else None
        started_at = None
        if category == "in-progress" or (category == "done" and h % 4):
            took = (0.2, 0.5, 1, 2, 3, 5, 8, 13)[(h >> 4) % 8]
            ended = done_days if category == "done" else 0
            started_at = iso(self.ago(min(created_days, ended + took)))
        fields = dict(
            project_id=project, status_id=status_id, title=title, notes=notes,
            made_at=iso(created), started_at=started_at,
            sort_key=self.next_key(("task", project, parent)),
            completed_at=iso(self.ago(done_days)) if done_days is not None else None,
            deleted_at=iso(self.ago(deleted_days)) if deleted_days is not None else None,
            parent_id=parent, priority=priority, start_date=self.day(start), due_date=self.day(due),
            due_time=time, created_by=EXPORTER, recurrence=repeat, recurrence_mode=mode,
            series_id=series, estimate_minutes=estimate)
        self.row("task", tid, at=created, **fields)
        if started_at is not None:
            self.touched(tid, dt.datetime.fromisoformat(started_at.replace("Z", "+00:00")), "started_at")
        if done_days is not None:
            self.touched(tid, self.ago(done_days), "completed_at", "status_id")
        if deleted_days is not None:
            self.touched(tid, self.ago(deleted_days), "deleted_at")
        self.task_project[tid] = project
        self.task_titles[tid] = title
        self.task_names[key or title] = tid
        if parent:
            self.history(parent, "task", tid, "created", created, **fields)
        self.history(tid, "task", tid, "created", created,
                     **{k: v for k, v in fields.items() if k not in ("completed_at", "deleted_at", "started_at")})
        for days, change in changes:
            self.history(tid, "task", tid, "changed", self.ago(days), **change)
        if done_days is not None:
            self.history(tid, "task", tid, "changed", self.ago(done_days), completed_at=fields["completed_at"])
        if deleted_days is not None:
            self.history(tid, "task", tid, "deleted", self.ago(deleted_days), deleted_at=fields["deleted_at"])

        for lid in labels:
            rid = self.row("task_label", pair_id(tid, lid), at=created, task_id=tid, label_id=lid, deleted_at=None)
            self.history(tid, "task_label", rid, "created", created, task_id=tid, label_id=lid, deleted_at=None)
        if assigned:
            rid = self.row("task_assignee", pair_id(tid, EXPORTER), at=created, task_id=tid, user_id=EXPORTER, deleted_at=None)
            self.history(tid, "task_assignee", rid, "created", created, task_id=tid, user_id=EXPORTER, deleted_at=None)
        for i, (text, done) in enumerate(checklist):
            cid = self.row("checklist_item", self.id(), at=created, task_id=tid, text=text, done=done,
                           sort_key=sort_key(i), deleted_at=None)
            self.history(tid, "checklist_item", cid, "created", created, task_id=tid, text=text, done=False)
            if done:
                self.history(tid, "checklist_item", cid, "changed", self.ago(created_days / 2), done=True)
        for c in comments:
            self.comment(tid, c[0], c[1], c[2] if len(c) > 2 else None)
        for fid, value in (values or {}).items():
            self.row("custom_field_value", pair_id(tid, fid), at=created, task_id=tid, field_id=fid, value=value, deleted_at=None)
        if my_day:
            self.row("my_day_entry", pair_id(tid, EXPORTER), at=self.ago(max(-my_day_on, 0), 1), task_id=tid, user_id=EXPORTER,
                     day=self.day(my_day_on), sort_key=self.next_key("my_day"), deleted_at=None)
        for r in reminders:
            self.row("reminder", self.id(), at=created, task_id=tid, user_id=EXPORTER, deleted_at=None, **r)
        for entry in logged:
            self.log_time(tid, *entry)
        if timer:
            self.start_timer(tid, *timer)
        for f in files:
            self.attach(tid, f[0], f[1], f[2], deleted_days=f[3] if len(f) > 3 else None, created_days=created_days)
        for blocker in waits:
            self.depend(tid, blocker)
        return tid

    def comment(self, task: str, body: str, days: float, who: str | None = None) -> str:
        """A comment; "(edited) " or "(deleted) " at the start of the body says that happened to it."""
        author = self.who(who)
        edited = body.startswith("(edited) ")
        deleted = body.startswith("(deleted) ")
        body = body.removeprefix("(edited) ").removeprefix("(deleted) ")
        cid = self.row("comment", self.id(), at=self.ago(days), task_id=task, author_id=author, body=body,
                       edited_at=iso(self.ago(days / 2)) if edited else None,
                       deleted_at=iso(self.ago(days / 2)) if deleted else None)
        if edited:
            self.touched(cid, self.ago(days / 2), "body", "edited_at")
        if deleted:
            self.touched(cid, self.ago(days / 2), "deleted_at")
        self.history(task, "comment", cid, "created", self.ago(days), actor=author,
                     task_id=task, author_id=author, body=body)
        return cid

    def log_time(self, task: str, offset: int, minutes: int, note: str = "", who: str | None = None) -> str:
        """Time typed in on a task."""
        user = self.who(who)
        eid = self.row("time_entry", self.id(), at=self.ago(max(-offset, 0)), task_id=task, user_id=user, day=self.day(offset),
                       minutes=minutes, note=note, started_at=None, running=False, active_at=None, deleted_at=None)
        self.history(task, "time_entry", eid, "created", self.ago(-offset), actor=user, task_id=task, user_id=user,
                     day=self.day(offset), minutes=minutes, note=note)
        return eid

    def log_project_time(self, project: str, offset: int, minutes: int, note: str = "", who: str | None = None) -> str:
        """Time typed in on a project itself, not on one of its tasks: a meeting, planning."""
        user = self.who(who)
        return self.row("project_time_entry", self.id(), at=self.ago(max(-offset, 0)), project_id=project, user_id=user,
                        day=self.day(offset), minutes=minutes, note=note, started_at=None, running=False, active_at=None,
                        deleted_at=None)

    def start_timer(self, task: str, minutes_ago: int, note: str = "") -> str:
        """The exporter's running timer. A person has one at a time, so a workspace here has one."""
        self.timers += 1
        if self.timers > 1:
            raise ValueError("a person has one timer running at a time")
        started = self.now - dt.timedelta(minutes=minutes_ago)
        return self.row("time_entry", self.id(), at=started, task_id=task, user_id=EXPORTER, day=self.today.isoformat(), minutes=0,
                        note=note, started_at=iso(started), running=True, active_at=iso(self.now), deleted_at=None)

    def attach(self, task: str, filename: str, mime: str, data: bytes, *, deleted_days: float | None = None,
               created_days: float = 7) -> str:
        """A file on a task."""
        aid = self.row("attachment", self.id(), at=self.ago(created_days), task_id=task, filename=filename, mime=mime, size=len(data),
                       storage_key="sample", uploaded_by=EXPORTER,
                       deleted_at=iso(self.ago(deleted_days)) if deleted_days is not None else None)
        self.files[aid] = (filename, data)
        self.file_names[filename] = aid
        self.history(task, "attachment", aid, "created", self.ago(created_days), filename=filename)
        return aid

    def depend(self, task: str, blocker: str, created_days: float = 2) -> str:
        """Makes task wait for blocker (docs/PLAN.md section 3)."""
        rid = self.row("task_dependency", pair_id(task, blocker), at=self.ago(created_days), task_id=task, blocker_id=blocker, deleted_at=None)
        self.history(task, "task_dependency", rid, "created", self.ago(created_days),
                     task_id=task, blocker_id=blocker, deleted_at=None)
        return rid

    def taken_off(self, task: str, *, label: str, blocker: str, field: str, value: object) -> None:
        """What people took off a task again: a label, an assignee, a dependency, a field's value, a
        checklist item, a My Day pick, a reminder and an entry of logged time. None of it shows, and
        none of it comes back when the same pair is made again. label and field must not be on the
        task, and blocker must not be waited for by it."""
        at = iso(self.ago(1))
        for entity, rid, fields in [
                ("task_label", pair_id(task, label), dict(task_id=task, label_id=label)),
                ("task_assignee", pair_id(task, EXPORTER), dict(task_id=task, user_id=EXPORTER)),
                ("task_dependency", pair_id(task, blocker), dict(task_id=task, blocker_id=blocker)),
                ("checklist_item", self.id(), dict(task_id=task, text="Taken off the list again", done=False, sort_key=sort_key(99)))]:
            self.row(entity, rid, deleted_at=at, **fields)
            self.history(task, entity, rid, "created", self.ago(3), deleted_at=None, **fields)
            self.history(task, entity, rid, "deleted", self.ago(1), deleted_at=at)
        self.row("custom_field_value", pair_id(task, field), task_id=task, field_id=field, value=value, deleted_at=at)
        self.row("my_day_entry", pair_id(task, EXPORTER), task_id=task, user_id=EXPORTER, day=self.day(-1), sort_key=self.next_key("my_day"), deleted_at=at)
        self.row("reminder", self.id(), task_id=task, user_id=EXPORTER, deleted_at=at, **self.remind_at(1, "12:00"))
        rid = self.log_time(task, -2, 60, "Logged on the wrong task")
        next(r for r in self.rows if r["id"] == rid)["fields"]["deleted_at"] = at

    def repeating(self, title: str, rule: str, *, project: str | None = None, past_dues: list[int] = (),
                  mode: str = "schedule", **kw) -> str:
        """A repeating task with its history: copies finished on past_dues (day offsets), then the open one with kw, under the IDs the app gives them."""
        at = first = self.id()
        for i, day in enumerate(past_dues):
            self.task(title, project=project, id=at, due=day, done_days=max(-day, 0) + 0.2, repeat=rule, mode=mode,
                      series=first, key=f"{title} (finished {i + 1})", created_days=max(-day, 0) + 7,
                      labels=kw.get("labels", ()), checklist=[(c, True) for c, _ in kw.get("checklist", ())])
            at = next_task_id(at)
        return self.task(title, project=project, id=at, repeat=rule, mode=mode, series=first if past_dues else None, **kw)

    def remind_at(self, day: int, clock: str) -> dict:
        """A reminder at a set moment (UTC)."""
        h, m = map(int, clock.split(":"))
        at = dt.datetime.combine(self.today + dt.timedelta(days=day), dt.time(h, m), dt.timezone.utc)
        return {"remind_at": iso(at), "offset_minutes": None}

    def remind_before(self, due: int, clock: str, minutes: int) -> dict:
        """A reminder relative to the due moment, which follows the task."""
        r = self.remind_at(due, clock)
        at = dt.datetime.fromisoformat(r["remind_at"].replace("Z", "+00:00")) - dt.timedelta(minutes=minutes)
        return {"remind_at": iso(at), "offset_minutes": minutes}

    def template(self, kind: str, name: str, data: dict, days: float = 5, deleted_days: float | None = None) -> str:
        return self.row("template", self.id(), kind=kind, name=name, saved_by=EXPORTER, saved_at=iso(self.ago(days)),
                        sort_key=self.next_key("template"),
                        deleted_at=iso(self.ago(deleted_days)) if deleted_days is not None else None, data=data)

    def theme(self, name: str, *, base: str = "standard", hue: float | None = None, target: str = "AA",
              tokens: dict | None = None, light: dict | None = None, dark: dict | None = None,
              suggested_days: float | None = None, deleted_days: float | None = None, source: str | None = None) -> str:
        """A workspace theme: a base and the tokens it changes (web/src/lib/theme/document.ts). source is the publisher's own theme it copies."""
        tid = self.id()
        body = dict(mila="theme", version=1, id=tid, name=name, base=base, target=target,
                    accent={} if hue is None else {"hue": hue}, tokens=tokens or {}, light=light or {}, dark=dark or {})
        return self.row("workspace_theme", tid, name=name, body=body, source_id=source,
                        suggested_at=iso(self.ago(suggested_days)) if suggested_days is not None else None,
                        published_by=EXPORTER, published_at=iso(self.ago(9)), sort_key=self.next_key("theme"),
                        deleted_at=iso(self.ago(deleted_days)) if deleted_days is not None else None)

    # tokens

    def address(self, page: str) -> str:
        project = self.page_info[page][1]
        return f"/projects/{project}?tab=wiki&page={page}" if project else f"/wiki?page={page}"

    def fill(self, text: str) -> str:
        """Text with its tokens made into links, mentions and images."""
        def one(m: re.Match) -> str:
            kind, name = m.group(1), m.group(2)
            if kind == "@":
                person = next(p for p in self.people if p["id"] == self.person_ids[name])
                return f"@[{person['name']}](user:{person['id']})"
            if kind == "t":
                tid = name if UUID.match(name) else self.task_names[name]
                return f"[{self.task_titles[tid].replace('[', ' ').replace(']', ' ').strip()}](/?task={tid})"
            if kind == "img":
                return f"![{name}]({self.image_href(self.file_names[name])})"
            title, _, heading = name.partition("#")
            page = self.page_names[title]
            if not heading:
                return f"[{title}]({self.address(page)})"
            if heading not in headings_in(self.page_info[page][2]):
                raise KeyError(f"page {title!r} has no heading {heading!r}")
            return f"[{title}{SECTION_MARK}{heading}]({self.address(page)}#{slug_of(heading)})"
        return TOKEN.sub(one, text)

    def filled(self, v):
        if isinstance(v, str):
            return self.fill(v) if "{" in v else v
        if isinstance(v, list):
            return [self.filled(x) for x in v]
        if isinstance(v, dict):
            return {k: self.filled(x) for k, x in v.items()}
        return v

    def write(self, path: Path) -> dict:
        rows = [{**r, "fields": self.filled(r["fields"])} for r in self.rows]
        activity = [{**h, "fields": self.filled(h["fields"])} for h in self.activity]
        versions = [{**pv, "body": self.fill(pv["body"])} for pv in self.versions]
        counts: dict[str, int] = {}
        for r in rows:
            counts[r["entity"]] = counts.get(r["entity"], 0) + 1
        exporter = self.people[0]
        manifest = {
            "format": "mila-export", "version": 1, "schema": SCHEMA, "app": "sample-data",
            "exportedAt": iso(self.now),
            "exportedBy": {"id": exporter["id"], "name": exporter["name"], "email": exporter["email"]},
            "workspace": {"id": self.ws_id, "name": self.name},
            "counts": counts,
        }
        content = {"rows": rows, "activity": activity, "pageVersions": versions, "people": self.people}
        with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as z:
            z.writestr("manifest.json", json.dumps(manifest, indent=1, ensure_ascii=False))
            z.writestr("workspace.json", json.dumps(content, ensure_ascii=False))
            for aid, (filename, data) in self.files.items():
                z.writestr(f"attachments/{aid}/{filename}", data)
        return counts


# The workspaces.

RICH_NOTES = """## What this note is for

It uses **every kind of formatting** the editor reads, so you can see how each one looks.

- A bullet with *italic*, **bold** and ~~struck~~ text
- A bullet with `inline code` and a [web link](https://example.com/guide)
  - A nested bullet
    - Nested again

1. A numbered step
2. Another step

> A block quote. Use it for someone's own words.

```python
def total(prices):
    return sum(prices)
```

- [x] A checklist item that is done
- [ ] One that is not, with **bold** in it
  - [ ] And one nested under it

| Column | Right aligned | Centered |
| --- | ---: | :-: |
| Plain text | 42 | yes |
| `code` in a cell | 3.14 | *no* |

> [!NOTE]
> A note box.

> [!WARNING]
> A warning box, with a list:
>
> - one thing
> - another

---

### A smaller heading

A last paragraph after a horizontal rule, with a line break
right here."""

def household(today: dt.date, now: dt.datetime, rng: random.Random) -> Workspace:
    """A family running a home: small and personal, with one of everything an export carries."""
    w = Workspace("Castellan Household", "household", today, now, rng, [
        ("Nora Castellan", "owner"), ("Theo Castellan", "admin"), ("Jonah Castellan", "member"),
        ("Wren Castellan", "member"), ("Bea Castellan", "guest"), ("Gus Ferreira", "guest")])

    L = {n: w.label(n, c) for n, c in [
        ("errand", "green"), ("call", "cyan"), ("online", "blue"), ("kids", "yellow"), ("money", "lime"),
        ("weekend", "orange"), ("waiting", "amber"), ("grandma", "pink")]}
    w.label("old-idea (deleted)", "purple", deleted=True)

    # Fields every project has.
    cost = w.field("Cost", "number")
    link = w.field("Link", "url")
    vendor = w.field("Vendor", "text")
    receipt = w.field("Receipt kept", "checkbox")
    warranty = w.field("Warranty until", "date")
    lead = w.field("Who's on it", "user")
    room, ROOM = w.choices("Room", ["Kitchen", "Upstairs bath", "Downstairs bath", "Living room", "Bedrooms", "Garage", "Yard", "Whole house"])
    needs, NEEDS = w.choices("Needs", ["Tools", "Parts", "A helper", "A day off", "A permit"], multi=True)
    w.field("Paint code (retired)", "text", deleted=True)

    # Repairs.
    repairs = w.project("Home repairs", icon="lucide:wrench", color="teal",
                        description="Everything that drips, squeaks or came loose, from noticing it to fixing it.",
                        statuses=[("Noticed", "backlog"), ("To schedule", "todo"), ("Booked", "in-progress"),
                                  ("Waiting on parts", "in-progress"), ("Fixed", "done"), ("Living with it", "canceled")])
    w.status(repairs, "Argued about", "todo", deleted=True)
    faucet = w.task("Fix the dripping downstairs faucet", project=repairs, status="Waiting on parts", priority="medium", due=2,
                    labels=[L["errand"]], assigned=True, created_days=9, estimate=60,
                    values={cost: 24.99, room: ROOM["Downstairs bath"], needs: [NEEDS["Parts"], NEEDS["Tools"]],
                            link: "https://example.com/parts/cartridge-1225", vendor: "Hollis Hardware", lead: EXPORTER},
                    checklist=[("Turn off water under the sink", True), ("Find the cartridge model", True),
                               ("Buy cartridge", False), ("Replace cartridge", False), ("Check for leaks", False)],
                    files=[("faucet-model.png", "image/png", png(320, 200, (226, 240, 238), [(120, 30, 80, 20, (120, 130, 140)), (150, 50, 20, 100, (120, 130, 140))]))],
                    notes="The drip got worse after the cold snap. The model number is on the base.\n\n{img:faucet-model.png}\n\nSteps are in {p:Who to call#Plumbing} if this turns into a bigger job.",
                    comments=[("Hollis has the cartridge. {@:Theo} can you pick it up Saturday? Their page says they open at 8: https://example.com/hollis-hardware", 3),
                              ("Got it, on my list with {t:Buy filters for the furnace}.", 2, "Theo")])
    w.task("Tighten the loose porch railing", project=repairs, status="To schedule", priority="urgent", due=-3, assigned=True, labels=[L["weekend"]],
           values={room: ROOM["Yard"], needs: [NEEDS["Tools"], NEEDS["A helper"]], cost: 18},
           notes="Bea leans on it coming up the steps. Do this before she visits.",
           comments=[("{@:Bea} is here on the 14th, so this one comes first.", 4)], reminders=[w.remind_at(1, "16:00")],
           changes=[(5, {"priority": "urgent"}), (6, {"due_date": w.day(-3)})])
    fan = w.task("Replace the upstairs bath fan", project=repairs, status="Booked", priority="high", start=-1, due=w.on("TH"), time="09:00",
                 values={cost: 185, vendor: "Bright Spark Electric", room: ROOM["Upstairs bath"], needs: [NEEDS["A day off"]],
                         warranty: w.day(730), receipt: False, link: "https://example.com/bright-spark/quote-2291"},
                 reminders=[w.remind_before(w.on("TH"), "09:00", 120)], estimate=120,
                 comments=[("Electrician confirmed Thursday morning. Quote is on {t:Replace the upstairs bath fan}'s link.", 1)])
    w.task("Clear the attic for the fan vent", project=repairs, parent=fan, status="To schedule", due=w.on("WE"), estimate=45, logged=[(-1, 20, "Moved the holiday boxes", "Theo")])
    w.task("Buy filters for the furnace", project=repairs, parent=fan, status="Fixed", done_days=2, values={cost: 32.40, vendor: "Hollis Hardware", receipt: True})
    w.repeating("Replace the furnace filter", "FREQ=MONTHLY;INTERVAL=3", project=repairs, past_dues=[-120, -30], mode="completion", due=-1, priority="medium", status="To schedule",
                values={room: ROOM["Whole house"], cost: 16.20}, reminders=[w.remind_at(1, "18:00")], notes="16x25x1, MERV 11. Spares are on the shelf in the garage. Counted from the day it is done, so a late change moves the next one.")
    w.repeating("Clean the gutters", "FREQ=MONTHLY;INTERVAL=6;BYMONTHDAY=1", project=repairs, past_dues=[-185], due=w.on("SA", 1), priority="low", status="To schedule", labels=[L["weekend"]], values={room: ROOM["Yard"], needs: [NEEDS["A helper"]]})
    w.task("Test the smoke alarms", project=repairs, status="To schedule", due=w.on("SU"), repeat="FREQ=YEARLY", values={room: ROOM["Whole house"]},
           checklist=[("Kitchen", False), ("Hall", False), ("Upstairs landing", False), ("Basement", False)])
    w.task("Fix the squeaky stair", project=repairs, status="Noticed", priority="low", labels=[L["weekend"]], values={room: ROOM["Whole house"]}, notes="Fourth from the top. Jonah says it is load-bearing for sneaking out.")
    w.task("Regrout the shower", project=repairs, status="Noticed", values={room: ROOM["Upstairs bath"], needs: [NEEDS["A day off"], NEEDS["Tools"]]})
    w.task("Garage door opener", project=repairs, status="Fixed", done_days=24, due=-26, priority="high", values={cost: 310, vendor: "Lakeside Door", receipt: True, warranty: w.day(340), room: ROOM["Garage"]},
           logged=[(-27, 90, "Waiting for the installer")], estimate=60, files=[("garage-door-invoice.pdf", "application/pdf", pdf("Lakeside Door invoice 4471"))])
    w.task("Leaky window seal in the living room", project=repairs, status="Living with it", done_days=40, values={room: ROOM["Living room"]}, notes="Quote was more than the windows. Revisit when we replace them.", comments=[("Fine. We buy a draft stopper and move on.", 40, "Theo")])
    w.task("Flush the water heater", project=repairs, status="To schedule", due=30, repeat="FREQ=YEARLY", values={room: ROOM["Garage"]})
    w.task("Dishwasher making a noise", project=repairs, status="Noticed", deleted_days=5, notes="It stopped. In the trash; restore it if it starts again.")

    # The kitchen.
    kitchen = w.project("Kitchen renovation", icon="lucide:hammer", color="orange",
                        description="New cabinets, counters and a wall coming down. Gus Ferreira is the contractor; we do the demo and the paint ourselves.",
                        statuses=[("Ideas", "backlog"), ("Planning", "todo"), ("Ordered", "todo"), ("On site", "in-progress"),
                                  ("Snagging", "in-progress"), ("Done", "done"), ("Scrapped", "canceled")])
    budget = w.field("Budget", "number", project=kitchen)
    actual = w.field("Actual", "number", project=kitchen)
    quote = w.field("Quote", "url", project=kitchen)
    phase, PHASE = w.choices("Phase", ["Design", "Demo", "Rough-in", "Install", "Finish"], project=kitchen)
    deposit = w.field("Deposit paid", "checkbox", project=kitchen)
    delivery = w.field("Delivery", "date", project=kitchen)
    finishes, FIN = w.choices("Finishes", ["Oak", "Brass", "Matte black", "Terrazzo", "Sage paint"], multi=True, project=kitchen)
    signoff = w.task("Sign off the final design", project=kitchen, status="Done", done_days=30, due=-32, priority="high", assigned=True, created_days=60, estimate=240,
                     values={phase: PHASE["Design"], budget: 1800, actual: 1800, deposit: True, quote: "https://example.com/ferreira/design-v3"},
                     logged=[(-34, 120, "Walkthrough with Gus"), (-33, 90, "Second look at the island", "Theo")],
                     notes="""## What we agreed

| Item | Choice | Why |
| --- | --- | --- |
| Cabinets | Rift oak, slab doors | Hides fingerprints |
| Counter | Terrazzo | Bea's idea |
| Handles | Brass | Against Theo's vote |

- [x] Island stays 90 cm wide
- [x] Wall to the dining room comes down
- [ ] Decide on the pantry door (see {p:Design decisions#Pantry door})

> [!IMPORTANT]
> Anything changed after this date is an extra on the invoice.""",
                     files=[("kitchen-plan-v3.png", "image/png", png(480, 300, (245, 243, 236), [(20, 20, 440, 12, (90, 90, 96)), (20, 20, 12, 260, (90, 90, 96)), (40, 40, 240, 40, (181, 137, 90)), (170, 170, 180, 60, (120, 134, 107))])),
                            ("quote-ferreira.pdf", "application/pdf", pdf("Ferreira Build quote, kitchen, rev 3"))],
                     comments=[("(edited) Signed. Deposit went out today, see {t:Pay the contractor deposit}.", 30),
                               ("(deleted) Wrong task, ignore this.", 29)])
    deposit_task = w.task("Pay the contractor deposit", project=kitchen, status="Done", done_days=29, due=-30, values={phase: PHASE["Design"], budget: 6500, actual: 6500, deposit: True}, labels=[L["money"]], waits=[signoff])
    cabinets = w.task("Order the cabinets", project=kitchen, status="Ordered", start=-28, due=-20, priority="high", done_days=None, values={phase: PHASE["Install"], budget: 14200, actual: 14860, delivery: w.day(11), finishes: [FIN["Oak"], FIN["Brass"]], quote: "https://example.com/oakline/order-55127", deposit: True},
                      waits=[signoff], comments=[("Lead time moved a week. Delivery is now the {t:Install the cabinets} week at the earliest. Tracking: https://example.com/oakline/track/55127", 6, "Gus")])
    demo = w.task("Demo: take out the old cabinets and the wall", project=kitchen, status="On site", start=-6, due=-1, priority="high", estimate=16 * 60, assigned=True, waits=[deposit_task], labels=[L["weekend"]],
                  values={phase: PHASE["Demo"], budget: 900, actual: 640, needs: [NEEDS["A helper"], NEEDS["Tools"]], room: ROOM["Kitchen"]},
                  logged=[(-6, 300, "Upper cabinets out"), (-5, 360, "Lower cabinets and the sink", "Theo"), (-5, 180, "Skip loaded", "Jonah"), (-2, 240, "Half the wall"), (-1, 310, "Rest of the wall", "Theo"), (-1, 300, "Patching and sweeping", "Theo")],
                  comments=[("Wall is out. Found old knob and tube behind it, so {t:Electrical rough-in} just got longer. {@:Gus} can you look Tuesday?", 1, "Theo"),
                            ("Looked. Adds a day. Notes are on {p:Contractors#Ferreira Build}.", 0.5, "Gus"),
                            ("The bank wants to see this first: {t:Draw down the reno loan: second stage}. Who to ring about the skip is in {p:Who to call}.", 0.3)],
                  checklist=[("Shut off the gas at the range", True), ("Strip the upper cabinets", True), ("Strip the lower cabinets", True), ("Take out the wall", True), ("Haul it all to the skip", False)])
    w.task("Sand the old door frames", project=kitchen, parent=demo, status="On site", due=1, estimate=180, assigned=True, logged=[(-1, 45, "First frame")], timer=(25, "Second frame"), my_day=True)
    w.task("Set up the camp kitchen in the dining room", project=kitchen, parent=demo, status="Done", done_days=7, values={actual: 85}, checklist=[("Microwave", True), ("Induction plate", True), ("Paper plates, many", True)])
    plumbing = w.task("Plumbing rough-in", project=kitchen, status="Planning", start=1, due=3, priority="high", values={phase: PHASE["Rough-in"], budget: 2200, vendor: "Rivera and Sons", deposit: False}, estimate=12 * 60, waits=[demo])
    electric = w.task("Electrical rough-in", project=kitchen, status="Planning", start=2, due=5, priority="high", values={phase: PHASE["Rough-in"], budget: 2800, vendor: "Bright Spark Electric", deposit: True}, estimate=14 * 60, waits=[demo])
    inspect = w.task("City inspection of the rough-in", project=kitchen, status="Planning", due=7, time="10:30", priority="high", values={phase: PHASE["Rough-in"], needs: [NEEDS["A permit"]]}, waits=[plumbing, electric], reminders=[w.remind_before(7, "10:30", 1440)], notes="Permit number is on {p:Contractors}. Someone has to be home.")
    drywall = w.task("Hang and finish the drywall", project=kitchen, status="Planning", start=8, due=11, values={phase: PHASE["Rough-in"], budget: 1400}, waits=[inspect], estimate=10 * 60)
    install = w.task("Install the cabinets", project=kitchen, status="Planning", start=12, due=15, priority="high", values={phase: PHASE["Install"], budget: 1800, finishes: [FIN["Oak"]]}, waits=[drywall, cabinets], estimate=16 * 60)
    template_day = w.task("Countertop template visit", project=kitchen, status="Planning", due=16, time="08:00", values={phase: PHASE["Install"]}, waits=[install])
    counters = w.task("Install the terrazzo counters", project=kitchen, status="Ordered", start=26, due=27, values={phase: PHASE["Install"], budget: 6100, finishes: [FIN["Terrazzo"]], delivery: w.day(25), quote: "https://example.com/stoneworks/quote-889"}, waits=[template_day], estimate=6 * 60)
    backsplash = w.task("Tile the backsplash", project=kitchen, status="Planning", start=28, due=30, values={phase: PHASE["Finish"], budget: 780}, waits=[counters], estimate=12 * 60)
    paint = w.task("Paint the walls sage", project=kitchen, status="Planning", start=31, due=33, values={phase: PHASE["Finish"], budget: 210, finishes: [FIN["Sage paint"]]}, waits=[backsplash], assigned=True, estimate=8 * 60)
    samples = w.task("Buy sage paint", project=kitchen, parent=paint, status="Planning", due=26, values={cost: 96, vendor: "Hollis Hardware"})
    w.task("Try three samples on the wall", project=kitchen, parent=samples, status="Done", done_days=4, notes="The middle one. Everyone agreed, which has never happened.")
    w.task("Tape the edges", project=kitchen, parent=paint, status="Planning", due=31, estimate=60)
    w.task("Final walkthrough and snag list", project=kitchen, status="Planning", due=35, values={phase: PHASE["Finish"]}, waits=[paint], checklist=[("Doors hang straight", False), ("Drawers close", False), ("All sockets work", False), ("Touch-ups", False)])
    w.task("Choose a pendant light", project=kitchen, status="Ideas", priority="low", values={finishes: [FIN["Brass"], FIN["Matte black"]], link: "https://example.com/lamps/pendant-12"})
    w.task("Under-cabinet lighting", project=kitchen, status="Ideas", notes="Only if the budget has anything left.")
    w.task("Heated floor", project=kitchen, status="Scrapped", done_days=45, values={budget: 3800}, notes="Too much for what it is.", comments=[("Scrapped. {@:Theo} agrees, reluctantly.", 45)])

    # Meals.
    meals = w.project("Meals", icon="lucide:utensils", color="green", description="This week's dinners, what to buy and who cooks.",
                      statuses=[("Planned", "todo"), ("Shopping", "in-progress"), ("Cooked", "done"), ("Skipped", "canceled")])
    meal, MEAL = w.choices("Meal", ["Breakfast", "School lunch", "Dinner", "Baking"], project=meals)
    cook, COOK = w.choices("Cook", ["Nora", "Theo", "Jonah", "Bea"], project=meals)
    diet, DIET = w.choices("Diet", ["Vegetarian", "Nut free", "Quick", "Freezer friendly"], multi=True, project=meals)
    for offset, name, who, tags, page in [
            (0, "Sheet-pan chicken and potatoes", "Theo", ["Nut free", "Quick"], "Sheet-pan chicken"), (1, "Red lentil soup", "Nora", ["Vegetarian", "Freezer friendly"], "Red lentil soup"),
            (2, "Fish tacos", "Theo", ["Quick"], None), (3, "Leftover soup and toast", "Jonah", ["Vegetarian", "Quick"], "Red lentil soup"),
            (4, "Pizza night", "Jonah", ["Vegetarian"], None), (5, "Roast with Bea", "Bea", ["Nut free"], None), (6, "Pasta with whatever is left", "Nora", ["Quick"], None)]:
        w.task(name, project=meals, status="Planned", due=offset, time="18:30", key=f"Dinner: {name}", values={meal: MEAL["Dinner"], cook: COOK[who], diet: [DIET[t] for t in tags]},
               notes=f"Recipe: {{p:{page}}}" if page else "", my_day=offset == 0)
    for offset, name, who in [(-7, "Chili", "Theo"), (-6, "Roast chicken", "Nora"), (-5, "Stir fry", "Theo"), (-3, "Pizza night, last week", "Jonah"), (-2, "Pancakes for dinner", "Jonah")]:
        w.task(name, project=meals, status="Cooked", done_days=-offset, due=offset, values={meal: MEAL["Dinner"], cook: COOK[who]})
    w.task("Takeout instead (kitchen dust)", project=meals, status="Skipped", done_days=4, due=-4, values={meal: MEAL["Dinner"]})
    w.task("Weekly shop", project=meals, status="Shopping", due=1, labels=[L["errand"]], my_day=True, values={vendor: "Greenfield Market", cost: 164.00, link: "https://example.com/greenfield/list"}, assigned=True, estimate=60,
           checklist=[(x, i < 3) for i, x in enumerate(["Chicken thighs", "Potatoes", "Red lentils", "White fish", "Tortillas", "Mozzarella", "Milk", "Eggs", "Bread", "Coffee", "Bananas", "Spinach"])])
    w.repeating("Sunday batch cook", "FREQ=WEEKLY;BYDAY=SU", project=meals, past_dues=[-15, -8, -1], due=w.on("SU"), time="15:00", status="Planned", values={meal: MEAL["School lunch"], cook: COOK["Nora"], diet: [DIET["Freezer friendly"]]}, estimate=120, logged=[(-1, 110, "Soup and two trays of muffins")], checklist=[("Soup", False), ("Muffins", False), ("Chop vegetables", False)])
    w.task("Pack school lunches", project=meals, status="Planned", due=0, time="07:15", repeat="FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", values={meal: MEAL["School lunch"], cook: COOK["Theo"], diet: [DIET["Nut free"]]}, notes="Wren's class is nut free. Check before you pack the bars.")
    w.task("Bea's cardamom buns for the bake sale", project=meals, status="Planned", due=11, values={meal: MEAL["Baking"], cook: COOK["Bea"]}, labels=[L["grandma"], L["kids"]], notes="Recipe: {p:Bea's cardamom buns#The dough}. Bake sale is {t:School bake sale}.")

    # School.
    school = w.project("School", icon="lucide:graduation-cap", color="indigo", description="Wren is in grade 4, Jonah in grade 9. Forms, fairs, meetings and money.",
                       statuses=[("Backlog", "backlog"), ("To do", "todo"), ("Waiting on school", "in-progress"), ("Doing", "in-progress"), ("Done", "done"), ("Not needed", "canceled")])
    kid, KID = w.choices("Kid", ["Jonah", "Wren"], project=school)
    subject, SUBJ = w.choices("Subject", ["Math", "Science", "English", "History", "Art", "Sport"], project=school)
    signed = w.field("Signed", "checkbox", project=school)
    topic = w.task("Science fair: pick a topic", project=school, status="Done", done_days=12, due=-13, values={kid: KID["Wren"], subject: SUBJ["Science"]}, labels=[L["kids"]], comments=[("Wren chose: do plants grow toward colored light? {@:Wren} has opinions about the red ones.", 12)])
    materials = w.task("Science fair: buy pots, seeds and colored film", project=school, status="Doing", due=-1, values={kid: KID["Wren"], subject: SUBJ["Science"], cost: 22.5}, labels=[L["errand"], L["kids"]], waits=[topic], assigned=True, my_day=True, my_day_on=-1)
    experiment = w.task("Science fair: run the experiment", project=school, status="To do", start=1, due=15, values={kid: KID["Wren"], subject: SUBJ["Science"]}, waits=[materials], estimate=180, checklist=[("Plant ten pots", False), ("Measure on Mondays and Thursdays", False), ("Photograph each pot", False)], labels=[L["kids"]])
    board = w.task("Science fair: make the poster", project=school, status="To do", start=16, due=19, values={kid: KID["Wren"], subject: SUBJ["Art"]}, waits=[experiment])
    w.task("Science fair: fair day", project=school, status="To do", due=20, time="17:30", priority="high", values={kid: KID["Wren"], subject: SUBJ["Science"]}, waits=[board], reminders=[w.remind_before(20, "17:30", 180)])
    w.task("Sign the field trip form", project=school, status="To do", priority="high", due=-2, values={kid: KID["Jonah"], subject: SUBJ["History"], signed: False, cost: 14}, files=[("field-trip-form.pdf", "application/pdf", pdf("Museum field trip permission form"))], labels=[L["kids"]], notes="Form is attached. Money goes in the envelope with it.")
    w.task("Parent-teacher conference: Jonah", project=school, status="Waiting on school", due=w.on("WE"), time="16:10", values={kid: KID["Jonah"]}, reminders=[w.remind_before(w.on("WE"), "16:10", 60)], comments=[("{@:Theo} takes this one, I have the inspector call. Agenda is under {p:School contacts#Questions for teachers}.", 2)])
    w.task("Parent-teacher conference: Wren", project=school, status="Done", done_days=6, due=-6, values={kid: KID["Wren"]}, notes="Reading well above grade. Asked about the extra math sheets.")
    w.repeating("Wren's spelling test practice", "FREQ=WEEKLY;BYDAY=TH", project=school, past_dues=[-11, -4], due=w.on("WE"), status="To do", values={kid: KID["Wren"], subject: SUBJ["English"]}, estimate=20, logged=[(-4, 25, "Fifteen words", "Theo")])
    w.task("Reading log", project=school, status="Doing", repeat="FREQ=DAILY", due=0, values={kid: KID["Wren"], subject: SUBJ["English"], signed: False})
    w.task("Choose Jonah's spring electives", project=school, status="Backlog", values={kid: KID["Jonah"]}, notes="Form opens in January.")
    w.task("School bake sale", project=school, status="To do", due=12, time="15:00", labels=[L["kids"], L["grandma"]], values={subject: SUBJ["Art"]}, waits=[], comments=[("{t:Bea's cardamom buns for the bake sale} is the plan.", 1)])
    w.task("Jonah: basketball tryouts form", project=school, status="Not needed", done_days=15, values={kid: KID["Jonah"], subject: SUBJ["Sport"], signed: True}, notes="He changed his mind.")
    w.task("Buy new math calculator", project=school, status="Done", done_days=20, values={kid: KID["Jonah"], subject: SUBJ["Math"], cost: 19.99, receipt: True})

    # Money.
    money = w.project("Money", icon="lucide:piggy-bank", color="lime", description="Bills, savings and the renovation's money. The numbers are made up.",
                      statuses=[("To pay", "todo"), ("Scheduled", "in-progress"), ("Disputed", "in-progress"), ("Paid", "done"), ("Waived", "canceled")])
    amount = w.field("Amount", "number", project=money)
    account, ACC = w.choices("Account", ["Joint checking", "Savings", "Credit card", "Reno loan"], project=money)
    w.repeating("Pay the mortgage", "FREQ=MONTHLY;BYMONTHDAY=1", project=money, past_dues=[-32, -1], due=(today.replace(day=1) + dt.timedelta(days=32)).replace(day=1).toordinal() - today.toordinal(), time="09:00", status="Scheduled", priority="high", labels=[L["money"]], values={amount: 1842.17, account: ACC["Joint checking"]}, assigned=True)
    w.repeating("Electric bill", "FREQ=MONTHLY;BYMONTHDAY=15", project=money, past_dues=[-18], due=13, status="To pay", values={amount: 118.32, account: ACC["Credit card"]}, reminders=[w.remind_at(10, "17:00")])
    w.task("Internet bill", project=money, status="Disputed", due=-6, priority="medium", repeat="FREQ=MONTHLY;BYDAY=2TU", values={amount: 79.0, account: ACC["Credit card"]}, notes="Charged twice in October. Case number 88213.", comments=[("Called. Credit in the next cycle, they say.", 2)], labels=[L["call"], L["waiting"]])
    w.task("Car insurance renewal", project=money, status="To pay", due=9, priority="high", values={amount: 642.5, account: ACC["Joint checking"], link: "https://example.com/insurer/renewal"}, labels=[L["online"]], checklist=[("Get a second quote", False), ("Check the mileage they have", False)])
    fund = w.task("Move money to the holiday fund", project=money, status="Scheduled", due=4, repeat="FREQ=MONTHLY;BYDAY=1FR", values={amount: 300, account: ACC["Savings"]})
    w.task("Last Friday budget check", project=money, status="To pay", due=25, repeat="FREQ=MONTHLY;BYDAY=-1FR", assigned=True, estimate=45, logged=[(-3, 50, "October, with the kitchen in it")], notes=RICH_NOTES.split("\n---\n")[0].replace("What this note is for", "How we do it"))
    w.task("Draw down the reno loan: second stage", project=money, status="To pay", due=3, priority="high", values={amount: 12000, account: ACC["Reno loan"]}, comments=[("Bank needs the {t:Sign off the final design} PDF first.", 4)])
    w.task("Cancel unused subscriptions", project=money, status="Paid", done_days=5, values={amount: -38.97, account: ACC["Credit card"]}, checklist=[("Second streaming service", True), ("Old cloud storage", True), ("Magazine", True)])
    w.task("Gather papers for the accountant", project=money, status="To pay", start=-10, priority="medium", notes="No date yet: the accountant writes in January.", files=[("2025-summary.csv", *csv_file("month,income,spent", "January,6120,5230", "February,6120,4910", "March,6480,5840"))])
    w.task("Late fee on the library book", project=money, status="Waived", done_days=33, values={amount: 4.5})

    # The holiday.
    trip = w.project("Winter holiday in Lisbon", icon="lucide:plane", color="blue", description="December 19 to 28, all five of us. Bea flies from her sister's.",
                     statuses=[("Ideas", "backlog"), ("To book", "todo"), ("Booked", "in-progress"), ("Sorted", "done"), ("Dropped", "canceled")])
    who_goes, TRAV = w.choices("Travelers", ["Nora", "Theo", "Jonah", "Wren", "Bea"], multi=True, project=trip)
    conf = w.field("Confirmation", "text", project=trip)
    booking = w.field("Booking link", "url", project=trip)
    refund = w.field("Refundable", "checkbox", project=trip)
    when = w.field("On the day", "date", project=trip)
    everyone = list(TRAV.values())
    passport = w.task("Renew Jonah's passport", project=trip, status="To book", priority="urgent", due=-4, assigned=True, values={who_goes: [TRAV["Jonah"]], cost: 135}, labels=[L["errand"]],
                      comments=[("It runs out in March, and they want six months. This holds up {t:Book the flights}.", 8)], checklist=[("Photos", True), ("Parent consent form", False), ("Appointment", False)], reminders=[w.remind_at(1, "08:00")], notes="Appointment line opens at 8. Rules are at https://example.com/passports/under-16")
    flights = w.task("Book the flights", project=trip, status="To book", priority="high", due=5, waits=[passport], values={who_goes: everyone, cost: 3120, refund: False, booking: "https://example.com/air/search?lis"}, labels=[L["online"]], estimate=60)
    w.task("Book the apartment in Alfama", project=trip, status="Booked", done_days=None, due=-9, values={who_goes: everyone, conf: "LX-77120", cost: 1860, refund: True, booking: "https://example.com/stays/alfama-3br", when: w.day(47)}, files=[("apartment-confirmation.pdf", "application/pdf", pdf("Alfama apartment, December 19 to 28"))], labels=[L["online"]])
    w.task("Compare Tuesday and Wednesday fares", project=trip, parent=flights, status="To book", due=3, labels=[L["online"]])
    w.task("Choose seats", project=trip, parent=flights, status="Ideas", notes="Wren wants a window. So does Jonah.")
    w.task("Travel insurance", project=trip, status="To book", due=6, waits=[flights], values={cost: 210, booking: "https://example.com/insure/family"})
    sitter = w.task("Find someone for the cat", project=trip, status="To book", due=14, labels=[L["call"]], notes="Ines did it last year. Ask before Thanksgiving.")
    w.task("Tram 28 and the castle", project=trip, status="Ideas", values={when: w.day(49), who_goes: everyone}, notes="See {p:Itinerary#Day 3}.")
    w.task("Day trip to Sintra", project=trip, status="Ideas", values={when: w.day(51), who_goes: [TRAV["Nora"], TRAV["Theo"], TRAV["Wren"]], cost: 120}, comments=[("I will stay in with a book, thank you. {@:Bea}", 3, "Jonah")])
    w.task("Pack", project=trip, status="To book", due=46, waits=[sitter, flights], checklist=[("Passports", False), ("Adapters", False), ("Rain jackets", False), ("Wren's rabbit", False)], notes="The full list is {p:Packing list}.")
    w.task("Cruise instead?", project=trip, status="Dropped", done_days=70, notes="Nobody wanted it but me.", created_days=90)

    w.depend(flights, fund)  # across projects: the trip waits on the money
    # Finished and gone.
    garage = w.project("Garage clean-out", icon="🧹", color="amber", archived_days=21, description="Done in September. Archived; find it under the archived projects.")
    for i, t in enumerate(["Sort the shelves", "Dump run", "Hang the bikes", "Label the paint cans", "Sell the old mower"]):
        w.task(t, project=garage, status="Done", done_days=30 + i * 3, values={room: ROOM["Garage"]}, created_days=60)
    w.task("Drop the leftover paint at the depot", project=garage, due=0, priority="medium", values={room: ROOM["Garage"]},
           notes="Still open, in an archived project. It stays out of Upcoming, My Day's suggestions and the palette; open the project to reach it.")
    w.task("Offer the old mower to the neighbors", project=garage, due=4)
    tree = w.project("Treehouse", icon="lucide:leaf", color="green", deleted_days=6, description="In the trash with its tasks. Restore it from the trash to try that.")
    w.task("Ask the neighbors about the oak", project=tree)
    w.task("Sketch a platform", project=tree, status="In progress")

    # The inbox: tasks in no project.
    w.task("Call Aunt Ines about Thanksgiving", labels=[L["call"]], priority="high", due=1, time="11:00", my_day=True, comments=[("Ask her about the cat at the same time: {t:Find someone for the cat}.", 1)])
    w.task("Return the library books", labels=[L["errand"]], due=-1)
    w.task("Book the dentist for the kids", labels=[L["call"], L["kids"]], due=10, time="08:30", reminders=[w.remind_before(10, "08:30", 120)], assigned=True)
    w.task("Weekly family meeting", priority="medium", due=w.on("SU"), time="17:00", repeat="FREQ=WEEKLY;BYDAY=SU", checklist=[("Calendar for the week", False), ("Who drives whom", False), ("Allowance", False)], reminders=[w.remind_before(w.on("SU"), "17:00", 30)])
    w.task("Water the plants", due=1, repeat="FREQ=DAILY;INTERVAL=3", mode="completion", labels=[L["weekend"]])
    w.task("Swim lessons", repeat="FREQ=WEEKLY;BYDAY=SA;UNTIL=" + (today + dt.timedelta(days=60)).strftime("%Y%m%d"), due=w.on("SA"), time="10:00", labels=[L["kids"]])
    w.task("Back up the family photos", due=None, notes="The command, so nobody has to remember it:\n\n```bash\nrsync -av --delete ~/Pictures/ /mnt/shelf-drive/pictures/\n```\n\nRun it from the study computer.", estimate=30)
    w.task("Mom's birthday: November 27", due=25, repeat="FREQ=YEARLY", priority="high", checklist=[("Gift", False), ("Card", False), ("Dinner reservation", False)], labels=[L["grandma"]])
    w.task("Idea: compost bin for the side yard", notes="Maybe in spring.")
    w.task("Renew the car registration", done_days=8, due=-9, labels=[L["online"]], values={cost: 96})
    w.task("Take out the recycling", due=w.on("TU"), time="19:00", repeat="FREQ=WEEKLY;BYDAY=TU,FR", assigned=True)

    w.attach(signoff, "kitchen-plan-v2.png", "image/png", png(240, 150, (245, 243, 236)), deleted_days=31)
    # Saved views: every layout, shared and private.
    w.automation("Overdue chores get a look", {"when": {"on": "due", "value": "overdue"},
                                               "then": [{"do": "priority", "value": "high"}]})
    w.view("This week at home", "list", filter={"due": "week"}, display={"sort": [{"by": "due", "dir": "asc"}, {"by": "priority", "dir": "desc"}], "group": {"by": "project"}, "fields": {"list": {"show": ["due", "project", "labels", "assignees"]}}})
    w.view("Overdue", "list", filter={"due": "overdue"}, display={"sort": [{"by": "priority", "dir": "desc"}], "group": {"by": "priority"}})
    w.view("Family calendar", "calendar", filter={})
    w.view("Waiting on something", "list", filter={"dependencies": ["blocked"]}, display={"group": {"by": "project"}})
    w.view("Holding things up", "list", filter={"dependencies": ["blocking"]}, display={"sort": [{"by": "due", "dir": "asc"}]})
    w.view("Mine", "list", filter={"assignees": ["me"]}, display={"group": {"by": "due"}}, private=True)
    w.view("Weekend jobs", "list", filter={"labels": [L["weekend"]]}, display={"group": {"by": "project"}, "fields": {"list": {"show": ["due", "project", "labels"]}}})
    w.view("Time logged lately", "table", filter={"time": ["recent"], "showDone": True}, display={"sort": [{"by": "logged", "dir": "desc"}], "fields": {"table": {"show": ["estimate", "logged", "remaining"]}}})
    w.view("Over the estimate", "list", filter={"time": ["over"], "showDone": True}, display={"fields": {"list": {"show": ["estimate", "logged", "remaining", "project"]}}})
    w.view("Costs anything", "table", filter={"fields": [{"field": cost, "op": "not_empty"}], "showDone": True}, display={"group": {"by": "project"}, "sort": [{"by": "field", "field": cost, "dir": "desc"}], "table": {"order": [cost, vendor, receipt, "status", "due"], "summaries": [{"col": cost, "fn": "sum"}]}})
    w.view("Finished lately", "list", filter={"showDone": True, "statuses": ["done", "canceled"]}, display={"sort": [{"by": "completed", "dir": "desc"}], "group": {"by": "status"}})
    w.view("Inbox to sort", "list", filter={"projects": ["inbox"]}, display={"group": {"by": "priority"}, "sort": [{"by": "due", "dir": "asc"}, {"by": "priority", "dir": "desc"}, {"by": "title", "dir": "asc"}], "fields": {"list": {"show": ["start", "priority", "reminders", "repeat", "comments", "checklist"]}}})
    w.view("High on the list", "list", filter={"priorities": ["urgent", "high"], "statuses": ["backlog", "todo"]}, display={"sort": [{"by": "status", "dir": "asc"}, {"by": "project", "dir": "asc"}], "group": {"by": "assignee"}})
    w.view("On me, no estimate", "list", filter={"assignees": [EXPORTER], "time": ["unestimated"]}, display={"sort": [{"by": "created", "dir": "desc"}]})
    w.view("Cheap fixes", "list", filter={"fields": [{"field": cost, "op": "lt", "value": 50}, {"field": vendor, "op": "contains", "value": "Hollis"}], "showDone": True}, display={"sort": [{"by": "field", "field": vendor, "dir": "asc"}], "fields": {"list": {"show": [cost, vendor, link, warranty]}}})
    w.view("Everywhere but the kitchen", "list", filter={"fields": [{"field": room, "op": "none_of", "value": [ROOM["Kitchen"]]}, {"field": needs, "op": "none_of", "value": [NEEDS["A permit"]]}]}, display={"group": {"by": "field", "field": room}, "sort": [{"by": "field", "field": room, "dir": "asc"}]})
    w.view("Receipts kept", "table", filter={"fields": [{"field": receipt, "op": "checked"}], "showDone": True}, display={"sort": [{"by": "field", "field": receipt, "dir": "desc"}, {"by": "field", "field": link, "dir": "asc"}], "table": {"order": [cost, vendor, warranty, receipt, link], "summaries": [{"col": "title", "fn": "count"}, {"col": cost, "fn": "min"}, {"col": vendor, "fn": "filled"}, {"col": warranty, "fn": "none"}]}})
    w.view("No room set", "list", filter={"projects": [repairs, school], "fields": [{"field": room, "op": "empty"}]}, display={"sort": [{"by": "updated", "dir": "desc"}]})
    w.view("Scrapped idea", "list", filter={"text": "scrapped"}, deleted_days=6)
    w.view("Repairs board", "board", project=repairs, filter={"projects": [repairs]}, display={"fields": {"board": {"show": [room, cost, "due", "checklist"]}}})
    w.view("Repairs by room", "list", project=repairs, filter={"projects": [repairs]}, display={"group": {"by": "field", "field": room}, "fields": {"list": {"show": [room, needs, vendor]}}})
    w.view("Warranties", "table", project=repairs, filter={"projects": [repairs], "showDone": True, "fields": [{"field": warranty, "op": "after", "value": 0}]}, display={"sort": [{"by": "field", "field": warranty, "dir": "asc"}]})
    w.view("Arriving soon", "list", project=kitchen, filter={"projects": [kitchen], "fields": [{"field": delivery, "op": "before", "value": 30}]}, display={"fields": {"list": {"show": [delivery, quote]}}})
    w.view("Cabinet delivery day", "list", project=kitchen, filter={"projects": [kitchen], "fields": [{"field": delivery, "op": "on", "value": w.day(11)}]})
    w.view("Kitchen by phase", "board", project=kitchen, filter={"projects": [kitchen], "showDone": True}, display={"columns": {"by": "field", "field": phase}, "fields": {"board": {"show": [budget, actual, "due", "estimate", "logged"]}}})
    w.view("Kitchen budget", "table", project=kitchen, filter={"projects": [kitchen], "showDone": True}, display={"group": {"by": "field", "field": phase}, "sort": [{"by": "field", "field": budget, "dir": "desc"}], "fields": {"table": {"show": ["status"], "hide": ["assignees", "project"]}}, "table": {"order": [budget, actual, deposit, delivery, quote, "status", "due"], "widths": [{"col": "title", "px": 320}, {"col": budget, "px": 90}, {"col": actual, "px": 90}], "summaries": [{"col": budget, "fn": "sum"}, {"col": actual, "fn": "sum"}, {"col": deposit, "fn": "filled"}, {"col": "due", "fn": "latest"}, {"col": "status", "fn": "bar"}]}})
    w.view("Kitchen schedule", "timeline", project=kitchen, filter={"projects": [kitchen], "showDone": True}, display={"timeline": {"zoom": "week"}, "group": {"by": "field", "field": phase}})
    w.view("Kitchen report", "report", project=kitchen, filter={}, display={"report": {"tab": "status", "period": "weeks6", "from": None, "to": None, "project": None}})
    w.view("Who cooks", "board", project=meals, filter={"projects": [meals]}, display={"columns": {"by": "field", "field": cook}, "fields": {"board": {"show": [diet, "due"]}}})
    w.view("Dinners", "calendar", project=meals, filter={"projects": [meals], "showDone": True, "fields": [{"field": meal, "op": "any", "value": [MEAL["Dinner"]]}]})
    w.view("By kid", "list", project=school, filter={"projects": [school]}, display={"group": {"by": "field", "field": kid}, "fields": {"list": {"show": [kid, subject, signed, "due"]}}})
    w.view("Wren's science fair", "timeline", project=school, filter={"projects": [school], "text": "science fair", "showDone": True}, display={"timeline": {"zoom": "month"}})
    w.view("Bills to pay", "table", project=money, filter={"projects": [money], "statuses": ["todo", "in-progress"]}, display={"sort": [{"by": "due", "dir": "asc"}], "table": {"order": [amount, account, "status", "due", "repeat"], "summaries": [{"col": amount, "fn": "sum"}, {"col": "due", "fn": "overdue"}]}})
    w.view("Countdown to Lisbon", "timeline", project=trip, filter={"projects": [trip]}, display={"timeline": {"zoom": "quarter"}})
    w.view("Bookings", "table", project=trip, filter={"projects": [trip], "fields": [{"field": cost, "op": "gte", "value": 100}]}, display={"table": {"order": [cost, refund, conf, booking, who_goes, "status"], "summaries": [{"col": cost, "fn": "sum"}, {"col": refund, "fn": "count"}]}})
    w.view("My own trip list", "list", project=trip, filter={"projects": [trip], "assignees": ["me"]}, private=True)

    # Themes the household offers.
    w.theme("Kitchen table", base="calm", hue=35, suggested_days=4, tokens={"radius-md": {"px": 10}, "font-ui": {"font": "nunito"}, "font-heading": {"ref": "font-ui"}, "neutral-hue": {"hue": 60}, "neutral-chroma": {"n": 0.012}})
    w.theme("Night feed", base="standard", hue=265, source="0190a000-0000-7000-8000-0000000071e0", dark={"surface-0": {"oklch": [0.17, 0.02, 265]}, "sidebar-bg": {"oklch": [0.15, 0.02, 265]}})
    w.theme("Old beige", base="calm", hue=70, deleted_days=20)

    # Templates.
    party, games, cake = w.id(), w.id(), w.id()
    w.template("task", "Birthday party", dict(v=1, tasks=[
        tpl_task(party, "Birthday party", priority="medium", category="todo", due=21, estimate=240, labels=[dict(id=L["kids"], name="kids", color="yellow")],
                 checklist=["Guest list", "Invitations", "Thank-you notes"], notes="Budget and ideas go here. Check {p:Who to call} for the hall.",
                 values={cost: 150}),
        tpl_task(games, "Plan the games", parent=party, due=14, estimate=60),
        tpl_task(cake, "Order or bake the cake", parent=party, due=18, waits=[games]),
    ]), days=30)
    callout, visit = w.id(), w.id()
    w.template("task", "Repair call-out", dict(v=1, tasks=[
        tpl_task(callout, "Repair call-out", priority="high", category="todo", due=3, checklist=["Photos of the problem", "Two quotes", "Book a day someone is home"], assignees=[EXPORTER, w.person_ids["Theo"]], values={room: ROOM["Whole house"], needs: [NEEDS["A day off"]]}),
        tpl_task(visit, "Pay and file the invoice", parent=callout, due=7, labels=[dict(id=L["money"], name="money", color="lime")]),
    ]))
    trip_id, s_idea, s_book, s_done = w.id(), w.id(), w.id(), w.id()
    f_cost, f_conf = w.id(), w.id()
    t_dates, t_book, t_pack = w.id(), w.id(), w.id()
    main_id = pair_id(trip_id, MAIN_PAGE_SPACE)
    w.template("project", "Family trip", dict(v=1, project=dict(
        id=trip_id, name="Family trip", description="From choosing dates to packing the car.", icon="lucide:plane", color="cyan",
        statuses=[dict(id=s_idea, name="Ideas", category="backlog"), dict(id=s_book, name="To book", category="todo"), dict(id=s_done, name="Sorted", category="done")],
        fields=[dict(id=f_cost, name="Cost", type="number", options=[]), dict(id=f_conf, name="Confirmation", type="text", options=[])],
        views=[dict(name="Bookings", type="table", filter={"projects": [trip_id]}, display={"table": {"order": [f_cost, f_conf, "status"], "summaries": [{"col": f_cost, "fn": "sum"}]}}),
               dict(name="Countdown", type="timeline", filter={"projects": [trip_id]}, display={"timeline": {"zoom": "month"}})],
        pages=[dict(id=main_id, parent=None, title="", body="Where, when and who is coming."),
               dict(id=w.id(), parent=main_id, title="Packing list", body="- [ ] Passports\n- [ ] Chargers\n- [ ] Rain jackets")]),
        tasks=[tpl_task(t_dates, "Agree the dates", status=s_book, category="todo", start=0, due=3, estimate=30, labels=[dict(id=L["call"], name="call", color="cyan")]),
               tpl_task(t_book, "Book travel and a place to stay", status=s_book, category="todo", due=14, waits=[t_dates], values={f_cost: 1500}, estimate=120),
               tpl_task(t_pack, "Pack", status=s_book, category="todo", due=30, waits=[t_book], checklist=["Documents", "Medicine", "Snacks"])]), days=12)

    meeting = w.id()
    w.template("page", "Meeting notes", dict(v=1, tasks=[], pages=[
        dict(id=meeting, parent=None, title="Notes, {{today}}", body="## Attending\n\n## Decisions\n\n## Next steps\n"),
        dict(id=w.id(), parent=meeting, title="Actions", body="- [ ] Who does what, by when\n"),
    ]))
    w.template("task", "Old chore chart", dict(v=1, tasks=[tpl_task(w.id(), "Chores", category="todo")]), days=80, deleted_days=5)
    w.taken_off(w.task_names["Regrout the shower"], label=L["grandma"], blocker=w.task_names["Test the smoke alarms"], field=warranty, value=w.day(30))

    # The household's wiki.
    top = w.main_page(name="Household home", body="""What we know about this house, so nobody has to remember it. Start with {p:Who to call}, or {p:If the power goes out} when something has gone wrong.

## This month

- The kitchen is a building site. See the {p:Kitchen renovation wiki}.
- {t:Renew Jonah's passport} is the thing holding up Lisbon.

## How this wiki is laid out

| Section | What is there |
| --- | --- |
| House | Who to call, manuals, Wi-Fi |
| Food | Recipes that survived the kids |
| School | Contacts and what to ask |
""")
    house_page = w.page("House", top, body="Everything about the building itself.\n\n## Quick facts\n\n- Built 1962, wired twice\n- Water shutoff: basement, north wall\n- Gas shutoff: outside, by the meter")
    w.page_thread(house_page, "Built 1962, wired twice", [("Is the second wiring the panel too, or only the kitchen?", 6, None)])
    w.page_thread(house_page, "Everything about the building itself", [("Should the deed go here?", 40, None), ("No, in the safe.", 39, None)],
                  resolved_days=38)
    w.page("Who to call", house_page, body="""## Plumbing

Rivera and Sons, ask for Dani. They came for the {t:Fix the dripping downstairs faucet} job.

## Electrical

Bright Spark Electric. Quote for the fan is on {t:Replace the upstairs bath fan}.

## Everything else

| Who | For what | Number |
| --- | --- | --- |
| Gus Ferreira | The kitchen | 555-0142 |
| Lakeside Door | Garage door | 555-0178 |
| Hollis Hardware | Parts | 555-0119 |
""", versions=[("Plumber: Rivera and Sons.\nElectrician: Bright Spark.", 90, ""), ("## Plumbing\n\nRivera and Sons.\n\n## Electrical\n\nBright Spark Electric.", 40, "Theo"), ("## Plumbing\n\nRivera and Sons, ask for Dani.\n\n## Electrical\n\nBright Spark Electric.\n\n## Everything else\n\nGus Ferreira for the kitchen.", 6, None)])
    manuals = w.page("Manuals", house_page, body="One page per machine. The paper ones are in the blue folder.")
    w.page("Furnace", manuals, body="Filter size 16x25x1. Changed by {t:Replace the furnace filter}.\n\n> [!WARNING]\n> The pilot needs a long lighter. Do not use the short one.")
    w.page("Water heater", manuals, body="Flush once a year: {t:Flush the water heater}.")
    w.page("Wi-Fi and printer", house_page, body="Guest network: **Visitors**. Ask for the password.\n\nTo reset the printer:\n\n```bash\nlpadmin -p study-printer -E -v socket://192.0.2.40\n```")
    w.page("If the power goes out", house_page, body="## First\n\n1. Check the breaker in the garage\n2. Flashlights are in the hall closet\n\n## Then\n\n- [ ] Call the utility on 555-0100\n- [ ] Do not open the freezer")
    food = w.page("Food", top, body="Recipes that survived the kids. Plans are in the Meals project.")
    w.page("Sheet-pan chicken", food, body="## Ingredients\n\n- 8 chicken thighs\n- 1 kg potatoes\n- Lemon, garlic\n\n## Method\n\n1. 220 C oven\n2. Everything on one pan\n3. 40 minutes")
    w.page("Red lentil soup", food, body="Freezes well. Makes twice what you think.\n\n## Ingredients\n\n| What | How much |\n| --- | ---: |\n| Red lentils | 400 g |\n| Carrots | 3 |\n| Stock | 1.5 l |")
    buns = w.page("Bea's cardamom buns", food, body="## The dough\n\nStart the night before.\n\n## The filling\n\nMore cardamom than seems right.\n\n{img:buns.png}")
    w.page_file(buns, "buns.png", "image/png", png(320, 200, (246, 233, 214), [(60 + i * 70, 70 + (i % 2) * 20, 50, 50, (196, 140, 80)) for i in range(3)]))
    w.page("School contacts", top, body="## Teachers\n\n- Ms. Adeyemi: Wren, grade 4\n- Mr. Sorensen: Jonah, homeroom\n\n## Questions for teachers\n\n1. Is Wren bored in math?\n2. How is Jonah doing with deadlines?\n\nLast notes: {t:Parent-teacher conference: Wren}.")
    w.page("Old alarm codes", top, deleted_days=9, body="Removed on purpose. It is in the trash so you can restore it.")

    kmain = w.main_page(kitchen, name="Kitchen renovation wiki", body="""The plan, the people and the money. The schedule itself is the {t:Plumbing rough-in} chain of tasks; open the Timeline to see it.

{img:floor-plan.png}

## Where to look

- {p:Design decisions} for why it looks the way it does
- {p:Contractors} for who is doing what
- {p:Budget} for what it costs
""")
    w.page_file(kmain, "floor-plan.png", "image/png", png(640, 400, (246, 244, 238), [(30, 30, 580, 14, (70, 70, 76)), (30, 30, 14, 340, (70, 70, 76)), (30, 356, 580, 14, (70, 70, 76)), (596, 30, 14, 340, (70, 70, 76)), (60, 50, 360, 50, (181, 137, 90)), (240, 210, 240, 80, (120, 134, 107))]))
    w.page("Design decisions", kmain, project=kitchen, body="""## Island

Stays at 90 cm. {t:Sign off the final design} has the rest.

## Pantry door

Sliding or swing? Still open. Theo wants a sliding door.

## Counters

Terrazzo, from {t:Install the terrazzo counters}.
""", versions=[("## Island\n\nMaybe 100 cm.", 50, None), ("## Island\n\nStays at 90 cm.\n\n## Pantry door\n\nSliding?", 20, "Theo")])
    w.page("Contractors", kmain, project=kitchen, body="""## Ferreira Build

Gus runs the job. Permit 2026-44871. Found old wiring behind the dining room wall, adds a day to {t:Electrical rough-in}.

## Subcontractors

| Trade | Company | Status |
| --- | --- | --- |
| Plumbing | Rivera and Sons | Booked |
| Electrical | Bright Spark Electric | Booked |
| Stone | Stoneworks Co-op | Template after cabinets |
""")
    w.page("Budget", kmain, project=kitchen, body="""Totals are in the Kitchen budget view. This is the reasoning.

| Line | Planned | Note |
| --- | ---: | --- |
| Cabinets | 14,200 | Came in 660 over |
| Counters | 6,100 | |
| Labor | 18,500 | Fixed price |
| Contingency | 4,000 | Do not touch |

{img:spend.png}
""")
    w.page_file(w.page_names["Budget"], "spend.png", "image/png", bars([14, 6, 18, 4], (181, 137, 90)))
    w.page_file(w.page_names["Budget"], "spend-old.png", "image/png", bars([12, 5, 16, 4], (150, 150, 150)), deleted_days=12)
    tmain = w.main_page(trip, name="Lisbon wiki", body="December 19 to 28. Start with the {p:Itinerary}.")
    w.page("Itinerary", tmain, project=trip, body="## Day 1\n\nLand, sleep.\n\n## Day 2\n\nMarket at Campo de Ourique.\n\n## Day 3\n\nTram 28 and the castle: {t:Tram 28 and the castle}.\n\n## Day 5\n\nSintra, for those who want it: {t:Day trip to Sintra}.")
    gmain = w.main_page(garage, name="Garage wiki", body="What the clean-out decided, kept with the archived project. This page and {p:Where everything went} stay out of the palette.")
    w.page("Where everything went", gmain, project=garage, body="## Shelves\n\nPaint on the top, tools at waist height.\n\n## Bikes\n\nOn the hooks by the door.")
    w.page("Packing list", tmain, project=trip, body="- [x] Check passports\n- [ ] Adapters\n- [ ] Rain jackets\n- [ ] Wren's rabbit\n\nThe task is {t:Pack}.")
    return w


def small_business(today: dt.date, now: dt.datetime, rng: random.Random) -> Workspace:
    """A coffee roaster with six people: clients, orders, marketing, hiring and the books."""
    w = Workspace("Bright Anvil Coffee Roasters", "small-business", today, now, rng, [
        ("Imani Reyes", "owner"), ("Tomasz Wolny", "admin"), ("Beth Alvarez", "member"), ("Arjun Mehta", "member"),
        ("Lena Hart", "guest"), ("Paolo Ruiz", "guest")])
    r = rng

    L = {n: w.label(n, c) for n, c in [
        ("wholesale", "blue"), ("online", "cyan"), ("rush", "red"), ("quality", "green"), ("follow up", "amber"),
        ("cash", "lime"), ("holiday", "pink"), ("equipment", "indigo"), ("compliance", "purple")]}
    w.label("2024 promo (deleted)", "yellow", deleted=True)

    owner = w.field("Account owner", "user")
    site = w.field("Website", "url")
    contact = w.field("Contact", "text")
    blends_names = ["Foundry House", "Night Shift", "Daybreak", "Ethiopia Guji", "Colombia Huila", "Decaf Swiss Water"]
    blend, BLEND = w.choices("Blends", blends_names, multi=True)
    w.field("Fax number (retired)", "text", deleted=True)

    # Clients: a pipeline.
    clients = w.project("Clients", icon="lucide:users", color="blue", description="Every wholesale account, from first sample to renewal. Beth owns this.",
                        statuses=[("Lead", "backlog"), ("Sampling", "todo"), ("Quoted", "todo"), ("Onboarding", "in-progress"), ("Active", "in-progress"), ("Renewed", "done"), ("Lost", "canceled")])
    w.status(clients, "Maybe later", "backlog", deleted=True)
    ctype, CT = w.choices("Account type", ["Cafe", "Restaurant", "Office", "Grocer", "Hotel"], project=clients)
    monthly = w.field("Monthly kg", "number", project=clients)
    renewal = w.field("Renewal", "date", project=clients)
    terms, TERMS = w.choices("Terms", ["Prepay", "Net 15", "Net 30"], project=clients)
    signed = w.field("Contract signed", "checkbox", project=clients)
    client_rows = [
        ("Lantern Cafe", "Cafe", 60, "Active", "Priya Nand", 25, "Net 15", ["Foundry House", "Decaf Swiss Water"]),
        ("Harbor Street Espresso", "Cafe", 85, "Active", "Tom Oyelaran", 6, "Net 15", ["Foundry House", "Ethiopia Guji"]),
        ("The Copper Pan", "Restaurant", 22, "Active", "Mei Tanaka", 90, "Net 30", ["Night Shift"]),
        ("Northgate Hotel", "Hotel", 120, "Onboarding", "Dario Conti", 330, "Net 30", ["Daybreak", "Decaf Swiss Water"]),
        ("Fennel & Rye", "Restaurant", 15, "Active", "Sam Whitlock", -5, "Net 15", ["Colombia Huila"]),
        ("Greenfield Market", "Grocer", 200, "Active", "Ana Beltran", 45, "Net 30", ["Foundry House", "Daybreak", "Night Shift"]),
        ("Calder Architects", "Office", 12, "Renewed", "Joss Calder", 300, "Prepay", ["Daybreak"]),
        ("Blue Door Bakery", "Cafe", 34, "Active", "Hana Lindgren", 12, "Net 15", ["Ethiopia Guji"]),
        ("Millrace Coworking", "Office", 28, "Quoted", "Kofi Mensah", None, None, ["Foundry House"]),
        ("The Station Diner", "Restaurant", 40, "Sampling", "Rae Donnelly", None, None, ["Night Shift", "Daybreak"]),
        ("Elm Park Clinic", "Office", 8, "Lead", "Oskar Berg", None, None, []),
        ("Sparrow Bookshop", "Cafe", 10, "Sampling", "Ines Carvalho", None, None, ["Colombia Huila"]),
        ("Tidewater Grocers", "Grocer", 300, "Quoted", "Bill Hargrove", None, None, ["Foundry House", "Daybreak"]),
        ("Quarry Hill Hotel", "Hotel", 90, "Lost", "Lina Farah", None, None, ["Daybreak"]),
        ("Bramble Tea Room", "Cafe", 6, "Lost", "Ellis Grant", None, None, ["Decaf Swiss Water"]),
        ("Ironworks Gym", "Office", 5, "Lead", "Dee Summers", None, None, []),
        ("Salt & Vine", "Restaurant", 18, "Lead", "Marco Reale", None, None, ["Night Shift"]),
        ("Pier 9 Kiosk", "Cafe", 26, "Active", "Yusuf Kaya", 60, "Prepay", ["Foundry House"]),
    ]
    client_task = {}
    for i, (name, kind, kg, st, person, renew, term, blends) in enumerate(client_rows):
        vals = {ctype: CT[kind], monthly: kg, contact: person, site: f"https://example.com/{slug_of(name)}"}
        if renew is not None:
            vals[renewal] = w.day(renew)
            vals[signed] = True
            vals[owner] = EXPORTER
        if term:
            vals[terms] = TERMS[term]
        if blends:
            vals[blend] = [BLEND[b] for b in blends]
        done = 40 if st == "Lost" else 12 if st == "Renewed" else None
        client_task[name] = w.task(name, project=clients, status=st, done_days=done, values=vals, labels=[L["wholesale"]] + ([L["follow up"]] if st in ("Quoted", "Sampling") else []), created_days=20 + i * 4,
                                   due={"Quoted": 3 + i % 5, "Sampling": 1 + i % 6, "Lead": None}.get(st, renew if renew is not None and renew < 40 else None), assigned=i % 3 == 0, priority="high" if kg >= 100 else "medium" if kg >= 40 else "none",
                                   notes=f"{person} runs the coffee side. Average {kg} kg a month." if i % 2 == 0 else "")
    hotel = client_task["Northgate Hotel"]
    w.task("Northgate: tasting with the F&B team", project=clients, parent=hotel, status="Renewed", done_days=9, due=-9, time="14:00", assigned=True, estimate=120, logged=[(-9, 150, "Tasting and notes")], notes="They chose Daybreak for the rooms and Decaf for room service.")
    contract = w.task("Northgate: contract and credit check", project=clients, parent=hotel, status="Onboarding", due=-1, priority="high", assigned=True, comments=[("Credit check came back fine. {@:Beth} send the Net 30 version from {p:Price list#Wholesale terms}.", 2), ("(edited) Sent the Net 30 version. Waiting on Dario's signature.", 1, "Beth"), ("Standard clauses are in {p:Price list}, and Marketing has the one-pager: {t:Wholesale one-pager}.", 0.5)], files=[("northgate-contract-draft.pdf", "application/pdf", pdf("Bright Anvil supply agreement, Northgate Hotel, draft 2"))])
    grinders = w.task("Northgate: install two grinders", project=clients, parent=hotel, status="Quoted", due=9, waits=[contract], estimate=180)
    w.task("Northgate: staff training", project=clients, parent=hotel, status="Sampling", due=14, time="10:00", waits=[grinders], estimate=240, checklist=[("Dial in the grinder", False), ("Milk steaming", False), ("Cleaning routine", False)], reminders=[w.remind_before(14, "10:00", 1440)])
    w.task("Fennel & Rye renewal is past due", project=clients, status="Active", due=-5, priority="high", labels=[L["follow up"]], assigned=True, comments=[("Sam has not answered two emails. Calling Thursday. Their current price is under {p:Price list#Wholesale prices}.", 1, "Beth")])
    w.task("Send Tidewater the 300 kg quote", project=clients, status="Quoted", due=0, priority="urgent", my_day=True, assigned=True, comments=[("At 300 kg we need the second shift. See {t:Decide on the second shift}.", 3, "Tomasz")], reminders=[w.remind_at(0, "23:00")], notes="Quote notes:\n\n| Blend | kg a month | Price per kg |\n| --- | ---: | ---: |\n| Foundry House | 200 | 16.40 |\n| Daybreak | 100 | 17.10 |\n\nFreight is theirs.")
    w.task("Sample boxes for The Station Diner", project=clients, status="Sampling", due=1, time="11:00", labels=[L["follow up"]], checklist=[("Night Shift, 250 g", True), ("Daybreak, 250 g", True), ("Tasting card", False), ("Drop off in person", False)])
    w.repeating("Quarterly account review", "FREQ=MONTHLY;INTERVAL=3;BYMONTHDAY=5", project=clients, past_dues=[-90], due=3, status="Quoted", assigned=True, estimate=120, notes="Go through the Active accounts table, biggest first.")

    # Orders.
    orders = w.project("Orders", icon="lucide:shopping-cart", color="orange", description="Every order from the moment it comes in to the invoice. Arjun runs the floor.",
                       statuses=[("New", "todo"), ("Roasting", "in-progress"), ("Bagging", "in-progress"), ("Ready", "in-progress"), ("Shipped", "in-progress"), ("Invoiced", "done"), ("Canceled", "canceled")])
    order_no = w.field("Order number", "number", project=orders)
    client_f = w.field("Client", "text", project=orders)
    kilos = w.field("Kilos", "number", project=orders)
    total = w.field("Invoice total", "number", project=orders)
    paid = w.field("Paid", "checkbox", project=orders)
    ship = w.field("Ship by", "date", project=orders)
    tracking = w.field("Tracking", "url", project=orders)
    active_names = ["Lantern Cafe", "Harbor Street Espresso", "The Copper Pan", "Fennel & Rye", "Greenfield Market", "Blue Door Bakery", "Pier 9 Kiosk", "Northgate Hotel", "Calder Architects"]
    order_ids = []
    for i in range(64):
        no = 5120 + i
        name = active_names[i % len(active_names)]
        kg = dict(zip(active_names, [15, 22, 6, 5, 50, 9, 7, 30, 4]))[name] * r.choice([1, 1, 2])
        day = -38 + i  # ship by: from over a month ago to a little ahead
        names = r.sample(blends_names, r.choice([1, 1, 2]))
        if day < -6:
            st, done = "Invoiced", -day - 1 + r.random()
        elif day < -1:
            st, done = r.choice(["Invoiced", "Shipped", "Shipped"]), None
        elif day <= 1:
            st, done = r.choice(["Bagging", "Ready", "Roasting"]), None
        elif day <= 8:
            st, done = r.choice(["Roasting", "New", "New"]), None
        else:
            st, done = "New", None
        if st == "Invoiced":
            done = max(-day - 1, 1) + r.random()
        if i in (11, 37):
            st, done = "Canceled", 30 - i * 0.5
        vals = {order_no: no, client_f: name, kilos: kg, ship: w.day(day), blend: [BLEND[b] for b in names], total: round(kg * r.uniform(15.5, 19.5), 2)}
        if st in ("Invoiced", "Shipped"):
            vals[tracking] = f"https://example.com/track/BA{no}"
        if st == "Invoiced":
            vals[paid] = i % 5 != 0
        order_ids.append(w.task(f"#{no} {name}: {kg} kg", project=orders, status=st, done_days=done, due=day, values=vals, created_days=max(-day, 0) + 6,
                                priority="urgent" if i % 23 == 0 and st != "Invoiced" else "high" if i % 9 == 0 else "none",
                                labels=[L["wholesale"]] + ([L["rush"]] if i % 11 == 0 else []), assigned=st in ("Roasting", "Bagging") and i % 2 == 0,
                                checklist=[("Roast", st != "New"), ("Cup", st in ("Bagging", "Ready", "Shipped", "Invoiced")), ("Bag and label", st in ("Ready", "Shipped", "Invoiced"))] if i % 4 == 0 else ()))
    late = order_ids[37 + 2]
    w.comment(late, "Green coffee for this one is on {t:Receive the Guji container}. Do not promise a day to the client yet. {@:Beth}", 1, "Tomasz")
    w.comment(order_ids[34], "Client asks for a split delivery. Rules are at {p:Shipping#Split deliveries}.", 2, "Arjun")
    for tid in order_ids[40:43]:
        w.comment(tid, "Roast slot booked. Tracking will be on https://example.com/track/ once it ships.", 1, "Arjun")

    # Roastery operations.
    ops = w.project("Roastery", icon="lucide:coffee", color="amber", description="The machine, the green coffee, food safety and the floor.",
                    statuses=[("Later", "backlog"), ("This month", "todo"), ("Doing", "in-progress"), ("Waiting on supplier", "in-progress"), ("Done", "done"), ("Dropped", "canceled")])
    area, AREA = w.choices("Area", ["Roaster", "Green coffee", "Packing", "Food safety", "Building"], project=ops)
    supplier = w.field("Supplier", "text", project=ops)
    spend = w.field("Spend", "number", project=ops)
    inspect = w.field("Next inspection", "date", project=ops)
    guji = w.task("Receive the Guji container", project=ops, status="Waiting on supplier", due=-2, priority="high", key="Receive the Guji container", values={area: AREA["Green coffee"], supplier: "Sidama Trading", spend: 18400}, comments=[("Stuck at the port: customs paperwork. New ETA Friday, maybe. https://example.com/sidama/shipment/7712", 1, "Tomasz")], files=[("guji-bill-of-lading.pdf", "application/pdf", pdf("Bill of lading, Sidama Trading, 7712"))], labels=[L["quality"]])
    for tid in order_ids[39:44]:
        w.depend(tid, guji)
    second = w.task("Decide on the second shift", project=ops, status="This month", due=6, priority="high", assigned=True, estimate=180, notes="Tidewater alone would take us past what one shift roasts.\n\n{img:capacity.png}\n\nSee {p:Roast profiles#Batch sizes} for the numbers.", files=[("capacity.png", "image/png", bars([62, 70, 74, 81, 88, 96, 104], (180, 110, 60)))], logged=[(-3, 95, "Spreadsheet")], comments=[("I can do three evenings a week to start.", 2, "Arjun"), ("{@:Tomasz} what does this do to the afterburner schedule?", 1)])
    w.task("Hire for the second shift", project=ops, status="Later", waits=[second], notes="Goes to the Hiring project once decided.")
    w.repeating("Clean the roaster's chaff collector", "FREQ=WEEKLY;BYDAY=FR", project=ops, past_dues=[-17, -10, -3], due=w.on("FR"), status="This month", values={area: AREA["Roaster"]}, estimate=45, logged=[(-3, 50, "With the door off"), (-10, 40, "")], labels=[L["equipment"]], checklist=[("Cool down fully", False), ("Empty the chaff", False), ("Vacuum the duct", False)])
    w.repeating("Service the afterburner", "FREQ=MONTHLY;INTERVAL=3", project=ops, past_dues=[-89], due=2, status="Doing", priority="high", mode="completion", values={area: AREA["Roaster"], supplier: "Burnwell Service", spend: 640}, labels=[L["equipment"]], reminders=[w.remind_at(1, "15:00")])
    audit = w.task("Food safety audit", project=ops, status="This month", start=4, due=18, time="09:00", priority="high", labels=[L["compliance"]], values={area: AREA["Food safety"], inspect: w.day(18)}, estimate=16 * 60, notes="The auditor uses the checklist in {p:Food safety#The audit}.",
                   checklist=[("Pest log up to date", False), ("Lot tracing drill", False), ("Allergen statement", False), ("Thermometer calibration", False)], reminders=[w.remind_before(18, "09:00", 7200)])
    w.task("Run the lot tracing drill", project=ops, parent=audit, status="Doing", due=7, estimate=120, assigned=True, logged=[(-1, 30, "Picked lot 2291")], timer=(40, "Tracing lot 2291 back to the green coffee"), my_day=True)
    w.task("Calibrate the thermometers", project=ops, parent=audit, status="This month", due=10, values={area: AREA["Roaster"]})
    w.task("Rewrite the allergen statement", project=ops, parent=audit, status="Done", done_days=3, due=-2, labels=[L["compliance"]])
    w.task("Replace the bag sealer", project=ops, status="Done", done_days=18, due=-20, values={area: AREA["Packing"], spend: 1240, supplier: "Pactec"}, labels=[L["equipment"]], logged=[(-19, 200, "Install and test")], estimate=120)
    w.task("Fix the leaking skylight", project=ops, status="Waiting on supplier", due=12, values={area: AREA["Building"], supplier: "Landlord"}, comments=[("Landlord says \"next week\" again.", 3, "Arjun")])
    w.task("Solar panels", project=ops, status="Dropped", done_days=50, values={area: AREA["Building"]}, notes="Landlord said no.")
    w.task("Second roaster, 15 kg", project=ops, status="Later", values={area: AREA["Roaster"], spend: 48000}, priority="low", notes="Only after the second shift has run for six months.")

    # Marketing.
    mkt = w.project("Marketing", icon="lucide:palette", color="pink", description="Newsletter, social, events and the holiday box. Paolo helps with design.",
                    statuses=[("Idea", "backlog"), ("Drafting", "todo"), ("In review", "in-progress"), ("Scheduled", "in-progress"), ("Published", "done"), ("Dropped", "canceled")])
    channel, CH = w.choices("Channel", ["Newsletter", "Instagram", "Website", "Events", "Print"], multi=True, project=mkt)
    campaign, CMP = w.choices("Campaign", ["Holiday box", "Single origin series", "Wholesale push", "Always on"], project=mkt)
    budget = w.field("Budget", "number", project=mkt)
    publish = w.field("Publish on", "date", project=mkt)
    asset = w.field("Asset", "url", project=mkt)
    box = w.task("Holiday box: decide the three coffees", project=mkt, status="Published", done_days=16, due=-18, priority="high", values={campaign: CMP["Holiday box"]}, labels=[L["holiday"]], comments=[("Going with Guji, Huila and a new Night Shift. {t:Receive the Guji container} had better land.", 16, "Tomasz")])
    design = w.task("Holiday box: packaging design", project=mkt, status="In review", start=-12, due=-1, priority="high", waits=[box], values={campaign: CMP["Holiday box"], channel: [CH["Print"]], budget: 1800, asset: "https://example.com/paolo/holiday-box-v4"}, labels=[L["holiday"]], files=[("box-proof-v4.png", "image/png", png(400, 300, (32, 58, 60), [(40, 40, 320, 220, (196, 150, 90)), (80, 110, 240, 60, (32, 58, 60))]))], comments=[("Proof v4 attached. Fold line is still off by 2 mm.", 1, "Paolo"), ("{@:Paolo} approved once the fold is fixed. Printer needs it by Friday.", 0.5)], notes="{img:box-proof-v4.png}\n\nBrand rules are in {p:Brand voice#Packaging}.")
    printing = w.task("Holiday box: print run", project=mkt, status="Drafting", start=4, due=16, waits=[design], values={campaign: CMP["Holiday box"], channel: [CH["Print"]], budget: 4200}, labels=[L["holiday"], L["cash"]])
    store = w.task("Holiday box: online store listing", project=mkt, status="Drafting", due=9, waits=[design], values={campaign: CMP["Holiday box"], channel: [CH["Website"]], publish: w.day(18)}, assigned=True, labels=[L["holiday"], L["online"]], estimate=180)
    w.task("Holiday box: launch newsletter", project=mkt, status="Drafting", due=18, time="08:00", waits=[store, printing], values={campaign: CMP["Holiday box"], channel: [CH["Newsletter"], CH["Instagram"]], publish: w.day(18)}, labels=[L["holiday"]], reminders=[w.remind_before(18, "08:00", 1440)])
    w.task("Holiday box: wholesale sell-in to grocers", project=mkt, status="Idea", due=20, values={campaign: CMP["Holiday box"]}, comments=[("Greenfield and Tidewater first, see {t:Send Tidewater the 300 kg quote}.", 3, "Beth")])
    posts = ["Roaster's notes: why we rest Guji for a week", "Meet Tomasz: twenty years of first crack", "How to dial in Daybreak at home", "Behind the Foundry House blend", "What decaf actually is", "Grinder cleaning, in 60 seconds", "Our origin trip to Huila, part 2", "Coffee and cardamom buns: a pairing", "The chaff video", "Shop dog of the month"]
    for i, title in enumerate(posts):
        day = -28 + i * 6
        st = "Published" if day < -3 else "In review" if day < 2 else "Scheduled" if day < 10 else "Drafting"
        w.task(title, project=mkt, status=st, done_days=-day if st == "Published" else None, due=day, time="09:00" if i % 3 == 0 else None, created_days=-day + 14 if day < 0 else 10, values={campaign: CMP["Single origin series" if i % 2 else "Always on"], channel: [CH["Instagram"]] + ([CH["Newsletter"]] if i % 3 == 0 else []) + ([CH["Website"]] if i % 4 == 0 else []), publish: w.day(day)}, estimate=90, logged=[(min(day, 0) - 1, 60 + i * 10, "Writing and photos")] if st == "Published" else ())
    w.task("Wholesale one-pager", project=mkt, status="In review", due=2, values={campaign: CMP["Wholesale push"], channel: [CH["Print"]], budget: 180, asset: "https://example.com/paolo/one-pager"}, notes="Prices come from {p:Price list}. Keep them off the PDF if they might change.")
    w.task("Coffee festival booth", project=mkt, status="Scheduled", start=40, due=42, values={campaign: CMP["Always on"], channel: [CH["Events"]], budget: 950}, checklist=[("Booth fee paid", True), ("Banner", False), ("Sample cups", False), ("Rota", False)], labels=[L["cash"]])
    w.task("Billboard on Route 9", project=mkt, status="Dropped", done_days=25, values={budget: 6000}, comments=[("No. Just no.", 25, "Tomasz")])
    w.task("Loyalty card idea", project=mkt, status="Idea", priority="low", values={channel: [CH["Print"], CH["Website"]]})
    w.task("Refresh the About page", project=mkt, status="Idea", values={channel: [CH["Website"]]}, deleted_days=2, notes="In the trash on purpose.")

    # Hiring.
    hiring = w.project("Hiring", icon="lucide:briefcase", color="purple", description="Two open roles. Candidates are made up.",
                       statuses=[("Applied", "backlog"), ("Phone screen", "todo"), ("Trial shift", "in-progress"), ("Offer", "in-progress"), ("Hired", "done"), ("Passed", "canceled")])
    role, ROLE = w.choices("Role", ["Production assistant", "Delivery driver"], project=hiring)
    source, SRC = w.choices("Source", ["Referral", "Job board", "Walked in", "Instagram"], project=hiring)
    rating = w.field("Rating", "number", project=hiring)
    resume = w.field("Resume", "url", project=hiring)
    trial = w.field("Trial shift", "date", project=hiring)
    refs = w.field("References checked", "checkbox", project=hiring)
    for i, (name, rl, src, st, score, tday) in enumerate([
            ("Candidate: Noor Haddad", "Production assistant", "Referral", "Offer", 4.5, -6), ("Candidate: Ben Sato", "Production assistant", "Job board", "Trial shift", 3.5, 3),
            ("Candidate: Clara Nowak", "Production assistant", "Walked in", "Trial shift", 4.0, 1), ("Candidate: Devon Price", "Production assistant", "Job board", "Phone screen", None, None),
            ("Candidate: Ife Adebayo", "Production assistant", "Instagram", "Phone screen", None, None), ("Candidate: Gus Lindahl", "Production assistant", "Job board", "Applied", None, None),
            ("Candidate: Maya Roth", "Production assistant", "Job board", "Passed", 2.5, -14), ("Candidate: Omar Fayed", "Delivery driver", "Referral", "Hired", 5.0, -21),
            ("Candidate: Tess Morgan", "Delivery driver", "Job board", "Passed", 3.0, -18), ("Candidate: Rob Kerr", "Delivery driver", "Walked in", "Passed", None, None),
            ("Candidate: Lucia Fontana", "Production assistant", "Job board", "Applied", None, None), ("Candidate: Sean Doyle", "Production assistant", "Job board", "Applied", None, None)]):
        vals = {role: ROLE[rl], source: SRC[src], resume: f"https://example.com/applicants/{i + 1}"}
        if score is not None:
            vals[rating] = score
        if tday is not None:
            vals[trial] = w.day(tday)
        if st in ("Offer", "Hired"):
            vals[refs] = st == "Hired"
        w.task(name, project=hiring, status=st, done_days=10 + i if st in ("Hired", "Passed") else None, values=vals, due=tday if st == "Trial shift" else 2 if st == "Offer" else None, created_days=30 - i, assigned=st == "Offer")
    noor = w.task_names["Candidate: Noor Haddad"]
    w.comment(noor, "Fast, careful and asked good questions about the cooling tray. Offer per {p:Job descriptions#Production assistant}. {@:Arjun} agrees?", 4)
    w.comment(noor, "Agreed. She spotted the sticky damper without being told.", 3, "Arjun")
    w.comment(noor, "(deleted) Meant for the other Noor, sorry.", 3, "Beth")
    w.attach(noor, "old-resume-scan.pdf", "application/pdf", pdf("Superseded resume scan"), deleted_days=4)
    w.attach(noor, "noor-haddad-resume.pdf", "application/pdf", pdf("Noor Haddad, resume"))
    w.task("Check Noor's references", project=hiring, parent=noor, status="Trial shift", due=1, assigned=True, my_day=True, checklist=[("Last manager", True), ("Second reference", False)])
    w.task("Post the production job on the board", project=hiring, status="Hired", done_days=26, due=-27, values={role: ROLE["Production assistant"]}, logged=[(-27, 45, "Ad copy")])
    w.task("Set up payroll for Omar", project=hiring, status="Offer", due=-2, priority="high", comments=[("{@:Lena} needs his W-4. See {t:Payroll: November first half}.", 3)], values={role: ROLE["Delivery driver"]})

    # Bookkeeping.
    books = w.project("Bookkeeping", icon="lucide:piggy-bank", color="lime", description="Invoices, payroll, sales tax and the month-end close. Lena does this two days a week.",
                      statuses=[("To do", "todo"), ("Waiting on docs", "in-progress"), ("With Lena", "in-progress"), ("Filed", "done"), ("Not needed", "canceled")])
    period, PER = w.choices("Period", ["August", "September", "October", "November", "Q3", "Q4", "2026"], project=books)
    amount = w.field("Amount", "number", project=books)
    gl, GL = w.choices("Account", ["Sales", "Green coffee", "Payroll", "Freight", "Rent", "Marketing", "Sales tax"], project=books)
    recon = w.field("Reconciled", "checkbox", project=books)
    filing = w.field("File by", "date", project=books)
    close_list = [("Bank feeds imported", True), ("Invoices matched", True), ("Inventory counted", False), ("Accruals posted", False), ("Owner review", False)]
    w.task("Close October", project=books, status="With Lena", start=1, due=8, priority="high", values={period: PER["October"], recon: False}, checklist=close_list, assigned=True, estimate=8 * 60, logged=[(-1, 120, "Bank feeds", "Lena")], notes="Follows {p:Month-end close}.", files=[("october-trial-balance.csv", *csv_file("account,debit,credit", "Sales,,48210.55", "Green coffee,21840.10,", "Payroll,14200.00,", "Freight,2210.45,", "Rent,3400.00,"))], comments=[("{@:Lena} inventory count is booked for Tuesday night.", 1)])
    for name, days, per in [("Close September", -28, "September"), ("Close August", -60, "August")]:
        w.task(name, project=books, status="Filed", done_days=-days, due=days, values={period: PER[per], recon: True}, checklist=[(c, True) for c, _ in close_list], logged=[(days - 2, 240, "Close", "Lena"), (days - 1, 180, "Close", "Lena")], estimate=8 * 60, created_days=-days + 10)
    w.task("Sales tax: Q3 return", project=books, status="Filed", done_days=11, due=-12, priority="high", values={period: PER["Q3"], gl: GL["Sales tax"], amount: 3128.40, filing: w.day(-12)}, labels=[L["compliance"]])
    w.task("Sales tax: Q4 return", project=books, status="To do", start=60, due=75, values={period: PER["Q4"], gl: GL["Sales tax"], filing: w.day(75)}, labels=[L["compliance"]])
    w.repeating("Payroll: November first half", "FREQ=WEEKLY;INTERVAL=2;BYDAY=FR", project=books, past_dues=[-24, -10], due=w.on("FR"), status="To do", priority="high", values={gl: GL["Payroll"], amount: 7120.0}, checklist=[("Hours from the time sheet", False), ("New starters", False), ("Approve", False)], reminders=[w.remind_at(w.on("TH"), "16:00")], assigned=True)
    w.task("Chase overdue invoices", project=books, status="To do", due=0, priority="high", repeat="FREQ=WEEKLY;BYDAY=MO", values={gl: GL["Sales"]}, labels=[L["cash"]], assigned=True, comments=[("The Unpaid invoices view in Orders lists them. Biggest first.", 3, "Lena")], estimate=45)
    w.task("Pay Sidama Trading", project=books, status="Waiting on docs", due=-1, priority="urgent", values={gl: GL["Green coffee"], amount: 18400}, labels=[L["cash"]], waits=[guji], comments=[("Do not pay until the container clears: {t:Receive the Guji container}.", 2, "Tomasz")])
    w.task("Rent for the unit", project=books, status="To do", due=(today.replace(day=1) + dt.timedelta(days=32)).replace(day=1).toordinal() - today.toordinal(), repeat="FREQ=MONTHLY;BYMONTHDAY=1", values={gl: GL["Rent"], amount: 3400})
    w.task("Year-end: fixed asset register", project=books, status="To do", start=30, due=58, values={period: PER["2026"]}, estimate=6 * 60)
    w.task("Loan covenant report", project=books, status="Waiting on docs", due=4, values={period: PER["Q3"]}, notes=RICH_NOTES.split("### A smaller")[0].replace("What this note is for", "What the bank wants"))
    w.task("Old merchant account: close it", project=books, status="Not needed", done_days=35, notes="Closed by the bank itself.")
    w.task("Software subscription refunds", project=books, status="With Lena", values={gl: GL["Marketing"], amount: -149.0})

    # Finished and gone.
    box25 = w.project("Holiday box 2025", icon="lucide:gift", color="red", archived_days=250, description="Last year's box. Archived; useful to look back at.")
    for i, t in enumerate(["Choose coffees", "Print 800 boxes", "Fill and seal", "Ship online orders", "Count what was left"]):
        w.task(t, project=box25, status="Done", done_days=300 - i * 8, created_days=340, values={blend: [BLEND["Night Shift"]]})
    w.task("Refund the last damaged holiday box", project=box25, due=-1, priority="high",
           notes="Still open, in an archived project. It stays out of Upcoming, My Day's suggestions and the palette; open the project to reach it.")
    w.task("Sell the leftover 2025 sleeves", project=box25, due=6)
    pop = w.project("Farmers market stall", icon="lucide:sprout", color="green", deleted_days=8, description="In the trash. The market closed for the winter.")
    w.task("Apply for a stall", project=pop)
    w.task("Buy a folding table", project=pop, status="Done", done_days=30)

    # The inbox.
    w.task("Call the insurance broker about the new van", due=1, priority="high", time="10:30", assigned=True, reminders=[w.remind_before(1, "10:30", 30)])
    w.task("Read the lease renewal", due=-4, priority="medium", notes="The rent clause is on page 6.", files=[("lease-renewal-2027.pdf", "application/pdf", pdf("Unit 14 lease renewal"))])
    w.task("Thank-you note to Lantern Cafe", due=0, my_day=True)
    w.task("Monday stand-up", due=0, time="08:30", repeat="FREQ=WEEKLY;BYDAY=MO", checklist=[("Orders for the week", False), ("Who is out", False), ("Anything broken", False)], priority="low")
    w.task("Idea: subscription for offices", notes="Calder asked about one.")
    w.task("Renew the domain", done_days=3, due=-3)
    w.task("Order more bag labels", due=4, labels=[L["online"]], my_day=True, my_day_on=-1)

    # Saved views.
    w.view("Today and overdue", "list", filter={"due": "today"}, display={"group": {"by": "project"}, "sort": [{"by": "priority", "dir": "desc"}], "fields": {"list": {"show": ["project", "due", "assignees", "labels"]}}})
    w.view("Due this week", "calendar", filter={"due": "week"})
    w.view("Stuck behind something", "list", filter={"dependencies": ["blocked"]}, display={"group": {"by": "project"}, "sort": [{"by": "due", "dir": "asc"}]})
    w.view("Mine", "list", filter={"assignees": ["me"]}, display={"group": {"by": "due"}}, private=True)
    # Time on the projects themselves, not on any task.
    w.log_project_time(ops, -1, 60, "Weekly floor meeting", "Tomasz Wolny")
    w.log_project_time(ops, -8, 60, "Weekly floor meeting", "Tomasz Wolny")
    w.log_project_time(clients, -2, 45, "Pipeline review")
    w.log_project_time(mkt, -4, 90, "Planning the holiday box", "Beth Alvarez")
    w.log_project_time(books, -6, 30, "Month-end call with the accountant")
    w.view("Everyone's hours lately", "table", filter={"time": ["recent"], "showDone": True}, display={"group": {"by": "project"}, "sort": [{"by": "logged", "dir": "desc"}], "fields": {"table": {"show": ["logged", "estimate", "remaining"]}}, "table": {"summaries": [{"col": "logged", "fn": "sum"}]}})
    w.view("No estimate yet", "list", filter={"time": ["unestimated"], "statuses": ["in-progress"]}, display={"group": {"by": "project"}})
    w.view("Holiday push", "timeline", filter={"labels": [L["holiday"]], "showDone": True}, display={"timeline": {"zoom": "week"}, "group": {"by": "status"}})
    w.view("All projects report", "report", filter={"labels": [L["holiday"]]}, display={"report": {"tab": "flow", "period": "months3", "from": None, "to": None, "project": None}})
    w.view("Everything by label", "list", filter={"labels": [L["rush"], L["follow up"], L["cash"]]}, display={"group": {"by": "label"}, "fields": {"list": {"show": ["project", "due", "labels"]}}})
    w.view("Urgent and estimated", "list", filter={"priorities": ["urgent", "high"], "time": ["estimated"]}, display={"sort": [{"by": "estimate", "dir": "desc"}, {"by": "remaining", "dir": "desc"}], "group": {"by": "assignee"}, "fields": {"list": {"show": ["estimate", "remaining", "priority", "assignees"]}}})
    w.view("Over budget on time", "table", filter={"time": ["over", "mine"], "showDone": True}, display={"sort": [{"by": "start", "dir": "asc"}], "table": {"summaries": [{"col": "title", "fn": "count"}, {"col": "logged", "fn": "max"}]}})
    w.view("Not started", "list", filter={"statuses": ["backlog"], "showDone": False}, display={"group": {"by": "status"}, "sort": [{"by": "created", "dir": "desc"}, {"by": "updated", "dir": "desc"}, {"by": "title", "dir": "asc"}]})
    w.view("By account owner", "board", project=clients, filter={"projects": [clients]}, display={"columns": {"by": "field", "field": owner}, "fields": {"board": {"show": [owner, site, signed, "reminders", "comments"]}}})
    w.view("Owned by me", "list", project=clients, filter={"projects": [clients], "fields": [{"field": owner, "op": "any", "value": ["me"]}]}, display={"group": {"by": "field", "field": owner}, "sort": [{"by": "field", "field": owner, "dir": "asc"}, {"by": "field", "field": site, "dir": "asc"}, {"by": "field", "field": signed, "dir": "desc"}]})
    w.view("Nobody owns it", "list", project=clients, filter={"projects": [clients], "fields": [{"field": owner, "op": "empty"}]}, display={"sort": [{"by": "field", "field": ctype, "dir": "asc"}]})
    w.view("Signed, not a cafe", "list", project=clients, filter={"projects": [clients], "showDone": True, "fields": [{"field": signed, "op": "checked"}, {"field": ctype, "op": "none_of", "value": [CT["Cafe"]]}, {"field": blend, "op": "none_of", "value": [BLEND["Decaf Swiss Water"]]}]})
    w.view("Foundry and Daybreak", "list", project=clients, filter={"projects": [clients], "showDone": True, "fields": [{"field": blend, "op": "all", "value": [BLEND["Foundry House"], BLEND["Daybreak"]]}]})
    w.view("Small accounts", "list", project=clients, filter={"projects": [clients], "showDone": True, "fields": [{"field": monthly, "op": "lte", "value": 12}, {"field": contact, "op": "not_contains", "value": "Calder"}, {"field": site, "op": "contains", "value": "example.com"}]}, display={"fields": {"list": {"show": [monthly, site, contact]}}})
    w.view("Renewal soon or lapsed", "table", project=clients, filter={"projects": [clients], "showDone": True, "fields": [{"field": renewal, "op": "before", "value": 30}]}, display={"sort": [{"by": "field", "field": renewal, "dir": "asc"}], "table": {"order": [renewal, monthly, contact], "summaries": [{"col": monthly, "fn": "min"}, {"col": contact, "fn": "none"}, {"col": renewal, "fn": "overdue"}]}})
    w.view("Sixty kilo accounts", "list", project=clients, filter={"projects": [clients], "fields": [{"field": monthly, "op": "eq", "value": 60}]})
    w.view("Last year's pipeline", "board", project=clients, filter={"projects": [clients], "text": "2025"}, deleted_days=40)
    w.view("Pipeline", "board", project=clients, filter={"projects": [clients], "showDone": True}, display={"fields": {"board": {"show": [monthly, ctype, contact, "due"]}}})
    w.view("Active accounts", "table", project=clients, filter={"projects": [clients], "statuses": ["in-progress", "done"]}, display={"group": {"by": "field", "field": ctype}, "sort": [{"by": "field", "field": monthly, "dir": "desc"}], "table": {"order": [monthly, ctype, terms, renewal, signed, contact, site, blend, owner], "widths": [{"col": "title", "px": 260}, {"col": monthly, "px": 90}], "summaries": [{"col": monthly, "fn": "sum"}, {"col": renewal, "fn": "earliest"}, {"col": signed, "fn": "filled"}]}})
    w.view("Renewals", "calendar", project=clients, filter={"projects": [clients], "fields": [{"field": renewal, "op": "not_empty"}]})
    w.view("Big accounts", "list", project=clients, filter={"projects": [clients], "fields": [{"field": monthly, "op": "gte", "value": 50}]}, display={"sort": [{"by": "field", "field": monthly, "dir": "desc"}], "fields": {"list": {"show": [monthly, ctype, "status"]}}})
    w.view("Foundry House accounts", "list", project=clients, filter={"projects": [clients], "fields": [{"field": blend, "op": "any", "value": [BLEND["Foundry House"]]}]})
    w.view("The floor", "board", project=orders, filter={"projects": [orders]}, display={"fields": {"board": {"show": [kilos, blend, ship, "checklist"]}}, "sort": [{"by": "field", "field": ship, "dir": "asc"}]})
    w.view("Shipping this week", "list", project=orders, filter={"projects": [orders], "fields": [{"field": ship, "op": "between", "value": [-1, 7]}]}, display={"sort": [{"by": "field", "field": ship, "dir": "asc"}], "group": {"by": "status"}, "fields": {"list": {"show": [client_f, kilos, ship, "status"]}}})
    w.view("Friday's shipments", "list", project=orders, filter={"projects": [orders], "fields": [{"field": ship, "op": "on", "value": 4}]})
    w.view("Unpaid invoices", "table", project=orders, filter={"projects": [orders], "showDone": True, "statuses": ["done"], "fields": [{"field": paid, "op": "unchecked"}]}, display={"sort": [{"by": "field", "field": total, "dir": "desc"}], "table": {"order": [client_f, total, paid, tracking, "due"], "summaries": [{"col": total, "fn": "sum"}, {"col": "due", "fn": "range"}]}})
    w.view("Order table", "table", project=orders, filter={"projects": [orders], "showDone": True}, display={"sort": [{"by": "field", "field": order_no, "dir": "desc"}], "table": {"order": [order_no, client_f, kilos, total, blend, ship, "status"], "summaries": [{"col": kilos, "fn": "sum"}, {"col": total, "fn": "avg"}, {"col": "status", "fn": "bar"}]}})
    w.view("Ship calendar", "calendar", project=orders, filter={"projects": [orders], "showDone": True})
    w.view("Roastery by area", "board", project=ops, filter={"projects": [ops]}, display={"columns": {"by": "field", "field": area}, "fields": {"board": {"show": [supplier, spend, "due"]}}})
    w.view("Audit timeline", "timeline", project=ops, filter={"projects": [ops], "labels": [L["compliance"]], "showDone": True}, display={"timeline": {"zoom": "month"}})
    w.view("Roastery by priority", "board", project=ops, filter={"projects": [ops]}, display={"columns": {"by": "priority"}})
    w.view("By campaign", "board", project=mkt, filter={"projects": [mkt], "showDone": True}, display={"columns": {"by": "field", "field": campaign}, "fields": {"board": {"show": [channel, publish]}}})
    w.view("Publishing calendar", "calendar", project=mkt, filter={"projects": [mkt], "showDone": True})
    w.view("Holiday box plan", "timeline", project=mkt, filter={"projects": [mkt], "text": "Holiday box", "showDone": True}, display={"timeline": {"zoom": "week"}})
    w.view("Spend", "table", project=mkt, filter={"projects": [mkt], "fields": [{"field": budget, "op": "not_empty"}], "showDone": True}, display={"table": {"order": [budget, campaign, channel, "status"], "summaries": [{"col": budget, "fn": "sum"}, {"col": budget, "fn": "max"}]}})
    w.view("Candidates", "board", project=hiring, filter={"projects": [hiring], "showDone": True}, display={"fields": {"board": {"show": [role, rating, source]}}, "sort": [{"by": "field", "field": rating, "dir": "desc"}]})
    w.view("Best first", "table", project=hiring, filter={"projects": [hiring], "fields": [{"field": rating, "op": "gt", "value": 3}], "showDone": True}, display={"sort": [{"by": "field", "field": rating, "dir": "desc"}], "group": {"by": "field", "field": role}, "table": {"order": [rating, role, source, trial, refs, resume], "summaries": [{"col": rating, "fn": "avg"}]}})
    w.view("My candidates", "list", project=hiring, filter={"projects": [hiring], "assignees": ["me"]}, private=True)
    w.view("Close calendar", "calendar", project=books, filter={"projects": [books], "showDone": True})
    w.view("Unreconciled", "table", project=books, filter={"projects": [books], "fields": [{"field": recon, "op": "unchecked"}]}, display={"group": {"by": "field", "field": gl}, "table": {"order": [amount, gl, period, recon, filing, "due"], "summaries": [{"col": amount, "fn": "sum"}]}})
    w.view("Year at a glance", "timeline", project=books, filter={"projects": [books], "showDone": True}, display={"timeline": {"zoom": "quarter"}})

    # Themes.
    w.theme("Dark roast", base="calm", hue=45, suggested_days=3, tokens={"neutral-hue": {"hue": 50}, "neutral-chroma": {"n": 0.02}, "radius-md": {"px": 4}, "font-heading": {"font": "source-serif"}})
    w.theme("Packaging proof", base="playful", hue=200, tokens={"card-radius": {"ref": "radius-sm"}, "dur-panel": {"ms": 200}})
    w.theme("High contrast floor screen", base="contrast", target="AAA")
    w.theme("Old logo blue", base="standard", hue=240, deleted_days=30)

    # Templates.
    t1, t2, t3 = w.id(), w.id(), w.id()
    w.template("task", "New wholesale client", dict(v=1, tasks=[
        tpl_task(t1, "Onboard the new client", priority="high", category="todo", due=14, estimate=240, labels=[dict(id=L["wholesale"], name="wholesale", color="blue")], assignees=[EXPORTER, w.person_ids["Beth"]],
                 checklist=["Credit check", "Contract signed", "First order placed", "Added to the Monday route"], values={owner: EXPORTER}, notes="Terms and prices: {p:Price list#Wholesale terms}."),
        tpl_task(t2, "Sampling and tasting", parent=t1, due=3, estimate=120),
        tpl_task(t3, "Install grinder and train staff", parent=t1, due=10, waits=[t2], estimate=240),
    ]), days=20)
    w.template("task", "Weekly newsletter", dict(v=1, tasks=[tpl_task(w.id(), "Weekly newsletter", category="todo", due=4, recurrence=None, checklist=["Pick the coffee of the week", "Photo", "Write 150 words", "Schedule for 8:00"], labels=[dict(id=L["online"], name="online", color="cyan")], estimate=90)]))
    pid, s1, s2, s3, s4 = w.id(), w.id(), w.id(), w.id(), w.id()
    f1, f2 = w.id(), w.id()
    a, b, c, d = w.id(), w.id(), w.id(), w.id()
    main_id = pair_id(pid, MAIN_PAGE_SPACE)
    sub = w.id()
    w.template("project", "Seasonal blend launch", dict(v=1, project=dict(
        id=pid, name="Seasonal blend launch", description="From first cupping to the shelf, about six weeks.", icon="lucide:coffee", color="amber",
        statuses=[dict(id=s1, name="Cupping", category="backlog"), dict(id=s2, name="To do", category="todo"), dict(id=s3, name="Doing", category="in-progress"), dict(id=s4, name="On the shelf", category="done")],
        fields=[dict(id=f1, name="Lot", type="text", options=[]), dict(id=f2, name="Green cost per kg", type="number", options=[])],
        views=[dict(name="Launch board", type="board", filter={"projects": [pid]}, display={"fields": {"board": {"show": [f1, f2, "due"]}}}),
               dict(name="Six weeks", type="timeline", filter={"projects": [pid]}, display={"timeline": {"zoom": "week"}})],
        pages=[dict(id=main_id, parent=None, title="", body="Why this blend, and for whom."),
               dict(id=sub, parent=main_id, title="Roast profile", body="## First crack\n\nWrite the curve down here.")]),
        tasks=[tpl_task(a, "Cup the shortlist", status=s1, category="backlog", start=0, due=5, estimate=240, values={f2: 8.2}),
               tpl_task(b, "Lock the roast profile", status=s2, category="todo", start=6, due=14, waits=[a], estimate=180),
               tpl_task(c, "Bag design and label", status=s2, category="todo", start=10, due=24, waits=[a], labels=[dict(id=L["holiday"], name="holiday", color="pink")]),
               tpl_task(d, "Launch newsletter and shelf talkers", status=s2, category="todo", due=40, waits=[b, c], checklist=["Newsletter", "Instagram", "Wholesale email"])]), days=45)

    w.template("task", "Market stall weekend", dict(v=1, tasks=[tpl_task(w.id(), "Market weekend", category="todo", checklist=["Table", "Cups", "Float"])]), days=120, deleted_days=8)
    w.taken_off(w.task_names["Fix the leaking skylight"], label=L["rush"], blocker=w.task_names["Close October"], field=contact, value="The old landlord")

    # The wiki.
    top = w.main_page(name="Company home", body="""How Bright Anvil runs. New here? Read {p:Your first week}.

## This month

1. Get the holiday box out: {t:Holiday box: packaging design}
2. Close October: {t:Close October}
3. Decide on {t:Decide on the second shift}

## Who does what

| Person | Runs |
| --- | --- |
| Imani | The company, and the money |
| Tomasz | Roasting and quality |
| Beth | Clients |
| Arjun | The floor |
| Lena | The books, Tuesdays and Thursdays |
""")
    people = w.page("People", top, body="What we promise each other.\n\n## Hours\n\nThe floor runs 6:30 to 15:00. Office hours are looser.\n\n## Time off\n\nTwo weeks' notice in the holiday rush. Otherwise ask Imani.")
    w.page("Your first week", people, body="- [ ] Food handler card\n- [ ] Safety walk with Arjun\n- [ ] Cup with Tomasz on Friday\n- [ ] Read {p:Roasting}\n\n> [!TIP]\n> Bring your own mug. The good ones get stolen.", versions=[("Food handler card. Safety walk.", 200, ""), ("- [ ] Food handler card\n- [ ] Safety walk with Arjun", 90, "Arjun")])
    craft = w.page("Procedures", top, body="How the work is done, step by step. Change these when the work changes.")
    w.page("Receiving green coffee", craft, body="## Check\n\n1. Bags match the bill of lading\n2. Moisture under 12%\n\n## Log\n\nEnter the lot in the sheet. Current shipment: {t:Receive the Guji container}.")
    roast = w.page("Roasting", craft, body="## Before the first batch\n\n- [ ] Afterburner at temperature\n- [ ] Chaff collector clean (see {t:Clean the roaster's chaff collector})\n\n## Each batch\n\nUse {p:Roast profiles}. Log the curve.")
    w.page("Roast profiles", roast, body="""## Batch sizes

The 12 kg roaster runs full at 10 kg. That is what {t:Decide on the second shift} is working from.

## Profiles

| Coffee | Charge | First crack | Drop | Rest |
| --- | ---: | ---: | ---: | --- |
| Foundry House | 200 C | 9:30 | 12:10 | 3 days |
| Night Shift | 205 C | 9:10 | 13:00 | 2 days |
| Ethiopia Guji | 195 C | 8:50 | 10:40 | 7 days |
| Decaf Swiss Water | 185 C | 8:20 | 10:10 | 2 days |

## The log export

```python
import csv
with open("roast-log.csv") as f:
    for row in csv.DictReader(f):
        print(row["batch"], row["drop_temp"])
```

{img:roast-curve.png}
""", versions=[("Foundry House: 12:10 drop.", 120, ""), ("## Profiles\n\nFoundry House: 12:10 drop.\nNight Shift: 13:00 drop.", 60, "Tomasz"), ("## Batch sizes\n\nFull at 10 kg.\n\n## Profiles\n\n| Coffee | Drop |\n| --- | --- |\n| Foundry House | 12:10 |\n| Night Shift | 13:00 |", 14, "Tomasz")])
    w.page_file(w.page_names["Roast profiles"], "roast-curve.png", "image/png", bars([120, 150, 170, 185, 195, 203, 210, 214, 217], (160, 90, 50)))
    w.page("Bagging", craft, body="Weigh, seal, label, date. Rest times are on {p:Roast profiles#Profiles}.")
    w.page("Shipping", craft, body="## Normal orders\n\nMonday and Thursday routes.\n\n## Split deliveries\n\nOnly over 50 kg, and the freight is charged twice.\n\n## Couriers\n\nAccount numbers are in the safe, not here.")
    food = w.page("Food safety", craft, body="## The audit\n\nOnce a year: {t:Food safety audit}.\n\n- Pest log\n- Lot tracing\n- Allergens\n- Thermometers\n\n## Allergens\n\nNo nuts on the floor, ever.")
    w.page_file(food, "pest-log-template.csv", "text/csv", csv_file("date,trap,found,initials", "2026-10-01,1,no,AM")[1])
    w.page("Price list", top, body="""## Wholesale prices

| Blend | per kg | Over 100 kg |
| --- | ---: | ---: |
| Foundry House | 17.20 | 16.40 |
| Daybreak | 17.90 | 17.10 |
| Night Shift | 18.10 | 17.30 |
| Ethiopia Guji | 24.50 | 23.20 |

## Wholesale terms

New accounts prepay for three months, then Net 15. Hotels and grocers get Net 30 after a credit check.
""", versions=[("Foundry House 16.80.", 300, ""), ("## Wholesale prices\n\nFoundry House 17.20.\n\n## Wholesale terms\n\nPrepay.", 70, "Beth")])
    w.page_file(w.page_names["Roast profiles"], "roast-curve-old.png", "image/png", bars([110, 140, 160, 180], (150, 150, 150)), deleted_days=20)
    w.page("Old van rota", top, deleted_days=4, body="We have a driver now. In the trash on purpose.")

    amain = w.main_page(box25, name="Holiday box 2025 wiki", body="What we would do again, kept with the archived project. See {p:Box 2025 numbers}.")
    w.page("Box 2025 numbers", amain, project=box25, body="| Item | Count |\n| --- | ---: |\n| Boxes printed | 800 |\n| Boxes sold | 741 |")
    hmain = w.main_page(hiring, name="Hiring wiki", body="How we hire: a phone screen, a paid trial shift, references, then an offer.")
    w.page("Job descriptions", hmain, project=hiring, body="## Production assistant\n\n$22 an hour, 6:30 to 15:00. Heavy lifting, careful hands.\n\n## Delivery driver\n\n$21 an hour plus van. Clean license.")
    w.page("Interview guide", hmain, project=hiring, body="## Phone screen\n\n1. Why coffee?\n2. Can you lift 30 kg?\n\n## Trial shift\n\nPay them for it. Watch how they handle the cooling tray.\n\nCurrent candidates: see {t:Candidate: Noor Haddad}.")
    mmain = w.main_page(mkt, name="Marketing wiki", body="Brand, calendar and what we are pushing now.")
    w.page("Brand voice", mmain, project=mkt, body="## How we sound\n\nPlain and a little dry. No \"artisanal\".\n\n## Packaging\n\nTwo colors at most. The anvil stays at the top left.")
    w.page("Holiday box brief", mmain, project=mkt, body="800 boxes at $48. Three coffees, one card.\n\nWork starts with {t:Holiday box: decide the three coffees}.")
    bmain = w.main_page(books, name="Bookkeeping wiki", body="For Lena, and for whoever covers for Lena.")
    w.page("Month-end close", bmain, project=books, body="1. Import the bank feeds\n2. Match invoices to the Orders project\n3. Count inventory\n4. Post accruals\n5. Owner review\n\n> [!NOTE]\n> The close should be done by the 8th.")
    w.page("Chart of accounts", bmain, project=books, body="| Code | Name |\n| --- | --- |\n| 4000 | Sales |\n| 5000 | Green coffee |\n| 6100 | Payroll |\n| 6200 | Freight |\n| 6300 | Rent |")
    return w


def enterprise(today: dt.date, now: dt.datetime, rng: random.Random) -> Workspace:
    """A large company with six launches in different phases and shared teams they all wait on: real load."""
    cast = ["Dana Whitfield", "Rohan Pillai", "Sofia Marchetti", "Kenji Arai", "Amara Obi", "Lukas Brandt", "Hye-jin Seo", "Mateo Vargas",
            "Fatima Zahra", "Owen Gallagher", "Nadia Petrova", "Tunde Bakare", "Elise Moreau", "Sanjay Kulkarni", "Greta Holm", "Chidi Eze",
            "Marisol Reyes", "Paul Novak", "Ingrid Solberg", "Wei Zhang", "Leila Haddad", "Bram de Vries", "Yara Nassar", "Caleb Stone"]
    w = Workspace("Corvane Industries", "enterprise", today, now, rng,
                  [(cast[0], "owner")] + [(c, "admin" if i < 2 else "member") for i, c in enumerate(cast[1:-2])] + [(cast[-2], "guest"), (cast[-1], "guest")])
    r = rng
    firsts = [c.split()[0] for c in cast[1:]]

    # Labels.
    LABELS = {n: w.label(n, c) for n, c in [
        ("launch-critical", "red"), ("exec-visible", "orange"), ("customer-facing", "pink"), ("tech-debt", "amber"), ("needs-decision", "yellow"),
        ("cross-team", "indigo"), ("regulatory", "purple"), ("quick-win", "lime"), ("design", "cyan"), ("security", "teal"), ("localization", "blue"), ("vendor", "green"),
        ("documentation", "cyan"), ("data", "teal"), ("experiment", "lime"), ("regression", "red")]}
    w.label("fy25-carryover (deleted)", "yellow", deleted=True)

    # Fields on every project.
    TEAMS = ["Firmware", "Hardware", "Cloud", "Mobile", "QA", "Design", "Product", "Legal", "Marketing", "Supply Chain", "Security", "Support", "Data", "Finance"]
    team, TEAM = w.choices("Team", TEAMS)
    LAUNCHES = ["Atlas X1", "Harbor 4.0", "Beacon 5", "Juniper Hub", "Ridgeline", "Solstice EU", "Cross-launch"]
    launch, LAUNCH = w.choices("Launch", LAUNCHES)
    gate, GATE = w.choices("Gate", ["G0 Concept", "G1 Feasibility", "G2 Plan", "G3 Build", "G4 Validate", "G5 Ship"])
    risk, RISK = w.choices("Risk", ["Low", "Medium", "High", "Critical"])
    regions, REGION = w.choices("Regions", ["North America", "EMEA", "APAC", "LATAM"], multi=True)
    dri = w.field("DRI", "text")
    tracker = w.field("Tracker ID", "text")
    spec = w.field("Spec", "url")
    visible = w.field("Customer visible", "checkbox")
    budget = w.field("Budget USD", "number")
    committed = w.field("Committed date", "date")
    sponsor = w.field("Exec sponsor", "user")
    w.field("Legacy WBS code (retired)", "text", deleted=True)

    def workflow(names: str) -> list[tuple[str, str]]:
        """Statuses from a line such as "Backlog:backlog, Ready:todo"."""
        return [tuple(s.strip().split(":")) for s in names.split(",")]

    number = [1000]

    def tracker_id(prefix: str) -> str:
        number[0] += r.randint(1, 4)
        return f"{prefix}-{number[0]}"

    class Launch:
        """A project's way of making tasks: its workflow, and how far along and how late it is."""

        def __init__(self, name, prefix, tag, phase_gate, *, icon, color, description, statuses, backlog, todo, doing, done, canceled, slip, unscheduled=0.05, archived_days=None, regions_of=("North America",)):
            self.pid = w.project(name, icon=icon, color=color, description=description, statuses=workflow(statuses), archived_days=archived_days)
            self.name, self.prefix, self.tag, self.gate = name, prefix, tag, phase_gate
            self.backlog, self.todo, self.doing, self.done, self.canceled = backlog, todo, doing, done, canceled
            self.slip, self.unscheduled, self.regions = slip, unscheduled, regions_of
            self.tasks: list[str] = []
            self.extra = lambda title, i, due: {}

        def state(self, start, due):
            """The status, and days since it finished, for a task with these dates today."""
            if due is None:
                return r.choice([self.backlog, self.backlog, self.todo]), None
            if r.random() < 0.015:
                return self.canceled, max(-due, 2) + r.random()
            if due < -1:
                if r.random() < self.slip:
                    return r.choice(self.doing), None
                return self.done, max(-due + r.randint(-3, 2), 1) + r.random()
            if start is not None and start <= 0:
                return r.choice(self.doing), None
            return (self.done, 1 + r.random()) if r.random() < 0.02 else (self.todo, None)

        def make(self, title, team_name, *, parent=None, start=None, due=None, priority=None, notes="", key=None, labels=(), waits=(), values=None, **kw):
            if "status" in kw:
                status, done = kw.pop("status"), kw.pop("done_days", None)
            else:
                if r.random() < self.unscheduled:
                    start = due = None
                status, done = self.state(start, due)
            finished = done is not None or status == self.done
            if priority is None:
                priority = r.choices(["none", "low", "medium", "high", "urgent"], [32, 20, 28, 15, 5])[0]
                if due is not None and due < 0 and not finished and priority in ("none", "low"):
                    priority = "medium"
            vals = {team: TEAM[team_name], launch: LAUNCH[self.tag], dri: r.choice(cast[1:]), tracker: tracker_id(self.prefix),
                    gate: GATE[self.gate], risk: RISK[r.choices(["Low", "Medium", "High", "Critical"], [50, 30, 15, 5])[0]]}
            if r.random() < 0.55:
                vals[regions] = [REGION[x] for x in r.sample(self.regions, r.randint(1, len(self.regions)))]
            if r.random() < 0.25:
                vals[visible] = r.random() < 0.6
            if r.random() < 0.3:
                vals[spec] = f"https://example.com/specs/{self.prefix.lower()}/{slug_of(title)[:40]}"
            if due is not None and r.random() < 0.35:
                vals[committed] = w.day(due - r.randint(0, 10))
            if r.random() < 0.06:
                vals[sponsor] = EXPORTER
            vals.update(self.extra(title, len(self.tasks), due))
            vals.update(values or {})
            estimate = kw.pop("estimate", None)
            if estimate is None and r.random() < 0.6:
                estimate = r.choice([60, 120, 180, 240, 360, 480, 720, 960, 1440, 2400])
            logged = kw.pop("logged", None)
            if logged is None:
                logged = []
                if estimate and (finished or status in self.doing) and due is not None and r.random() < 0.6:
                    left = int(estimate * r.uniform(0.3, 1.5))
                    day = min(due, 0) - r.randint(0, 6)
                    while left > 0 and len(logged) < 4:
                        m = min(left, r.choice([60, 90, 120, 180, 240, 360, 480]))
                        logged.append((day, m, r.choice(LOG_NOTES), r.choice(firsts)))
                        left -= m
                        day -= r.randint(1, 3)
            checklist = kw.pop("checklist", None)
            if checklist is None:
                checklist = []
                if r.random() < 0.3:
                    steps = r.sample(CHECK_STEPS, r.randint(2, 6))
                    checklist = [(s, finished or (status in self.doing and j < len(steps) // 2)) for j, s in enumerate(steps)]
            label_ids = list(labels)
            if r.random() < 0.55:
                label_ids += r.sample(list(LABELS.values()), r.randint(1, 2))
            tid = w.task(title, project=self.pid, parent=parent, status=status, done_days=done, start=start, due=due, priority=priority, notes=notes, key=key,
                         labels=list(dict.fromkeys(label_ids)), values=vals, estimate=estimate, logged=logged, checklist=checklist, waits=waits,
                         assigned=kw.pop("assigned", r.random() < 0.14), created_days=kw.pop("created_days", max(-(start if start is not None else 0), 0) + r.randint(5, 40)), **kw)
            self.tasks.append(tid)
            return tid

        def stream(self, name, team_name, begin, end, items, *, chain=0.2, notes="", labels=(), parent_due=None, flat=False, **kw):
            """A workstream: a parent spanning from begin to end, with items spread across it as subtasks. Some wait for the one before.
            flat leaves out the parent, so the items are rows and cards of their own: lists and boards draw only the top level."""
            parent = None if flat else self.make(name, team_name, start=begin, due=parent_due if parent_due is not None else end, notes=notes or f"Workstream for {self.name}. What it must show at its gate: {{p:Launch playbook#Gate 3: Build}}.", priority=kw.pop("priority", None), labels=labels, key=f"{self.tag}: {name}", estimate=None, logged=[], checklist=[], values={budget: r.randint(4, 95) * 1000})
            out, span = [], max(end - begin, 1)
            for i, item in enumerate(items):
                s = begin + span * i // max(len(items), 1)
                d = min(s + r.randint(2, max(span // max(len(items) // 2, 1), 3)), end)
                waits = [out[-1]] if out and r.random() < chain else []
                out.append(self.make(item, team_name, parent=parent, start=s, due=d, waits=waits, labels=labels, key=f"{self.tag}: {item}"))
            return parent, out

    LOG_NOTES = ["", "", "", "Review with the team", "First pass", "Fixing what review found", "Test run", "Vendor call", "Writing it up", "Pairing", "Back to it after the sync", "Second pass", "Dry run"]
    CHECK_STEPS = ["Scope agreed", "Owner named", "Draft shared", "Review done", "Sign-off from the DRI", "Test evidence attached", "Linked from the launch plan", "Announced in the channel", "Risks written down", "Rollback plan written", "Support told", "Legal told", "Dashboards updated", "Runbook updated"]
    OS = ["North America", "EMEA", "APAC"]

    # Shared teams first, so launches can wait on them.
    legal = Launch("Legal and Compliance", "LGL", "Cross-launch", "G3 Build", icon="lucide:briefcase", color="purple", description="Shared team. Certification, contracts, privacy reviews and claims. Every launch waits on this queue.",
                   statuses="Intake:backlog, In review:todo, Changes requested:in-progress, With outside counsel:in-progress, Approved:done, Declined:canceled",
                   backlog="Intake", todo="In review", doing=["Changes requested", "With outside counsel"], done="Approved", canceled="Declined", slip=0.18, regions_of=["North America", "EMEA", "APAC", "LATAM"])
    matter, MATTER = w.choices("Matter type", ["Certification", "Contract", "Privacy", "Claims review", "Trademark", "Export control"], project=legal.pid)
    jurisdiction, JUR = w.choices("Jurisdiction", ["US", "Canada", "EU", "UK", "Japan", "Australia", "Brazil"], multi=True, project=legal.pid)
    counsel = w.field("Counsel", "text", project=legal.pid)
    approved_on = w.field("Approved on", "date", project=legal.pid)
    counsel_cost = w.field("Outside counsel USD", "number", project=legal.pid)
    legal.extra = lambda title, i, due: {**({approved_on: w.day(due)} if due is not None and due < -1 else {}), matter: MATTER[("Certification", "Contract", "Privacy", "Claims review", "Trademark", "Export control")[i % 6]], jurisdiction: [JUR[x] for x in r.sample(list(JUR), r.randint(1, 3))], counsel: r.choice(["Holm & Reyes LLP", "In-house", "Kessler Partners", "In-house"]), **({counsel_cost: r.randint(2, 60) * 500} if i % 4 == 0 else {})}

    mktg = Launch("Brand and Marketing", "MKT", "Cross-launch", "G3 Build", icon="lucide:palette", color="pink", description="Shared team. Campaigns, launch assets, press and events for every launch.",
                  statuses="Brief:backlog, Drafting:todo, In review:in-progress, Scheduled:in-progress, Live:done, Dropped:canceled",
                  backlog="Brief", todo="Drafting", doing=["In review", "Scheduled"], done="Live", canceled="Dropped", slip=0.12, regions_of=OS + ["LATAM"])
    channel, CHAN = w.choices("Channel", ["Web", "Email", "Paid social", "Press", "Retail", "Events", "Partner"], multi=True, project=mktg.pid)
    asset, ASSET = w.choices("Asset type", ["Video", "Landing page", "Press release", "Sales deck", "Packaging", "Photo set", "Email"], project=mktg.pid)
    media = w.field("Media spend USD", "number", project=mktg.pid)
    goes_live = w.field("Goes live", "date", project=mktg.pid)
    mktg.extra = lambda title, i, due: {**({goes_live: w.day(due)} if due is not None else {}), channel: [CHAN[x] for x in r.sample(list(CHAN), r.randint(1, 3))], asset: ASSET[("Video", "Landing page", "Press release", "Sales deck", "Packaging", "Photo set", "Email")[i % 7]], **({media: r.randint(5, 180) * 500} if i % 3 == 0 else {})}

    supply = Launch("Supply Chain", "SCM", "Cross-launch", "G4 Validate", icon="lucide:car", color="amber", description="Shared team. Suppliers, purchase orders, freight and factory readiness.",
                    statuses="To source:backlog, RFQ out:todo, PO placed:in-progress, In transit:in-progress, At customs:in-progress, Received:done, Canceled:canceled",
                    backlog="To source", todo="RFQ out", doing=["PO placed", "In transit", "At customs"], done="Received", canceled="Canceled", slip=0.22, regions_of=OS)
    supplier = w.field("Supplier", "text", project=supply.pid)
    po = w.field("PO number", "text", project=supply.pid)
    lead_time = w.field("Lead time days", "number", project=supply.pid)
    landed = w.field("Landed cost USD", "number", project=supply.pid)
    eta = w.field("ETA", "date", project=supply.pid)
    incoterm, INCO = w.choices("Incoterm", ["EXW", "FOB", "CIF", "DDP"], project=supply.pid)
    SUPPLIERS = ["Hanover Circuits", "Vantage Plastics", "Lumen Glass Co", "Pearl River EMS", "Stellar Sensors", "Kirkwood Packaging", "Ostrava Tooling"]
    supply.extra = lambda title, i, due: {**({eta: w.day(due)} if due is not None else {}), supplier: r.choice(SUPPLIERS), po: f"PO-{48000 + i * 7}", lead_time: r.choice([14, 21, 28, 45, 60, 90]), landed: r.randint(3, 400) * 100, incoterm: INCO[r.choice(list(INCO))]}

    sec = Launch("Security and Privacy", "SEC", "Cross-launch", "G3 Build", icon="lucide:target", color="teal", description="Shared team. Threat models, penetration tests, findings and privacy impact assessments.",
                 statuses="Triage:backlog, Accepted:todo, Fixing:in-progress, Verifying:in-progress, Closed:done, Won't fix:canceled",
                 backlog="Triage", todo="Accepted", doing=["Fixing", "Verifying"], done="Closed", canceled="Won't fix", slip=0.2)
    severity, SEV = w.choices("Severity", ["Sev 1", "Sev 2", "Sev 3", "Sev 4"], project=sec.pid)
    cvss = w.field("CVSS", "number", project=sec.pid)
    found_by, FOUND = w.choices("Found by", ["Pen test", "Bug bounty", "Code review", "Scanner"], project=sec.pid)
    fix_by = w.field("Fix by", "date", project=sec.pid)
    sec.extra = lambda title, i, due: {**({fix_by: w.day(due + 14)} if due is not None else {}), severity: SEV[r.choice(["Sev 2", "Sev 3", "Sev 3", "Sev 4"])], cvss: round(r.uniform(2.1, 9.4), 1), found_by: FOUND[r.choice(list(FOUND))]}

    support = Launch("Support Readiness", "SUP", "Cross-launch", "G4 Validate", icon="lucide:users", color="cyan", description="Shared team. Knowledge base, agent training and escalation paths before each launch.",
                     statuses="Requested:backlog, Planned:todo, Writing:in-progress, In review:in-progress, Published:done, Not needed:canceled",
                     backlog="Requested", todo="Planned", doing=["Writing", "In review"], done="Published", canceled="Not needed", slip=0.1, regions_of=OS)
    kb = w.field("KB article", "url", project=support.pid)
    langs, LANG = w.choices("Languages", ["English", "German", "French", "Spanish", "Japanese", "Dutch", "Italian"], multi=True, project=support.pid)
    deflect = w.field("Expected deflection %", "number", project=support.pid)
    support.extra = lambda title, i, due: {langs: [LANG[x] for x in r.sample(list(LANG), r.randint(1, 4))], deflect: r.randint(5, 60), kb: f"https://example.com/kb/{1000 + i}"}

    data_team = Launch("Data and Analytics", "DAT", "Cross-launch", "G3 Build", icon="lucide:code", color="indigo", description="Shared team. Telemetry, launch dashboards and experiment readouts.",
                       statuses="Backlog:backlog, Next:todo, Building:in-progress, Validating:in-progress, In production:done, Dropped:canceled",
                       backlog="Backlog", todo="Next", doing=["Building", "Validating"], done="In production", canceled="Dropped", slip=0.15)
    dash = w.field("Dashboard", "url", project=data_team.pid)
    metric = w.field("Metric", "text", project=data_team.pid)
    data_team.extra = lambda title, i, due: {metric: r.choice(["Activation rate", "D30 retention", "Pairing success", "Crash-free sessions", "Time to first reading", "NPS"]), dash: f"https://example.com/dash/{i + 40}"}

    # The launches.

    atlas = Launch("Atlas X1 launch", "ATL", "Atlas X1", "G5 Ship", icon="lucide:rocket", color="red",
                   description="The smart thermostat. Was due October 28, now November 18, and the certification retest could move it again. Everything here is about shipping.",
                   statuses="Backlog:backlog, Planned:todo, In work:in-progress, In validation:in-progress, Blocked at gate:in-progress, Released:done, Dropped:canceled",
                   backlog="Backlog", todo="Planned", doing=["In work", "In validation", "Blocked at gate"], done="Released", canceled="Dropped", slip=0.3, unscheduled=0.03, regions_of=["North America", "EMEA"])
    build, BUILD = w.choices("Build stage", ["EVT", "DVT", "PVT", "MP"], project=atlas.pid)
    component, COMP = w.choices("Component", ["Main board", "Radio", "Display", "Enclosure", "Power", "Firmware", "App pairing"], project=atlas.pid)
    certs, CERT = w.choices("Certifications", ["FCC", "IC", "CE", "UKCA", "UL", "Energy Star"], multi=True, project=atlas.pid)
    unit = w.field("Unit cost USD", "number", project=atlas.pid)
    passed = w.field("Test passed", "checkbox", project=atlas.pid)
    signed_on = w.field("Signed off", "date", project=atlas.pid)
    atlas.extra = lambda title, i, due: {**({signed_on: w.day(due)} if due is not None and due < -5 and i % 4 == 0 else {}), build: BUILD[r.choice(["DVT", "PVT", "PVT", "MP"])], component: COMP[r.choice(list(COMP))], **({unit: round(r.uniform(0.2, 19.0), 2)} if i % 5 == 0 else {}), **({passed: i % 3 != 0} if i % 2 == 0 else {})}

    harbor = Launch("Harbor Cloud 4.0", "HBR", "Harbor 4.0", "G3 Build", icon="lucide:globe", color="blue",
                    description="The platform move: one API, regional data, and the old monolith switched off service by service. Mid-build, on schedule so far.",
                    statuses="Backlog:backlog, Ready:todo, In dev:in-progress, Code review:in-progress, In staging:in-progress, Live:done, Won't do:canceled",
                    backlog="Backlog", todo="Ready", doing=["In dev", "Code review", "In staging"], done="Live", canceled="Won't do", slip=0.08, unscheduled=0.08, regions_of=OS)
    service, SVC = w.choices("Service", ["Devices", "Identity", "Billing", "Telemetry", "Notifications", "Rules", "Audit", "Search", "Gateway", "Exports"], project=harbor.pid)
    points = w.field("Story points", "number", project=harbor.pid)
    sprint, SPRINT = w.choices("Sprint", ["Sprint 44", "Sprint 45", "Sprint 46", "Sprint 47", "Sprint 48", "Sprint 49"], project=harbor.pid)
    envs, ENV = w.choices("Environments", ["Dev", "Staging", "Prod US", "Prod EU"], multi=True, project=harbor.pid)
    runbook = w.field("Runbook", "url", project=harbor.pid)
    reviewer = w.field("Reviewer", "user", project=harbor.pid)
    moved = w.field("Traffic moved %", "number", project=harbor.pid)
    harbor.extra = lambda title, i, due: {**({runbook: f"https://example.com/runbooks/{slug_of(title)[:40]}"} if "runbook" in title.lower() else {}), **({moved: r.choice([0, 5, 25, 50, 100])} if "Shadow" in title else {}), points: r.choice([1, 2, 3, 5, 8, 13]), sprint: SPRINT[r.choice(list(SPRINT))], envs: [ENV[x] for x in r.sample(list(ENV), r.randint(1, 3))], **({reviewer: EXPORTER} if i % 11 == 0 else {})}

    beacon = Launch("Beacon Mobile 5", "BCN", "Beacon 5", "G3 Build", icon="lucide:star", color="green",
                    description="The companion app, redrawn. Internal beta now, public beta in January. Pairing depends on Harbor; copy and legal depend on everyone.",
                    statuses="Ideas:backlog, Designed:todo, Ready:todo, Building:in-progress, Code review:in-progress, QA:in-progress, Shipped:done, Cut:canceled",
                    backlog="Ideas", todo="Ready", doing=["Building", "Code review", "QA"], done="Shipped", canceled="Cut", slip=0.1, unscheduled=0.06, regions_of=OS)
    platform, PLAT = w.choices("Platform", ["iOS", "Android", "Tablet"], multi=True, project=beacon.pid)
    screen = w.field("Screen", "text", project=beacon.pid)
    b_points = w.field("Points", "number", project=beacon.pid)
    cohort, COHORT = w.choices("Beta cohort", ["Staff", "Friends", "Waitlist A", "Waitlist B", "Public"], project=beacon.pid)
    flag = w.field("Feature flag", "text", project=beacon.pid)
    beacon.extra = lambda title, i, due: {platform: [PLAT[x] for x in r.sample(list(PLAT), r.randint(1, 2))], b_points: r.choice([1, 2, 3, 5, 8]), **({flag: f"beacon5_{slug_of(title)[:24]}"} if i % 6 == 0 else {}), **({cohort: COHORT[r.choice(list(COHORT))]} if i % 4 == 0 else {})}

    juniper = Launch("Juniper Hub", "JNP", "Juniper Hub", "G0 Concept", icon="lucide:lightbulb", color="lime",
                     description="A home gateway for 2028. Early: research, feasibility and a business case. Most of this has no dates yet.",
                     statuses="Parking lot:backlog, Question:backlog, Researching:in-progress, Answered:done, Dead end:canceled",
                     backlog="Parking lot", todo="Question", doing=["Researching"], done="Answered", canceled="Dead end", slip=0.05, unscheduled=0.55)
    confidence = w.field("Confidence %", "number", project=juniper.pid)
    method, METHOD = w.choices("Method", ["Interview", "Teardown", "Prototype", "Desk research", "Survey"], project=juniper.pid)
    juniper.extra = lambda title, i, due: {method: METHOD[r.choice(list(METHOD))], **({confidence: r.randint(10, 80)} if i % 2 == 0 else {})}

    ridge = Launch("Ridgeline Enterprise", "RDG", "Ridgeline", "G1 Feasibility", icon="lucide:flag", color="purple",
                   description="A fleet console for building managers. Discovery with design partners; the pilot is not funded until the January review.",
                   statuses="Open question:backlog, Planned:todo, In discovery:in-progress, With customer:in-progress, Decided:done, Parked:canceled",
                   backlog="Open question", todo="Planned", doing=["In discovery", "With customer"], done="Decided", canceled="Parked", slip=0.1, unscheduled=0.3, regions_of=["North America", "EMEA"])
    customer = w.field("Design partner", "text", project=ridge.pid)
    arr = w.field("ARR at stake USD", "number", project=ridge.pid)
    decision_by = w.field("Decision by", "date", project=ridge.pid)
    PARTNERS = ["Larkfield Properties", "Summit Campus Group", "Oak & Iron REIT", "Harwell University", "Tessera Offices", "Bluewater Hospitals"]
    ridge.extra = lambda title, i, due: {**({decision_by: w.day(due + 7)} if due is not None and i % 4 == 0 else {}), customer: r.choice(PARTNERS), **({arr: r.randint(4, 90) * 5000} if i % 3 == 0 else {})}

    solstice = Launch("Solstice EU rollout", "SOL", "Solstice EU", "G5 Ship", icon="lucide:leaf", color="orange",
                      description="The energy plan, launched in eight European countries on October 14. In hypercare: nearly everything is finished, and a few countries still have loose ends.",
                      statuses="Backlog:backlog, Planned:todo, Doing:in-progress, In hypercare:in-progress, Done:done, Canceled:canceled",
                      backlog="Backlog", todo="Planned", doing=["Doing", "In hypercare"], done="Done", canceled="Canceled", slip=0.05, unscheduled=0.01, regions_of=["EMEA"])
    country, COUNTRY = w.choices("Country", ["Germany", "France", "Netherlands", "Spain", "Italy", "Sweden", "Poland", "Ireland"], project=solstice.pid)
    partner = w.field("Retail partner", "text", project=solstice.pid)
    go_live = w.field("Go-live", "date", project=solstice.pid)
    sev = w.field("Open incidents", "number", project=solstice.pid)
    solstice.extra = lambda title, i, due: {country: COUNTRY[title.split()[0].rstrip(":")]} if title.split()[0].rstrip(":") in COUNTRY else {}

    # Atlas: about to ship and slipping. Its story comes first, by hand.
    samples = supply.make("Rev C PCB samples from Hanover Circuits", "Supply Chain", start=-14, due=-3, status="At customs", priority="urgent", labels=[LABELS["launch-critical"], LABELS["vendor"]], assigned=True, my_day=True, key="Rev C samples",
                          notes="Forty boards, air freight. Held at customs on a tariff code question since Thursday.\n\nThis is the start of the chain that moves the {t:Ship Atlas X1 to retail DCs} date.",
                          values={supplier: "Hanover Circuits", po: "PO-48127", lead_time: 21, eta: w.day(1), landed: 8200},
                          files=[("rev-c-commercial-invoice.pdf", "application/pdf", pdf("Commercial invoice, Hanover Circuits, PO-48127"))],
                          reminders=[w.remind_at(0, "23:30")], comments=[("Customs broker says tomorrow morning at the earliest. https://example.com/freight/track/HC-48127 {@:Rohan} please chase it.", 1, "Paul"), ("Chased. They want the HS code letter again. {@:Paul} can you resend?", 0.4, "Rohan")])
    matching = atlas.make("Antenna matching network, rev C", "Hardware", start=-10, due=-1, status="Blocked at gate", priority="urgent", waits=[samples], labels=[LABELS["launch-critical"]], key="Antenna matching", values={component: COMP["Radio"], build: BUILD["PVT"], passed: False, unit: 0.84},
                          notes="""## What went wrong

Radiated emissions failed at 2.48 GHz by 1.8 dB in the FCC pre-scan.

## The fix

Rev C moves C14 and changes the pi network.

| Rev | Margin at 2.48 GHz | Result |
| --- | ---: | --- |
| A | -3.1 dB | Fail |
| B | -1.8 dB | Fail |
| C (simulated) | +1.2 dB | Pass |

{img:emissions-plot.png}

Full write-up: {p:Atlas certification log#Radiated emissions}.""",
                          files=[("emissions-plot.png", "image/png", bars([31, 34, 38, 44, 52, 49, 47, 41, 36, 33], (200, 60, 60))), ("rf-scan-rev-b.csv", *csv_file("freq_ghz,dbuv_m,limit,margin", "2.40,48.1,54.0,5.9", "2.44,50.2,54.0,3.8", "2.48,55.8,54.0,-1.8"))],
                          comments=[("(edited) Simulation says we clear by 1.2 dB. I would not bet the launch on 1.2 dB, but it is what we have.", 3, "Kenji"), ("{@:Kenji} what is plan B if retest fails? {t:Ship Atlas X1 to retail DCs} has no slack left.", 2), ("Plan B is the shielding can, in {p:Atlas plan#Slip scenarios}. Adds three weeks.", 1, "Kenji")], estimate=2400, logged=[(-9, 360, "Simulation", "Kenji"), (-8, 480, "Layout", "Kenji"), (-5, 240, "Review with the lab", "Greta")])
    retest = legal.make("FCC radiated emissions retest", "Legal", start=2, due=6, status="In review", priority="urgent", waits=[matching], labels=[LABELS["launch-critical"], LABELS["regulatory"]], key="FCC retest", values={matter: MATTER["Certification"], jurisdiction: [JUR["US"]], counsel: "In-house"}, reminders=[w.remind_at(2, "14:00")])
    grant = legal.make("FCC and IC grant for Atlas X1", "Legal", due=11, status="In review", priority="urgent", waits=[retest], labels=[LABELS["launch-critical"], LABELS["regulatory"], LABELS["exec-visible"]], key="FCC grant", values={matter: MATTER["Certification"], jurisdiction: [JUR["US"], JUR["Canada"]], approved_on: w.day(11), counsel: "Holm & Reyes LLP", counsel_cost: 18500}, comments=[("TCB turnaround is five working days if nothing is missing. See {p:Launch playbook#Gate 5: Ship}.", 4, "Elise")])
    mp = atlas.make("Mass production release", "Hardware", due=12, status="Planned", priority="urgent", waits=[grant], labels=[LABELS["launch-critical"], LABELS["exec-visible"]], key="MP release", values={build: BUILD["MP"], signed_on: w.day(12)})
    ship = atlas.make("Ship Atlas X1 to retail DCs", "Supply Chain", due=16, status="Planned", priority="urgent", waits=[mp], labels=[LABELS["launch-critical"], LABELS["exec-visible"], LABELS["cross-team"]], key="Ship Atlas X1 to retail DCs", values={build: BUILD["MP"], committed: w.day(-5)}, changes=[(30, {"due_date": w.day(-5)}), (12, {"due_date": w.day(9)}), (2, {"due_date": w.day(16)})],
                      notes="Date history: October 28, then November 11, now November 18. Retail needs units two days before the on-shelf date.", comments=[("Moved again. Told the exec staff at the Thursday readout. {@:Sofia} marketing needs to know today.", 2), ("On it. Embargo moves with it: {t:Atlas X1 press embargo lifts}.", 1.5, "Sofia")], reminders=[w.remind_before(16, "17:00", 2880)], assigned=True)
    embargo = mktg.make("Atlas X1 press embargo lifts", "Marketing", due=18, time="06:00", status="Scheduled", priority="high", waits=[ship], labels=[LABELS["launch-critical"], LABELS["exec-visible"], LABELS["customer-facing"]], key="Atlas X1 press embargo lifts", values={goes_live: w.day(18), channel: [CHAN["Press"]]})
    onboarding_fw = atlas.make("Cloud onboarding in firmware 1.0", "Firmware", start=-6, due=3, status="In work", priority="high", key="Cloud onboarding in firmware", values={component: COMP["Firmware"]}, labels=[LABELS["cross-team"]], assigned=True, timer=(35, "Retry logic against Harbor staging"))
    atlas.make("Release 1.0 firmware candidate", "Firmware", start=-3, due=7, status="In validation", priority="high", waits=[onboarding_fw], key="Firmware candidate", values={component: COMP["Firmware"], passed: False})
    w.depend(mp, w.task_names["Firmware candidate"])
    atlas.stream("DVT close-out", "Hardware", -70, -24, ["Drop test report", "Thermal soak at 40 C", "Thermal soak at -10 C", "ESD test report", "Button life cycle test", "Display burn-in test", "Mechanical tolerance review", "DVT build report", "Sign DVT exit"], chain=0.3)
    atlas.stream("PVT build", "Hardware", -30, 8, ["Line balance study", "Pilot run of 500 units", "First-pass yield review", "Fixture rev 3", "Fixture rev 4", "Rework procedure", "Golden sample set", "OQC plan", "Reliability sample pull", "PVT exit review"], chain=0.35, labels=[LABELS["launch-critical"]])
    atlas.stream("HVAC compatibility", "QA", -40, 10, [f"Compatibility: {s}" for s in ["single-stage gas", "two-stage gas", "heat pump, one stage", "heat pump, two stage", "dual fuel", "boiler, 24 V", "boiler, millivolt", "zoned, two zones", "zoned, three zones", "no C wire adapter", "line voltage adapter", "electric baseboard", "mini-split IR bridge", "humidifier relay", "ventilator relay"]], chain=0.05, flat=True)
    for build_no, begin in [(7, -30), (8, -14), (9, 0)]:
        atlas.stream(f"Firmware regression, 0.9.{build_no}", "QA", begin, begin + 14, [f"Regression 0.9.{build_no}: {f}" for f in ["schedule", "away mode", "geofence", "OTA", "pairing", "wiring check", "display dim", "safety cutoff", "energy report", "offline mode", "time zone change", "power loss recovery"]], labels=[LABELS["regression"]], chain=0.1, flat=True)
    atlas.stream("Factory test stations", "Hardware", -45, 6, [f"Station bring-up: {x}" for x in ["ICT", "Flash and key injection", "RF calibration", "Display check", "Button check", "Relay load test", "Burn-in rack", "Final function", "Pack-out scan", "Label print", "Sample audit", "Traceability database"]], chain=0.2)
    atlas.stream("Firmware bug burn-down", "Firmware", -18, 9, ["Bug: schedule skips the DST change", "Bug: away mode ignores a second phone", "Bug: display stays lit after a reboot", "Bug: OTA stalls at 91%", "Bug: wrong icon on heat pump aux", "Bug: humidity reads 2% high", "Bug: Wi-Fi retries too fast on 5 GHz", "Bug: clock drifts offline", "Bug: fan runs after a cancel", "Bug: pairing code times out at 60 s", "Bug: boot loop if EEPROM is blank", "Bug: geofence fires twice", "Bug: energy report off by one day", "Bug: setpoint jumps when the dial is fast", "Bug: safety cutoff logs no event", "Bug: Spanish strings overflow", "Bug: Fahrenheit rounding", "Bug: BLE scan drains battery on C-wire adapter", "Bug: factory reset keeps Wi-Fi", "Bug: time zone defaults to UTC", "Bug: relay click at midnight", "Bug: settings menu wraps at the end", "Bug: crash on empty schedule", "Bug: wrong model string in the app", "Bug: slow first reading"], labels=[LABELS["regression"]], chain=0.03, flat=True)
    atlas.stream("Packaging and retail", "Supply Chain", -35, 16, ["Retail box dieline", "Quick start guide layout", "Barcode and UPC registration", "Pallet configuration", "Drop test of shipping carton", "Retailer portal setup", "Planogram approval", "Store demo units"] + [f"Retailer onboarding: {x}" for x in ["Northline Home", "Crestwood Supply", "BuildRight", "Volta Electric", "HomeGear Online", "Fairmont Stores"]], chain=0.15)
    atlas.stream("Install guides", "Support", -20, 14, [f"Install guide: {x}" for x in ["English (US)", "English (UK)", "French (Canada)", "French (France)", "German", "Dutch", "Spanish (Mexico)", "Italian"]], labels=[LABELS["localization"]], chain=0.0)
    atlas.stream("Launch day", "Product", 5, 20, ["Go/no-go review", "War room rota", "Rollback plan for OTA", "Status page copy", "Exec readout, launch week", "Day-one metrics review"], chain=0.2, labels=[LABELS["exec-visible"]])
    for t, kw in [("Cost-down second source for the relay", dict(status="Backlog")), ("Energy Star application", dict(status="Backlog", values={certs: [CERT["Energy Star"]]})), ("White enclosure option", dict(status="Dropped", done_days=40)), ("Touch glass supplier audit", dict(start=-16, due=-6, status="In work", priority="high"))]:
        atlas.make(t, "Hardware", **kw)
    atlas.make("UL listing for the line voltage adapter", "Legal", start=-25, due=-2, status="Blocked at gate", priority="high", values={certs: [CERT["UL"]]}, labels=[LABELS["regulatory"]])
    atlas.make("CE and UKCA declaration of conformity", "Legal", start=-12, due=4, status="In work", values={certs: [CERT["CE"], CERT["UKCA"]]}, labels=[LABELS["regulatory"]], checklist=[("Test reports collected", True), ("Technical file", False), ("Signature", False)])

    # Harbor: mid-build, service by service.
    SERVICES = ["Devices", "Identity", "Billing", "Telemetry", "Notifications", "Rules", "Audit", "Search", "Gateway", "Exports"]
    STEPS = ["Schema migration", "API v4 contract", "Contract tests", "Backfill job", "Shadow traffic", "Load test at 3x", "Cutover runbook", "Dashboards and alerts", "Turn off the v3 path"]
    harbor_api = {}
    for i, svc in enumerate(SERVICES):
        begin = -55 + i * 12
        parent, tasks = harbor.stream(f"{svc} service move", "Cloud", begin, begin + 70, [f"{svc}: {s}" for s in STEPS], chain=0.7, labels=[LABELS["tech-debt"]] if i % 3 == 0 else (), flat=True)
        harbor_api[svc] = tasks
        for t in tasks:
            w.row("custom_field_value", pair_id(t, service), task_id=t, field_id=service, value=SVC[svc], deleted_at=None)
    harbor.stream("Regional data", "Cloud", -20, 60, ["EU region: network design", "EU region: data residency review", "EU region: failover drill", "Tenant routing by region", "Backfill EU tenants", "Region picker in admin"], chain=0.4, labels=[LABELS["regulatory"]])
    harbor.stream("Developer experience", "Cloud", -30, 40, ["Generated client for Go", "Generated client for TypeScript", "API reference site", "Sandbox tenants", "Rate limit headers", "Deprecation notices"], chain=0.1)
    harbor.stream("Reliability", "Cloud", -10, 50, ["SLOs for each service", "Chaos day 1", "Chaos day 2", "On-call rota for v4", "Incident review template", "Capacity plan for 2027"], chain=0.2)
    harbor.stream("Tenant migration waves", "Cloud", 10, 85, [f"Migrate tenant wave {n}: {size}" for n, size in enumerate(["staff", "200 small", "500 small", "1,000 small", "2,000 mixed", "4,000 mixed", "retail partners", "EU small", "EU large", "enterprise pilots", "stragglers", "last sweep"], 1)], chain=0.9)
    harbor.stream("Tech debt to clear first", "Cloud", -40, 30, ["Retire the XML export", "Remove the legacy session table", "Split the shared Redis", "Fix the N+1 in device lists", "Upgrade the ORM", "Delete the v2 webhook path", "Pin base images", "Move cron jobs to the scheduler", "Unify error codes", "Clean up feature flags", "Drop unused indexes", "Replace the hand-written client", "Make migrations reversible", "Consolidate logging", "Remove the old rate limiter", "Rotate the signing keys procedure", "Fix flaky contract tests", "Time out slow queries", "Document the on-call scripts", "Delete dead dashboards"], labels=[LABELS["tech-debt"]], chain=0.03, flat=True)
    harbor.stream("Cost", "Finance", 0, 70, ["Cost per tenant report", "Reserved capacity decision", "Egress review", "Storage tiering", "Budget checkpoint"], chain=0.0)
    w.depend(onboarding_fw, harbor_api["Devices"][1])
    harbor.make("Decide on the event bus", "Cloud", status="Backlog", labels=[LABELS["needs-decision"]], notes="Three options, written up in {p:Harbor architecture#Event bus options}. Needs a call before Sprint 46.", comments=[("Leaning toward the managed option. Costs more, sleeps better.", 6, "Rohan"), ("{@:Wei} can you put a number on \"costs more\"?", 3)])
    harbor.make("Audit log retention policy", "Legal", due=21, status="Ready", labels=[LABELS["regulatory"], LABELS["cross-team"]])

    # Beacon: mid-build, screen by screen.
    SCREENS = ["Home", "Pairing", "Schedule", "Energy report", "Rooms", "Away mode", "Notifications", "Settings", "Account", "Help", "Household sharing", "Installer mode"]
    pairing_tasks = {}
    for i, sc in enumerate(SCREENS):
        begin = -35 + i * 6
        parent, tasks = beacon.stream(f"{sc} screen", "Mobile", begin, begin + 50, [f"{sc}: design", f"{sc}: iOS", f"{sc}: Android", f"{sc}: accessibility pass", f"{sc}: analytics events", f"{sc}: copy review", f"{sc}: QA sign-off"], chain=0.6, labels=[LABELS["design"]] if i % 4 == 0 else (), flat=i % 2 == 0)
        pairing_tasks[sc] = tasks
        for t in tasks:
            w.row("custom_field_value", pair_id(t, screen), task_id=t, field_id=screen, value=sc, deleted_at=None)
    w.depend(pairing_tasks["Pairing"][1], harbor_api["Devices"][1])
    w.depend(pairing_tasks["Pairing"][2], harbor_api["Devices"][1])
    w.depend(pairing_tasks["Account"][1], harbor_api["Identity"][1])
    w.depend(pairing_tasks["Pairing"][1], onboarding_fw)
    beacon.stream("Beta program", "Product", -14, 75, ["Internal beta build", "Staff cohort of 300", "Friends cohort of 1,000", "Waitlist A invite", "Waitlist B invite", "Crash triage rota", "Feedback tagging", "Public beta go/no-go"], chain=0.5, labels=[LABELS["customer-facing"]])
    beacon.stream("Experiments", "Data", -7, 80, ["Pairing flow A/B", "Home layout test", "Energy report teaser", "Notification opt-in timing", "Upsell placement"], chain=0.0, labels=[LABELS["experiment"]])
    beacon.stream("Localization", "Product", 10, 80, [f"Translate app strings: {x}" for x in ["German", "French", "Spanish", "Dutch", "Italian", "Japanese"]], labels=[LABELS["localization"]], chain=0.0)
    beacon.stream("Beta bugs", "Mobile", -12, 25, ["Crash on rotate in schedule", "Back swipe loses the draft", "Dark mode chart contrast", "Android 15 notification channel", "iOS widget stale after pairing", "Keyboard covers the save button", "Slow first open offline", "Duplicate rooms after sync", "Wrong currency in the energy report", "Avatar upload fails on large photos", "VoiceOver order on the home screen", "TalkBack label on the dial", "Sign-out leaves push on", "Deep link opens the wrong home", "Pull to refresh spins forever", "Tablet split view clips the dial", "German strings truncate the tabs", "Time format ignores the system", "Pairing retry reuses a dead code", "Battery use while backgrounded", "Household invite link expires early", "Empty state flashes on launch", "Energy chart off by a day at DST", "Schedule drag jitter", "Crash when a device is removed"], labels=[LABELS["regression"]], chain=0.02, flat=True)
    beacon.make("Drop tablet layout from 5.0?", "Product", status="Ideas", labels=[LABELS["needs-decision"]], notes="Saves six weeks of Android work. Tablets are 4% of sessions.", comments=[("Cut it. Put it in 5.1.", 2, "Hye-jin")], key="Tablet decision")
    beacon.make("Old icon set cleanup", "Design", status="Cut", done_days=18, notes="Replaced by the design system work.")

    # Juniper: early.
    for t in ["How many homes have a Matter controller already?", "Teardown: Vantage hub", "Teardown: Ostrava Home gateway", "Thread radio coexistence", "Battery backup: worth it?", "Installer interviews, round 1", "Installer interviews, round 2", "Do people want a screen on it?", "Price point survey", "Local-only mode: how much does it cost us?", "Which radios are table stakes?", "Business case draft", "Competitive map", "Retail vs. subscription model", "Thermal envelope in a closet", "Prototype on the eval board", "Cloudless pairing spike", "Regulatory cost estimate", "Who else wants this? Partner calls", "Name shortlist"]:
        juniper.make(t, "Product", created_days=r.randint(3, 60))
    juniper.stream("Feasibility", "Hardware", -15, 45, ["Radio board bring-up", "Power budget", "BOM estimate at 50k units", "BOM estimate at 200k units", "Software stack options", "G1 review pack"], chain=0.3)
    w.task("Juniper G1 review", project=juniper.pid, status="Question", due=46, priority="high", labels=[LABELS["exec-visible"]], notes="Go to {p:Launch playbook#Gate 1: Feasibility} for what this needs.")

    # Ridgeline: early, with customers.
    for name in PARTNERS:
        ridge.stream(f"Design partner: {name}", "Product", -25 + r.randint(0, 15), 40, [f"{name}: discovery call", f"{name}: site visit", f"{name}: security questionnaire", f"{name}: readout"], chain=0.8, labels=[LABELS["customer-facing"]])
    for t in ["Pricing: per site or per device?", "Do we need SSO on day one?", "Pilot funding request for the January review", "Single-tenant demand: how real?", "Console prototype: fleet list", "Console prototype: alarms", "Console prototype: reports", "Competitor pricing teardown", "Partner agreement template", "Ridgeline name check"]:
        ridge.make(t, "Product")

    # Solstice: just shipped.
    STEPS_EU = ["Tariff data load", "Local legal review", "Localized plan page", "Payment methods", "Retail partner training", "Customer support rota", "Go-live checklist", "Week 1 review"]
    for i, c in enumerate(["Germany", "France", "Netherlands", "Spain", "Italy", "Sweden", "Poland", "Ireland"]):
        parent, tasks = solstice.stream(f"{c} go-live", "Product", -75 + i * 2, -14 + (i % 3) * 8, [f"{c}: {s}" for s in STEPS_EU], chain=0.8, labels=[LABELS["localization"]] if i % 2 == 0 else ())
        for t in tasks + [parent]:
            w.row("custom_field_value", pair_id(t, go_live), task_id=t, field_id=go_live, value=w.day(-19), deleted_at=None)
    for t, kw in [("Hypercare: daily standup until November 14", dict(start=-19, due=12, status="In hypercare", repeat="FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", assigned=True)), ("Poland: tariff rounding error on invoices", dict(start=-5, due=-1, status="Doing", priority="urgent", labels=[LABELS["customer-facing"]], values={country: COUNTRY["Poland"], sev: 2})), ("Sweden: retailer feed missing the winter plan", dict(start=-3, due=1, status="Doing", priority="high", values={country: COUNTRY["Sweden"], partner: "Nordhem", sev: 1})), ("Launch retrospective", dict(due=9, status="Planned", priority="medium")), ("Move Solstice to run-state ownership", dict(due=14, status="Planned", priority="high"))]:
        solstice.make(t, "Product", **kw)

    # The shared teams' own queues.
    for prefix_t, kw in [("Privacy impact assessment: ", dict(s=-40, e=50)), ("Marketing claims review: ", dict(s=-30, e=30))]:
        for prod in ["Atlas X1", "Harbor 4.0", "Beacon 5", "Solstice EU", "Ridgeline"]:
            s = kw["s"] + r.randint(0, 30)
            legal.make(prefix_t + prod, "Legal", start=s, due=s + r.randint(10, 35), key=prefix_t + prod, values={launch: LAUNCH[prod]})
    legal.stream("Atlas X1 country approvals", "Legal", -30, 20, [f"Radio approval: {x}" for x in ["United States", "Canada", "United Kingdom", "Germany", "France", "Netherlands", "Italy", "Spain"]], labels=[LABELS["regulatory"]], chain=0.1)
    for t in ["Data processing addendum, EU template", "Beacon 5 terms of service update", "Beacon 5 privacy notice update", "Harwell University master agreement", "Supplier code of conduct refresh", "Export classification for Atlas X1", "Trademark: Beacon in Japan", "Trademark: Juniper in the EU", "Open source license review, Harbor", "Open source license review, Beacon", "Warranty terms for Atlas X1", "Retailer indemnity clause", "Children's data review for household sharing", "RED cybersecurity article review", "Battery regulation review for 2027"]:
        s = r.randint(-45, 40)
        legal.make(t, "Legal", start=s, due=s + r.randint(7, 40))
    legal_dpa = legal.make("EU data residency sign-off", "Legal", start=-5, due=10, status="With outside counsel", priority="high", labels=[LABELS["regulatory"], LABELS["cross-team"]], values={jurisdiction: [JUR["EU"]], counsel: "Kessler Partners", counsel_cost: 12000}, key="EU data residency sign-off")
    w.depend(w.task_names["Harbor 4.0: EU region: data residency review"], legal_dpa)
    w.depend(w.task_names["Beacon 5 terms of service update"], w.task_names["Privacy impact assessment: Beacon 5"])
    for prod, begin in [("Atlas X1", -35), ("Beacon 5", 20), ("Harbor 4.0", 30), ("Solstice EU", -70)]:
        mktg.stream(f"{prod} launch campaign", "Marketing", begin, begin + 60, [f"{prod}: messaging house", f"{prod}: hero video", f"{prod}: landing page", f"{prod}: press release", f"{prod}: launch email", f"{prod}: paid social set", f"{prod}: partner kit", f"{prod}: sales deck", f"{prod}: photo shoot", f"{prod}: review units"], chain=0.3, labels=[LABELS["customer-facing"]])
    for prod, begin in [("Atlas X1", -20), ("Beacon 5", 30)]:
        mktg.stream(f"{prod} regional assets", "Marketing", begin, begin + 45, [f"{prod}: {a}, {loc}" for a in ["landing page", "launch email", "retail banner"] for loc in ["DE", "FR", "UK", "NL", "ES"]], labels=[LABELS["localization"]], chain=0.0, flat=True)
    for t in ["Brand refresh: logo lockups", "Brand refresh: color system", "Brand refresh: templates", "Trade show: spring expo booth", "Analyst briefing round", "Customer story: Larkfield", "Customer story: Harwell", "Website navigation rework", "Newsletter redesign", "Q1 media plan"]:
        s = r.randint(-20, 50)
        mktg.make(t, "Marketing", start=s, due=s + r.randint(10, 30))
    w.depend(w.task_names["Cross-launch: Atlas X1: press release"], w.task_names["Marketing claims review: Atlas X1"])
    w.depend(embargo, w.task_names["Cross-launch: Atlas X1: press release"])
    for part, s in [("Display module", -60), ("Relay", -50), ("Enclosure tooling", -80), ("Main board assembly", -40), ("Retail box", -30), ("Shipping carton", -20), ("Power module", -45)]:
        supply.stream(f"Atlas X1: {part}", "Supply Chain", s, s + 62, [f"{part}: RFQ", f"{part}: first article", f"{part}: PO", f"{part}: freight booked", f"{part}: incoming inspection"], chain=0.9)
    for t in ["Second source for Hanover Circuits", "Tariff exposure review", "Q1 forecast with retail", "Freight contract renewal", "Supplier scorecards", "Conflict minerals report"]:
        s = r.randint(-20, 40)
        supply.make(t, "Supply Chain", start=s, due=s + r.randint(10, 40))
    w.depend(mp, w.task_names["Cross-launch: Main board assembly: incoming inspection"])
    for area_name, begin in [("Atlas X1 firmware", -30), ("Harbor 4.0 API", -10), ("Beacon 5", 15)]:
        _, tasks = sec.stream(f"Pen test: {area_name}", "Security", begin, begin + 45, [f"{area_name}: scoping", f"{area_name}: test window", f"{area_name}: finding, auth bypass in debug mode", f"{area_name}: finding, verbose errors", f"{area_name}: finding, weak session lifetime", f"{area_name}: retest", f"{area_name}: report"], chain=0.5, labels=[LABELS["security"]], flat=True)
    w.depend(w.task_names["Beacon 5: Waitlist A invite"], w.task_names["Cross-launch: Beacon 5: report"])
    w.depend(w.task_names["Firmware candidate"], w.task_names["Cross-launch: Atlas X1 firmware: retest"])
    sec.stream("Bug bounty findings", "Security", -30, 30, [f"Bounty: {x}" for x in ["open redirect on sign-in", "IDOR in device share", "stored XSS in a room name", "rate limit bypass on codes", "token in a referrer", "verbose stack trace", "subdomain takeover, old docs", "clickjacking on billing", "weak password reset token", "CSV injection in exports"]], labels=[LABELS["security"]], chain=0.0, flat=True)
    for t in ["Threat model: Harbor gateway", "Threat model: Beacon pairing", "Threat model: Atlas OTA", "Secrets rotation for v4", "Dependency scanning in CI", "Bug bounty scope update", "SOC 2 evidence for Q4", "Access review, Q4", "Vendor security review: Pearl River EMS", "Vendor security review: analytics tool"]:
        s = r.randint(-25, 45)
        sec.make(t, "Security", start=s, due=s + r.randint(5, 30), values={fix_by: w.day(s + 30)})
    support.stream("Atlas X1 knowledge base", "Support", -18, 14, [f"KB: {x}" for x in ["Which wires go where", "Wiring a heat pump", "No C wire: the adapter", "Line voltage: the adapter", "Pairing with the app", "Wi-Fi will not connect", "Reading the wiring check", "Setting a schedule", "Away mode and geofence", "Energy reports explained", "Updating firmware", "Factory reset", "Blank display", "Short cycling", "Fan will not stop", "Humidity control", "Boilers and zones", "Moving house", "Warranty and returns", "Who sees my data"]], labels=[LABELS["documentation"]], chain=0.0, flat=True)
    for prod, begin in [("Atlas X1", -15), ("Beacon 5", 25), ("Harbor 4.0", 35), ("Solstice EU", -60)]:
        support.stream(f"{prod} readiness", "Support", begin, begin + 40, [f"{prod}: KB articles", f"{prod}: agent training", f"{prod}: macro set", f"{prod}: escalation path", f"{prod}: top 20 questions", f"{prod}: phone tree update"], chain=0.3)
    w.depend(w.task_names["Cross-launch: Atlas X1: KB articles"], w.task_names["Atlas X1: Install guide: English (US)"])
    w.depend(w.task_names["Atlas X1: Go/no-go review"], w.task_names["Cross-launch: Atlas X1: agent training"])
    for t in ["Atlas X1 launch dashboard", "Beacon 5 beta dashboard", "Harbor 4.0 migration dashboard", "Solstice EU weekly report", "Telemetry schema v4", "Consent-aware event pipeline", "Retention cohort model", "Churn model refresh", "Exec launch scorecard", "Data catalog cleanup", "Warehouse cost review", "Experiment readout template"]:
        s = r.randint(-30, 35)
        data_team.make(t, "Data", start=s, due=s + r.randint(5, 25), labels=[LABELS["data"]])

    # Rhythm: repeating work, with its history.
    w.repeating("Exec launch readout", "FREQ=WEEKLY;BYDAY=TH", project=atlas.pid, past_dues=[-18, -11, -4], due=w.on("TH"), time="15:00", status="Planned", priority="high", labels=[LABELS["exec-visible"]], assigned=True, reminders=[w.remind_before(w.on("TH"), "15:00", 60)], checklist=[("Atlas status", False), ("Harbor status", False), ("Beacon status", False), ("Red items", False)], notes="Template for the deck: {p:Launch playbook#Exec readout}.")
    w.repeating("Monthly risk review", "FREQ=MONTHLY;BYDAY=2TU", project=legal.pid, past_dues=[-55, -20], due=8, status="In review", notes="Goes through the {p:Risk register}.", assigned=True)
    w.repeating("Sprint planning", "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO", project=harbor.pid, past_dues=[-28, -14], due=0, time="10:00", status="Ready", my_day=True)
    w.repeating("Close out timesheets", "FREQ=MONTHLY;BYMONTHDAY=-1", project=None, past_dues=[-33, -3], due=28, priority="low", key="Close out timesheets")
    w.task("Weekly Beacon crash triage", project=beacon.pid, status="Ready", due=w.on("WE"), repeat="FREQ=WEEKLY;BYDAY=WE", estimate=60, labels=[LABELS["regression"]])
    w.task("Rotate on-call handover", project=harbor.pid, status="Ready", due=w.on("FR"), repeat="FREQ=WEEKLY;BYDAY=FR", mode="completion")

    # Finished, archived and thrown away.
    s2 = Launch("Atlas S2 launch", "ATS", "Atlas X1", "G5 Ship", icon="lucide:rocket", color="yellow", description="The last thermostat launch, shipped in the spring. Archived, and useful to look back at.",
                statuses="Planned:todo, In work:in-progress, Released:done, Dropped:canceled", backlog="Planned", todo="Planned", doing=["In work"], done="Released", canceled="Dropped", slip=0.0, unscheduled=0.0, archived_days=120)
    for t in ["S2 PVT exit", "S2 FCC grant", "S2 retail sell-in", "S2 press release", "S2 launch day", "S2 retrospective", "S2 warranty review", "S2 cost-down review"]:
        s2.make(t, "Product", start=-260, due=-220, status="Released", done_days=r.randint(200, 230), created_days=300)
    w.task("Close the last S2 warranty claim", project=s2.pid, status="In work", due=0, priority="high",
           notes="Still open, in an archived project. It stays out of Upcoming, My Day's suggestions, saved views across projects and the palette; open the project to reach it.")
    w.task("S2 spare parts sell-off", project=s2.pid, status="Planned", due=5)
    w.status(atlas.pid, "Waiting on the old lab", "in-progress", deleted=True)
    mistral = w.project("Project Mistral", icon="lucide:gamepad-2", color="red", deleted_days=11, description="A canceled wearable. In the trash with its tasks.", statuses=STARTER)
    for t in ["Mistral concept review", "Mistral sensor shortlist", "Mistral kill memo"]:
        w.task(t, project=mistral, created_days=80)
    w.task("Duplicate of Rev C samples", project=supply.pid, status="To source", deleted_days=3, notes="Opened twice. In the trash on purpose.")
    w.task("Old Beacon 4 hotfix plan", project=beacon.pid, status="Ideas", deleted_days=7)

    # A launch-wide cloud of small things in the inbox.
    w.task("Read the board pre-read before Thursday", due=w.on("WE"), priority="high", my_day=True, labels=[LABELS["exec-visible"]], reminders=[w.remind_at(w.on("WE"), "16:00")])
    w.task("Approve Q4 contractor renewals", due=-2, priority="medium", assigned=True, labels=[LABELS["needs-decision"]])
    w.task("Book travel for the supplier visit", due=8, labels=[LABELS["vendor"]])
    w.task("Reply to the analyst's question about Atlas", due=0, priority="high", my_day=True)
    w.task("Skip-level with Hye-jin", due=1, time="11:30", repeat="FREQ=MONTHLY;BYDAY=1TU", my_day=True, my_day_on=-1)
    w.task("Idea: launch calendar on the intranet", notes="Could this just be the calendar view? See {p:Launch calendar notes}.")
    w.task("Expense report for the Shenzhen trip", done_days=5, due=-5)
    w.task("Clear the shared drive before the audit", due=20, labels=[LABELS["documentation"]])

    # Comments through the whole set: mentions, web links, references to tasks and to wiki sections.
    PAGES_TO_CITE = ["Launch playbook", "RACI", "Launch playbook#Gate 3: Build", "Launch playbook#Gate 4: Validate", "RACI#Who approves what", "Glossary#Gates", "Risk register#Open risks", "Engineering standards#Code review", "Brand voice#Claims we can make", "Incident response#Severity levels"]
    SAYINGS = [
        "{@} can you take a look at this before the sync?",
        "Moved the date to match {t}. {@} shout if that breaks anything on your side.",
        "This depends on {t}, which is why it has not moved.",
        "The rule we agreed is in {p}.",
        "Numbers are here: https://example.com/reports/{n}",
        "Done on my side. {@} over to you.",
        "(edited) Correction: the vendor said two weeks, not one. Updated the date.",
        "Discussed in the launch sync. Decision: keep the scope, drop the stretch goal. Notes in {p}.",
        "I do not think this is ours. {@} is this with your team?",
        "Blocked until {t} lands. Raising it Thursday.",
        "Looks fine to me.",
        "Can we get this in before the freeze? See {p}.",
        "Test results attached to the build: https://example.com/ci/run/{n}",
        "{@} and {@} to pair on this Wednesday.",
        "(deleted) Wrong thread, sorry.",
        "Same root cause as {t}. Closing the duplicate there.",
        "Customer asked about this again: https://example.com/support/ticket/{n}",
        "Heads up, this slipped a week. Nothing downstream moves yet.",
    ]
    everything = [t for lane in (atlas, harbor, beacon, juniper, ridge, solstice, legal, mktg, supply, sec, support, data_team) for t in lane.tasks]
    for tid in r.sample(everything, len(everything) // 3):
        for _ in range(r.choice([1, 1, 1, 2, 3])):
            text = r.choice(SAYINGS)
            text = re.sub(r"\{@\}", lambda m: "{@:" + r.choice(firsts) + "}", text)
            text = text.replace("{t}", "{t:" + r.choice(everything) + "}").replace("{p}", "{p:" + r.choice(PAGES_TO_CITE) + "}").replace("{n}", str(r.randint(10000, 99999)))
            w.comment(tid, text, r.uniform(0.2, 20), r.choice(firsts))
    # Notes on some tasks, so documents and links are made at import as well.
    NOTE_TEXT = ["See {p} for the rule this follows.", "Context:\n\n- Asked for in the last gate review\n- Owner named in the DRI field\n- Waits on {t}", "## Acceptance\n\n- [ ] Evidence attached\n- [ ] DRI signed off\n- [ ] Linked from the plan\n\nBackground in {p}.", "Keep this small. If it grows, split it and link back to {t}.", "| Item | State |\n| --- | --- |\n| Owner | named |\n| Date | agreed |\n\nSee {p}."]
    task_rows = {x["id"]: x for x in w.rows if x["entity"] == "task"}
    for tid in r.sample(everything, 180):
        if task_rows[tid]["fields"]["notes"]:
            continue
        text = r.choice(NOTE_TEXT).replace("{p}", "{p:" + r.choice(PAGES_TO_CITE) + "}").replace("{t}", "{t:" + r.choice(everything) + "}")
        task_rows[tid]["fields"]["notes"] = text
    # A few files around the big launches.
    for i, (name, mime, data) in enumerate([
            ("harbor-cutover-order.csv", *csv_file("order,service,owner", "1,Identity,Rohan", "2,Devices,Rohan", "3,Telemetry,Wei", "4,Billing,Nadia")),
            ("beacon-home-v7.png", "image/png", png(390, 600, (240, 244, 248), [(20, 40, 350, 120, (70, 130, 180)), (20, 180, 165, 160, (200, 220, 235)), (205, 180, 165, 160, (200, 220, 235))])),
            ("atlas-box-dieline.pdf", "application/pdf", pdf("Atlas X1 retail box dieline, rev 5")),
            ("q4-launch-budget.csv", *csv_file("launch,budget,spent", "Atlas X1,4200000,3910000", "Harbor 4.0,2800000,1650000", "Beacon 5,1900000,840000", "Solstice EU,1200000,1185000")),
            ("hypercare-week-3.pdf", "application/pdf", pdf("Solstice EU hypercare, week 3")),
            ("pen-test-summary.txt", "text/plain", b"Scope: Atlas X1 firmware 0.9.7\nFindings: 1 high, 2 medium, 5 low\nRetest window: this week\n"),
            ("yield-trend.png", "image/png", bars([71, 78, 83, 86, 88, 91, 93, 92], (60, 140, 90)))]):
        w.attach(r.choice([atlas, harbor, beacon, solstice][i % 4].tasks), name, mime, data)
    w.attach(ship, "old-launch-plan.pdf", "application/pdf", pdf("Superseded launch plan"), deleted_days=9)

    # Saved views.
    T = lambda *fields: {"show": list(fields)}
    w.view("All launches: at risk", "list", filter={"fields": [{"field": risk, "op": "any", "value": [RISK["High"], RISK["Critical"]]}]}, display={"group": {"by": "field", "field": launch}, "sort": [{"by": "due", "dir": "asc"}, {"by": "priority", "dir": "desc"}], "fields": {"list": T(risk, gate, dri, "due", "project")}})
    w.view("Slipping now", "list", filter={"due": "overdue", "priorities": ["urgent", "high"]}, display={"group": {"by": "project"}, "sort": [{"by": "due", "dir": "asc"}], "fields": {"list": T("due", "status", dri, team)}})
    w.view("Launch blockers", "list", filter={"dependencies": ["blocked"], "labels": [LABELS["launch-critical"]]}, display={"sort": [{"by": "due", "dir": "asc"}]})
    w.view("Holding up other teams", "list", filter={"dependencies": ["blocking"], "due": "overdue"}, display={"group": {"by": "field", "field": team}, "fields": {"list": T(team, dri, "due")}})
    w.view("Exec table", "table", filter={"labels": [LABELS["exec-visible"]], "showDone": True}, display={"group": {"by": "field", "field": launch}, "sort": [{"by": "due", "dir": "asc"}], "table": {"order": [launch, gate, risk, dri, budget, committed, "due", "status"], "widths": [{"col": "title", "px": 360}], "summaries": [{"col": budget, "fn": "sum"}, {"col": risk, "fn": "bar"}, {"col": "due", "fn": "overdue"}, {"col": committed, "fn": "latest"}]}})
    w.view("All launches timeline", "timeline", filter={"labels": [LABELS["launch-critical"], LABELS["exec-visible"]], "showDone": True}, display={"timeline": {"zoom": "quarter"}, "group": {"by": "project"}})
    w.view("Everything, by project", "table", filter={}, display={"group": {"by": "project"}, "fields": {"table": {"hide": ["assignees"]}}})
    w.view("Everything on the calendar", "calendar", filter={})
    w.view("Certification calendar", "calendar", filter={"labels": [LABELS["regulatory"]], "showDone": True})
    w.view("By team", "list", filter={"priorities": ["urgent", "high"]}, display={"group": {"by": "field", "field": team}, "fields": {"list": T(team, dri, "project", "due")}})
    w.view("EMEA work this week", "list", filter={"due": "week", "fields": [{"field": regions, "op": "any", "value": [REGION["EMEA"]]}]}, display={"group": {"by": "project"}})
    w.view("Sponsored by me", "list", filter={"fields": [{"field": sponsor, "op": "any", "value": ["me"]}]}, display={"group": {"by": "field", "field": launch}})
    w.view("Over the estimate", "table", filter={"time": ["over"], "showDone": True}, display={"group": {"by": "field", "field": team}, "sort": [{"by": "logged", "dir": "desc"}], "fields": {"table": T("estimate", "logged", "remaining", team)}, "table": {"summaries": [{"col": "logged", "fn": "sum"}, {"col": "estimate", "fn": "sum"}]}})
    w.view("Assigned to me", "list", filter={"assignees": ["me"]}, display={"group": {"by": "due"}}, private=True)
    w.view("My time this week", "table", filter={"time": ["mine", "recent"], "showDone": True}, display={"sort": [{"by": "logged", "dir": "desc"}]}, private=True)
    w.view("Decisions wanted", "list", filter={"labels": [LABELS["needs-decision"]]}, display={"group": {"by": "project"}, "fields": {"list": T("project", dri, "due")}})
    w.view("Busiest by label", "list", filter={"labels": [LABELS["cross-team"], LABELS["needs-decision"]], "statuses": ["backlog", "todo", "in-progress"]}, display={"group": {"by": "label"}, "sort": [{"by": "estimate", "dir": "desc"}, {"by": "remaining", "dir": "desc"}, {"by": "due", "dir": "asc"}], "fields": {"list": T("labels", "priority", "start", "checklist", "comments", "reminders", "repeat", sponsor)}})
    w.view("Who has what", "table", filter={"assignees": ["me", EXPORTER], "time": ["estimated"]}, display={"group": {"by": "assignee"}, "sort": [{"by": "start", "dir": "asc"}, {"by": "created", "dir": "desc"}], "table": {"summaries": [{"col": "title", "fn": "count"}, {"col": "remaining", "fn": "min"}, {"col": "status", "fn": "none"}]}})
    w.view("Not sponsored, not North America", "list", filter={"fields": [{"field": sponsor, "op": "none_of", "value": ["me"]}, {"field": regions, "op": "none_of", "value": [REGION["North America"]]}, {"field": risk, "op": "none_of", "value": [RISK["Low"]]}]}, display={"sort": [{"by": "updated", "dir": "desc"}], "group": {"by": "field", "field": sponsor}})
    w.view("No DRI field checks", "list", filter={"fields": [{"field": dri, "op": "not_contains", "value": "Rohan"}, {"field": spec, "op": "contains", "value": "/specs/atl"}, {"field": visible, "op": "checked"}]}, display={"sort": [{"by": "field", "field": spec, "dir": "asc"}, {"by": "field", "field": visible, "dir": "desc"}, {"by": "field", "field": sponsor, "dir": "asc"}], "fields": {"list": T(spec, visible, dri)}})
    w.view("Committed before today", "list", filter={"fields": [{"field": committed, "op": "before", "value": 0}]}, display={"group": {"by": "field", "field": team}, "sort": [{"by": "field", "field": committed, "dir": "asc"}]})
    w.view("Committed this fortnight", "calendar", filter={"fields": [{"field": committed, "op": "between", "value": [0, 14]}]})
    w.view("Missing a budget", "list", filter={"fields": [{"field": budget, "op": "lte", "value": 20000}, {"field": tracker, "op": "contains", "value": "ATL"}], "showDone": True}, display={"sort": [{"by": "field", "field": budget, "dir": "asc"}]})
    w.view("Not customer visible", "list", filter={"fields": [{"field": visible, "op": "unchecked"}, {"field": spec, "op": "not_empty"}]}, display={"sort": [{"by": "project", "dir": "asc"}, {"by": "status", "dir": "asc"}]})
    w.view("Old launch radar", "timeline", filter={"text": "radar"}, deleted_days=30)
    w.view("Atlas status board", "board", project=atlas.pid, filter={"projects": [atlas.pid], "showDone": True}, display={"fields": {"board": T(component, dri, "due", "subtasks")}, "sort": [{"by": "due", "dir": "asc"}]})
    w.view("Atlas by build stage", "board", project=atlas.pid, filter={"projects": [atlas.pid]}, display={"columns": {"by": "field", "field": build}, "fields": {"board": T(component, passed, "due")}})
    w.view("Atlas schedule", "timeline", project=atlas.pid, filter={"projects": [atlas.pid], "showDone": True}, display={"timeline": {"zoom": "month"}, "group": {"by": "field", "field": team}})
    w.view("Atlas critical path", "timeline", project=atlas.pid, filter={"projects": [atlas.pid], "labels": [LABELS["launch-critical"]]}, display={"timeline": {"zoom": "day"}})
    w.view("Atlas test matrix", "table", project=atlas.pid, filter={"projects": [atlas.pid], "text": "Compatibility", "showDone": True}, display={"sort": [{"by": "title", "dir": "asc"}], "table": {"order": [passed, component, build, dri, "status", "due"], "summaries": [{"col": passed, "fn": "filled"}, {"col": "status", "fn": "bar"}]}})
    w.view("Atlas overdue", "list", project=atlas.pid, filter={"projects": [atlas.pid], "due": "overdue"}, display={"sort": [{"by": "priority", "dir": "desc"}, {"by": "due", "dir": "asc"}], "group": {"by": "field", "field": component}})
    w.view("Atlas costs", "table", project=atlas.pid, filter={"projects": [atlas.pid], "fields": [{"field": unit, "op": "not_empty"}], "showDone": True}, display={"sort": [{"by": "field", "field": unit, "dir": "desc"}], "table": {"order": [unit, component, build], "summaries": [{"col": unit, "fn": "sum"}, {"col": unit, "fn": "avg"}]}})
    w.view("Atlas by priority", "board", project=atlas.pid, filter={"projects": [atlas.pid]}, display={"columns": {"by": "priority"}, "fields": {"board": T("labels", "checklist", "priority")}})
    w.view("Sign-offs due now", "list", project=atlas.pid, filter={"projects": [atlas.pid], "showDone": True, "fields": [{"field": signed_on, "op": "on", "value": 12}]})
    w.view("Harbor by reviewer", "board", project=harbor.pid, filter={"projects": [harbor.pid]}, display={"columns": {"by": "field", "field": reviewer}, "fields": {"board": T(reviewer, points)}})
    w.view("Harbor by status", "board", project=harbor.pid, filter={"projects": [harbor.pid], "showDone": True}, display={"columns": {"by": "status"}, "fields": {"board": T(points, service)}})
    w.view("Small stories", "list", project=harbor.pid, filter={"projects": [harbor.pid], "fields": [{"field": points, "op": "lt", "value": 2}]}, display={"group": {"by": "field", "field": reviewer}})
    w.view("Three-pointers", "list", project=harbor.pid, filter={"projects": [harbor.pid], "fields": [{"field": points, "op": "eq", "value": 3}]})
    w.view("Harbor board", "board", project=harbor.pid, filter={"projects": [harbor.pid]}, display={"fields": {"board": T(service, points, "assignees")}})
    w.view("Harbor by service", "board", project=harbor.pid, filter={"projects": [harbor.pid], "showDone": True}, display={"columns": {"by": "field", "field": service}, "fields": {"board": T(points, sprint, "status")}})
    w.view("Harbor sprint table", "table", project=harbor.pid, filter={"projects": [harbor.pid]}, display={"group": {"by": "field", "field": sprint}, "sort": [{"by": "field", "field": points, "dir": "desc"}], "table": {"order": [points, service, sprint, envs, reviewer, runbook, "status"], "summaries": [{"col": points, "fn": "sum"}, {"col": points, "fn": "avg"}]}})
    w.view("Harbor roadmap", "timeline", project=harbor.pid, filter={"projects": [harbor.pid], "showDone": True}, display={"timeline": {"zoom": "quarter"}, "group": {"by": "field", "field": service}})
    w.view("Harbor calendar", "calendar", project=harbor.pid, filter={"projects": [harbor.pid]})
    w.view("Big stories", "list", project=harbor.pid, filter={"projects": [harbor.pid], "fields": [{"field": points, "op": "gte", "value": 8}]}, display={"sort": [{"by": "field", "field": points, "dir": "desc"}], "fields": {"list": T(points, service, sprint)}})
    w.view("My reviews", "list", project=harbor.pid, filter={"projects": [harbor.pid], "fields": [{"field": reviewer, "op": "any", "value": ["me"]}]}, private=True)
    w.view("Beacon board", "board", project=beacon.pid, filter={"projects": [beacon.pid]}, display={"fields": {"board": T(platform, b_points, screen)}})
    w.view("Beacon by screen", "table", project=beacon.pid, filter={"projects": [beacon.pid], "showDone": True}, display={"sort": [{"by": "field", "field": screen, "dir": "asc"}, {"by": "due", "dir": "asc"}], "table": {"order": [screen, platform, b_points, flag, cohort, "status"], "summaries": [{"col": b_points, "fn": "sum"}]}})
    w.view("Beacon plan", "timeline", project=beacon.pid, filter={"projects": [beacon.pid]}, display={"timeline": {"zoom": "month"}})
    w.view("iOS only", "list", project=beacon.pid, filter={"projects": [beacon.pid], "fields": [{"field": platform, "op": "all", "value": [PLAT["iOS"]]}]}, display={"group": {"by": "status"}})
    w.view("Juniper questions", "board", project=juniper.pid, filter={"projects": [juniper.pid], "showDone": True}, display={"fields": {"board": T(method, confidence)}, "sort": [{"by": "field", "field": confidence, "dir": "asc"}]})
    w.view("Unscheduled", "list", project=juniper.pid, filter={"projects": [juniper.pid], "due": "none"}, display={"group": {"by": "field", "field": method}})
    w.view("Partner pipeline", "table", project=ridge.pid, filter={"projects": [ridge.pid], "fields": [{"field": customer, "op": "not_empty"}], "showDone": True}, display={"sort": [{"by": "field", "field": arr, "dir": "desc"}], "table": {"order": [customer, arr, decision_by, "status", "due"], "summaries": [{"col": arr, "fn": "sum"}]}})
    w.view("Ridgeline discovery plan", "timeline", project=ridge.pid, filter={"projects": [ridge.pid]}, display={"timeline": {"zoom": "month"}})
    w.view("Countries", "board", project=solstice.pid, filter={"projects": [solstice.pid], "showDone": True}, display={"columns": {"by": "field", "field": country}, "fields": {"board": T("status", partner, sev)}})
    w.view("Still open after launch", "list", project=solstice.pid, filter={"projects": [solstice.pid]}, display={"sort": [{"by": "priority", "dir": "desc"}], "group": {"by": "field", "field": country}})
    w.view("How it went", "table", project=solstice.pid, filter={"projects": [solstice.pid], "statuses": ["done"], "showDone": True}, display={"sort": [{"by": "completed", "dir": "desc"}], "table": {"order": [country, go_live, "estimate", "logged", "remaining"], "summaries": [{"col": "logged", "fn": "sum"}, {"col": "estimate", "fn": "sum"}, {"col": go_live, "fn": "range"}]}})
    w.view("Legal queue", "board", project=legal.pid, filter={"projects": [legal.pid]}, display={"fields": {"board": T(matter, counsel, jurisdiction, "due")}, "sort": [{"by": "due", "dir": "asc"}]})
    w.view("Outside counsel spend", "table", project=legal.pid, filter={"projects": [legal.pid], "fields": [{"field": counsel_cost, "op": "not_empty"}], "showDone": True}, display={"group": {"by": "field", "field": matter}, "table": {"order": [counsel_cost, counsel, matter, jurisdiction], "summaries": [{"col": counsel_cost, "fn": "sum"}]}})
    w.view("Sign-offs by date", "calendar", project=legal.pid, filter={"projects": [legal.pid], "showDone": True})
    w.view("Campaign calendar", "calendar", project=mktg.pid, filter={"projects": [mktg.pid], "showDone": True})
    w.view("Asset board", "board", project=mktg.pid, filter={"projects": [mktg.pid]}, display={"columns": {"by": "field", "field": asset}, "fields": {"board": T(channel, "due")}})
    w.view("Media plan", "table", project=mktg.pid, filter={"projects": [mktg.pid], "fields": [{"field": media, "op": "gt", "value": 0}], "showDone": True}, display={"sort": [{"by": "field", "field": media, "dir": "desc"}], "table": {"order": [media, channel, asset, goes_live], "summaries": [{"col": media, "fn": "sum"}]}})
    w.view("Inbound freight", "table", project=supply.pid, filter={"projects": [supply.pid]}, display={"group": {"by": "field", "field": incoterm}, "sort": [{"by": "field", "field": eta, "dir": "asc"}], "table": {"order": [supplier, po, eta, lead_time, landed, incoterm, "status"], "summaries": [{"col": landed, "fn": "sum"}, {"col": lead_time, "fn": "max"}, {"col": eta, "fn": "earliest"}]}})
    w.view("Supply board", "board", project=supply.pid, filter={"projects": [supply.pid]}, display={"fields": {"board": T(supplier, eta, "due")}})
    w.view("Open findings", "table", project=sec.pid, filter={"projects": [sec.pid], "text": "finding"}, display={"sort": [{"by": "field", "field": cvss, "dir": "desc"}], "group": {"by": "field", "field": severity}, "table": {"order": [cvss, severity, found_by, fix_by, "status"], "summaries": [{"col": cvss, "fn": "max"}, {"col": cvss, "fn": "avg"}]}})
    w.view("Security board", "board", project=sec.pid, filter={"projects": [sec.pid], "showDone": True}, display={"columns": {"by": "field", "field": severity}})
    w.view("Readiness by launch", "list", project=support.pid, filter={"projects": [support.pid]}, display={"group": {"by": "status"}, "fields": {"list": T(langs, deflect, kb)}})
    w.view("Dashboards", "table", project=data_team.pid, filter={"projects": [data_team.pid], "showDone": True}, display={"table": {"order": [metric, dash, "status", "due"]}})

    # Themes.
    w.theme("Corvane corporate", base="standard", hue=222, suggested_days=6, tokens={"radius-md": {"px": 6}, "font-heading": {"font": "inter"}, "neutral-hue": {"hue": 222}, "neutral-chroma": {"n": 0.008}})
    w.theme("Launch war room", base="contrast", hue=20, target="AAA", dark={"surface-0": {"oklch": [0.14, 0.01, 20]}})
    w.theme("Quiet evenings", base="calm", hue=160, source="0190a000-0000-7000-8000-0000000071e1")
    w.theme("Dense data", base="standard", tokens={"radius-md": {"px": 2}, "font-ui": {"font": "system-sans"}, "dur-panel": {"ms": 120}})
    w.theme("2024 brand (retired)", base="playful", hue=320, deleted_days=60)

    # Templates: enough of them to fill the manager's list.
    def gate_review(n: str, day: int, label: str) -> dict:
        a, b, c = w.id(), w.id(), w.id()
        return dict(v=1, tasks=[
            tpl_task(a, f"{n} gate review", priority="high", category="todo", due=day, estimate=240, labels=[dict(id=LABELS["exec-visible"], name="exec-visible", color="orange")], checklist=["Pack sent 48 hours ahead", "Risks updated", "Decision recorded in the log"], assignees=[EXPORTER, w.person_ids["Rohan"]], values={gate: GATE[label], risk: RISK["Medium"]}, notes="What this gate needs: {p:Launch playbook}."),
            tpl_task(b, "Prepare the review pack", parent=a, due=max(day - 5, 0), estimate=480),
            tpl_task(c, "Pre-read with the DRI", parent=a, due=max(day - 2, 0), waits=[b], estimate=60)])
    for n, day, label in [("G1", 10, "G1 Feasibility"), ("G2", 10, "G2 Plan"), ("G3", 14, "G3 Build"), ("G4", 14, "G4 Validate"), ("G5", 7, "G5 Ship")]:
        w.template("task", f"{n} gate review", gate_review(n, day, label), days=30)
    bug, repro = w.id(), w.id()
    w.template("task", "Launch-blocking bug", dict(v=1, tasks=[tpl_task(bug, "Launch-blocking bug", priority="urgent", category="todo", due=1, labels=[dict(id=LABELS["launch-critical"], name="launch-critical", color="red"), dict(id=LABELS["regression"], name="regression", color="red")], checklist=["Version and device", "Steps to reproduce", "Expected and actual", "Who is told"]), tpl_task(repro, "Reproduce it", parent=bug, due=0), tpl_task(w.id(), "Write a failing test", parent=bug, due=1, waits=[repro])]))
    w.template("task", "Vendor security review", dict(v=1, tasks=[tpl_task(w.id(), "Vendor security review", category="todo", due=20, checklist=["Questionnaire sent", "SOC 2 report read", "DPA signed", "Risk rated"], labels=[dict(id=LABELS["vendor"], name="vendor", color="green"), dict(id=LABELS["security"], name="security", color="teal")])]))
    # A launch as a project template, with its workflow, fields, views and wiki.
    pid = w.id()
    st = {n: w.id() for n in ["Backlog", "Planned", "In work", "In validation", "Released", "Dropped"]}
    cats = dict(zip(st, ["backlog", "todo", "in-progress", "in-progress", "done", "canceled"]))
    fid = {n: w.id() for n in ["Component", "Unit cost USD", "Test passed", "Signed off"]}
    main_id = pair_id(pid, MAIN_PAGE_SPACE)
    plan_id, risks_id = w.id(), w.id()
    tasks, prev = [], None
    for i, (title, due) in enumerate([("Write the launch brief", 5), ("G1 feasibility review", 20), ("G2 plan review", 45), ("EVT build", 70), ("DVT build", 110), ("Certification test plan", 120), ("PVT build", 150), ("G4 validation review", 165), ("Mass production release", 185), ("Ship to retail", 200), ("Launch day", 210), ("Launch retrospective", 240)]):
        tid = w.id()
        tasks.append(tpl_task(tid, title, status=st["Planned"], category="todo", start=max(due - 14, 0), due=due, estimate=480, waits=[prev] if prev else [], priority="high" if i in (1, 2, 7, 8) else "medium", values={fid["Test passed"]: False} if "build" in title else {}, labels=[dict(id=LABELS["launch-critical"], name="launch-critical", color="red")] if i in (8, 9) else [], checklist=["Owner named", "Dates agreed", "Risks logged"] if i % 3 == 0 else []))
        prev = tid
    w.template("project", "Hardware launch", dict(v=1, project=dict(
        id=pid, name="Hardware launch", description="The standard path for a hardware launch, from brief to retrospective, as the Atlas team ran it.", icon="lucide:rocket", color="red",
        statuses=[dict(id=st[n], name=n, category=cats[n]) for n in st],
        fields=[dict(id=fid["Component"], name="Component", type="select", options=[dict(id="board", name="Main board", color="blue"), dict(id="radio", name="Radio", color="red"), dict(id="enclosure", name="Enclosure", color="teal")]),
                dict(id=fid["Unit cost USD"], name="Unit cost USD", type="number", options=[]), dict(id=fid["Test passed"], name="Test passed", type="checkbox", options=[]), dict(id=fid["Signed off"], name="Signed off", type="date", options=[])],
        views=[dict(name="Status board", type="board", filter={"projects": [pid]}, display={"fields": {"board": {"show": [fid["Component"], "due"]}}}),
               dict(name="Schedule", type="timeline", filter={"projects": [pid], "showDone": True}, display={"timeline": {"zoom": "month"}}),
               dict(name="Costs", type="table", filter={"projects": [pid]}, display={"table": {"order": [fid["Unit cost USD"], fid["Component"], "status"], "summaries": [{"col": fid["Unit cost USD"], "fn": "sum"}]}}),
               dict(name="Key dates", type="calendar", filter={"projects": [pid]}, display={})],
        pages=[dict(id=main_id, parent=None, title="", body="What we are launching, for whom, and by when."),
               dict(id=plan_id, parent=main_id, title="Plan", body="## Gates\n\nFollow the company playbook.\n\n## Slip scenarios\n\nWrite these before you need them."),
               dict(id=risks_id, parent=main_id, title="Risks", body="| Risk | Owner | Plan |\n| --- | --- | --- |\n| | | |")]),
        tasks=tasks), days=20)

    w.template("task", "FY25 budget request", dict(v=1, tasks=[tpl_task(w.id(), "Budget request", category="todo")]), days=200, deleted_days=20)
    w.taken_off(w.task_names["Book travel for the supplier visit"], label=LABELS["data"], blocker=w.task_names["Rev C samples"], field=committed, value=w.day(3))

    # Wikis. The company's first, shared by every launch.
    top = w.main_page(name="Corvane home", body="""The handbook for how Corvane ships things. It belongs to everyone; change what is wrong.

## Start here

- New to launches: {p:Launch playbook}
- Who decides what: {p:RACI}
- Words we use: {p:Glossary}

## What is live this quarter

| Launch | Phase | Plan |
| --- | --- | --- |
| Atlas X1 | Shipping, slipping | {p:Atlas plan} |
| Harbor Cloud 4.0 | Building | {p:Harbor architecture} |
| Beacon Mobile 5 | Building | {p:Beacon design notes} |
| Juniper Hub | Concept | {p:Juniper business case} |
| Ridgeline Enterprise | Discovery | {p:Ridgeline discovery notes} |
| Solstice EU | Hypercare | {p:Solstice country notes} |

{img:launch-spend.png}
""")
    w.page_file(top, "launch-spend.png", "image/png", bars([4200, 2800, 1900, 300, 450, 1200], (70, 100, 170), 640, 300))
    playbook = w.page("Launch playbook", top, body="""Every launch goes through the same gates. A gate is a meeting and a decision, written down.

## Gate 0: Concept

A one-page brief and a sponsor.

## Gate 1: Feasibility

Can we build it, and would anyone pay? See {t:Juniper G1 review} for one in progress.

## Gate 2: Plan

Dates, budget and a named DRI for every workstream.

## Gate 3: Build

- [x] Workstreams exist as tasks
- [x] Dependencies are entered, not implied
- [ ] Risks are in the {p:Risk register}

## Gate 4: Validate

Test evidence, certification status and support readiness.

## Gate 5: Ship

| Check | Who | Evidence |
| --- | --- | --- |
| Certification | Legal | Grant letters |
| Factory | Supply Chain | PVT exit |
| Support | Support | Trained agents |
| Security | Security | Retest clean |

> [!WARNING]
> Nothing ships with an open Sev 1.

## Exec readout

Five slides at most: status, red items, next gate, dates moved, and asks.
""", versions=[("Gates: concept, plan, build, ship.", 400, ""), ("## Gate 1\n\nBrief.\n\n## Gate 2\n\nPlan.\n\n## Gate 3\n\nBuild.\n\n## Gate 4\n\nShip.", 200, "Rohan"), ("Six gates now, after the S2 retrospective.\n\n## Gate 0: Concept\n\nA one-page brief.\n\n## Gate 5: Ship\n\nNothing ships with an open Sev 1.", 60, "Dana")])
    w.page("Gate review pack", playbook, body="What goes in the pack, and how early. Use the gate review task templates.\n\n1. One-page summary\n2. Red items\n3. Decision wanted")
    w.page("RACI", top, body="""## Who approves what

| Decision | Responsible | Accountable | Consulted |
| --- | --- | --- | --- |
| Ship date | Launch DRI | Exec sponsor | Everyone |
| Claims | Marketing | Legal | Product |
| Second source | Supply Chain | Hardware lead | Finance |
| Waive a test | QA | Hardware lead | Legal |

## How to escalate

Name it in the launch sync first. Then the {p:Risk register}. Then Dana.
""")
    w.page("Glossary", top, body="## Gates\n\nG0 to G5, see the {p:Launch playbook}.\n\n## Builds\n\nEVT, DVT, PVT, MP: engineering, design and production validation, then mass production.\n\n## DRI\n\nDirectly responsible individual. One name, not a team.")
    w.page("Risk register", top, body="""## Open risks

| Risk | Launch | Rating | Owner |
| --- | --- | --- | --- |
| Rev C fails retest | Atlas X1 | Critical | Kenji |
| Harbor Devices API late | Atlas X1, Beacon 5 | High | Rohan |
| EU residency sign-off | Harbor 4.0 | Medium | Elise |
| Tablet scope | Beacon 5 | Low | Hye-jin |

## Closed risks

Moved to the launch's own wiki when closed.
""", versions=[("## Open risks\n\nRev B fails pre-scan.", 30, "Kenji")])
    w.page("Engineering standards", top, body="""## Code review

Two approvals on anything that touches pairing or billing.

## Testing

```yaml
required:
  - unit
  - integration
  - contract
gates:
  coverage_floor: 70
```

## Releases

Trunk based. Flags over branches.
""")
    w.page("Brand voice", top, body="## How we sound\n\nPlain and exact. Numbers over adjectives.\n\n## Claims we can make\n\nSavings claims need a test report and a Legal review. See {t:Marketing claims review: Atlas X1}.")
    w.page("Incident response", top, body="## Severity levels\n\n| Level | Meaning | Page? |\n| --- | --- | --- |\n| Sev 1 | Customers down | Yes |\n| Sev 2 | Customers degraded | Yes |\n| Sev 3 | Workaround exists | No |\n\n## After\n\nA review within five days, no blame.")
    w.page("Launch calendar notes", top, body="How to read the launch calendar: the Everything on the calendar view shows all of it, Certification calendar only regulatory dates.")
    w.page("2024 org chart", top, deleted_days=14, body="Out of date. In the trash on purpose.")

    amain = w.main_page(atlas.pid, name="Atlas wiki", body="The Atlas X1 thermostat launch. The date, the risks and the log. Start with the {p:Atlas plan}.")
    w.page("Atlas plan", amain, project=atlas.pid, body="""## The date

October 28, then November 11, now November 18. Each move came from {t:Rev C samples}'s chain.

## Critical path

1. {t:Rev C samples}
2. {t:Antenna matching}
3. {t:FCC retest}
4. {t:FCC grant}
5. {t:MP release}
6. {t:Ship Atlas X1 to retail DCs}
7. {t:Atlas X1 press embargo lifts}

## Slip scenarios

| If | Then | New date |
| --- | --- | --- |
| Retest passes | Ship as planned | Nov 18 |
| Retest fails, shielding can | Three weeks | Dec 9 |
| Grant takes the long path | One week | Nov 25 |

## What moved

See the history of this page.
""", versions=[("## The date\n\nOctober 28.", 60, ""), ("## The date\n\nNovember 11. Rev B failed the pre-scan.", 20, "Dana"), ("## The date\n\nNovember 18 if the retest passes.\n\n## Slip scenarios\n\nStill writing these.", 3, "Dana")])
    w.page("Atlas certification log", amain, project=atlas.pid, body="## Radiated emissions\n\nRev B failed at 2.48 GHz. Rev C simulated at +1.2 dB. Retest is {t:FCC retest}.\n\n## Conducted emissions\n\nPassed in September.\n\n## Safety\n\nUL in progress for the line voltage adapter.")
    w.page("Atlas factory notes", amain, project=atlas.pid, body="Line at Pearl River EMS, building 4.\n\n{img:line-yield.png}\n\n## Yield\n\nFirst-pass yield reached 93% in the last pilot.")
    w.page_file(w.page_names["Atlas factory notes"], "line-yield.png", "image/png", bars([71, 78, 83, 86, 88, 91, 93], (60, 140, 90)))
    w.page_file(w.page_names["Atlas factory notes"], "line-yield-old.png", "image/png", bars([60, 66, 70], (150, 150, 150)), deleted_days=10)
    s2main = w.main_page(s2.pid, name="Atlas S2 wiki", body="How the last launch went, kept with the archived project. See {p:S2 retrospective notes}.")
    w.page("S2 retrospective notes", s2main, project=s2.pid, body="## What worked\n\nOne date owner.\n\n## What did not\n\nThe lab was booked too late.")
    w.page("S2 lessons", amain, project=atlas.pid, body="What the last launch taught us:\n\n- Book the lab before you need it\n- Two sources for anything on the critical path\n- Tell Marketing the day a date moves")
    hmain = w.main_page(harbor.pid, name="Harbor wiki", body="Harbor Cloud 4.0: one API, regional data, and the monolith switched off.")
    arch = w.page("Harbor architecture", hmain, project=harbor.pid, body="""## Services

Ten services, moved one at a time. Order is in the cutover plan.

```mermaid
graph TD
  Gateway --> Identity
  Gateway --> Devices
  Devices --> Telemetry
  Telemetry --> Rules
  Rules --> Notifications
```

## Event bus options

| Option | Cost | Ops load |
| --- | ---: | --- |
| Managed | 38k a month | Low |
| Self-run | 14k a month | High |

Decision is {t:Decide on the event bus}.
""", versions=[("## Services\n\nStill drawing this.", 45, "Rohan")])
    w.page("Cutover plan", arch, project=harbor.pid, body="## Order\n\n1. Identity\n2. Devices\n3. Telemetry\n4. Billing\n\n## Rules\n\n- Shadow for two weeks first\n- Roll back within ten minutes\n\nDevices is {t:Harbor 4.0: Devices: Cutover runbook}.")
    w.page("API conventions", arch, project=harbor.pid, body="Resources are plural. Timestamps are UTC. Errors say what to do next.")
    w.page("Regional data", hmain, project=harbor.pid, body="EU tenants stay in the EU. Sign-off is {t:EU data residency sign-off}.")
    bmain = w.main_page(beacon.pid, name="Beacon wiki", body="Beacon Mobile 5, redrawn from the pairing flow out.")
    w.page("Beacon design notes", bmain, project=beacon.pid, body="## Principles\n\n- Pairing in under two minutes\n- Offline first\n\n## Open questions\n\n{t:Tablet decision}\n\n{img:home-v7.png}")
    w.page_file(w.page_names["Beacon design notes"], "home-v7.png", "image/png", png(390, 600, (240, 244, 248), [(20, 40, 350, 120, (70, 130, 180)), (20, 180, 165, 160, (200, 220, 235)), (205, 180, 165, 160, (200, 220, 235))]))
    w.page("Beta plan", bmain, project=beacon.pid, body="Staff, then friends, then two waitlists, then public. Each cohort must hold 99.5% crash-free before the next.")
    jmain = w.main_page(juniper.pid, name="Juniper wiki", body="A home gateway for 2028, if the case holds up.")
    w.page("Juniper business case", jmain, project=juniper.pid, body="## The bet\n\nInstallers want one box.\n\n## What we do not know\n\nAlmost everything. See the Juniper questions view.")
    rmain = w.main_page(ridge.pid, name="Ridgeline wiki", body="A fleet console for people who manage hundreds of thermostats.")
    w.page("Ridgeline discovery notes", rmain, project=ridge.pid, body="Six design partners. Larkfield has the most sites, Harwell the strictest security review.")
    smain = w.main_page(solstice.pid, name="Solstice wiki", body="Launched October 14 in eight countries.")
    w.page("Solstice country notes", smain, project=solstice.pid, body="## Germany\n\nBiggest volume. No open issues.\n\n## Poland\n\nRounding error on invoices: {t:Poland: tariff rounding error on invoices}.\n\n## Sweden\n\nRetail feed: {t:Sweden: retailer feed missing the winter plan}.")
    lmain = w.main_page(legal.pid, name="Legal wiki", body="How to ask Legal for something, and how long it takes.")
    w.page("How to file a request", lmain, project=legal.pid, body="1. Make a task in this project\n2. Say the launch and the date\n3. Attach what you have\n\n> [!NOTE]\n> Certification work needs six weeks' notice.")
    mmain = w.main_page(mktg.pid, name="Marketing wiki", body="Campaign calendar, briefs and who to ask.")
    w.page("Campaign brief template", mmain, project=mktg.pid, body="## Audience\n\n## Message\n\n## Proof\n\n## Channels and budget")
    return w


def edge_cases(today: dt.date, now: dt.datetime, rng: random.Random) -> Workspace:
    w = Workspace("Sample: Edge Cases", "edge-cases", today, now, rng, [("Sample data", "owner"), ("Someone Else", "member")])

    # Every palette color, as a label and as a project.
    labels = [w.label(f"{c} label", c) for c in PALETTE]
    longest = w.label("L" * 60, "blue")  # the longest label name
    icons = ["lucide:folder", "📦", "", "lucide:bug", "🌍", "lucide:music", "", "lucide:camera", "🧪",
             "lucide:leaf", "lucide:gamepad-2", "🚀"]
    palette_projects = [w.project(f"Color {c}", icon=icons[i], color=c, statuses=STARTER[:2] + STARTER[3:4])
                        for i, c in enumerate(PALETTE)]
    for i, p in enumerate(palette_projects):
        w.task(f"A task in the {PALETTE[i]} project", project=p, labels=[labels[i]])

    # Limits.
    limits = w.project("Limits " + "x" * 193, icon="lucide:target", color="red",
                       description="The project name is 200 characters, the most allowed. " + "Long description. " * 200,
                       statuses=[("S" * 60, "todo"), ("Done", "done")])
    sel = w.field("F" * 60, "select", project=limits,
                  options=[(f"o{i}", f"Option {i} " + "long " * (i % 4), PALETTE[i % 12]) for i in range(24)])
    multi = w.field("Every color", "multi_select", project=limits,
                    options=[(c, c.title(), c) for c in PALETTE])
    big = w.task("T" * 500, project=limits, priority="urgent", labels=labels + [longest],
                 notes=RICH_NOTES + "\n\n" + ("Long paragraph. " * 400),
                 values={sel: "o23", multi: PALETTE},
                 checklist=[(f"Checklist item {i + 1}" + (" with a much longer text that wraps onto a second line" if i % 9 == 0 else ""),
                             i % 3 == 0) for i in range(80)],
                 comments=[(f"Comment {i + 1}. " + "Some words. " * (i % 7), 10 - i * 0.3) for i in range(30)],
                 estimate=999 * 60, logged=[(-1, 24 * 60, "N" * 500), (0, 1, "")])
    w.task("Checklist text at its limit", project=limits,
           checklist=[("K" * 500, False)])

    # Nesting: six levels of subtasks.
    nest = w.project("Deep nesting", icon="lucide:folder", color="indigo")
    parent = None
    for depth in range(1, 7):
        parent = w.task(f"Level {depth} task", project=nest, parent=parent, due=depth)
        w.task(f"Level {depth} sibling", project=nest, parent=parent if depth > 1 else None)
    wide = w.task("A parent with 25 subtasks", project=nest)
    for i in range(25):
        w.task(f"Subtask {i + 1}", project=nest, parent=wide, done_days=1 if i % 4 == 0 else None)

    # Many statuses, several per category, and a deleted one.
    flow = w.project("Twelve statuses", icon="lucide:wrench", color="cyan",
                     statuses=[("Idea", "backlog"), ("Someday", "backlog"), ("Next", "todo"), ("Scheduled", "todo"),
                               ("Started", "in-progress"), ("Paused", "in-progress"), ("Review", "in-progress"),
                               ("Testing", "in-progress"), ("Staging", "in-progress"), ("Live", "done"),
                               ("Archived", "done"), ("Dropped", "canceled")])
    w.status(flow, "Removed status", "todo", deleted=True)
    for name in w.project_statuses[flow]:
        w.task(f"In {name}", project=flow, status=name)

    # A project with no statuses at all.
    bare = w.project("No statuses", color="orange", statuses=None,
                     description="Every task here has no status; boards by status have nowhere to put them.")
    for t in ["Loose task one", "Loose task two"]:
        w.task(t, project=bare)

    # A long list, for scrolling and drawing a screenful at a time.
    long = w.project("Long list (150 tasks)", icon="lucide:book", color="blue")
    words = ["Draft", "Review", "Update", "Check", "Plan", "Fix", "Ship", "Test", "Call", "Write"]
    things = ["the report", "invoices", "the roadmap", "release notes", "the backlog", "a demo", "the budget", "slides"]
    for i in range(150):
        w.task(f"{rng.choice(words)} {rng.choice(things)} #{i + 1}", project=long,
               priority=rng.choice(["none", "none", "low", "medium", "high", "urgent"]),
               due=rng.choice([None, None, rng.randint(-20, 40)]),
               labels=rng.sample(labels, rng.choice([0, 0, 1, 2])),
               status=rng.choice(["Backlog", "To do", "To do", "In progress", "Done"]))

    # Text that is not plain ASCII.
    world = w.project("Unicode 🌐 Ünïcödé", icon="🌐", color="green")
    for t in ["日本語のタスク: 会議の準備", "مهمة باللغة العربية", "Задача на русском", "Emoji only 🎉🎂🎈",
              "Combining: é ä ñ", "Ελληνικά και עברית μαζί", "Math: ∑ ∫ √ ≠ ≤ ≥ ∞"]:
        w.task(t, project=world, notes=t + "\n\n" + t)

    # The trash and archive.
    gone = w.project("Deleted project", color="red", deleted_days=2,
                     description="In the trash with its tasks.")
    w.task("Task in a deleted project", project=gone)
    trash = w.project("Trash and archive", icon="lucide:hammer", color="amber")
    w.task("Deleted task", project=trash, deleted_days=1)
    keep = w.task("Kept parent", project=trash)
    w.task("Deleted subtask", project=trash, parent=keep, deleted_days=1)
    w.project("Archived empty project", color="purple", archived_days=30)

    # Dates around today, in the inbox.
    for label, start, due, clock in [("Due a year ago", None, -365, None), ("Due yesterday", None, -1, None),
                                     ("Due today at 23:59", None, 0, "23:59"), ("Due today at 00:00", None, 0, "00:00"),
                                     ("Due tomorrow", None, 1, None), ("Starts today, due in a week", 0, 7, None),
                                     ("Starts in the past, no due date", -5, None, None),
                                     ("Starts and is due on the same day", 3, 3, "09:15"),
                                     ("Due in two years", None, 730, None)]:
        w.task(label, start=start, due=due, time=clock)
    for p in ["none", "low", "medium", "high", "urgent"]:
        w.task(f"Priority {p}", priority=p)
    w.task("Repeats on the last day of the month", due=0, repeat="FREQ=MONTHLY;BYMONTHDAY=-1")
    w.task("Repeats every weekday", due=0, repeat="FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR")
    w.task("Repeats every 999 days", due=0, repeat="FREQ=DAILY;INTERVAL=999")

    # What an import does with other people: the zip credits them with work and
    # gives them private rows, and only the first two kinds come in, as the
    # importer's. The report says how many rows were left out.
    other = w.person_ids["Someone Else"]
    people = w.project("Someone else's work", icon="lucide:users", color="pink",
                       description="An import keeps one person. Compare this with the zip: the task's assignee, My Day entry and reminder for someone else, and their personal view, are left out.")
    who = w.field("Owner", "user", project=people)
    theirs = w.task("Task with another person on it", project=people, values={who: other},
                    comments=[("By someone else, so the importer's now. @[Someone Else](user:" + other + ") stays as words.", 2, "Someone Else")],
                    logged=[(-1, 30, "By someone else, so the importer's now", "Someone Else")],
                    notes="Its Owner field names someone who is not here, so the panel's list shows nothing chosen.")
    w.row("task_assignee", pair_id(theirs, other), task_id=theirs, user_id=other, deleted_at=None)
    w.row("my_day_entry", pair_id(theirs, other), task_id=theirs, user_id=other, day=today.isoformat(), sort_key=w.next_key("my_day"), deleted_at=None)
    w.row("reminder", w.id(), task_id=theirs, user_id=other, deleted_at=None, **w.remind_at(1, "15:00"))
    w.row("saved_view", w.id(), project_id=None, owner_id=other, name="Someone else's personal view", type="list",
          config={"filter": {}}, display=None, sort_key=w.next_key("view"), deleted_at=None)
    # An attachment whose file is not in the zip.
    w.row("attachment", w.id(), task_id=theirs, filename="left-out.txt", mime="text/plain", size=4, storage_key="sample", uploaded_by=EXPORTER, deleted_at=None)

    w.view("V" * 80, "list", filter={})
    w.view("Matches nothing", "list", filter={"text": "zzzz-no-such-task"})
    # The app only makes boards inside a project. A board across projects can
    # still come in, and shows "Nothing matches" over tasks that do match.
    w.view("Board across projects", "board", filter={})
    w.theme("T" * 60, base="contrast", target="AAA")
    w.template("task", "N" * 120, dict(v=1, tasks=[tpl_task(w.id(), "A task from a template with the longest name", category="todo", assignees=[other])]))
    w.view("Shows done and canceled", "list", filter={"showDone": True, "statuses": ["done", "canceled"]},
           display={"group": {"by": "project"}})
    w.view("Sorted three ways", "list", filter={"projects": [long]},
           display={"sort": [{"by": "status", "dir": "asc"}, {"by": "priority", "dir": "desc"}, {"by": "title", "dir": "asc"}]})
    w.view("Long list board by status", "board", project=long, filter={"projects": [long]})
    w.view("Inbox only", "list", filter={"projects": ["inbox"]}, display={"group": {"by": "priority"}})

    # Wikis: the longest title, pages nested deep, an empty page, a long
    # one, a deleted page with its subpage, and a project's.
    top = w.main_page()
    w.page("P" * 200, top, body="The title is 200 characters, the most allowed.")
    at = top
    for depth in range(1, 13):
        at = w.page(f"Level {depth}", at, body=f"This page is {depth} levels down.")
    w.page("", top)
    w.page("A long page", top, body=RICH_NOTES + "\n\n" + ("Long paragraph. " * 2000))
    gone_page = w.page("Deleted page", top, deleted_days=1, body="In the trash.")
    w.page("Under the deleted page", gone_page, deleted_days=1, body="Deleted with its parent, and comes back with it.")
    limits_main = w.main_page(limits, body="A wiki in the limits project.")
    w.page("Subpage", limits_main, project=limits)
    return w


SETS = {"household": household, "business": small_business, "enterprise": enterprise, "edge-cases": edge_cases}


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("out", type=Path)
    ap.add_argument("--today", type=dt.date.fromisoformat, default=dt.date.today())
    ap.add_argument("--seed", type=int, default=7)
    ap.add_argument("--set", nargs="+", choices=list(SETS), default=["household", "business", "enterprise"], dest="sets",
                    help="which workspaces to write (default: household business enterprise)")
    args = ap.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)
    now = dt.datetime.now(dt.timezone.utc)
    if args.today != now.date():
        now = dt.datetime.combine(args.today, dt.time(17, 0), dt.timezone.utc)
    for name in args.sets:
        i = list(SETS).index(name) + 1
        w = SETS[name](args.today, now, random.Random(args.seed * 100 + i))
        path = args.out / f"mila-sample-{w.slug}.zip"
        counts = w.write(path)
        print(f"{path.name}: {path.stat().st_size} bytes: " + ", ".join(f"{n} {k}" for k, n in sorted(counts.items())))


if __name__ == "__main__":
    main()
