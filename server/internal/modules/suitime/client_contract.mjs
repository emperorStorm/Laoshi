import assert from 'node:assert/strict'
import vm from 'node:vm'
import http from 'node:http'
import { readFile } from 'node:fs/promises'
import { resolve, dirname } from 'node:path'

const [base, root] = process.argv.slice(2)
const clone = value => JSON.parse(JSON.stringify(value))
const now = Date.now()
const username = `contract_${now}`

async function client(device, initial = {}) {
  let data = { categories: [], tasks: [], anniversaries: [], anniversaryRecords: [],
    settings: { hideCompleted: true }, profile: { name: '测试' },
    syncMeta: { deviceId: device, cursor: 0, remoteUserId: null, outbox: {}, dataOrigin: 'user' }, ...initial }
  let auth = null
  let offline = false
  const context = vm.createContext({ console, setTimeout, clearTimeout, uni: {
    getStorageSync: () => auth,
    setStorageSync: (_, value) => { auth = value },
    removeStorageSync: () => { auth = null },
    $emit() {},
    request(options) {
      if (offline) { options.fail({ errMsg: 'offline' }); return }
      const req = http.request(options.url, { method: options.method, headers: options.header }, response => {
        let body = ''
        response.setEncoding('utf8')
        response.on('data', chunk => { body += chunk })
        response.on('end', () => {
          try { options.success({ statusCode: response.statusCode, data: body ? JSON.parse(body) : null }) }
          catch (error) { options.fail({ errMsg: error.message }) }
        })
      })
      req.on('error', error => options.fail({ errMsg: error.message }))
      req.end(options.data ? JSON.stringify(options.data) : undefined)
    }
  } })
  const modules = new Map()
  async function load(path) {
    if (modules.has(path)) return modules.get(path)
    const module = new vm.SourceTextModule(await readFile(path, 'utf8'), {
      context, identifier: path,
      initializeImportMeta(meta) { meta.env = { VITE_SUI_TIME_SYNC_BASE_URL: base } }
    })
    modules.set(path, module)
    return module
  }
  const engineModule = await load(resolve(root, 'api/sync.js'))
  await engineModule.link((specifier, referencing) => load(resolve(dirname(referencing.identifier), specifier)))
  await engineModule.evaluate()
  const api = modules.get(resolve(root, 'api/auth.js')).namespace
  const engine = engineModule.namespace.createSyncEngine({
    load: () => clone(data), save: value => { data = clone(value) },
    readAuth: () => auth, writeAuth: value => { auth = value },
    request: api.request, authenticate: api.authenticate, revokeAuth: api.revokeAuth
  })
  return { engine, get data() { return data }, get auth() { return auth },
    edit(fn) { fn(data) }, offline(value) { offline = value } }
}

const a = await client('device-a', {
  categories: [{ id: 'c1', name: '学习', color: '#123456', createdAt: now - 100, updatedAt: now }],
  tasks: [{ id: 't1', title: '联调事项', status: 'todo', notes: '', createdAt: now - 100, updatedAt: now,
    repeatRule: '{"kind":"none"}', occurrenceOverrides: '{}' }],
  anniversaries: [{ id: 'a1', title: '纪念日', kind: 'anniversary', date: '2020-01-01', notes: '',
    photos: [], createdAt: now - 100, updatedAt: now }],
  anniversaryRecords: [{ anniversaryId: 'a1', date: '2026-01-01', title: '纪念日', kind: 'anniversary',
    originalDate: '2020-01-01', notes: '', createdAt: now - 100, updatedAt: now }]
})
await a.engine.bind({ mode: 'register', username, password: 'local_password_123', strategy: 'upload' })
const b = await client('device-b')
await b.engine.bind({ mode: 'login', username, password: 'local_password_123', strategy: 'remote' })
assert.equal(b.data.tasks[0].title, '联调事项')
assert.equal(b.data.tasks[0].createdAt, now - 100)
assert.equal(b.data.anniversaries.length, 1)
assert.equal(b.data.anniversaryRecords.length, 1)
b.edit(data => { const task = data.tasks[0]; task.title = '另一设备修改'; task.updatedAt = Date.now();
  task.deviceId = 'device-b'; data.syncMeta.outbox.task = { t1: task.updatedAt } })
b.offline(true)
const before = clone(b.data)
await assert.rejects(b.engine.syncNow(), /offline/)
assert.deepEqual(b.data, before)
b.offline(false)
await b.engine.syncNow()
await a.engine.syncNow()
assert.equal(a.data.tasks[0].title, '另一设备修改')
b.edit(data => { data.tasks = []; data.syncMeta.outbox.task = { t1: Date.now() } })
await b.engine.syncNow()
await a.engine.syncNow()
assert.ok(a.data.tasks[0].deletedAt > 0)
await b.engine.unbind()
assert.equal(b.auth, null)
assert.equal(b.data.anniversaries.length, 1)
console.log('PASS real uni-client modules: auth, upload, remote bind, five entities, createdAt, edits, offline retry, tombstones, unbind')
