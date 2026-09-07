import { StrictMode, useEffect, useState, type FormEvent } from 'react'
import { createRoot } from 'react-dom/client'
import './styles.css'

type View = 'overview' | 'devices' | 'monitoring' | 'rmm' | 'policies' | 'automation' | 'audit'
type Device = { id: string; hostname: string; platform: string; os_name: string; os_version: string; status: string; agent_version: string; last_seen?: string }
type Metric = { device_id?: string; hostname?: string; status?: string; cpu_percent?: number; memory_used_bytes?: number; memory_total_bytes?: number; collected_at?: string }
type Inventory = { hostname?: string; platform?: string; os_name?: string; architecture?: string; cpu_cores?: number; memory_total_bytes?: number; memory_used_bytes?: number; network?: { name: string; mac: string; ips: string[] }[]; disks?: { name: string; size_bytes?: number; used_bytes?: number; free_bytes?: number; filesystem?: string }[]; software?: string[]; services?: { name: string; status: string; description?: string }[] }
type RMMTask = { id: string; status: string; command_type: string; command?: string; params?: Record<string, unknown>; stdout?: string; stderr?: string; created_at: string; started_at?: string; completed_at?: string; file_name?: string; file_path?: string; file_size?: number }
type RMMCategory = 'terminal' | 'process' | 'service' | 'system' | 'files'
const API = import.meta.env.VITE_API_URL ?? 'https://app.cyverratech.my.id'
const authHeaders = () => ({ Authorization: `Bearer ${localStorage.getItem('token') ?? ''}` })

