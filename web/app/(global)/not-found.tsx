import Link from "next/link";
export default function NotFound() { return <main className="empty-page"><span>404</span><h1>المنتج ده مش موجود عندنا</h1><p>يمكن الرابط اتغير. ارجع للكتالوج وجرب البحث بالاسم.</p><Link href="/" className="button-primary">العودة للمنتجات ←</Link></main>; }
