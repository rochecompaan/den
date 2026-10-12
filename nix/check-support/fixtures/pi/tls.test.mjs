import assert from 'node:assert/strict';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import http from 'node:http';
import https from 'node:https';
import net from 'node:net';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { spawn } from 'node:child_process';

const listen = server => new Promise(resolve => server.listen(0, '127.0.0.1', resolve));

export async function checkTrust() {
  const fixtures = process.env.PI_TLS_FIXTURES;
  const servers = [];
  const sockets = new Set();
  const requests = [];
  const track = socket => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
  };
  try {
    for (const name of ['provider', 'broker', 'untrusted']) {
      const server = https.createServer({
        key: readFileSync(join(fixtures, `${name}.key`)),
        cert: readFileSync(join(fixtures, `${name}.pem`)),
      }, (_request, response) => { requests.push(name); response.end(name); });
      server.on('connection', track);
      await listen(server);
      servers.push(server);
    }
    const proxy = http.createServer();
    proxy.on('connection', track);
    proxy.on('connect', (request, client, head) => {
      const [host, port] = request.url.split(':');
      assert.ok(host === 'localhost' || host === '127.0.0.1');
      const upstream = net.connect(Number(port), '127.0.0.1', () => {
        client.write('HTTP/1.1 200 Connection Established\r\n\r\n');
        upstream.write(head);
        upstream.pipe(client); client.pipe(upstream);
      });
      track(upstream);
      upstream.on('error', () => client.destroy());
      client.on('error', () => upstream.destroy());
    });
    await listen(proxy);
    servers.push(proxy);
    process.env.HTTP_PROXY = process.env.HTTPS_PROXY = `http://127.0.0.1:${proxy.address().port}`;
    process.env.NO_PROXY = process.env.no_proxy = '';
    const { configureHttpDispatcher } = await import(pathToFileURL(join(process.env.PI_PACKAGE_DIR, 'dist/core/http-dispatcher.js')));
    configureHttpDispatcher(3000);
    const url = (index, host = 'localhost') => `https://${host}:${servers[index].address().port}/`;
    const get = async target => {
      const response = await fetch(target, { signal: AbortSignal.timeout(3000) });
      return response.text();
    };
    // Test effective trust by handshake, not the platform's CA enumeration.
    // Without public CA selection, this generated provider root is untrusted.
    const provider = await get(url(0)).catch(error => {
      throw new Error(`public/provider TLS failed: ${error.cause?.code}`, { cause: error });
    });
    assert.equal(provider, 'provider', 'public/provider CA trust');
    assert.equal(await get(url(1)), 'broker', 'separate NODE_EXTRA_CA_CERTS broker trust');
    await assert.rejects(get(url(2)), error =>
      error.cause?.code === 'UNABLE_TO_VERIFY_LEAF_SIGNATURE' ||
      error.cause?.code === 'UNABLE_TO_GET_ISSUER_CERT_LOCALLY', 'untrusted CA must fail');
    await assert.rejects(get(url(0, '127.0.0.1')), error =>
      error.cause?.code === 'ERR_TLS_CERT_ALTNAME_INVALID', 'hostname verification must remain enabled');
    assert.deepEqual(requests, ['provider', 'broker']);
    writeFileSync(join(process.env.PI_CODING_AGENT_DIR, 'tls-passed'), 'passed\n');
  } finally {
    for (const socket of sockets) socket.destroy();
    await Promise.all(servers.map(server => new Promise(resolve => server.close(resolve))));
  }
}

// Drive the real Pi adapter over RPC, keeping stdin open until its command ends.
if (process.argv[1] === new URL(import.meta.url).pathname) {
  const child = spawn(process.env.PI_TLS_AGENT, [
    '--mode', 'rpc', '--no-tools', '--no-session', '--extension', process.env.PI_TLS_EXTENSION,
  ], { stdio: ['pipe', 'pipe', 'pipe'] });
  let stdout = '', stderr = '', buffer = '';
  child.stdout.on('data', chunk => {
    stdout += chunk; buffer += chunk;
    while (buffer.includes('\n')) {
      const end = buffer.indexOf('\n');
      const line = buffer.slice(0, end); buffer = buffer.slice(end + 1);
      let response;
      try { response = JSON.parse(line); } catch { continue; }
      if (response.type === 'response' && response.id === 'tls') child.stdin.end();
    }
  });
  child.stderr.on('data', chunk => { stderr += chunk; });
  child.stdin.write(JSON.stringify({ id: 'tls', type: 'prompt', message: '/tls-test' }) + '\n');
  const timer = setTimeout(() => child.kill('SIGTERM'), 15000);
  const code = await new Promise((resolve, reject) => {
    child.on('error', reject); child.on('exit', resolve);
  });
  clearTimeout(timer);
  assert.equal(code, 0, stderr + stdout);
  const response = stdout.split('\n').filter(Boolean).map(line => JSON.parse(line))
    .find(record => record.type === 'response' && record.id === 'tls');
  assert.equal(response?.success, true, stderr + stdout);
  const report = join(process.env.PI_CODING_AGENT_DIR, 'tls-passed');
  assert.ok(existsSync(report), stderr + stdout);
  assert.equal(readFileSync(report, 'utf8'), 'passed\n');
  console.log('Pi TLS: provider and broker trusted; unknown CA and wrong hostname rejected.');
}
