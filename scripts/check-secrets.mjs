import { execFileSync } from 'node:child_process'
import { readFile } from 'node:fs/promises'

// Inspect staged blobs rather than the working tree, including the initial commit.
const files = execFileSync('git', ['ls-files', '--cached', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean)
const privateValues = new Set()
for (const file of ['.env', '.env.local', '.env.production', '.env.production.local']) {
  try {
    const content = await readFile(file, 'utf8')
    for (const line of content.split(/\r?\n/)) {
      const match = line.match(/^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$/)
      if (!match || !/(KEY|SECRET|TOKEN|PASSWORD|CREDENTIAL|SECURITY_CODE)/i.test(match[1])) continue
      const value = match[2].replace(/^(['"])(.*)\1$/, '$2')
      if (value.length >= 8 && !/^your_|^replace_|^example/i.test(value)) privateValues.add(value)
    }
  } catch (error) { if (error.code !== 'ENOENT') throw error }
}

const forbidden = /(^|\/)(?:\.env(?:\..*)?|\.local|dist|node_modules|test-results|playwright-report|coverage)(\/|$)|\.(?:db(?:-.*)?|sqlite3?(?:-.*)?|pem|key|pfx|p12|exe|log|dump|bak|zip|tsbuildinfo)$/i
const signatures = [
  /\bgh[pousr]_[A-Za-z0-9]{20,}\b/,
  /\bgithub_pat_[A-Za-z0-9_]{20,}\b/,
  /\bAKIA[A-Z0-9]{16}\b/,
  /-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/,
  /["'][a-f0-9]{32}["']/i,
  /[A-Z]:\\Users\\[^\\\s]+/i,
]
const violations = []
for (const file of files) {
  if (file !== '.env.example' && forbidden.test(file)) { violations.push(`${file}: private or generated file`); continue }
  const blob = execFileSync('git', ['show', `:${file}`], { maxBuffer: 32 * 1024 * 1024 })
  if ([...privateValues].some(value => blob.includes(Buffer.from(value)))) { violations.push(`${file}: contains a local credential`); continue }
  if (/\.(?:jpg|jpeg|png|webp|gif|ico|woff2?)$/i.test(file)) continue
  const text = blob.toString('utf8')
  if (signatures.some(pattern => pattern.test(text))) violations.push(`${file}: credential or personal path pattern`)
}
if (violations.length) {
  console.error('Privacy check rejected the staged upload. Secret values are never printed.')
  violations.forEach(issue => console.error(issue))
  process.exit(1)
}
console.log(`Privacy check passed: ${files.length} staged files; local credentials, generated bundles, databases and private keys excluded.`)
