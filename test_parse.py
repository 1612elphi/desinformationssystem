"""Offline validation of the scraper parsers against saved HTML in /tmp."""
import scraper

for f in ("gremien", "termine", "termin"):
    pass

g = open("/tmp/gremien.html", encoding="utf-8", errors="replace").read()
committees = scraper.parse_committees(g)
print(f"committees parsed: {len(committees)}")
for k, v in list(committees.items())[:5]:
    print("   ", k, "->", v)

cal = open("/tmp/termine.html", encoding="utf-8", errors="replace").read()
ids = scraper.parse_calendar_termine(cal)
print(f"\ncalendar termin ids: {len(ids)} -> {ids[:6]}")

h = open("/tmp/termin.html", encoding="utf-8", errors="replace").read()
m = scraper.parse_meeting(h, "https://x/termin-10526", "termin-10526")
print(f"\nmeeting title: {m['title']!r}")
print(f"body_name:     {m['body_name']!r}")
print(f"date/time/loc: {m['date']} | {m['time']} | {m['location']}")
print(f"public flag:   {m['public']}")
print(f"agenda items:  {len(m['agenda'])}")
for ai in m["agenda"][:3]:
    print(f"   [{ai['anchor']}] #{ai['number']} pub={ai['public']} {ai['title'][:60]!r}")
print(f"files:         {len(m['files'])}")
from collections import Counter
print("doc_type dist:", dict(Counter(f["doc_type"] for f in m["files"])))
for fl in m["files"][:6]:
    print(f"   {fl['id']} [{fl['doc_type']:16}] anchor={fl['agenda_anchor']} :: {fl['label'][:50]!r}")
print("files with no anchor (meeting-level):",
      sum(1 for f in m["files"] if not f["agenda_anchor"]))
