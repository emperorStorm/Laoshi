#!/usr/bin/env node
import { promises as fs } from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const safeId = /^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$/;
let temporary;
let pendingArchive;

function git(...args) {
  try {
    return execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch {
    return null;
  }
}

async function copyWeb(source, target) {
  const stat = await fs.lstat(source);
  if (stat.isSymbolicLink()) throw new Error(`dist 不允许符号链接：${source}`);
  const name = path.basename(source);
  if (/^(?:node_modules|src|\.git|\.env(?:\..*)?|\.ssh)$/i.test(name)
      || /\.(?:pem|key|p12|pfx|map|vue|tsx?|jsx)$/i.test(name)
      || /^(?:id_rsa|id_ed25519|credentials(?:\..*)?)$/i.test(name)) {
    throw new Error(`dist 包含不应发布的文件或目录：${source}`);
  }
  if (stat.isDirectory()) {
    await fs.mkdir(target, { recursive: true, mode: 0o755 });
    for (const entry of await fs.readdir(source)) {
      if (entry === '.DS_Store') continue;
      await copyWeb(path.join(source, entry), path.join(target, entry));
    }
  } else if (stat.isFile()) {
    const data = await fs.readFile(source);
    if (/-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----/.test(data.toString('utf8'))) {
      throw new Error(`dist 包含私钥内容：${source}`);
    }
    await fs.writeFile(target, data, { mode: 0o644 });
  } else {
    throw new Error(`dist 仅允许普通文件和目录：${source}`);
  }
}

// API 附件采用白名单；不会递归复制 server、deploy 或真实环境文件。
async function copyApiFiles(source, target, extension) {
  const stat = await fs.lstat(source);
  if (stat.isSymbolicLink()) throw new Error(`API 附件不允许符号链接：${source}`);
  if (stat.isDirectory()) {
    await fs.mkdir(target, { recursive: true, mode: 0o755 });
    let count = 0;
    for (const entry of await fs.readdir(source)) {
      count += await copyApiFiles(path.join(source, entry), path.join(target, entry), extension);
    }
    return count;
  }
  if (!stat.isFile()) throw new Error(`API 附件仅允许普通文件：${source}`);
  if (!extension.test(path.basename(source))) return 0;
  const data = await fs.readFile(source);
  if (/-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----/.test(data.toString('utf8'))) {
    throw new Error(`API 附件包含私钥：${source}`);
  }
  await fs.mkdir(path.dirname(target), { recursive: true });
  await fs.writeFile(target, data, { mode: 0o644 });
  return 1;
}

async function packageApi() {
  const arch = process.env.GOARCH ?? 'amd64';
  if (!['amd64', 'arm64'].includes(arch)) throw new Error('API GOARCH 仅支持 amd64 或 arm64。');
  const server = path.join(root, 'server');
  const api = path.join(temporary, 'api');
  await fs.mkdir(api, { mode: 0o755 });
  const env = { ...process.env, GOOS: 'linux', GOARCH: arch, CGO_ENABLED: '0' };
  // 不捕获构建失败：缺 Go、依赖下载/网络错误均阻断发布，不降级为静态包。
  execFileSync('go', ['build', '-trimpath', '-o', path.join(api, 'laoshi-api'), './cmd/api'], {
    cwd: server, env, stdio: 'inherit',
  });
  await fs.chmod(path.join(api, 'laoshi-api'), 0o755);
  const migrations = await copyApiFiles(path.join(server, 'db', 'migrations'), path.join(api, 'db', 'migrations'), /\.sql$/i);
  if (!migrations) throw new Error('API 包缺少迁移 SQL。');
  await copyApiFiles(path.join(server, 'api', 'openapi.yaml'), path.join(api, 'api', 'openapi.yaml'), /^openapi\.yaml$/);
  await copyApiFiles(path.join(root, 'deploy', 'laoshi-api.service'), path.join(temporary, 'deploy', 'laoshi-api.service'), /^laoshi-api\.service$/);
  await copyApiFiles(path.join(root, 'deploy', 'laoshi-api.env.example'), path.join(temporary, 'deploy', 'laoshi-api.env.example'), /^laoshi-api\.env\.example$/);
  return { goos: 'linux', goarch: arch, cgoEnabled: false, entrypoint: 'cmd/api', migration: 'embedded-goose', migrations };
}

try {
  if (process.env.INCLUDE_API !== undefined && !['0', '1'].includes(process.env.INCLUDE_API)) {
    throw new Error('INCLUDE_API 仅支持 0 或 1；默认只打包静态站。');
  }
  const includeApi = process.env.INCLUDE_API === '1';
  const builtAt = new Date().toISOString();
  const requestedId = process.env.RELEASE_ID;
  if (requestedId !== undefined && !safeId.test(requestedId)) {
    throw new Error('RELEASE_ID 必须为 1–96 位安全目录名：以字母或数字开头，仅含字母、数字、点、下划线、短横线。');
  }
  let siteUrl = null;
  if (process.env.SITE_URL !== undefined) {
    const url = new URL(process.env.SITE_URL);
    if (url.protocol !== 'https:' || url.username || url.password
        || url.pathname !== '/' || url.search || url.hash) {
      throw new Error('SITE_URL 必须为不带凭据、路径、查询或片段的 HTTPS 站点根地址。');
    }
    siteUrl = url.origin;
  }
  let appVersion = null;
  try {
    appVersion = JSON.parse(await fs.readFile(path.join(root, 'package.json'), 'utf8')).version ?? null;
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
  }
  const commit = git('rev-parse', '--verify', 'HEAD');
  const status = commit ? git('status', '--porcelain') : null;
  const version = requestedId ?? `${builtAt.replace(/[-:]/g, '').replace(/\.\d{3}Z$/, 'Z')}-${commit ? commit.slice(0, 12) : 'nogit'}-${randomBytes(3).toString('hex')}`;
  const dist = path.join(root, 'dist');
  if (!(await fs.lstat(dist)).isDirectory()) throw new Error('dist 必须为真实目录，请先执行 npm run build。');
  for (const required of ['index.html', '404.html']) {
    if (!(await fs.lstat(path.join(dist, required))).isFile()) throw new Error(`dist 缺少普通文件 ${required}`);
  }

  // 确认父目录存在，随后才创建 artifacts。
  if (!(await fs.stat(root)).isDirectory()) throw new Error('项目根目录不存在。');
  const artifacts = path.join(root, 'artifacts');
  await fs.mkdir(artifacts, { recursive: true });
  if ((await fs.lstat(artifacts)).isSymbolicLink()) throw new Error('artifacts 不允许符号链接。');
  const archive = path.join(artifacts, `laoshi-${version}.tar.gz`);
  temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'laoshi-package-'));
  await copyWeb(dist, path.join(temporary, 'web'));
  await fs.mkdir(path.join(temporary, 'nginx'));
  for (const name of ['http.conf', 'tls.conf', 'security-headers.conf']) {
    await fs.copyFile(path.join(root, 'deploy', 'nginx', name), path.join(temporary, 'nginx', name));
  }
  await fs.copyFile(path.join(root, 'deploy', 'README.md'), path.join(temporary, 'README.md'));
  const api = includeApi ? await packageApi() : null;
  const release = { version, appVersion, packagedAt: builtAt, siteUrl, api, git: { commit, dirty: status === null ? null : status.length > 0 } };
  await fs.writeFile(path.join(temporary, 'release.json'), `${JSON.stringify(release, null, 2)}\n`);
  pendingArchive = path.join(artifacts, `.laoshi-${version}-${randomBytes(6).toString('hex')}.tmp`);
  const members = ['web', 'nginx', 'README.md', 'release.json', ...(includeApi ? ['api', 'deploy'] : [])];
  execFileSync('tar', ['-czf', pendingArchive, '-C', temporary, ...members], {
    stdio: 'inherit', env: { ...process.env, COPYFILE_DISABLE: '1' },
  });
  // 硬链接以排他方式发布归档，已有同名版本绝不覆盖。
  await fs.link(pendingArchive, archive);
  console.log(`部署包：${archive}`);
  console.log(`版本：${version}；SITE_URL：${siteUrl ?? '未设置（生产发布前请设置并重新构建）'}`);
} catch (error) {
  console.error(`打包失败：${error.message}`);
  process.exitCode = 1;
} finally {
  if (pendingArchive) await fs.rm(pendingArchive, { force: true });
  if (temporary) await fs.rm(temporary, { recursive: true, force: true });
}
