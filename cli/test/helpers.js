import { createServer } from 'node:http';
import { once } from 'node:events';

export const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l9sAAAAASUVORK5CYII=', 'base64');

export async function serve(handler, t) {
  const requests = [];
  const server = createServer(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    const body = Buffer.concat(chunks).toString('utf8');
    requests.push({ method: request.method, url: request.url, headers: request.headers, body });
    await handler(request, response, body);
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  t.after(() => { server.closeAllConnections(); server.close(); });
  return { origin: `http://127.0.0.1:${server.address().port}`, requests };
}

export function json(response, data, status = 200, headers = {}) {
  response.writeHead(status, { 'Content-Type': 'application/json', ...headers });
  response.end(JSON.stringify(data));
}

export function wireMeme(origin, id = 'meme-1') {
  return { id, url: `${origin}/image.png`, image_info: { width: 1, height: 1, format: 2 }, tags: ['打工', '开会'], category: 'reaction' };
}
