import { NextRequest, NextResponse } from 'next/server';

const ALLOWED_HOSTS = [
  'm.media-amazon.com',
  'images-na.ssl-images-amazon.com',
  'images-eu.ssl-images-amazon.com',
  'cf1.s3.souqcdn.com',
  'z.nooncdn.com',
];

export async function GET(request: NextRequest) {
  const url = request.nextUrl.searchParams.get('url');
  if (!url) return new NextResponse('Missing url', { status: 400 });
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return new NextResponse('Invalid url', { status: 400 });
  }
  if (!ALLOWED_HOSTS.includes(parsed.hostname)) {
    return new NextResponse('Host not allowed', { status: 403 });
  }
  try {
    const upstream = await fetch(url, {
      headers: { Referer: parsed.origin + '/' },
      next: { revalidate: 86400 },
    });
    if (!upstream.ok) {
      return new NextResponse('Upstream error', { status: upstream.status });
    }
    const contentType = upstream.headers.get('content-type') || 'image/jpeg';
    const body = await upstream.arrayBuffer();
    return new NextResponse(body, {
      status: 200,
      headers: {
        'Content-Type': contentType,
        'Cache-Control': 'public, max-age=86400, stale-while-revalidate=604800',
      },
    });
  } catch {
    return new NextResponse('Fetch failed', { status: 502 });
  }
}
