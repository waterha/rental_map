import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Check, LoaderCircle, LogOut, Mail, ShieldCheck, UserRound, X } from 'lucide-react'
import { api } from './types'

interface Account { id: string; email: string }
interface AuthConfig { allowedDomains: string[]; codeExpiresIn: number; resendAfter: number; mailAvailable: boolean; passwordMinLength: number; passwordMaxBytes: number }

export default function AccountPanel() {
  const [user, setUser] = useState<Account | null>(null)
  const [open, setOpen] = useState(false)
  const [mode, setMode] = useState<'login' | 'register'>('register')
  const [config, setConfig] = useState<AuthConfig | null>(null)
  const [configError, setConfigError] = useState(false)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [sent, setSent] = useState<{ email: string; expires: number; resend: number } | null>(null)
  const [now, setNow] = useState(Date.now())
  const [sending, setSending] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [verified, setVerified] = useState(false)
  const panel = useRef<HTMLDivElement>(null)
  const pending = useRef(false)
  const emailValue = useRef(email)
  emailValue.current = email

  useEffect(() => {
    const abort = new AbortController()
    api<{ user: Account | null }>('/api/auth/me', { signal: abort.signal }).then(result => setUser(result.user)).catch(() => {})
    return () => abort.abort()
  }, [])

  async function loadConfig() {
    setConfigError(false)
    try { setConfig(await api<AuthConfig>('/api/auth/config')) } catch { setConfigError(true) }
  }

  useEffect(() => { if (open && !config) void loadConfig() }, [open, config])
  useEffect(() => {
    if (!open) return
    const timer = setInterval(() => setNow(Date.now()), 500)
    const previous = document.activeElement as HTMLElement | null
    panel.current?.focus()
    const keydown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !pending.current) { setOpen(false); setPassword(''); setCode('') }
      if (event.key === 'Tab') {
        const elements = [...(panel.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),[tabindex="0"]') || [])].filter(element => element.getClientRects().length)
        const first = elements[0], last = elements.at(-1)
        if (event.shiftKey && (document.activeElement === first || document.activeElement === panel.current)) { event.preventDefault(); last?.focus() }
        else if (!event.shiftKey && (document.activeElement === last || document.activeElement === panel.current)) { event.preventDefault(); first?.focus() }
      }
    }
    document.addEventListener('keydown', keydown)
    return () => { clearInterval(timer); document.removeEventListener('keydown', keydown); previous?.focus() }
  }, [open])

  const normalizedEmail = email.trim().toLowerCase()
  const emailParts = normalizedEmail.split('@')
  const supportedEmail = emailParts.length === 2 && /^[a-z0-9]+(?:[._+-][a-z0-9]+)*$/.test(emailParts[0]) && emailParts[0].length <= 64 && normalizedEmail.length <= 254 && Boolean(config?.allowedDomains.includes(emailParts[1]))
  const currentSent = sent?.email === normalizedEmail ? sent : null
  const resendRemaining = currentSent ? Math.max(0, Math.ceil((currentSent.resend - now) / 1000)) : 0
  const expiryRemaining = currentSent ? Math.max(0, Math.ceil((currentSent.expires - now) / 1000)) : 0
  const validPassword = password.length >= 10 && new TextEncoder().encode(password).length <= 72 && /[a-zA-Z]/.test(password) && /[0-9]/.test(password) && !/[\x00-\x1f\x7f]/.test(password)
  pending.current = sending || submitting

  function close() { if (!pending.current) { setOpen(false); setPassword(''); setCode(''); setError(''); setNotice('') } }
  function switchMode(next: 'login' | 'register') { setMode(next); setError(''); setNotice(''); setPassword(''); setCode('') }

  async function requestCode() {
    if (pending.current) return
    if (!supportedEmail) { setError('请填写受支持域名的完整邮箱地址'); return }
    const target = normalizedEmail
    setSending(true); setError(''); setNotice('')
    try {
      const result = await api<{ expiresIn: number; resendAfter: number }>('/api/auth/code', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: target }) })
      const time = Date.now()
      setSent({ email: target, expires: time + result.expiresIn * 1000, resend: time + result.resendAfter * 1000 }); setNow(time)
      if (emailValue.current.trim().toLowerCase() === target) { setCode(''); setNotice('验证码已发送，请查收收件箱或垃圾邮件。') }
    } catch (error) { setError((error as Error).message) } finally { setSending(false) }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending.current) return
    setError(''); setNotice('')
    if (!supportedEmail) { setError('请填写受支持域名的完整邮箱地址'); return }
    if (mode === 'register' && (!validPassword || !/^\d{6}$/.test(code))) { setError('请检查密码要求和 6 位验证码'); return }
    setSubmitting(true)
    try {
      const result = await api<{ user: Account }>(`/api/auth/${mode}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(mode === 'register' ? { email: normalizedEmail, password, code } : { email: normalizedEmail, password }) })
      setUser(result.user); setPassword(''); setCode(''); setSent(null); setVerified(mode === 'register')
    } catch (error) { setError((error as Error).message) } finally { setSubmitting(false) }
  }

  async function logout() {
    setSubmitting(true); setError('')
    try { await api('/api/auth/logout', { method: 'POST' }); setUser(null); setVerified(false); setPassword(''); setCode(''); setMode('login') }
    catch (error) { setError((error as Error).message) } finally { setSubmitting(false) }
  }

  return <>
    <button className={`account-trigger ${user ? 'signed-in' : ''}`} aria-label={user ? '我的账号' : '注册或登录'} title={user ? '我的账号' : '注册 / 登录'} onClick={() => { setOpen(true); setError(''); setNotice(''); setVerified(false) }}><UserRound size={18} />{user && <i />}</button>
    {open && <div className="account-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) close() }}>
      <div className="account-panel" ref={panel} role="dialog" aria-modal="true" aria-label={user ? '我的账号' : '邮箱账号'} tabIndex={-1}>
        <button className="account-close" aria-label="关闭账号窗口" onClick={close} disabled={sending || submitting}><X size={19} /></button>
        <div className="account-symbol">{user ? <ShieldCheck size={27} /> : <Mail size={27} />}</div>
        <p className="account-eyebrow">RENTAL ATLAS</p>
        <h1>{user ? verified ? '验证成功，欢迎加入' : '你的版图账号' : mode === 'register' ? '从这里，开始新的生活' : '欢迎回到你的版图'}</h1>
        <p className="account-subtitle">{user ? '邮箱已验证，登录状态安全保存。' : mode === 'register' ? '用真实邮箱注册，找到属于你的坐标。' : '使用已验证的邮箱与密码登录。'}</p>
        {user ? <div className="account-profile"><span className="verified-email"><Check size={15} />{user.email}</span><p>邮箱验证已完成</p><button className="account-submit" onClick={() => { void logout() }} disabled={submitting}>{submitting ? <LoaderCircle className="spin" size={16} /> : <LogOut size={16} />}退出登录</button></div> : <>
          <div className="account-tabs"><button className={mode === 'register' ? 'selected' : ''} onClick={() => switchMode('register')} disabled={sending || submitting}>邮箱注册</button><button className={mode === 'login' ? 'selected' : ''} onClick={() => switchMode('login')} disabled={sending || submitting}>账号登录</button></div>
          {configError ? <div className="account-message error" role="alert">无法加载邮箱规则。<button onClick={() => { void loadConfig() }}>重试</button></div> : !config ? <div className="account-config-loading"><LoaderCircle className="spin" size={18} /></div> : <form onSubmit={event => { void submit(event) }}>
            <label htmlFor="account-email">邮箱地址</label><input id="account-email" type="email" autoComplete="email" placeholder="请输入邮箱地址" value={email} maxLength={254} required disabled={sending || submitting} onChange={event => { setEmail(event.target.value); setCode(''); setError(''); setNotice('') }} />
            <p className="email-domains">支持 {config.allowedDomains.map(domain => `@${domain}`).join('、')}</p>
            <label htmlFor="account-password">密码</label><input id="account-password" type="password" autoComplete={mode === 'register' ? 'new-password' : 'current-password'} placeholder={mode === 'register' ? '至少 10 位，包含英文字母和数字' : '请输入密码'} value={password} maxLength={72} minLength={mode === 'register' ? 10 : 1} required disabled={submitting} onChange={event => setPassword(event.target.value)} />
            {mode === 'register' && <>
              <p className="password-guidance">10–72 字节，必须包含英文字母和数字。</p>
              <label htmlFor="account-code">邮箱验证码</label><div className="verification-input"><input id="account-code" type="text" inputMode="numeric" autoComplete="one-time-code" placeholder="6 位数字验证码" value={code} maxLength={6} pattern="[0-9]{6}" required disabled={submitting} onChange={event => setCode(event.target.value.replace(/\D/g, ''))} /><button type="button" onClick={() => { void requestCode() }} disabled={!supportedEmail || sending || submitting || resendRemaining > 0 || !config.mailAvailable}>{sending ? <LoaderCircle size={14} className="spin" /> : resendRemaining > 0 ? `${resendRemaining}s 后重发` : currentSent ? '重新发送' : '发送验证码'}</button></div>
              <p className={`code-guidance ${currentSent && expiryRemaining === 0 ? 'expired' : ''}`}>{currentSent ? expiryRemaining > 0 ? `验证码剩余 ${Math.floor(expiryRemaining / 60)}:${String(expiryRemaining % 60).padStart(2, '0')}，仅可使用一次。` : '验证码已过期，请重新发送。' : '验证码 2 分钟内有效，验证通过后才会创建账号。'}</p>
              {!config.mailAvailable && <p className="account-message warning">邮件服务尚未配置，暂时无法注册。请联系管理员完成发信设置。</p>}
            </>}
            {notice && <p className="account-message success" role="status">{notice}</p>}
            {error && <p className="account-message error" role="alert">{error}</p>}
            <button type="submit" className="account-submit" disabled={submitting || sending || !supportedEmail || (mode === 'register' && (!config.mailAvailable || !validPassword || code.length !== 6 || !currentSent || expiryRemaining === 0))}>{submitting && <LoaderCircle size={16} className="spin" />}{submitting ? '正在验证…' : mode === 'register' ? '验证邮箱并注册' : '登录'}</button>
          </form>}
          <p className="account-private"><ShieldCheck size={12} />密码与验证码不会保存在浏览器本地存储中。</p>
        </>}
        {user && error && <p className="account-message error" role="alert">{error}</p>}
      </div>
    </div>}
  </>
}
