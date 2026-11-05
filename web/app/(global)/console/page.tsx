"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";

type Session = { username: string; csrf_token: string; expires_at: string };
type Overview = { listings: number; watched: number; current_offers: number; fresh_offers: number; due_jobs: number; held_candidates: number; publications: number };
type Retailer = { id: number; code: string; name: string; country: string; enabled: boolean; publish_enabled: boolean; health: string; offer_source: string; watched: number; last_success_at: string | null; last_error_class: string };
type Listing = { id: number; title: string; product_id: string; tracking_state: string; cadence: string; store: string; price: string | null; observed_at: string | null; imported: boolean };
type Rule = { id: number; country: string; historical_percent: string; freshness_hours: number; version: number; enabled: boolean };
type Channel = { id: number; name: string; country: string; platform: string; destination_id: string; enabled: boolean; private_test: boolean; active_routes: number };
type Country = { id: number; code: string; name: string; currency: string; enabled: boolean };
type Route = { id: number; country: string; channel: string; retailer: string; enabled: boolean; channel_enabled: boolean };
type Candidate = { id: number; listing_id: number; store: string; title: string; price: string; percent: string; state: string; has_evidence: boolean };
type Match = { id: number; listing_id: number; title: string; store: string; canonical_id: number; brand: string; model: string; method: string; confidence: string; state: string };
type Publication = { id: number; channel: string; status: string; attempt_count: number; message_id: string; last_error: string };
type Page<T> = { items: T[]; has_more?: boolean; page?: number };
type Tab = "overview" | "listings" | "rules" | "channels" | "matches" | "candidates" | "publications";

const tabNames: Record<Tab, string> = {
  overview: "النظرة العامة", listings: "المنتجات", rules: "قواعد العروض",
  channels: "القنوات", matches: "مراجعة المطابقة", candidates: "العروض المرشحة", publications: "المنشورات",
};

async function read<T>(path: string): Promise<T> {
  const response = await fetch(`/api/v1/admin/${path}`, { credentials: "same-origin", cache: "no-store" });
  if (!response.ok) throw new Error(response.status === 401 ? "login" : `HTTP ${response.status}`);
  return response.json() as Promise<T>;
}

