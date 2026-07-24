import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Theme,
  Header,
  HeaderName,
  HeaderGlobalBar,
  DataTableSkeleton,
  Search,
  Dropdown,
  DatePicker,
  DatePickerInput,
  Button,
  Table,
  TableHead,
  TableRow,
  TableHeader,
  TableBody,
  TableCell,
  TableContainer,
  Pagination,
  Tag,
  Modal,
  Loading,
  InlineLoading,
  Tile,
  Link,
  Tabs,
  TabList,
  Tab,
  TabPanels,
  TabPanel,
  Toggle,
  Accordion,
  AccordionItem,
  MultiSelect,
} from "@carbon/react";
import { Launch, Document as DocIcon, Time, Renew, Rss, Calendar } from "@carbon/icons-react";
import {
  search as apiSearch,
  getFacets,
  getStats,
  getDocument,
  fileUrl,
  versionUrl,
  getMeetings,
  getMeeting,
  getLive,
  refreshMeeting,
  getCommittees,
  getCommittee,
  getPerson,
  getVorlage,
  getAnalytics,
  type Doc,
  type Facets,
  type Stats,
  type Meeting,
  type Member,
  type CommitteeListItem,
  type Committee,
  type Vote,
  type Person,
  type VorlageChain,
  type Analytics,
} from "./api";

const PAGE_SIZE = 25;
const RIS_BASE = "https://sitzungskalender.karlsruhe.de/db/ratsinformation";

const PUBLIC_OPTS = [
  { id: "all", label: "Öffentlichkeit: alle" },
  { id: "public", label: "Nur öffentlich" },
  { id: "nonpublic", label: "Nur nicht öffentlich" },
];

// Submitter code → Carbon Tag colour + full name (used in chip tooltips + filter list).
const SUBMITTERS: Record<string, { color: any; name: string }> = {
  SVK: { color: "cool-gray", name: "Stadtverwaltung Karlsruhe" },
  CDU: { color: "gray", name: "Christlich Demokratische Union" },
  B90: { color: "green", name: "Bündnis 90/Die Grünen" },
  SPD: { color: "red", name: "Sozialdemokratische Partei Deutschlands" },
  LIN: { color: "magenta", name: "Die Linke" },
  FDP: { color: "warm-gray", name: "Freie Demokratische Partei" },
  AFD: { color: "blue", name: "Faschistische Nazi-Hurensöhne" },
  FWV: { color: "teal", name: "Freie Wähler" },
  KAL: { color: "purple", name: "Karlsruher Liste" },
  VOL: { color: "purple", name: "Volt" },
  PAR: { color: "magenta", name: "Die PARTEI" },
  HEI: { color: "high-contrast", name: "Die Heimat" },
};

// Preferred display order for the originator filter (SVK first, AfD near the end).
const SUBMITTER_ORDER = ["SVK", "CDU", "B90", "SPD", "LIN", "FDP", "FWV", "KAL", "VOL", "PAR", "AFD", "HEI"];

// Primary-table columns (customisable + persisted). render() returns the cell content.
interface ColumnDef {
  key: string;
  label: string;
  cls: string;
  render: (d: Doc) => any;
}
const ALL_COLUMNS: ColumnDef[] = [
  { key: "date", label: "Datum", cls: "dis-c-date", render: (d) => d.meeting_date || "—" },
  {
    key: "gremium", label: "Gremium", cls: "dis-c-gremium",
    render: (d) => <span className="dis-ellip" title={d.body_name || ""}>{d.body_name || "—"}</span>,
  },
  {
    key: "top", label: "TOP", cls: "dis-c-top",
    render: (d) => {
      const num = d.agenda_number ? `TOP ${d.agenda_number}` : "";
      const title = d.agenda_title || "";
      const full = [num, title].filter(Boolean).join(" — ");
      return full ? (
        <span className="dis-ellip" title={full}>
          {num && <span className="dis-top-num">{num}</span>}
          {title && <span className="dis-top-title">{title}</span>}
        </span>
      ) : (
        <span className="dis-top-none">—</span>
      );
    },
  },
  { key: "sub", label: "Einbringer", cls: "dis-c-sub", render: (d) => <SubmitterChips codes={d.submitters} /> },
  { key: "typ", label: "Typ", cls: "dis-c-typ", render: (d) => <Tag type="gray" size="sm">{d.doc_type}</Tag> },
  {
    key: "doc", label: "Dokument", cls: "dis-c-doc",
    render: (d) => (
      <span className="dis-doccell">
        <span className="dis-ellip" title={d.label}>{d.label}</span>
        {d.snippet && <Snippet text={d.snippet} />}
      </span>
    ),
  },
  {
    key: "cat", label: "Kategorie", cls: "dis-c-cat",
    render: (d) => (d.category ? <Tag type="blue" size="sm">{d.category}</Tag> : ""),
  },
];
const COL_KEYS = ALL_COLUMNS.map((c) => c.key);

function SubmitterChips({ codes }: { codes?: string[] }) {
  if (!codes || codes.length === 0) return null;
  return (
    <span className="dis-tags" style={{ display: "inline-flex" }}>
      {codes.map((c) => {
        const meta = SUBMITTERS[c] || { color: "cool-gray", name: c };
        return (
          <span key={c} title={meta.name}>
            <Tag type={meta.color} size="sm">
              {c}
            </Tag>
          </span>
        );
      })}
    </span>
  );
}

