import Link from "next/link";

export default function NotFound() {
  return <main className="empty-page"><span>404</span><h1>الصفقة دي مش متاحة</h1><p>يمكن العرض انتهى أو لسه لم يعتمد. ارجع للعروض المختارة المتاحة الآن.</p><Link href="/ar/deals" className="button-primary">العودة للعروض ←</Link></main>;
}