function App() {
  const [view, setView] = useState<View>('overview')
  const [devices, setDevices] = useState<Device[]>([])
  const [metrics, setMetrics] = useState<Metric[]>([])
  const [selected, setSelected] = useState<Device | null>(null)
  const [inventory, setInventory] = useState<Inventory | null>(null)
  const [deviceMetrics, setDeviceMetrics] = useState<Metric[]>([])
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const [authenticated, setAuthenticated] = useState(!!localStorage.getItem('token'))

  const loadDevices = async () => {
    const response = await fetch(`${API}/api/v1/devices`, { headers: authHeaders() })
    if (response.status === 401) { localStorage.removeItem('token'); setAuthenticated(false); return }
    if (!response.ok) { setError('Unable to load devices.'); return }
    setDevices((await response.json()).data)
  }
  const loadMetrics = async () => {
    const response = await fetch(`${API}/api/v1/monitoring/summary`, { headers: authHeaders() })
    if (response.ok) setMetrics((await response.json()).data)
  }
  useEffect(() => { if (authenticated) { void loadDevices(); void loadMetrics() } }, [authenticated])
  useEffect(() => { if (!authenticated) return; const timer = window.setInterval(() => { void loadDevices(); void loadMetrics() }, 30000); return () => window.clearInterval(timer) }, [authenticated])

  const createToken = async () => {
    setError('')
    const response = await fetch(`${API}/api/v1/enrollment-tokens`, { method: 'POST', headers: authHeaders() })
    const body = await response.json()
    if (response.ok) { setToken(body.token); setView('devices') } else setError(body.error?.message ?? 'Unable to create enrollment token')
  }
  const showDevice = async (device: Device) => {
    setSelected(device); setInventory(null); setDeviceMetrics([]); setView('devices')
    const response = await fetch(`${API}/api/v1/devices/${device.id}/inventory`, { headers: authHeaders() })
    if (response.ok) setInventory((await response.json()).data)
    const metricsResponse = await fetch(`${API}/api/v1/devices/${device.id}/metrics`, { headers: authHeaders() })
    if (metricsResponse.ok) setDeviceMetrics((await metricsResponse.json()).data)
  }
  if (!authenticated) return <Login onSuccess={() => setAuthenticated(true)} />
  const online = devices.filter(device => device.status === 'ONLINE').length
  const content = view === 'overview' ? <Overview devices={devices} online={online} metrics={metrics} onSelect={showDevice} onAdd={createToken} />
    : view === 'devices' ? <Devices devices={devices} token={token} selected={selected} inventory={inventory} deviceMetrics={deviceMetrics} onAdd={createToken} onSelect={showDevice} />
      : view === 'monitoring' ? <Monitoring devices={devices} metrics={metrics} onSelect={showDevice} />
        : view === 'rmm' ? <RMMPanel devices={devices} selected={selected} onSelect={showDevice} />
          : <ComingSoon view={view} />
  return <main><aside><div className="brand"><span className="mark">C</span><span>CYVERRA <b>NEXUS</b></span></div><nav>{([['overview', 'Overview'], ['devices', 'Devices'], ['monitoring', 'Monitoring'], ['rmm', 'Remote Mgmt'], ['policies', 'Policies'], ['automation', 'Automation'], ['audit', 'Audit trail']] as [View, string][]).map(([key, label]) => <button key={key} className={view === key ? 'active' : ''} onClick={() => { setView(key); setError('') }}>{label}{key === 'devices' && <small>{devices.length}</small>}</button>)}</nav><div className="tenant"><span className="avatar">CN</span><div><strong>Control room</strong><small>Organization workspace</small></div></div></aside><section className="content"><header><div><p className="eyebrow">OPERATIONS / {view.toUpperCase()}</p><h1>{view === 'overview' ? 'Fleet command center' : view[0].toUpperCase() + view.slice(1)}</h1><p className="subhead">{view === 'monitoring' ? 'Live health signals from enrolled endpoints.' : view === 'rmm' ? 'Remote management and monitoring operations.' : 'Manage your organization from one operational workspace.'}</p></div>{(view === 'overview' || view === 'devices') && <button onClick={createToken}>＋ Add device</button>}</header>{error && <div className="notice">{error}</div>}{content}</section></main>
}
function Overview({ devices, online, metrics, onSelect, onAdd }: { devices: Device[]; online: number; metrics: Metric[]; onSelect: (d: Device) => void; onAdd: () => void }) { return <><div className="stats"><Stat label="Total devices" value={devices.length} tone="ink" /><Stat label="Online now" value={online} tone="teal" /><Stat label="Needs attention" value={devices.filter(d => d.status !== 'ONLINE').length} tone="coral" /><Stat label="Metric streams" value={metrics.filter(m => m.collected_at).length} tone="gold" /></div><div className="workspace"><DeviceTable devices={devices} onSelect={onSelect} onAdd={onAdd} /><div className="panel activity"><div className="panel-head"><div><p className="eyebrow">SYSTEM PULSE</p><h2>Live signal</h2></div></div><div className="pulse"><Pulse label="Heartbeat service" text={`${online} endpoint${online === 1 ? '' : 's'} online`} tone="online" /><Pulse label="Metric pipeline" text={`${metrics.filter(m => m.collected_at).length} devices reporting`} tone="gold" /><Pulse label="Alert stream" text="No alert rules configured" tone="coral" /></div></div></div></> }
function Devices({ devices, token, selected, inventory, deviceMetrics, onAdd, onSelect }: { devices: Device[]; token: string; selected: Device | null; inventory: Inventory | null; deviceMetrics: Metric[]; onAdd: () => void; onSelect: (d: Device) => void }) { return <div className="device-layout"><div className="panel"><div className="panel-head"><div><p className="eyebrow">INVENTORY</p><h2>Managed devices</h2></div><button className="quiet" onClick={onAdd}>＋ Enrollment token</button></div><AgentDownloads />{token && <Enrollment token={token} />}{devices.length ? <DeviceTable devices={devices} onSelect={onSelect} onAdd={onAdd} /> : <Empty onAdd={onAdd} />}</div>{selected && <div className="panel detail"><div className="panel-head"><div><p className="eyebrow">DEVICE DETAIL</p><h2>{selected.hostname}</h2></div><span className="status"><i className={`dot ${selected.status.toLowerCase()}`} />{selected.status}</span></div><div className="detail-grid"><Info label="Platform" value={`${selected.os_name || selected.platform} ${selected.os_version || ''}`} /><Info label="Agent" value={selected.agent_version} /><Info label="Last seen" value={selected.last_seen ? new Date(selected.last_seen).toLocaleString() : 'Never'} /><Info label="Architecture" value={inventory?.architecture || 'Collecting'} /></div><MetricHistory metrics={deviceMetrics} />{inventory ? <div className="inventory"><h3>Hardware & network</h3><p>{inventory.cpu_cores ?? '--'} CPU cores · {formatBytes(inventory.memory_used_bytes)} / {formatBytes(inventory.memory_total_bytes)} memory</p><h3>Disks / SSD</h3>{inventory.disks?.length ? inventory.disks.map(disk => <div className="network" key={disk.name}><strong>{disk.name}</strong><span>{formatBytes(disk.used_bytes)} used / {formatBytes(disk.size_bytes)} total · {disk.filesystem || 'unknown'}</span></div>) : <p>No disk data reported.</p>}<h3>Running services</h3>{inventory.services?.length ? inventory.services.slice(0, 30).map(service => <div className="network" key={service.name}><strong>{service.name}</strong><span>{service.status}</span></div>) : <p>No services reported.</p>}<h3>Software</h3>{inventory.software?.length ? <div className="software-list">{inventory.software.slice(0, 40).map(item => <span key={item}>{item}</span>)}</div> : <p>No software inventory.</p>}</div> : <p className="muted">Waiting for inventory data from agent.</p>}<RemoteCommand deviceID={selected.id} /><RemoteDesktopSession deviceID={selected.id} /></div>}</div> }
function MetricHistory({ metrics }: { metrics: Metric[] }) { const recent = [...metrics].reverse(); const latest = recent[recent.length - 1]; const maxCpu = Math.max(100, ...recent.map(metric => metric.cpu_percent ?? 0)); return <div className="metric-history"><div className="section-title"><h3>Monitoring history</h3><span>{metrics.length ? `${metrics.length} samples` : 'Waiting for agent metrics'}</span></div>{recent.length ? <><div className="metric-chart">{recent.map((metric, index) => <div className="metric-bar" key={`${metric.collected_at}-${index}`} title={`${metric.cpu_percent?.toFixed(1) ?? '--'}% CPU`}><i style={{ height: `${Math.min(100, ((metric.cpu_percent ?? 0) / maxCpu) * 100)}%` }} /></div>)}</div><div className="metric-latest"><span>Latest CPU <strong>{latest?.cpu_percent?.toFixed(1) ?? '--'}%</strong></span><span>Latest RAM <strong>{memoryPercent(latest)}</strong></span><span>Collected <strong>{latest?.collected_at ? new Date(latest.collected_at).toLocaleTimeString() : '--'}</strong></span></div></> : <p className="muted">Agent harus aktif dan mengirim metrics terlebih dahulu.</p>}</div> }
function memoryPercent(metric?: Metric) { if (!metric?.memory_total_bytes) return '--'; return `${Math.round((metric.memory_used_bytes ?? 0) / metric.memory_total_bytes * 100)}%` }
function Monitoring({ devices, metrics, onSelect }: { devices: Device[]; metrics: Metric[]; onSelect: (d: Device) => void }) { return <div className="panel"><div className="panel-head"><div><p className="eyebrow">LIVE TELEMETRY</p><h2>Endpoint monitoring</h2></div><span className="muted">Refreshes every 30 seconds</span></div><div className="table"><div className="row heading"><span>Device</span><span>CPU</span><span>Memory</span><span>Collected</span></div>{devices.map(device => { const metric = metrics.find(item => item.device_id === device.id); const memory = metric?.memory_total_bytes ? `${Math.round((metric.memory_used_bytes ?? 0) / metric.memory_total_bytes * 100)}%` : '--'; return <button className="row clickable" key={device.id} onClick={() => onSelect(device)}><span><strong>{device.hostname}</strong><small>{device.status}</small></span><span>{metric?.cpu_percent != null ? `${metric.cpu_percent.toFixed(1)}%` : '--'}</span><span>{memory}</span><span>{metric?.collected_at ? new Date(metric.collected_at).toLocaleTimeString() : 'Waiting'}</span></button> })}</div>{devices.length === 0 && <Empty onAdd={() => undefined} />}</div> }
function DeviceTable({ devices, onSelect, onAdd }: { devices: Device[]; onSelect: (d: Device) => void; onAdd: () => void }) { return devices.length ? <div className="table"><div className="row heading"><span>Device</span><span>Platform</span><span>Agent</span><span>State</span></div>{devices.map(device => <button className="row clickable" key={device.id} onClick={() => onSelect(device)}><span><strong>{device.hostname}</strong><small>{device.os_name || device.platform}</small></span><span>{device.platform}</span><span>{device.agent_version}</span><span><i className={`dot ${device.status.toLowerCase()}`} />{device.status}</span></button>)}</div> : <Empty onAdd={onAdd} /> }
function Enrollment({ token }: { token: string }) { const windowsCommand = `.\\cyverra-agent-windows-amd64.exe --api ${API} --enrollment-token ${token}`; const linuxCommand = `./cyverra-agent-linux-amd64 --api ${API} --enrollment-token ${token}`; const copy = (command: string) => void navigator.clipboard?.writeText(command); return <div className="enrollment"><strong>Enrollment token</strong><span>Download the agent, open PowerShell or Terminal in its folder, and run the matching command within 24 hours.</span><code>{token}</code><label>Windows PowerShell<code>{windowsCommand}</code><button onClick={() => copy(windowsCommand)}>Copy Windows command</button></label><label>Linux<code>{linuxCommand}</code><button onClick={() => copy(linuxCommand)}>Copy Linux command</button></label></div> }
function AgentDownloads() { return <div className="agent-downloads"><span><strong>Endpoint agent</strong><small>Download the native binary and service installer, then use a one-time enrollment token.</small></span><a href="/downloads/cyverra-agent-windows-amd64.exe" download>Windows .exe</a><a href="/downloads/install-agent.ps1" download>Windows service script</a><a href="/downloads/cyverra-agent-linux-amd64" download>Linux amd64</a><a href="/downloads/install-agent.sh" download>Linux service script</a></div> }
function RemoteCommand({ deviceID }: { deviceID: string }) { const [command, setCommand] = useState(''); const [result, setResult] = useState(''); const run = async (event: FormEvent) => { event.preventDefault(); if (!command.trim()) return; const response = await fetch(`${API}/api/v1/devices/${deviceID}/commands`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ command }) }); setResult(response.ok ? 'Queued. The agent will execute it on the next poll.' : 'Command was rejected. Check your permissions.') }; return <div className="remote-command"><h3>Remote command</h3><p>Audited command execution with a 60 second agent timeout.</p><form onSubmit={run}><input value={command} onChange={event => setCommand(event.target.value)} placeholder="systemctl status nginx" maxLength={4096} /><button>Queue command</button></form>{result && <small>{result}</small>}</div> }
function RemoteDesktopSession({ deviceID }: { deviceID: string }) { const [session, setSession] = useState<{ session_id: string; status: string } | null>(null); const [message, setMessage] = useState(''); const start = async () => { const response = await fetch(`${API}/api/v1/devices/${deviceID}/remote-sessions`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ protocol: 'WEBRTC' }) }); const body = await response.json(); if (response.ok) { setSession(body); setMessage('Session request created. WebRTC capture agent must be installed and connected.'); } else setMessage(body.error?.message ?? 'Remote desktop request failed') }; const close = async () => { if (!session) return; await fetch(`${API}/api/v1/remote-sessions/${session.session_id}/close`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ reason: 'operator closed session' }) }); setSession(null); setMessage('Session closed') }; return <div className="remote-command remote-desktop"><h3>Remote desktop session</h3><p>Authenticated WebRTC session lifecycle. Screen capture/input gateway must be connected on the endpoint.</p>{session ? <><small>Session {session.session_id} · {session.status}</small><button onClick={close}>Close session</button></> : <button onClick={start}>Request remote desktop</button>}{message && <small>{message}</small>}</div> }
function Empty({ onAdd }: { onAdd: () => void }) { return <div className="empty"><strong>No devices connected</strong><span>Create an enrollment token, install the native agent, and it will appear here after its first heartbeat.</span><button onClick={onAdd}>＋ Add first device</button></div> }
function ComingSoon({ view }: { view: View }) { return <div className="panel empty page-empty"><strong>{view[0].toUpperCase() + view.slice(1)} workspace</strong><span>The navigation is active. This module is the next API-backed delivery slice; no fake records are shown.</span></div> }
function Pulse({ label, text, tone }: { label: string; text: string; tone: string }) { return <div className="pulse-line"><i className={`dot ${tone}`} /><span><strong>{label}</strong><small>{text}</small></span><b>Live</b></div> }
function Info({ label, value }: { label: string; value: string }) { return <div><span>{label}</span><strong>{value}</strong></div> }
function formatBytes(value?: number) { if (!value) return '--'; const units = ['B', 'KB', 'MB', 'GB', 'TB']; let size = value; let index = 0; while (size >= 1024 && index < units.length - 1) { size /= 1024; index++ } return `${size.toFixed(index ? 1 : 0)} ${units[index]}` }
function Login({ onSuccess }: { onSuccess: () => void }) { const [email, setEmail] = useState('admin@cyverra.local'); const [password, setPassword] = useState(''); const [error, setError] = useState(''); const submit = async (event: FormEvent) => { event.preventDefault(); const response = await fetch(`${API}/api/v1/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password }) }); const body = await response.json(); if (!response.ok) { setError(body.error?.message ?? 'Login failed'); return } localStorage.setItem('token', body.token); onSuccess() }; return <div className="login"><form onSubmit={submit}><span className="mark">C</span><p className="eyebrow">CYVERRA NEXUS</p><h1>Sign in to command</h1><p className="subhead">Access your organization workspace.</p><label>Email<input value={email} onChange={event => setEmail(event.target.value)} type="email" required /></label><label>Password<input value={password} onChange={event => setPassword(event.target.value)} type="password" required /></label>{error && <div className="notice">{error}</div>}<button>Continue →</button></form></div> }
function Stat({ label, value, tone }: { label: string; value: string | number; tone: string }) { return <div className={`stat ${tone}`}><span>{label}</span><strong>{value}</strong><i /></div> }

function RMMPanel({ devices, selected, onSelect }: { devices: Device[]; selected: Device | null; onSelect: (d: Device) => void }) {
  const [category, setCategory] = useState<RMMCategory>('terminal')
  const [tasks, setTasks] = useState<RMMTask[]>([])
  const [loading, setLoading] = useState(false)
  const [filterDevice, setFilterDevice] = useState<string>(selected?.id ?? '')

  const loadTasks = async (deviceId: string) => {
    if (!deviceId) return
    setLoading(true)
    const response = await fetch(`${API}/api/v1/devices/${deviceId}/rmm/tasks`, { headers: authHeaders() })
    if (response.ok) setTasks((await response.json()).data)
    setLoading(false)
  }

  useEffect(() => { if (filterDevice) void loadTasks(filterDevice) }, [filterDevice])

  const categories: { key: RMMCategory; label: string; icon: string }[] = [
    { key: 'terminal', label: 'Remote Terminal', icon: '>' },
    { key: 'process', label: 'Process', icon: '!' },
    { key: 'service', label: 'Service', icon: '#' },
    { key: 'system', label: 'System', icon: '*' },
    { key: 'files', label: 'Files', icon: '/' },
  ]

  return (
    <div className="rmm-layout">
      <div className="panel rmm-sidebar">
        <div className="panel-head"><div><p className="eyebrow">RMM OPERATIONS</p><h2>Device selection</h2></div></div>
        <label className="rmm-device-select">
          <span>Target device</span>
          <select value={filterDevice} onChange={event => { setFilterDevice(event.target.value); setTasks([]) }}>
            <option value="">-- Select device --</option>
            {devices.filter(d => d.status === 'ONLINE').map(d => <option key={d.id} value={d.id}>{d.hostname} ({d.platform})</option>)}
          </select>
        </label>
        {filterDevice && devices.find(d => d.id === filterDevice) && (
          <div className="rmm-device-info">
            <Info label="Device" value={devices.find(d => d.id === filterDevice)!.hostname} />
            <Info label="Status" value={devices.find(d => d.id === filterDevice)!.status} />
            <Info label="Platform" value={devices.find(d => d.id === filterDevice)!.os_name || devices.find(d => d.id === filterDevice)!.platform} />
          </div>
        )}
        <div className="rmm-categories">
          {categories.map(c => (
            <button key={c.key} className={category === c.key ? 'active' : ''} onClick={() => setCategory(c.key)}>
              <span className="rmm-icon">{c.icon}</span>{c.label}
            </button>
          ))}
        </div>
      </div>
      <div className="panel rmm-main">
        {!filterDevice ? (
          <div className="empty"><strong>Select a device</strong><span>Choose an online device from the sidebar to start remote management operations.</span></div>
        ) : (
          <>
            {category === 'terminal' && <RMMTerminal deviceID={filterDevice} onRefresh={() => loadTasks(filterDevice)} />}
            {category === 'process' && <RMMProcess deviceID={filterDevice} onRefresh={() => loadTasks(filterDevice)} />}
            {category === 'service' && <RMMService deviceID={filterDevice} onRefresh={() => loadTasks(filterDevice)} />}
            {category === 'system' && <RMMSystem deviceID={filterDevice} onRefresh={() => loadTasks(filterDevice)} />}
            {category === 'files' && <RMMFiles deviceID={filterDevice} onRefresh={() => loadTasks(filterDevice)} />}
          </>
        )}
        {filterDevice && (
          <div className="rmm-task-history">
            <div className="panel-head"><div><p className="eyebrow">AUDIT LOG</p><h2>Recent tasks</h2></div><button className="quiet" onClick={() => loadTasks(filterDevice)}>Refresh</button></div>
            {loading ? <p className="muted">Loading...</p> : tasks.length === 0 ? <p className="muted">No RMM tasks executed on this device yet.</p> : (
              <div className="table">
                <div className="row heading"><span>Type</span><span>Status</span><span>Output</span><span>Time</span></div>
                {tasks.map(task => (
                  <div className="row" key={task.id}>
                    <span><strong>{task.command_type}</strong><small>{task.command?.slice(0, 40) || task.file_name || '--'}</small></span>
                    <span><i className={`dot ${task.status === 'SUCCESS' ? 'online' : task.status === 'FAILED' ? 'coral' : 'gold'}`} />{task.status}</span>
                    <span className="rmm-output">{task.stdout?.slice(0, 80) || task.stderr?.slice(0, 80) || '--'}</span>
                    <span>{new Date(task.created_at).toLocaleTimeString()}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function RMMTerminal({ deviceID, onRefresh }: { deviceID: string; onRefresh: () => void }) {
  const [shell, setShell] = useState<'powershell' | 'cmd' | 'bash'>('powershell')
  const [command, setCommand] = useState('')
  const [message, setMessage] = useState('')
  const run = async (event: FormEvent) => {
    event.preventDefault()
    if (!command.trim()) return
    const response = await fetch(`${API}/api/v1/devices/${deviceID}/rmm/execute`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ command, command_type: shell }) })
    const body = await response.json()
    if (response.ok) { setMessage(`Task ${body.task_id} queued via ${shell}.`); setCommand(''); onRefresh() } else setMessage(body.error?.message ?? 'Failed')
  }
  return (
    <div className="rmm-action">
      <h3>Remote Terminal</h3><p>Execute commands via {shell} with full audit trail.</p>
      <div className="rmm-shell-select">
        {(['powershell', 'cmd', 'bash'] as const).map(s => <button key={s} className={shell === s ? 'active' : ''} onClick={() => setShell(s)}>{s}</button>)}
      </div>
      <form onSubmit={run}>
        <input value={command} onChange={event => setCommand(event.target.value)} placeholder={shell === 'powershell' ? 'Get-Service' : shell === 'cmd' ? 'dir' : 'ls -la'} maxLength={4096} />
        <button>Execute</button>
      </form>
      {message && <small>{message}</small>}
    </div>
  )
}

function RMMProcess({ deviceID, onRefresh }: { deviceID: string; onRefresh: () => void }) {
  const [action, setAction] = useState<'kill' | 'start'>('kill')
  const [pid, setPid] = useState('')
  const [name, setName] = useState('')
  const [path, setPath] = useState('')
  const [args, setArgs] = useState('')
  const [message, setMessage] = useState('')
  const run = async (event: FormEvent) => {
    event.preventDefault()
    const params: Record<string, unknown> = action === 'kill' ? (pid ? { pid } : { name }) : { path, args: args ? args.split(/\s+/) : [] }
    const response = await fetch(`${API}/api/v1/devices/${deviceID}/rmm/execute`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ command: action === 'kill' ? `kill ${pid || name}` : `start ${path}`, command_type: action === 'kill' ? 'kill_process' : 'start_process', params }) })
    const body = await response.json()
    if (response.ok) { setMessage(`Task ${body.task_id} queued.`); onRefresh() } else setMessage(body.error?.message ?? 'Failed')
  }
  return (
    <div className="rmm-action">
      <h3>Process Management</h3><p>Kill or start processes on the target device.</p>
      <div className="rmm-shell-select">
        <button className={action === 'kill' ? 'active' : ''} onClick={() => setAction('kill')}>Kill Process</button>
        <button className={action === 'start' ? 'active' : ''} onClick={() => setAction('start')}>Start Process</button>
      </div>
      <form onSubmit={run}>
        {action === 'kill' ? (
          <><input value={pid} onChange={event => setPid(event.target.value)} placeholder="PID (e.g. 1234)" /><input value={name} onChange={event => setName(event.target.value)} placeholder="Process name (e.g. notepad.exe)" /></>
        ) : (
          <><input value={path} onChange={event => setPath(event.target.value)} placeholder="Executable path (e.g. /usr/bin/top)" /><input value={args} onChange={event => setArgs(event.target.value)} placeholder="Arguments (space separated)" /></>
        )}
        <button>{action === 'kill' ? 'Kill' : 'Start'} Process</button>
      </form>
      {message && <small>{message}</small>}
    </div>
  )
}

function RMMService({ deviceID, onRefresh }: { deviceID: string; onRefresh: () => void }) {
  const [action, setAction] = useState<'start' | 'stop' | 'restart'>('start')
  const [serviceName, setServiceName] = useState('')
  const [message, setMessage] = useState('')
  const run = async (event: FormEvent) => {
    event.preventDefault()
    if (!serviceName.trim()) return
    const response = await fetch(`${API}/api/v1/devices/${deviceID}/rmm/execute`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ command: `${action} ${serviceName}`, command_type: `${action}_service`, params: { service: serviceName } }) })
    const body = await response.json()
    if (response.ok) { setMessage(`Task ${body.task_id} queued.`); onRefresh() } else setMessage(body.error?.message ?? 'Failed')
  }
  return (
    <div className="rmm-action">
      <h3>Service Control</h3><p>Start, stop, or restart system services.</p>
      <div className="rmm-shell-select">
        <button className={action === 'start' ? 'active' : ''} onClick={() => setAction('start')}>Start</button>
        <button className={action === 'stop' ? 'active' : ''} onClick={() => setAction('stop')}>Stop</button>
        <button className={action === 'restart' ? 'active' : ''} onClick={() => setAction('restart')}>Restart</button>
      </div>
      <form onSubmit={run}>
        <input value={serviceName} onChange={event => setServiceName(event.target.value)} placeholder="Service name (e.g. nginx, sshd)" />
        <button>{action.charAt(0).toUpperCase() + action.slice(1)} Service</button>
      </form>
      {message && <small>{message}</small>}
    </div>
  )
}

function RMMSystem({ deviceID, onRefresh }: { deviceID: string; onRefresh: () => void }) {
  const [action, setAction] = useState<'reboot' | 'shutdown' | 'lock' | 'logoff'>('reboot')
  const [delay, setDelay] = useState('0')
  const [message, setMessage] = useState('')
  const run = async () => {
    const response = await fetch(`${API}/api/v1/devices/${deviceID}/rmm/execute`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ command: action, command_type: action === 'lock' ? 'lock_workstation' : action === 'logoff' ? 'logoff_user' : action, params: { delay: parseInt(delay) || 0 } }) })
    const body = await response.json()
    if (response.ok) { setMessage(`Task ${body.task_id} queued.`); onRefresh() } else setMessage(body.error?.message ?? 'Failed')
  }
  const actions: { key: typeof action; label: string; desc: string }[] = [
    { key: 'reboot', label: 'Reboot', desc: 'Restart the device immediately or after a delay.' },
    { key: 'shutdown', label: 'Shutdown', desc: 'Power off the device.' },
    { key: 'lock', label: 'Lock Workstation', desc: 'Lock the current user session (Windows only).' },
    { key: 'logoff', label: 'Logoff User', desc: 'Log off the current user session.' },
  ]
  return (
    <div className="rmm-action">
      <h3>System Actions</h3>
      <div className="rmm-system-grid">
        {actions.map(a => (
          <button key={a.key} className={`rmm-system-btn ${action === a.key ? 'active' : ''}`} onClick={() => setAction(a.key)}>
            <strong>{a.label}</strong><small>{a.desc}</small>
          </button>
        ))}
      </div>
      {(action === 'reboot' || action === 'shutdown') && (
        <div className="rmm-delay"><label>Delay (seconds)</label><input type="number" value={delay} onChange={event => setDelay(event.target.value)} min="0" max="86400" /></div>
      )}
      <button className="rmm-danger" onClick={run}>Execute {actions.find(a => a.key === action)?.label}</button>
      {message && <small>{message}</small>}
    </div>
  )
}

function RMMFiles({ deviceID, onRefresh }: { deviceID: string; onRefresh: () => void }) {
  const [action, setAction] = useState<'browse' | 'retrieve' | 'upload' | 'delete' | 'rename'>('browse')
  const [path, setPath] = useState('/')
  const [content, setContent] = useState('')
  const [newPath, setNewPath] = useState('')
  const [browseResult, setBrowseResult] = useState<Array<{ name: string; isDir: boolean; size: number; mode: string; modTime: string }>>([])
  const [message, setMessage] = useState('')

  const browse = async () => {
    const response = await fetch(`${API}/api/v1/devices/${deviceID}/rmm/execute`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ command: `ls ${path}`, command_type: 'browse_filesystem', params: { path } }) })
    const body = await response.json()
    if (response.ok) {
      setMessage(`Task ${body.task_id} queued. Results will appear after agent execution.`)
      setTimeout(() => onRefresh(), 2000)
    } else setMessage(body.error?.message ?? 'Failed')
  }

  const runFileAction = async (event: FormEvent) => {
    event.preventDefault()
    if (!path.trim()) return
    const params: Record<string, unknown> = { path }
    const cmdType: string = action
    let command: string = action
    if (action === 'rename') { params.old_path = path; params.new_path = newPath; command = `mv ${path} ${newPath}` }
    if (action === 'upload') { params.content = content; command = `write ${path}` }
    if (action === 'retrieve') { command = `cat ${path}` }
    if (action === 'delete') { command = `rm ${path}` }
    if (action === 'browse') { params.path = path; command = `ls ${path}` }

    const response = await fetch(`${API}/api/v1/devices/${deviceID}/rmm/execute`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ command, command_type: cmdType, params }) })
    const body = await response.json()
    if (response.ok) { setMessage(`Task ${body.task_id} queued.`); onRefresh() } else setMessage(body.error?.message ?? 'Failed')
  }

  return (
    <div className="rmm-action">
      <h3>File Operations</h3><p>Browse, retrieve, upload, delete, or rename files on the target device.</p>
      <div className="rmm-shell-select">
        {(['browse', 'retrieve', 'upload', 'delete', 'rename'] as const).map(a => <button key={a} className={action === a ? 'active' : ''} onClick={() => setAction(a)}>{a.charAt(0).toUpperCase() + a.slice(1)}</button>)}
      </div>
      <form onSubmit={runFileAction}>
        <input value={path} onChange={event => setPath(event.target.value)} placeholder={action === 'browse' ? 'Directory path (e.g. /home, C:\\Users)' : 'File path'} />
        {action === 'rename' && <input value={newPath} onChange={event => setNewPath(event.target.value)} placeholder="New path" />}
        {action === 'upload' && <textarea value={content} onChange={event => setContent(event.target.value)} placeholder="File content to write" rows={4} />}
        <button>{action === 'browse' ? 'Browse' : action === 'retrieve' ? 'Retrieve' : action === 'upload' ? 'Upload' : action === 'delete' ? 'Delete' : 'Rename'}</button>
      </form>
      {message && <small>{message}</small>}
    </div>
  )
}

createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>)
