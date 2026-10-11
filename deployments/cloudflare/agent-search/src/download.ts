import { authenticate, admit, AccessError, accessError } from './access.js';
import { enforceInterval, rateLimited, uncached } from './rate-limit.js';
export { PerIpLimiter } from './rate-limit.js';

const IMAGE = /^\/collections\/reviewed\/[a-f0-9]{64}\.(png|jpg|webp)$/;
const TYPES: Record<string, string> = { png: 'image/png', jpg: 'image/jpeg', webp: 'image/webp' };
const MAX_BYTES = 25 * 1024 * 1024;
const error = (message: string, status: number) => uncached(Response.json({ error: message }, { status }));

export default {
  async fetch(request: Request, env: DownloadEnv, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    const match = IMAGE.exec(url.pathname);
    if (!match || url.search) return error('Image not found.', 404);
    if (!['GET', 'HEAD'].includes(request.method)) return error('Method not allowed.', 405);
    let reservation: Awaited<ReturnType<typeof admit>> | undefined;
    let backendFailed = false;
    try {
      const ip = request.headers.get('cf-connecting-ip');
      if (!ip) return error('Download protection unavailable.', 503);
      const coarse = await env.DOWNLOAD_RATE_LIMITER.limit({ key: ip });
      if (!coarse.success) return rateLimited(60);
      const identity = await authenticate(request, env.ACCESS_KEYS_JSON, 'download');
      const denied = await enforceInterval(request, env.DOWNLOAD_IP_LIMITER);
      if (denied) return denied;
      reservation = await admit(env, 'download', identity);
      // No ranges or conditional shortcuts: every full-image request consumes a slot.
      if (request.headers.has('range')) return error('Partial downloads are not supported.', 416);
      const key = url.pathname.slice(1);
      let stream: ReadableStream | null = null;
      let object: R2Object | null;
      if (request.method === 'HEAD') object = await env.IMAGES.head(key);
      else {
        const downloaded = await env.IMAGES.get(key);
        object = downloaded;
        stream = downloaded?.body ?? null;
      }
      if (!object) return error('Image not found.', 404);
      if (object.size > MAX_BYTES) {
        if (stream) await stream.cancel();
        return error('Image exceeds download size limit.', 413);
      }
      return uncached(new Response(stream, { headers: {
        'content-type': TYPES[match[1]],
        'content-length': String(object.size),
        'x-content-type-options': 'nosniff',
        'accept-ranges': 'none',
      } }));
    } catch (caught) {
      if (caught instanceof AccessError) return accessError(caught);
      backendFailed = !!reservation;
      console.error(JSON.stringify({ event: 'download_unavailable' }));
      return error('Download service unavailable.', 503);
    } finally {
      if (reservation) ctx.waitUntil(reservation.finish(backendFailed).catch(() => console.warn(JSON.stringify({ event: 'admission_finish_failed' }))));
    }
  },
} satisfies ExportedHandler<DownloadEnv>;
