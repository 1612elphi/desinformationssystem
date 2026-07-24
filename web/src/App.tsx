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
import { Launch, Document as DocIcon, Time, Renew } from "@carbon/icons-react";
import {
  search as apiSearch,
  getFacets,
  getStats,
  getDocument,
  fileUrl,
  getMeetings,
  getMeeting,
  getLive,
  refreshMeeting,
  getCommittees,
  getCommittee,
  type Doc,
  type Facets,
  type Stats,
  type Meeting,
  type Member,
  type CommitteeListItem,
  type Committee,
  type Vote,
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
  { key: "doc", label: "Dokument", cls: "dis-c-doc", render: (d) => <span className="dis-ellip" title={d.label}>{d.label}</span> },
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

export default function App() {
  const [facets, setFacets] = useState<Facets>({ committees: [], doc_types: [], submitters: [] });
  const [stats, setStats] = useState<Stats | null>(null);

  const [q, setQ] = useState("");
  const [committee, setCommittee] = useState("");
  const [docType, setDocType] = useState("");
  const [pub, setPub] = useState("all");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [topic, setTopic] = useState("");
  const [submitterCode, setSubmitterCode] = useState("");
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
  }, []);

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

        <Tabs>
        <TabList aria-label="Ansichten" contained>
          <Tab>Dokumente</Tab>
          <Tab>Sitzungen</Tab>
          <Tab>Gremien</Tab>
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
          <CommitteesView />
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
      <DocModal doc={selected} onClose={() => setSelected(null)} onTopic={applyTopic} />
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
}: {
  doc: Doc | null;
  onClose: () => void;
  onTopic: (t: string) => void;
}) {
  if (!doc) return null;
  const ents = doc.entities || {};
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
function CommitteesView() {
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
                  <TableRow key={m.person_id}>
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