function isoDate(d?: Date): string {
  if (!d) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// Shareable URLs: filters, tab and open modals live in the query string.
const initQS = new URLSearchParams(window.location.search);
const TAB_SLUGS = ["dokumente", "sitzungen", "gremien", "statistik"];

// FTS snippet highlighting: the backend delimits matches with control chars
// (\x01…\x02), never markup — split and wrap, so document text can't inject HTML.
function Snippet({ text }: { text: string }) {
  const parts = text.split("\u0001");
  return (
    <span className="dis-snippet">
      {parts.map((seg, i) => {
        if (i === 0) return <span key={i}>{seg}</span>;
        const cut = seg.indexOf("\u0002");
        const hl = cut >= 0 ? seg.slice(0, cut) : seg;
        const rest = cut >= 0 ? seg.slice(cut + 1) : "";
        return (
          <span key={i}>
            <mark>{hl}</mark>
            {rest}
          </span>
        );
      })}
    </span>
  );
}

export default function App() {
  const [facets, setFacets] = useState<Facets>({ committees: [], doc_types: [], submitters: [] });
  const [stats, setStats] = useState<Stats | null>(null);

  const [q, setQ] = useState(initQS.get("q") || "");
  const [committee, setCommittee] = useState(initQS.get("committee") || "");
  const [docType, setDocType] = useState(initQS.get("type") || "");
  const [pub, setPub] = useState(initQS.get("pub") || "all");
  const [from, setFrom] = useState(initQS.get("from") || "");
  const [to, setTo] = useState(initQS.get("to") || "");
  const [topic, setTopic] = useState(initQS.get("topic") || "");
  const [submitterCode, setSubmitterCode] = useState(initQS.get("sub") || "");
  const [tabIdx, setTabIdx] = useState(() =>
    Math.max(0, TAB_SLUGS.indexOf(initQS.get("tab") || "dokumente"))
  );
  const [personId, setPersonId] = useState(initQS.get("person") || "");
  const [vorlageNr, setVorlageNr] = useState(initQS.get("vorlage") || "");
  const [cols, setCols] = useState<string[]>(() => {
    try {
      const s = JSON.parse(localStorage.getItem("dis-cols") || "null");
      const valid = Array.isArray(s) ? s.filter((k: string) => COL_KEYS.includes(k)) : [];
      if (valid.length) return valid;
    } catch {
      /* ignore */
    }
    return COL_KEYS;
  });
  useEffect(() => {
    try {
      localStorage.setItem("dis-cols", JSON.stringify(cols));
    } catch {
      /* ignore */
    }
  }, [cols]);
  const visibleColumns = ALL_COLUMNS.filter((c) => cols.includes(c.key));

  const [results, setResults] = useState<Doc[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<Doc | null>(null);
  const [live, setLive] = useState<Meeting[]>([]);
  // single shared meeting-detail modal, opened from the live banner OR the Sitzungen tab
  const [openMeeting, setOpenMeeting] = useState<Meeting | null>(null);
  const openMeetingById = useCallback(
    (id: string) => getMeeting(id).then(setOpenMeeting).catch(() => {}),
    []
  );

  useEffect(() => {
    getFacets().then(setFacets).catch(() => {});
    getStats().then(setStats).catch(() => {});
    // deep links: open the doc/meeting the URL points at
    const doc = initQS.get("doc");
    if (doc) getDocument(doc).then(setSelected).catch(() => {});
    const mid = initQS.get("meeting");
    if (mid) openMeetingById(mid);
  }, [openMeetingById]);

  // keep the URL shareable: filters, tab and open modals round-trip through it
  useEffect(() => {
    const p = new URLSearchParams();
    if (q) p.set("q", q);
    if (committee) p.set("committee", committee);
    if (docType) p.set("type", docType);
    if (submitterCode) p.set("sub", submitterCode);
    if (pub !== "all") p.set("pub", pub);
    if (from) p.set("from", from);
    if (to) p.set("to", to);
    if (topic) p.set("topic", topic);
    if (tabIdx) p.set("tab", TAB_SLUGS[tabIdx]);
    if (selected) p.set("doc", selected.id);
    if (openMeeting) p.set("meeting", openMeeting.id);
    if (personId) p.set("person", personId);
    if (vorlageNr) p.set("vorlage", vorlageNr);
    const s = p.toString();
    window.history.replaceState(null, "", s ? `?${s}` : window.location.pathname);
  }, [q, committee, docType, submitterCode, pub, from, to, topic, tabIdx,
      selected, openMeeting, personId, vorlageNr]);

  // poll for a session happening right now
  useEffect(() => {
    let alive = true;
    const tick = () =>
      getLive()
        .then((r) => alive && setLive(r.meetings || []))
        .catch(() => {});
    tick();
    const iv = setInterval(tick, 60_000);
    return () => {
      alive = false;
      clearInterval(iv);
    };
  }, []);

  // Monotonic sequence so a slow, superseded response can never overwrite the
  // results of a newer search (rapid filter/pagination changes race otherwise).
  const searchSeq = useRef(0);
  const runSearch = useCallback(
    async (pageArg = 1) => {
      const seq = ++searchSeq.current;
      setLoading(true);
      setError("");
      try {
        const res = await apiSearch({
          q,
          committee,
          type: docType,
          public: pub,
          from,
          to,
          topic,
          submitter: submitterCode,
          limit: PAGE_SIZE,
          offset: (pageArg - 1) * PAGE_SIZE,
        });
        if (seq !== searchSeq.current) return;
        setResults(res.results);
        setTotal(res.total);
        setPage(pageArg);
      } catch (e: any) {
        if (seq !== searchSeq.current) return;
        setError(e?.message || "Fehler bei der Suche");
        setResults([]);
        setTotal(0);
      } finally {
        if (seq === searchSeq.current) setLoading(false);
      }
    },
    [q, committee, docType, pub, from, to, topic, submitterCode]
  );

  // initial + filter-driven load (debounced on the query string)
  useEffect(() => {
    const t = setTimeout(() => runSearch(1), q ? 350 : 0);
    return () => clearTimeout(t);
  }, [q, committee, docType, pub, from, to, topic, submitterCode, runSearch]);

  const committeeItems = useMemo(
    () => [{ id: "", label: "Alle Gremien" }, ...facets.committees.map((c) => ({ id: c.name, label: c.name }))],
    [facets]
  );
  const typeItems = useMemo(
    () => [{ id: "", label: "Alle Dokumenttypen" }, ...facets.doc_types.map((t) => ({ id: t, label: t }))],
    [facets]
  );
  const submitterItems = useMemo(() => {
    const present = [...(facets.submitters || [])].sort(
      (a, b) => (SUBMITTER_ORDER.indexOf(a) + 1 || 99) - (SUBMITTER_ORDER.indexOf(b) + 1 || 99)
    );
    return [
      { id: "", label: "Alle Einbringer" },
      ...present.map((c) => ({ id: c, label: SUBMITTERS[c]?.name || c })),
    ];
  }, [facets]);

  const applyTopic = (t: string) => {
    setSelected(null);
    setTopic(t);
  };

  const activeFilters = [
    committee && { k: "committee", label: `Gremium: ${committee}`, clear: () => setCommittee("") },
    docType && { k: "type", label: `Typ: ${docType}`, clear: () => setDocType("") },
    submitterCode && {
      k: "sub",
      label: `Einbringer: ${SUBMITTERS[submitterCode]?.name || submitterCode}`,
      clear: () => setSubmitterCode(""),
    },
    pub !== "all" && { k: "pub", label: PUBLIC_OPTS.find((o) => o.id === pub)?.label || pub, clear: () => setPub("all") },
    from && { k: "from", label: `ab ${from}`, clear: () => setFrom("") },
    to && { k: "to", label: `bis ${to}`, clear: () => setTo("") },
    topic && { k: "topic", label: `Thema: ${topic}`, clear: () => setTopic("") },
  ].filter(Boolean) as { k: string; label: string; clear: () => void }[];

  const resetAll = () => {
    setQ("");
    setCommittee("");
    setDocType("");
    setSubmitterCode("");
    setPub("all");
    setFrom("");
    setTo("");
    setTopic("");
  };

  return (
    <Theme theme="g100">
      <Header aria-label="Desinformationssystem">
        <HeaderName href="/" prefix="Karlsruhe">
          Desinformationssystem
        </HeaderName>
        <HeaderGlobalBar>
          <div className="dis-header-right">
            <Tag type="cool-gray" size="sm">
              Open Data
            </Tag>
            <a
              className="dis-header-link"
              href="https://sitzungskalender.karlsruhe.de/db/ratsinformation/start"
              target="_blank"
              rel="noreferrer"
            >
              Original-RIS <Launch size={14} />
            </a>
          </div>
        </HeaderGlobalBar>
      </Header>

      <main className="dis-content">
        <div className="dis-hero">
          <h1>Karlsruher Ratsdokumente</h1>
          <p className="dis-sub">
            Durchsuchbares Archiv des Sitzungskalenders der Stadt Karlsruhe — täglich
            aktualisiert, volltextindiziert und kategorisiert.
          </p>
        </div>

        {stats && (
          <div className="dis-stats">
            <Stat n={stats.documents} l="Dokumente" />
            <Stat n={stats.meetings} l="Sitzungen" />
            <Stat n={stats.bodies} l="Gremien" />
            <Stat n={stats.with_text} l="Volltext" />
            <Stat n={stats.enriched} l="Analysiert" />
            <div className="dis-stat">
              <div className="num" style={{ fontSize: "0.95rem" }}>
                {stats.last_scrape ? stats.last_scrape.replace("T", " ").slice(0, 16) : "—"}
              </div>
              <div className="lbl">Letzter Abruf</div>
            </div>
          </div>
        )}

        {live.length > 0 && (
          <div className="dis-live" role="status">
            <span className="dis-live-pulse" aria-hidden />
            <span className="dis-live-label">Jetzt live</span>
            <span className="dis-live-text">
              {live.length === 1
                ? `${live[0].body_name || live[0].title || "Sitzung"} tagt gerade`
                : `${live.length} Sitzungen tagen gerade`}
              {live[0].time ? ` · ${live[0].time}` : ""}
            </span>
            <Button
              kind="primary"
              size="sm"
              renderIcon={Time}
              onClick={() => openMeetingById(live[0].id)}
            >
              Protokoll ansehen
            </Button>
          </div>
        )}

        <Tabs selectedIndex={tabIdx} onChange={({ selectedIndex }: any) => setTabIdx(selectedIndex)}>
        <TabList aria-label="Ansichten" contained>
          <Tab>Dokumente</Tab>
          <Tab>Sitzungen</Tab>
          <Tab>Gremien</Tab>
          <Tab>Statistik</Tab>
        </TabList>
        <TabPanels>
        <TabPanel>
        <div className="dis-panel">
        <Search
          size="lg"
          labelText="Suche"
          placeholder="Volltextsuche über alle Dokumente, Zusammenfassungen und Schlagworte…"
          value={q}
          onChange={(e) => setQ((e.target as HTMLInputElement).value)}
          onClear={() => setQ("")}
        />

        <div className="dis-filters">
          <Dropdown
            id="committee"
            titleText="Gremium"
            label="Alle Gremien"
            items={committeeItems}
            itemToString={(i: any) => (i ? i.label : "")}
            selectedItem={committeeItems.find((i) => i.id === committee) || committeeItems[0]}
            onChange={({ selectedItem }: any) => setCommittee(selectedItem?.id || "")}
          />
          <Dropdown
            id="doctype"
            titleText="Dokumenttyp"
            label="Alle Dokumenttypen"
            items={typeItems}
            itemToString={(i: any) => (i ? i.label : "")}
            selectedItem={typeItems.find((i) => i.id === docType) || typeItems[0]}
            onChange={({ selectedItem }: any) => setDocType(selectedItem?.id || "")}
          />
          <Dropdown
            id="submitter"
            titleText="Einbringer"
            label="Alle Einbringer"
            items={submitterItems}
            itemToString={(i: any) => (i ? i.label : "")}
            selectedItem={submitterItems.find((i) => i.id === submitterCode) || submitterItems[0]}
            onChange={({ selectedItem }: any) => setSubmitterCode(selectedItem?.id || "")}
          />
          <Dropdown
            id="public"
            titleText="Öffentlichkeit"
            label="alle"
            items={PUBLIC_OPTS}
            itemToString={(i: any) => (i ? i.label : "")}
            selectedItem={PUBLIC_OPTS.find((i) => i.id === pub)}
            onChange={({ selectedItem }: any) => setPub(selectedItem?.id || "all")}
          />
          <DatePicker
            datePickerType="single"
            dateFormat="Y-m-d"
            value={from}
            onChange={(d: Date[]) => setFrom(isoDate(d[0]))}
          >
            <DatePickerInput id="from" labelText="Von" placeholder="JJJJ-MM-TT" size="md" />
          </DatePicker>
          <DatePicker
            datePickerType="single"
            dateFormat="Y-m-d"
            value={to}
            onChange={(d: Date[]) => setTo(isoDate(d[0]))}
          >
            <DatePickerInput id="to" labelText="Bis" placeholder="JJJJ-MM-TT" size="md" />
          </DatePicker>
        </div>

        </div>

        <div className="dis-filterbar">
          <span className="dis-result-count">
            {loading ? "Lädt…" : `${total.toLocaleString("de-DE")} Treffer`}
            <a
              className="dis-feed-link"
              title="RSS-Feed neuer Dokumente (mit den aktiven Filtern)"
              href={`/api/feed.xml?${new URLSearchParams(
                Object.entries({ committee, type: docType, submitter: submitterCode, topic })
                  .filter(([, v]) => v) as [string, string][]
              ).toString()}`}
            >
              <Rss size={14} /> RSS
            </a>
          </span>
          <div className="dis-active-tags">
            {activeFilters.map((f) => (
              <Tag key={f.k} type="teal" filter size="sm" onClose={f.clear} title={f.label}>
                {f.label}
              </Tag>
            ))}
            {activeFilters.length > 0 && (
              <Button kind="ghost" size="sm" onClick={resetAll}>
                Alle zurücksetzen
              </Button>
            )}
            <div className="dis-colpick">
              <MultiSelect
                id="colsel"
                size="sm"
                label="Spalten"
                titleText=""
                items={ALL_COLUMNS}
                itemToString={(i: any) => (i ? i.label : "")}
                selectedItems={visibleColumns}
                onChange={({ selectedItems }: any) =>
                  setCols(selectedItems && selectedItems.length ? selectedItems.map((c: ColumnDef) => c.key) : COL_KEYS)
                }
              />
            </div>
          </div>
        </div>

        {error && <p style={{ color: "var(--cds-text-error)" }}>Fehler: {error}</p>}

        {loading && results.length === 0 && (
          <DataTableSkeleton columnCount={visibleColumns.length || 1} rowCount={10} showHeader={false} showToolbar={false} />
        )}

        {!loading && results.length === 0 && (
          <div className="dis-empty">
            Keine Dokumente gefunden — Suchbegriff anpassen oder Filter zurücksetzen.
          </div>
        )}

        {results.length > 0 && (
          <TableContainer
            title="Dokumente"
            description={`${total.toLocaleString("de-DE")} Treffer`}
          >
            <Table size="sm" useZebraStyles className="dis-doctable">
              <TableHead>
                <TableRow>
                  {visibleColumns.map((c) => (
                    <TableHeader key={c.key}>{c.label}</TableHeader>
                  ))}
                </TableRow>
              </TableHead>
              <TableBody>
                {results.map((d) => (
                  <TableRow key={d.id} className="dis-row-clickable" onClick={() => setSelected(d)}>
                    {visibleColumns.map((c) => (
                      <TableCell key={c.key} className={c.cls}>
                        {c.render(d)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <Pagination
              page={page}
              pageSize={PAGE_SIZE}
              pageSizes={[PAGE_SIZE]}
              totalItems={total}
              onChange={({ page: p }: any) => runSearch(p)}
            />
          </TableContainer>
        )}
        </TabPanel>

        <TabPanel>
          <MeetingsView committees={facets.committees} onOpenMeeting={openMeetingById} />
        </TabPanel>

        <TabPanel>
          <CommitteesView onOpenPerson={setPersonId} />
        </TabPanel>

        <TabPanel>
          <StatsView active={tabIdx === 3} />
        </TabPanel>
        </TabPanels>
        </Tabs>
      </main>

      <footer className="dis-footer">
        <span>Desinformationssystem</span>
        <span className="sep">·</span>
        <span>
          Quelle:{" "}
          <a href="https://sitzungskalender.karlsruhe.de/db/ratsinformation/start" target="_blank" rel="noreferrer">
            Sitzungskalender der Stadt Karlsruhe
          </a>
        </span>
        <span className="sep">·</span>
        <span>Open Data (CC BY-ND)</span>
        <span className="sep">·</span>
        <span>
          {/* Same path only works behind the path-routing tunnel (dis.delphi.tools);
              on a direct deployment the MCP server lives on its own port. */}
          MCP:{" "}
          <a
            href={
              window.location.hostname === "dis.delphi.tools"
                ? "/mcp"
                : `${window.location.protocol}//${window.location.hostname}:3651/mcp`
            }
          >
            /mcp
          </a>
        </span>
        {stats?.last_scrape && (
          <>
            <span className="sep">·</span>
            <span>Stand {stats.last_scrape.replace("T", " ").slice(0, 16)}</span>
          </>
        )}
      </footer>

      <MeetingModal
        meeting={openMeeting}
        onClose={() => setOpenMeeting(null)}
        onOpenDoc={(d) => {
          setOpenMeeting(null);
          setSelected(d);
        }}
        onRefreshed={setOpenMeeting}
      />
      <DocModal
        doc={selected}
        onClose={() => setSelected(null)}
        onTopic={applyTopic}
        onVorlage={(nr) => {
          setSelected(null);
          setVorlageNr(nr);
        }}
      />
      <PersonModal personId={personId} onClose={() => setPersonId("")} />
      <VorlageModal
        nr={vorlageNr}
        onClose={() => setVorlageNr("")}
        onOpenDoc={(id) => {
          setVorlageNr("");
          getDocument(id).then(setSelected).catch(() => {});
        }}
      />
    </Theme>
  );
}

function Stat({ n, l }: { n: number; l: string }) {
  return (
    <div className="dis-stat">
      <div className="num">{n?.toLocaleString("de-DE")}</div>
      <div className="lbl">{l}</div>
    </div>
  );
}

function DocModal({
  doc,
  onClose,
  onTopic,
  onVorlage,
}: {
  doc: Doc | null;
  onClose: () => void;
  onTopic: (t: string) => void;
  onVorlage: (nr: string) => void;
}) {
  if (!doc) return null;
  const ents = doc.entities || {};
  const referenced = (doc.vorlagen || []).filter((v) => !v.own && v.vorlage !== doc.vorlage);
  return (
    <Modal open passiveModal modalHeading={doc.label} size="lg" onRequestClose={onClose}>
      <div className="dis-detail">
        <dl>
          <dt>Gremium</dt>
          <dd>{doc.body_name || "—"}</dd>
          <dt>Sitzung</dt>
          <dd>
            {doc.meeting_id ? (
              <Link href={`${RIS_BASE}/${doc.meeting_id}`} target="_blank" rel="noreferrer">
                {doc.meeting_title || doc.meeting_id} <Launch size={12} />
              </Link>
            ) : (
              doc.meeting_title || "—"
            )}
          </dd>
          {(doc.agenda_number || doc.agenda_title) && (
            <>
              <dt>Tagesordnungspunkt</dt>
              <dd>
                {doc.meeting_id && doc.agenda_anchor ? (
                  <Link
                    href={`${RIS_BASE}/${doc.meeting_id}#${doc.agenda_anchor}`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {doc.agenda_number ? `TOP ${doc.agenda_number}` : "TOP"}
                    {doc.agenda_title ? ` — ${doc.agenda_title}` : ""} <Launch size={12} />
                  </Link>
                ) : (
                  `${doc.agenda_number ? "TOP " + doc.agenda_number : ""}${
                    doc.agenda_title ? " — " + doc.agenda_title : ""
                  }`
                )}
              </dd>
            </>
          )}
          <dt>Datum</dt>
          <dd>{doc.meeting_date || "—"}</dd>
          <dt>Typ</dt>
          <dd>{doc.doc_type}</dd>
          {doc.submitters && doc.submitters.length > 0 && (
            <>
              <dt>Einbringer</dt>
              <dd>
                <SubmitterChips codes={doc.submitters} />
              </dd>
            </>
          )}
          {doc.category && (
            <>
              <dt>Kategorie</dt>
              <dd>
                <Tag type="blue" size="sm">
                  {doc.category}
                </Tag>
              </dd>
            </>
          )}
          {doc.vorlage && (
            <>
              <dt>Vorlage</dt>
              <dd>
                <Tag
                  type="purple"
                  size="sm"
                  onClick={() => onVorlage(doc.vorlage!)}
                  style={{ cursor: "pointer" }}
                  title="Werdegang dieser Vorlage anzeigen"
                >
                  {doc.vorlage}
                </Tag>
              </dd>
            </>
          )}
        </dl>

        {doc.summary_de && (
          <>
            <h4>Zusammenfassung</h4>
            <p className="summary">{doc.summary_de}</p>
          </>
        )}
        {doc.summary_en && (
          <>
            <h4>Summary (EN)</h4>
            <p className="summary">{doc.summary_en}</p>
          </>
        )}

        {doc.topics && doc.topics.length > 0 && (
          <>
            <h4>Schlagworte</h4>
            <div className="dis-tags">
              {doc.topics.map((t) => (
                <Tag key={t} type="teal" onClick={() => onTopic(t)} style={{ cursor: "pointer" }}>
                  {t}
                </Tag>
              ))}
            </div>
          </>
        )}

        {((ents.people?.length || 0) + (ents.orgs?.length || 0) + (ents.locations?.length || 0)) > 0 && (
          <>
            <h4>Erwähnte Entitäten</h4>
            <EntityRow label="Personen" items={ents.people} />
            <EntityRow label="Organisationen" items={ents.orgs} />
            <EntityRow label="Orte" items={ents.locations} />
          </>
        )}

        {referenced.length > 0 && (
          <>
            <h4>Erwähnte Vorlagen</h4>
            <div className="dis-tags">
              {referenced.slice(0, 16).map((v) => (
                <Tag
                  key={v.vorlage}
                  type="purple"
                  size="sm"
                  onClick={() => onVorlage(v.vorlage)}
                  style={{ cursor: "pointer" }}
                >
                  {v.vorlage}
                </Tag>
              ))}
              {referenced.length > 16 && (
                <span className="dis-meta-line">+{referenced.length - 16} weitere</span>
              )}
            </div>
          </>
        )}

        {doc.versions && doc.versions.length > 0 && (
          <>
            <h4>Frühere Versionen ({doc.versions.length})</h4>
            <ul className="dis-versions">
              {doc.versions.map((v) => (
                <li key={v.id}>
                  <a href={versionUrl(doc.id, v.id)} target="_blank" rel="noreferrer">
                    Version bis {(v.superseded_at || "?").slice(0, 16).replace("T", " ")}
                  </a>
                  <span className="dis-meta-line">
                    {" "}
                    · abgerufen {(v.downloaded_at || "?").slice(0, 10)}
                    {v.size ? ` · ${Math.round(v.size / 1024)} kB` : ""}
                  </span>
                </li>
              ))}
            </ul>
            <p className="dis-meta-line">
              Dieses Dokument wurde im RIS nachträglich verändert; ältere Fassungen bleiben hier
              archiviert.
            </p>
          </>
        )}

        <div style={{ marginTop: "1.75rem", display: "flex", gap: "0.75rem", flexWrap: "wrap" }}>
          <Button kind="primary" renderIcon={DocIcon} href={fileUrl(doc.id)} target="_blank">
            PDF öffnen
          </Button>
          {doc.url?.startsWith("http") && (
            <Button kind="tertiary" renderIcon={Launch} href={doc.url} target="_blank">
              Original im RIS
            </Button>
          )}
        </div>
        <p className="dis-meta-line" style={{ marginTop: "1rem" }}>
          Datei-ID {doc.id} · Text: {doc.text_status || "—"} · Analyse: {doc.enrich_status || "—"}
        </p>
      </div>
    </Modal>
  );
}

function EntityRow({ label, items }: { label: string; items?: string[] }) {
  if (!items || items.length === 0) return null;
  return (
    <div style={{ marginBottom: "0.4rem" }}>
      <span className="dis-meta-line">{label}: </span>
      <span className="dis-tags" style={{ display: "inline-flex" }}>
        {items.map((i) => (
          <Tag key={i} type="cool-gray" size="sm">
            {i}
          </Tag>
        ))}
      </span>
    </div>
  );
}

function publicLabel(p?: number | null): { text: string; type: any } {
  if (p === 1) return { text: "öffentlich", type: "green" };
  if (p === 0) return { text: "nicht öffentlich", type: "red" };
  return { text: "gemischt", type: "gray" };
}

function PartyChip({ code, party }: { code?: string; party?: string }) {
  if (code && SUBMITTERS[code]) {
    const meta = SUBMITTERS[code];
    return (
      <span title={meta.name}>
        <Tag type={meta.color} size="sm">
          {code}
        </Tag>
      </span>
    );
  }
  if (party) return <Tag type="cool-gray" size="sm">{party}</Tag>;
  return null;
}

// ---- Sitzungen (meetings) view ---------------------------------------------
function MeetingsView({
  committees,
  onOpenMeeting,
}: {
  committees: { id: string; name: string }[];
  onOpenMeeting: (id: string) => void;
}) {
  const MEETINGS_PAGE = 100;
  const [upcoming, setUpcoming] = useState(true);
  const [committee, setCommittee] = useState("");
  const [meetings, setMeetings] = useState<Meeting[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);

  const committeeItems = useMemo(
    () => [{ id: "", label: "Alle Gremien" }, ...committees.map((c) => ({ id: c.name, label: c.name }))],
    [committees]
  );

  useEffect(() => {
    setPage(1);
  }, [upcoming, committee]);

  useEffect(() => {
    let alive = true;
    setLoading(true);
    getMeetings({ upcoming, committee, limit: MEETINGS_PAGE, offset: (page - 1) * MEETINGS_PAGE })
      .then((r) => {
        if (!alive) return;
        setMeetings(r.meetings);
        setTotal(r.total);
      })
      .catch(() => alive && setMeetings([]))
      .finally(() => alive && setLoading(false));
    return () => {
      alive = false;
    };
  }, [upcoming, committee, page]);

  return (
    <>
      <div className="dis-panel">
        <div className="dis-filters" style={{ gridTemplateColumns: "auto minmax(220px, 1fr)", marginTop: 0 }}>
          <Toggle
            id="upcoming"
            size="sm"
            labelText="Zeitraum"
            labelA="Alle Sitzungen"
            labelB="Nur kommende"
            toggled={upcoming}
            onToggle={(t: boolean) => setUpcoming(t)}
          />
          <Dropdown
            id="m-committee"
            titleText="Gremium"
            label="Alle Gremien"
            items={committeeItems}
            itemToString={(i: any) => (i ? i.label : "")}
            selectedItem={committeeItems.find((i) => i.id === committee) || committeeItems[0]}
            onChange={({ selectedItem }: any) => setCommittee(selectedItem?.id || "")}
          />
        </div>
        <p className="dis-meta-line" style={{ marginTop: "0.75rem" }}>
          <a
            className="dis-feed-link"
            href={`/api/meetings.ics${committee ? `?committee=${encodeURIComponent(committee)}` : ""}`}
            title="Sitzungen als Kalender abonnieren (ICS)"
          >
            <Calendar size={14} /> Kalender abonnieren (ICS)
          </a>
        </p>
      </div>
      {loading && (
        <DataTableSkeleton columnCount={6} rowCount={6} showHeader={false} showToolbar={false} />
      )}
      {!loading && meetings.length === 0 && (
        <div className="dis-empty">{upcoming ? "Keine kommenden Sitzungen." : "Keine Sitzungen."}</div>
      )}
      {meetings.length > 0 && (
        <TableContainer
          title={upcoming ? "Kommende Sitzungen" : "Alle Sitzungen"}
          description={`${total.toLocaleString("de-DE")} Sitzung(en)`}
        >
          <Table size="lg" useZebraStyles>
            <TableHead>
              <TableRow>
                <TableHeader>Datum</TableHeader>
                <TableHeader>Zeit</TableHeader>
                <TableHeader>Gremium</TableHeader>
                <TableHeader>Öffentlich</TableHeader>
                <TableHeader>TOPs</TableHeader>
                <TableHeader>Dok.</TableHeader>
              </TableRow>
            </TableHead>
            <TableBody>
              {meetings.map((m) => {
                const pl = publicLabel(m.public);
                return (
                  <TableRow key={m.id} className="dis-row-clickable" onClick={() => onOpenMeeting(m.id)}>
                    <TableCell>{m.date || "—"}</TableCell>
                    <TableCell>{m.time || "—"}</TableCell>
                    <TableCell>{m.body_name || "—"}</TableCell>
                    <TableCell>
                      <Tag type={pl.type} size="sm">
                        {pl.text}
                      </Tag>
                    </TableCell>
                    <TableCell>{m.agenda_count || 0}</TableCell>
                    <TableCell>{m.documents || 0}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
          {total > MEETINGS_PAGE && (
            <Pagination
              page={page}
              pageSize={MEETINGS_PAGE}
              pageSizes={[MEETINGS_PAGE]}
              totalItems={total}
              onChange={({ page: p }: any) => setPage(p)}
            />
          )}
        </TableContainer>
      )}
    </>
  );
}

function MeetingDocRow({
  file,
  onClose,
  onOpenDoc,
}: {
  file: Doc;
  onClose: () => void;
  onOpenDoc: (d: Doc) => void;
}) {
  return (
    <button
      type="button"
      className="dis-meeting-docrow"
      onClick={async () => {
        try {
          const full = await getDocument(file.id);
          onClose();
          onOpenDoc(full);
        } catch {
          /* ignore */
        }
      }}
    >
      <span className="dis-meeting-docrow__type">
        <Tag type="gray" size="sm">
          {file.doc_type}
        </Tag>
      </span>
      <span className="dis-meeting-docrow__label">{file.label}</span>
      {file.submitters && file.submitters.length > 0 && (
        <span className="dis-meeting-docrow__sub">
          <SubmitterChips codes={file.submitters} />
        </span>
      )}
    </button>
  );
}

function VoteBlock({ vote }: { vote: Vote }) {
  const hasTally = vote.ja != null || vote.nein != null || vote.enthaltung != null;
  return (
    <div className="dis-vote">
      {vote.result_text && <div className="dis-vote__result">Ergebnis: {vote.result_text}</div>}
      {hasTally && (
        <div className="dis-vote__tally">
          <span className="dis-vote__n dis-vote__ja">Ja {vote.ja ?? "—"}</span>
          <span className="dis-vote__n dis-vote__nein">Nein {vote.nein ?? "—"}</span>
          <span className="dis-vote__n dis-vote__enth">Enthaltung {vote.enthaltung ?? "—"}</span>
        </div>
      )}
      {vote.members && vote.members.length > 0 && (
        <details className="dis-vote__roll">
          <summary>Namentliche Abstimmung ({vote.members.length})</summary>
          <div className="dis-vote__members">
            {vote.members.map((m, i) => (
              <span key={i} className={`dis-vote__m dis-vote__m--${(m.vote || "").replace(/[^a-z]/g, "")}`} title={m.vote}>
                {m.name}
              </span>
            ))}
          </div>
        </details>
      )}
      {vote.image_url?.startsWith("http") && (
        <a className="dis-meta-line" href={vote.image_url} target="_blank" rel="noreferrer">
          Abstimmungsbild <Launch size={12} />
        </a>
      )}
    </div>
  );
}

function MeetingModal({
  meeting,
  onClose,
  onOpenDoc,
  onRefreshed,
}: {
  meeting: Meeting | null;
  onClose: () => void;
  onOpenDoc: (d: Doc) => void;
  onRefreshed: (m: Meeting) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");
  const { byAnchor, extra, votesByAnchor } = useMemo(() => {
    const map = new Map<string, Doc[]>();
    const extras: Doc[] = [];
    const known = new Set((meeting?.agenda_items || []).map((a) => a.anchor));
    for (const f of meeting?.files || []) {
      const a = f.agenda_anchor;
      if (a && known.has(a)) {
        const arr = map.get(a);
        if (arr) arr.push(f);
        else map.set(a, [f]);
      } else {
        extras.push(f);
      }
    }
    const vmap = new Map<string, Vote>();
    for (const v of meeting?.votes || []) if (v.agenda_anchor) vmap.set(v.agenda_anchor, v);
    return { byAnchor: map, extra: extras, votesByAnchor: vmap };
  }, [meeting]);
  useEffect(() => {
    setMsg("");
    setBusy(false);
  }, [meeting?.id]);

  if (!meeting) return null;
  const m = meeting;
  const pl = publicLabel(m.public);
  const agenda = m.agenda_items || [];
  const metaParts = [m.date, m.time, m.location].filter(Boolean);

  const doRefresh = async () => {
    setBusy(true);
    setMsg("");
    try {
      const fresh = await refreshMeeting(m.id);
      const c = fresh._refresh?.counts || {};
      const added = (c.files_new || 0) + (c.files_updated || 0);
      setMsg(
        added > 0
          ? `${added} neue/aktualisierte Dokument(e) übernommen.`
          : "Keine neuen Dokumente. Analyse folgt beim nächsten Lauf."
      );
      onRefreshed(fresh);
    } catch (e: any) {
      setMsg(`Aktualisierung fehlgeschlagen: ${e?.message || e}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open
      passiveModal
      size="lg"
      modalHeading={m.body_name || m.title || "Sitzung"}
      onRequestClose={onClose}
    >
      <div className="dis-detail dis-meeting">
        <div className="dis-meeting__head">
          <p className="dis-meeting__meta">{metaParts.join(" · ") || "—"}</p>
          <div className="dis-meeting__headtags">
            <Tag type={pl.type} size="sm">
              {pl.text}
            </Tag>
            <Link href={`${RIS_BASE}/${m.id}`} target="_blank" rel="noreferrer">
              Sitzung im RIS öffnen <Launch size={12} />
            </Link>
          </div>
        </div>

        <div className="dis-meeting__actions">
          {busy ? (
            <InlineLoading description="Aktualisiere aus dem RIS…" />
          ) : (
            <Button kind="tertiary" size="sm" renderIcon={Renew} onClick={doRefresh}>
              Aktualisieren
            </Button>
          )}
          {msg && <span className="dis-meta-line">{msg}</span>}
        </div>

        <h4>Tagesordnung ({agenda.length})</h4>
        {agenda.length === 0 && <p className="dis-meta-line">Keine Tagesordnung erfasst.</p>}
        {agenda.length > 0 && (
          <div className="dis-meeting__agenda">
            <Accordion size="sm" align="end">
              {agenda.map((a) => {
                const docs = byAnchor.get(a.anchor) || [];
                const vote = votesByAnchor.get(a.anchor);
                const hasTally = vote && (vote.ja != null || vote.nein != null || vote.enthaltung != null);
                const apl = publicLabel(a.public);
                const title = (
                  <span className="dis-meeting__topttl">
                    <span className="dis-meeting__topnum">{a.number ? `TOP ${a.number}` : "TOP"}</span>
                    <span className="dis-meeting__toptext">{a.title || "—"}</span>
                    <span className="dis-meeting__topmeta">
                      {hasTally && (
                        <Tag type="green" size="sm" title="Ja · Nein · Enthaltung">
                          {vote!.ja ?? 0}·{vote!.nein ?? 0}·{vote!.enthaltung ?? 0}
                        </Tag>
                      )}
                      {docs.length > 0 && (
                        <Tag type="outline" size="sm">
                          {docs.length} Dok.
                        </Tag>
                      )}
                      {a.public === 0 && (
                        <Tag type={apl.type} size="sm">
                          {apl.text}
                        </Tag>
                      )}
                    </span>
                  </span>
                );
                return (
                  <AccordionItem key={a.anchor} title={title} disabled={docs.length === 0 && !vote}>
                    {vote && <VoteBlock vote={vote} />}
                    <div className="dis-meeting__docs">
                      {docs.map((f) => (
                        <MeetingDocRow key={f.id} file={f} onClose={onClose} onOpenDoc={onOpenDoc} />
                      ))}
                    </div>
                  </AccordionItem>
                );
              })}
            </Accordion>
          </div>
        )}

        {extra.length > 0 && (
          <>
            <h4>Weitere Dokumente ({extra.length})</h4>
            <div className="dis-meeting__docs">
              {extra.map((f) => (
                <MeetingDocRow key={f.id} file={f} onClose={onClose} onOpenDoc={onOpenDoc} />
              ))}
            </div>
          </>
        )}

        {(m.files || []).length === 0 && (
          <p className="dis-meta-line" style={{ marginTop: "1rem" }}>
            Keine Dokumente zu dieser Sitzung.
          </p>
        )}
      </div>
    </Modal>
  );
}

// ---- Gremien (committees) view ---------------------------------------------
function CommitteesView({ onOpenPerson }: { onOpenPerson: (id: string) => void }) {
  const [list, setList] = useState<CommitteeListItem[]>([]);
  const [sel, setSel] = useState<Committee | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    getCommittees().then(setList).catch(() => {});
  }, []);

  const open = async (id: string) => {
    setLoading(true);
    try {
      setSel(await getCommittee(id));
    } catch {
      setSel(null); // failed fetch: fall back to the empty-state hint
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="dis-committees">
      <div className="dis-committee-list">
        <h4>Gremien ({list.length})</h4>
        {list.map((c) => (
          <button
            key={c.id}
            className={"dis-committee-item" + (sel?.id === c.id ? " is-selected" : "")}
            onClick={() => open(c.id)}
          >
            <div>{c.name}</div>
            <div className="dis-meta-line">
              {c.members} Mitglieder · {c.meetings} Sitzungen · {c.documents} Dok.
            </div>
          </button>
        ))}
      </div>
      <div className="dis-detail-panel dis-detail">
        {loading && <InlineLoading description="Lädt…" />}
        {!sel && !loading && (
          <div className="dis-empty" style={{ border: "none", background: "none" }}>
            Gremium auswählen, um Mitglieder und Zusammensetzung zu sehen.
          </div>
        )}
        {sel && (
          <>
            <h3>{sel.name}</h3>
            {sel.composition.length > 0 && (
              <>
                <h4>Zusammensetzung</h4>
                <div className="dis-tags">
                  {sel.composition.map((c) => (
                    <span key={c.code} style={{ marginRight: "0.5rem", display: "inline-flex", alignItems: "center", gap: "0.2rem" }}>
                      <PartyChip code={c.code} party={c.code} />
                      <span className="dis-meta-line">{c.n}</span>
                    </span>
                  ))}
                </div>
              </>
            )}
            <h4>Mitglieder ({sel.members.length})</h4>
            <Table size="sm" useZebraStyles>
              <TableHead>
                <TableRow>
                  <TableHeader>Name</TableHeader>
                  <TableHeader>Fraktion</TableHeader>
                  <TableHeader>Funktion</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {sel.members.map((m: Member) => (
                  <TableRow
                    key={m.person_id}
                    className="dis-row-clickable"
                    onClick={() => onOpenPerson(m.person_id)}
                    title="Profil und Abstimmungsverhalten anzeigen"
                  >
                    <TableCell>{m.name}</TableCell>
                    <TableCell>
                      <PartyChip code={m.party_code} party={m.party} />
                    </TableCell>
                    <TableCell>{m.role}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </>
        )}
      </div>
    </div>
  );
}

// ---- Person (Stadtrat/Stadträtin) profile ----------------------------------
const VOTE_LABEL: Record<string, string> = {
  ja: "Ja",
  nein: "Nein",
  enthaltung: "Enthaltung",
  abwesend: "Abwesend",
};

function PersonModal({ personId, onClose }: { personId: string; onClose: () => void }) {
  const [person, setPerson] = useState<Person | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setPerson(null);
    setFailed(false);
    if (!personId) return;
    let alive = true;
    getPerson(personId)
      .then((p) => alive && setPerson(p))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [personId]);

  if (!personId) return null;
  const summary = person?.vote_summary || {};
  return (
    <Modal
      open
      passiveModal
      size="lg"
      modalHeading={person?.name || "Person"}
      onRequestClose={onClose}
    >
      <div className="dis-detail">
        {!person && !failed && <InlineLoading description="Lädt…" />}
        {failed && <p className="dis-meta-line">Profil konnte nicht geladen werden.</p>}
        {person && (
          <>
            <dl>
              <dt>Fraktion</dt>
              <dd>
                <PartyChip code={person.party_code} party={person.party} />
                {person.party && <span className="dis-meta-line"> {person.party}</span>}
              </dd>
              <dt>Gremien</dt>
              <dd>
                {person.memberships.map((m) => (
                  <div key={m.body_id}>
                    {m.body_name}
                    {m.role ? <span className="dis-meta-line"> — {m.role}</span> : null}
                  </div>
                ))}
              </dd>
            </dl>

            <h4>Abstimmungsverhalten ({person.votes.length})</h4>
            {person.votes.length === 0 && (
              <p className="dis-meta-line">
                Keine namentlichen Abstimmungen erfasst — Auswertung wächst mit jeder
                Sitzung, deren Abstimmungspanel ausgelesen wird.
              </p>
            )}
            {person.votes.length > 0 && (
              <>
                <div className="dis-vote__tally" style={{ marginBottom: "0.75rem" }}>
                  <span className="dis-vote__n dis-vote__ja">Ja {summary.ja || 0}</span>
                  <span className="dis-vote__n dis-vote__nein">Nein {summary.nein || 0}</span>
                  <span className="dis-vote__n dis-vote__enth">
                    Enthaltung {summary.enthaltung || 0}
                  </span>
                  <span className="dis-vote__n">Abwesend {summary.abwesend || 0}</span>
                </div>
                <Table size="sm" useZebraStyles>
                  <TableHead>
                    <TableRow>
                      <TableHeader>Datum</TableHeader>
                      <TableHeader>Gremium</TableHeader>
                      <TableHeader>TOP</TableHeader>
                      <TableHeader>Stimme</TableHeader>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {person.votes.map((v, i) => (
                      <TableRow key={i}>
                        <TableCell>{v.meeting_date || "—"}</TableCell>
                        <TableCell>{v.body_name || "—"}</TableCell>
                        <TableCell>
                          <span
                            className="dis-ellip"
                            style={{ maxWidth: "24rem", display: "inline-block" }}
                            title={v.agenda_title || v.top_label || ""}
                          >
                            {v.top_label}
                            {v.agenda_title ? ` — ${v.agenda_title}` : ""}
                          </span>
                        </TableCell>
                        <TableCell>
                          <span className={`dis-votechip dis-votechip--${v.vote}`}>
                            {VOTE_LABEL[v.vote] || v.vote}
                          </span>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
                <p className="dis-meta-line" style={{ marginTop: "0.75rem" }}>
                  Zuordnung über den Nachnamen im Abstimmungspanel; mehrdeutige Einträge werden
                  nicht gezählt.
                </p>
              </>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}

// ---- Vorlage lifecycle (Werdegang) -----------------------------------------
function VorlageModal({
  nr,
  onClose,
  onOpenDoc,
}: {
  nr: string;
  onClose: () => void;
  onOpenDoc: (fileId: string) => void;
}) {
  const [chain, setChain] = useState<VorlageChain | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setChain(null);
    setFailed(false);
    if (!nr) return;
    let alive = true;
    getVorlage(nr)
      .then((c) => alive && setChain(c))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [nr]);

  if (!nr) return null;
  return (
    <Modal
      open
      passiveModal
      size="lg"
      modalHeading={`Vorlage ${nr}`}
      onRequestClose={onClose}
    >
      <div className="dis-detail">
        {!chain && !failed && <InlineLoading description="Lädt…" />}
        {failed && (
          <p className="dis-meta-line">
            Zu dieser Vorlagennummer ist (noch) nichts archiviert.
          </p>
        )}
        {chain && (
          <>
            {chain.title && <p className="dis-vorlage-title">{chain.title}</p>}
            <h4>Werdegang ({chain.stations.length} Station{chain.stations.length === 1 ? "" : "en"})</h4>
            <ol className="dis-timeline">
              {chain.stations.map((st, i) => (
                <li key={st.meeting_id || i}>
                  <div className="dis-timeline__head">
                    <span className="dis-timeline__date">{st.date || "—"}</span>
                    <span className="dis-timeline__body">{st.body_name || "Unbekanntes Gremium"}</span>
                    {st.votes.map((v) => {
                      const hasTally = v.ja != null || v.nein != null || v.enthaltung != null;
                      return hasTally ? (
                        <Tag key={v.agenda_anchor} type="green" size="sm" title={v.result_text || "Abstimmungsergebnis"}>
                          {v.ja ?? 0}·{v.nein ?? 0}·{v.enthaltung ?? 0}
                        </Tag>
                      ) : null;
                    })}
                  </div>
                  <div className="dis-timeline__docs">
                    {st.documents.map((d) => (
                      <button
                        key={d.id}
                        type="button"
                        className="dis-meeting-docrow"
                        onClick={() => onOpenDoc(d.id)}
                      >
                        <span className="dis-meeting-docrow__type">
                          <Tag type="gray" size="sm">
                            {d.doc_type}
                          </Tag>
                        </span>
                        <span className="dis-meeting-docrow__label">{d.label}</span>
                      </button>
                    ))}
                  </div>
                </li>
              ))}
            </ol>
            <p className="dis-meta-line">
              Stationen = Sitzungen, in deren Dokumenten diese Vorlagennummer vorkommt
              (Vorberatung im Ausschuss bis Entscheidung im Gemeinderat).
            </p>
          </>
        )}
      </div>
    </Modal>
  );
}

// ---- Statistik (voting analytics) ------------------------------------------
function pct(x: number | null | undefined): string {
  return x == null ? "—" : `${Math.round(x * 100)} %`;
}

function StatsView({ active }: { active: boolean }) {
  const [data, setData] = useState<Analytics | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!active || data) return;
    let alive = true;
    getAnalytics()
      .then((d) => alive && setData(d))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [active, data]);

  if (failed) return <div className="dis-empty">Statistik konnte nicht geladen werden.</div>;
  if (!data) return <DataTableSkeleton columnCount={4} rowCount={5} showHeader={false} showToolbar={false} />;

  const t = data.totals;
  const parties = data.parties;
  const codes = parties.map((p) => p.code);
  const agree = new Map<string, { agree: number; n: number }>();
  for (const a of data.agreement) {
    agree.set(`${a.a}|${a.b}`, a);
    agree.set(`${a.b}|${a.a}`, a);
  }
  const maxCast = Math.max(1, ...parties.map((p) => p.members_cast));

  return (
    <div className="dis-statsview">
      <div className="dis-stats">
        <Stat n={t.votes} l="Abstimmungen" />
        <Stat n={t.with_rollcall} l="Namentlich" />
        <div className="dis-stat">
          <div className="num">{t.votes ? pct(t.unanimous / t.votes) : "—"}</div>
          <div className="lbl">Einstimmig</div>
        </div>
        <div className="dis-stat">
          <div className="num">{t.votes ? pct(t.contested / t.votes) : "—"}</div>
          <div className="lbl">Mit Gegenstimmen</div>
        </div>
      </div>

      {parties.length === 0 && (
        <div className="dis-empty">
          Noch keine namentlichen Abstimmungen ausgewertet — die Statistik füllt sich, sobald
          Abstimmungspanels (live oder aus Ergebnis-PDFs) erfasst sind.
        </div>
      )}

      {parties.length > 0 && (
        <>
          <h4>Fraktionsdisziplin</h4>
          <p className="dis-meta-line">
            Anteil der abgegebenen Stimmen, die der Mehrheitslinie der eigenen Fraktion folgen.
          </p>
          <div className="dis-bars">
            {parties.map((p) => (
              <div className="dis-bar-row" key={p.code}>
                <span className="dis-bar-label">
                  <PartyChip code={p.code} party={p.code} />
                </span>
                <span className="dis-bar-track" aria-hidden>
                  <span
                    className="dis-bar-fill"
                    style={{ width: `${Math.round((p.cohesion || 0) * 100)}%` }}
                  />
                </span>
                <span className="dis-bar-value">{pct(p.cohesion)}</span>
                <span className="dis-meta-line">{p.votes} Abst. · {p.members_cast} Stimmen</span>
              </div>
            ))}
          </div>

          <h4>Stimmverteilung</h4>
          <p className="dis-meta-line">
            Alle zugeordneten Einzelstimmen je Fraktion (Gelb = Ja, Rot = Nein, Grau =
            Enthaltung — wie auf dem Abstimmungspanel).
          </p>
          <div className="dis-bars">
            {parties.map((p) => {
              const cast = p.ja + p.nein + p.enthaltung;
              const w = (n: number) => `${(cast ? (n / cast) : 0) * (cast / maxCast) * 100}%`;
              return (
                <div className="dis-bar-row" key={p.code}>
                  <span className="dis-bar-label">
                    <PartyChip code={p.code} party={p.code} />
                  </span>
                  <span className="dis-bar-track dis-bar-track--stacked" aria-hidden>
                    <span className="dis-seg dis-seg--ja" style={{ width: w(p.ja) }} />
                    <span className="dis-seg dis-seg--nein" style={{ width: w(p.nein) }} />
                    <span className="dis-seg dis-seg--enth" style={{ width: w(p.enthaltung) }} />
                  </span>
                  <span className="dis-bar-value dis-votetext">
                    <span className="dis-vote__ja">Ja {p.ja}</span>
                    {" · "}
                    <span className="dis-vote__nein">Nein {p.nein}</span>
                    {" · "}
                    <span className="dis-vote__enth">Enth. {p.enthaltung}</span>
                  </span>
                </div>
              );
            })}
          </div>

          {codes.length >= 2 && (
            <>
              <h4>Übereinstimmung zwischen Fraktionen</h4>
              <p className="dis-meta-line">
                Wie oft zwei Fraktionen bei derselben Abstimmung dieselbe Mehrheitslinie hatten.
              </p>
              <div className="dis-matrix-wrap">
                <table className="dis-matrix">
                  <thead>
                    <tr>
                      <th />
                      {codes.map((c) => (
                        <th key={c}>{c}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {codes.map((row) => (
                      <tr key={row}>
                        <th>{row}</th>
                        {codes.map((col) => {
                          if (row === col) return <td key={col} className="dis-matrix__self" />;
                          const cell = agree.get(`${row}|${col}`);
                          const v = cell?.agree;
                          return (
                            <td
                              key={col}
                              title={cell ? `${row} & ${col}: ${pct(v)} (${cell.n} gemeinsame Abstimmungen)` : "keine gemeinsamen Abstimmungen"}
                              style={
                                v == null
                                  ? undefined
                                  : { background: `rgba(69, 137, 255, ${0.08 + v * 0.55})` }
                              }
                            >
                              {v == null ? "" : Math.round(v * 100)}
                            </td>
                          );
                        })}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          {data.dissenters.length > 0 && (
            <>
              <h4>Abweichler</h4>
              <p className="dis-meta-line">
                Mitglieder, die am häufigsten gegen die Mehrheitslinie ihrer Fraktion gestimmt
                haben.
              </p>
              <Table size="sm" useZebraStyles className="dis-dissenters">
                <TableHead>
                  <TableRow>
                    <TableHeader>Name (Panel)</TableHeader>
                    <TableHeader>Fraktion</TableHeader>
                    <TableHeader>Abweichungen</TableHeader>
                    <TableHeader>Stimmen</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {data.dissenters.map((d) => (
                    <TableRow key={d.name}>
                      <TableCell>{d.name}</TableCell>
                      <TableCell>
                        <PartyChip code={d.party} party={d.party} />
                      </TableCell>
                      <TableCell>{d.dissents}</TableCell>
                      <TableCell>{d.votes}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </>
          )}

          <p className="dis-meta-line" style={{ marginTop: "1rem" }}>
            Basis: {t.with_rollcall} namentliche Abstimmungen. Zuordnung über Nachnamen;{" "}
            {data.unattributed_entries} Einzelstimmen blieben unzugeordnet und sind nicht
            enthalten.
          </p>
        </>
      )}
    </div>
  );
}
