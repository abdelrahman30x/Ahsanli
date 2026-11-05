"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

export default function ConsoleLogin() {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [totp, setTotp] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const response = await fetch("/api/v1/admin/session", {
        method: "POST", credentials: "same-origin", cache: "no-store",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, totp }),
      });
      if (!response.ok) {
        setError(response.status === 404 ? "لوحة Go غير مفعّلة حاليًا." : "بيانات الدخول أو رمز التحقق غير صحيحين.");
        return;
      }
      setPassword("");
      setTotp("");
      router.replace("/console");
    } catch {
      setError("تعذّر الاتصال بخدمة الإدارة. جرّب مرة أخرى.");
    } finally {
      setBusy(false);
    }
  }

  return <main className="console-login-wrap"><section className="console-login-card">
    <span className="section-kicker">إدارة أحسنلي | AHSANLY</span>
    <h1>دخول فريق الإدارة</h1>
    <p>استخدم حساب الموظف ورمز التحقق من تطبيق المصادقة. إعداد TOTP لأول مرة متاح من لوحة الإدارة الحالية.</p>
    <form onSubmit={submit} className="console-form">
      <label>اسم المستخدم<input autoComplete="username" required value={username} onChange={e => setUsername(e.target.value)} /></label>
      <label>كلمة المرور<input type="password" autoComplete="current-password" required value={password} onChange={e => setPassword(e.target.value)} /></label>
      <label>رمز التحقق<input inputMode="numeric" pattern="[0-9]{6,8}" autoComplete="one-time-code" required value={totp} onChange={e => setTotp(e.target.value)} /></label>
      {error && <p role="alert" className="console-error">{error}</p>}
      <button className="button-primary" disabled={busy}>{busy ? "جارٍ التحقق…" : "دخول آمن ←"}</button>
    </form>
  </section></main>;
}