export default function Console() {
  const router = useRouter();
  const [session, setSession] = useState<Session | null>(null);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [retailers, setRetailers] = useState<Retailer[]>([]);
  const [listings, setListings] = useState<Page<Listing>>({ items: [] });
  const [rules, setRules] = useState<Rule[]>([]);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [countries, setCountries] = useState<Country[]>([]);
  const [routes, setRoutes] = useState<Route[]>([]);
  const [candidates, setCandidates] = useState<Page<Candidate>>({ items: [] });
  const [matches, setMatches] = useState<Page<Match>>({ items: [] });
  const [publications, setPublications] = useState<Page<Publication>>({ items: [] });
  const [tab, setTab] = useState<Tab>("overview");
  const [query, setQuery] = useState("");
  const [draftQuery, setDraftQuery] = useState("");
  const [page, setPage] = useState(1);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(true);

  const load = useCallback(async () => {
    try {
      const [identity, summary, stores, offerRules, destinations, publicationRoutes, availableCountries] = await Promise.all([
        read<Session>("session"), read<Overview>("overview"), read<Page<Retailer>>("retailers"),
        read<Page<Rule>>("rules"), read<Page<Channel>>("channels"), read<Page<Route>>("routes"), read<Page<Country>>("countries"),
      ]);
      setSession(identity);
      setOverview(summary);
      setRetailers(stores.items);
      setRules(offerRules.items);
      setChannels(destinations.items);
      setCountries(availableCountries.items);
      setRoutes(publicationRoutes.items);
    } catch (error) {
      if (error instanceof Error && error.message === "login") router.replace("/console/login");
      else setMessage("خدمة الإدارة غير متاحة حاليًا. لوحة Django الحالية ما زالت متاحة.");
    } finally {
      setBusy(false);
    }
  }, [router]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    if (!session) return;
    const encoded = new URLSearchParams({ page: String(page) });
    if (tab === "listings") {
      if (query.trim()) encoded.set("q", query.trim());
      void read<Page<Listing>>(`listings?${encoded}`).then(setListings).catch(() => setMessage("تعذّر تحميل المنتجات."));
    } else if (tab === "candidates") {
      void read<Page<Candidate>>(`candidates?${encoded}`).then(setCandidates).catch(() => setMessage("تعذّر تحميل المرشحين."));
    } else if (tab === "matches") {
      void read<Page<Match>>(`matches?${encoded}`).then(setMatches).catch(() => setMessage("تعذّر تحميل مراجعات المطابقة."));
    } else if (tab === "publications") {
      void read<Page<Publication>>(`publications?${encoded}`).then(setPublications).catch(() => setMessage("تعذّر تحميل المنشورات."));
    }
  }, [tab, page, query, session]);

  async function patch(path: string, body: object) {
    if (!session) return;
    setMessage("");
    try {
      const response = await fetch(`/api/v1/admin/${path}`, {
        method: "PATCH", credentials: "same-origin", cache: "no-store",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrf_token },
        body: JSON.stringify(body),
      });
      if (!response.ok) {
        const result = await response.json();
        throw new Error(result.error || `HTTP ${response.status}`);
      }
      setMessage("اتحفظ التغيير في سجل الإدارة.");
      await load();
      if (tab === "listings") {
        const search = new URLSearchParams({ page: String(page) });
        if (query.trim()) search.set("q", query.trim());
        setListings(await read<Page<Listing>>(`listings?${search}`));
      }
      if (tab === "matches") setMatches(await read<Page<Match>>(`matches?page=${page}`));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "تعذّر حفظ التغيير.");
    }
  }

  async function createRoute(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session) return;
    const data = new FormData(event.currentTarget);
    const channelID = Number(data.get("channel"));
    const retailerValue = String(data.get("retailer") || "");
    setMessage("");
    try {
      const response = await fetch("/api/v1/admin/routes", {
        method: "POST", credentials: "same-origin", cache: "no-store",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrf_token },
        body: JSON.stringify({ channel_id: channelID, retailer_id: retailerValue ? Number(retailerValue) : null }),
      });
      if (!response.ok) {
        const result = await response.json();
        throw new Error(result.error || `HTTP ${response.status}`);
      }
      setRoutes((await read<Page<Route>>("routes")).items);
      setMessage("اتضاف المسار متوقفًا. راجع الوجهة ثم فعّله عند الحاجة.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "تعذّر إنشاء المسار.");
    }
  }

  async function createChannel(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!session) return;
    const data = new FormData(event.currentTarget);
    setMessage("");
    try {
      const response = await fetch("/api/v1/admin/channels", {
        method: "POST", credentials: "same-origin", cache: "no-store",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrf_token },
        body: JSON.stringify({ name: String(data.get("name") || ""), country_id: Number(data.get("country")),
          destination_id: String(data.get("destination") || ""), bot_token_env: String(data.get("token_env") || ""),
          private_test: data.get("private_test") === "on" }),
      });
      if (!response.ok) {
        const result = await response.json();
        throw new Error(result.error || `HTTP ${response.status}`);
      }
      setChannels((await read<Page<Channel>>("channels")).items);
      setMessage("اتضافت القناة متوقفة. تأكد من متغير التوكن على السيرفر قبل تفعيلها.");
      event.currentTarget.reset();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "تعذّر إنشاء القناة.");
    }
  }

  async function logout() {
    if (!session) return;
    await fetch("/api/v1/admin/session", { method: "DELETE", credentials: "same-origin",
      headers: { "X-CSRF-Token": session.csrf_token } });
    router.replace("/console/login");
  }

  if (busy) return <main className="console-shell"><p>جارٍ تحميل لوحة الإدارة…</p></main>;
  if (!session) return <main className="console-shell"><p role="alert">{message || "جارٍ التحقق من الجلسة…"}</p></main>;
  const stats = overview ? [
    ["كل المنتجات", overview.listings], ["تحت المتابعة", overview.watched],
    ["أسعار مباشرة حديثة", overview.fresh_offers], ["وظائف مستحقة", overview.due_jobs],
    ["عروض متوقفة للمراجعة", overview.held_candidates], ["سجل المنشورات", overview.publications],
  ] : [];

  return <main className="console-shell">
    <div className="console-heading"><div><span className="section-kicker">لوحة تشغيل أحسنلي</span><h1>إدارة AHSANLY</h1><p>مرحبًا {session.username} · بيانات التشغيل مباشرة من PostgreSQL</p></div><button className="console-quiet" onClick={logout}>تسجيل الخروج</button></div>
    <nav className="console-tabs" aria-label="أقسام الإدارة">{(Object.keys(tabNames) as Tab[]).map(key => <button key={key} className={tab === key ? "active" : ""} onClick={() => { setTab(key); setPage(1); setMessage(""); }}>{tabNames[key]}</button>)}</nav>
    {tab === "channels" && <section className="console-cards"><form onSubmit={event => void createChannel(event)}><h2>إضافة قناة Telegram</h2><p>القناة الجديدة تبدأ متوقفة. اكتب اسم متغير التوكن الموجود على السيرفر، بدون كتابة التوكن نفسه.</p><label>الاسم<input name="name" required maxLength={100} /></label><label>الدولة<select name="country" required defaultValue=""><option value="" disabled>اختر دولة</option>{countries.filter(country => country.enabled).map(country => <option key={country.id} value={country.id}>{country.name} ({country.code})</option>)}</select></label><label>معرّف القناة أو @username<input name="destination" required placeholder="@channel_name" /></label><label>اسم متغير التوكن<input name="token_env" required placeholder="DEAL_TELEGRAM_TOKEN_CHANNEL" /></label><label><input name="private_test" type="checkbox" /> قناة اختبار خاصة</label><button className="console-quiet" type="submit">إضافة متوقفة</button></form><div><h2>تفعيل القنوات</h2><p>يتطلب التفعيل دولة نشطة ووجهة صالحة ومتغير توكن مضبوطًا على السيرفر.</p>{channels.map(channel => <p key={channel.id}>{channel.name} ({channel.country}) · {channel.enabled ? "مفعّلة" : "متوقفة"} <button className="console-quiet" onClick={() => void patch(`channels/${channel.id}`, { enabled: !channel.enabled })}>{channel.enabled ? "إيقاف" : "تفعيل"}</button></p>)}</div></section>}
    {message && <p role="status" className="console-message">{message}</p>}
    {tab === "overview" && <><div className="console-stats">{stats.map(([label, value]) => <div key={label}><span>{label}</span><strong>{Number(value).toLocaleString("ar-EG")}</strong></div>)}</div><h2>حالة المتاجر</h2><div className="console-table-wrap"><table><thead><tr><th>المتجر</th><th>الدولة</th><th>الحالة</th><th>المصدر</th><th>منتجات المتابعة</th><th>آخر نجاح</th></tr></thead><tbody>{retailers.map(store => <tr key={store.id}><td>{store.name}</td><td>{store.country}</td><td>{store.health}{store.publish_enabled ? " · نشر مفعّل" : ""}</td><td>{store.offer_source}</td><td>{store.watched}</td><td>{store.last_success_at ? new Date(store.last_success_at).toLocaleString("ar-EG") : "—"}</td></tr>)}</tbody></table></div></>}
    {tab === "listings" && <><form className="console-search" onSubmit={e => { e.preventDefault(); setPage(1); setQuery(draftQuery); }}><input placeholder="ابحث بالاسم أو رقم المنتج" value={draftQuery} onChange={e => setDraftQuery(e.target.value)} /><button className="console-quiet" type="submit">بحث</button></form><div className="console-table-wrap"><table><thead><tr><th>المنتج</th><th>المتجر</th><th>السعر</th><th>الرصد</th><th>التكرار</th><th>الإجراء</th></tr></thead><tbody>{listings.items.map(item => <tr key={item.id}><td><a href={`/products/${item.id}`} target="_blank" rel="noreferrer">{item.title || item.product_id}</a><small>{item.product_id}</small></td><td>{item.store}</td><td>{item.price ?? "—"}{item.imported ? " · مستورد" : ""}</td><td>{item.tracking_state === "watch" ? "مراقب" : "متوقف"}</td><td>{item.cadence}</td><td><button className="console-quiet" onClick={() => void patch(`listings/${item.id}`, { tracking_state: item.tracking_state === "watch" ? "paused" : "watch", cadence: item.cadence })}>{item.tracking_state === "watch" ? "إيقاف" : "متابعة"}</button></td></tr>)}</tbody></table></div><Pager page={page} hasMore={!!listings.has_more} setPage={setPage} /></>}
    {tab === "rules" && <div className="console-cards">{rules.map(rule => <form key={rule.id} onSubmit={e => { e.preventDefault(); const data = new FormData(e.currentTarget); void patch(`rules/${rule.id}`, { historical_percent: String(data.get("percent")) }); }}><strong>قاعدة {rule.country}</strong><p>النسبة الحالية: {rule.historical_percent}% · الإصدار {rule.version}</p><label>أقل انخفاض يستحق النشر<input name="percent" type="number" min="1" max="90" step="0.01" defaultValue={rule.historical_percent} /></label><button className="button-primary">حفظ النسبة</button></form>)}</div>}
    {tab === "channels" && <><div className="console-table-wrap"><table><thead><tr><th>القناة</th><th>الدولة</th><th>المنصة</th><th>النوع</th><th>الحالة</th><th>المسارات</th></tr></thead><tbody>{channels.map(channel => <tr key={channel.id}><td>{channel.name}</td><td>{channel.country}</td><td>{channel.platform}</td><td>{channel.private_test ? "اختبار خاصة" : "عامة"}</td><td>{channel.enabled ? "مفعّلة" : "متوقفة"}</td><td>{channel.active_routes}</td></tr>)}</tbody></table></div><h2>مسارات النشر</h2><form className="console-search" onSubmit={event => void createRoute(event)}><label>القناة<select name="channel" required defaultValue=""><option value="" disabled>اختر قناة</option>{channels.map(channel => <option key={channel.id} value={channel.id}>{channel.name} ({channel.country})</option>)}</select></label><label>المصدر<select name="retailer" defaultValue=""><option value="">كل المتاجر في الدولة</option>{retailers.map(store => <option key={store.id} value={store.id}>{store.name} ({store.country})</option>)}</select></label><button className="console-quiet" type="submit">إضافة مسار متوقف</button></form><div className="console-table-wrap"><table><thead><tr><th>الدولة</th><th>القناة</th><th>المصدر</th><th>الحالة</th><th>الإجراء</th></tr></thead><tbody>{routes.map(route => <tr key={route.id}><td>{route.country}</td><td>{route.channel}</td><td>{route.retailer || "كل المتاجر"}</td><td>{route.enabled ? "مفعّل" : "متوقف"}</td><td><button className="console-quiet" disabled={!route.channel_enabled && !route.enabled} onClick={() => void patch(`routes/${route.id}`, { enabled: !route.enabled })}>{route.enabled ? "إيقاف المسار" : "تفعيل المسار"}</button></td></tr>)}</tbody></table></div></>}
    {tab === "matches" && <><div className="console-table-wrap"><table><thead><tr><th>المنتج</th><th>النسخة المرجعية المقترحة</th><th>سبب الاقتراح</th><th>القرار</th></tr></thead><tbody>{matches.items.map(item => <tr key={item.id}><td>{item.title}<small>{item.store}</small></td><td>{item.brand} {item.model}</td><td>{item.method} · {item.confidence}</td><td><button className="console-quiet" onClick={() => void patch(`matches/${item.id}`, { decision: "approved" })}>اعتماد</button> <button className="console-quiet" onClick={() => void patch(`matches/${item.id}`, { decision: "rejected" })}>رفض</button></td></tr>)}</tbody></table></div><Pager page={page} hasMore={!!matches.has_more} setPage={setPage} /></>}
    {tab === "candidates" && <><div className="console-table-wrap"><table><thead><tr><th>المنتج</th><th>السعر</th><th>الانخفاض</th><th>الحالة</th><th>الصورة</th></tr></thead><tbody>{candidates.items.map(item => <tr key={item.id}><td>{item.title}</td><td>{item.price}</td><td>{item.percent}%</td><td>{item.state}</td><td>{item.has_evidence ? "موجودة" : "—"}</td></tr>)}</tbody></table></div><Pager page={page} hasMore={!!candidates.has_more} setPage={setPage} /></>}
    {tab === "publications" && <><div className="console-table-wrap"><table><thead><tr><th>القناة</th><th>الحالة</th><th>المحاولات</th><th>رقم الرسالة</th><th>الخطأ</th></tr></thead><tbody>{publications.items.map(item => <tr key={item.id}><td>{item.channel}</td><td>{item.status}</td><td>{item.attempt_count}</td><td>{item.message_id || "—"}</td><td>{item.last_error || "—"}</td></tr>)}</tbody></table></div><Pager page={page} hasMore={!!publications.has_more} setPage={setPage} /></>}
  </main>;
}

function Pager({ page, hasMore, setPage }: { page: number; hasMore: boolean; setPage: (value: number) => void }) {
  return <div className="console-pager"><button disabled={page <= 1} onClick={() => setPage(page - 1)}>السابق</button><span>صفحة {page}</span><button disabled={!hasMore} onClick={() => setPage(page + 1)}>التالي</button></div>;
}
