import { StrictMode, useEffect, useRef, useState, type FormEvent } from 'react'
import { createRoot } from 'react-dom/client'
import './styles.css'

type View = 'dashboard' | 'devices-all' | 'devices-windows' | 'devices-linux' | 'devices-servers' | 'devices-groups'
  | 'monitoring-metrics' | 'monitoring-alerts' | 'monitoring-services' | 'monitoring-processes'
  | 'remote-terminal' | 'remote-desktop' | 'remote-files'
  | 'software-inventory' | 'software-catalog' | 'software-deployment'
  | 'patches'
  | 'automation-scripts' | 'automation-tasks' | 'automation-scheduler' | 'automation-workflows'
  | 'policies-security' | 'policies-device' | 'policies-software' | 'policies-compliance'
  | 'security-threats' | 'security-vulns' | 'security-antivirus' | 'security-firewall' | 'security-events'
  | 'network-discovery' | 'network-snmp' | 'network-devices'
  | 'reports' | 'audit'
  | 'admin-orgs' | 'admin-sites' | 'admin-users' | 'admin-roles' | 'admin-api' | 'admin-integrations' | 'admin-settings'

type Device = { id: string; hostname: string; platform: string; os_name: string; os_version: string; status: string; agent_version: string; last_seen?: string }
type Metric = { device_id?: string; hostname?: string; status?: string; cpu_percent?: number; memory_used_bytes?: number; memory_total_bytes?: number; collected_at?: string }
type Inventory = { hostname?: string; platform?: string; os_name?: string; architecture?: string; cpu_cores?: number; memory_total_bytes?: number; memory_used_bytes?: number; network?: { name: string; mac: string; ips: string[] }[]; disks?: { name: string; size_bytes?: number; used_bytes?: number; free_bytes?: number; filesystem?: string }[]; software?: string[]; services?: { name: string; status: string; description?: string }[] }
type RMMTask = { id: string; status: string; command_type: string; command?: string; params?: Record<string, unknown>; stdout?: string; stderr?: string; created_at: string; file_name?: string; file_path?: string }
type AuditLog = { id: number; user_id?: string; device_id?: string; action: string; resource: string; metadata?: Record<string, unknown>; created_at: string }
type NavItem = { key: View; label: string; icon: string }
type NavGroup = { title: string; items: NavItem[] }

const API = import.meta.env.VITE_API_URL ?? 'https://app.cyverratech.my.id'
const authHeaders = () => ({ Authorization: `Bearer ${localStorage.getItem('token') ?? ''}` })

const NAV_GROUPS: NavGroup[] = [
  { title: '', items: [{ key: 'dashboard', label: 'Dashboard', icon: '◆' }] },
  { title: 'DEVICES', items: [
    { key: 'devices-all', label: 'All Devices', icon: '●' },
    { key: 'devices-windows', label: 'Windows', icon: '⊞' },
    { key: 'devices-linux', label: 'Linux', icon: '◉' },
    { key: 'devices-servers', label: 'Servers', icon: '▣' },
    { key: 'devices-groups', label: 'Device Groups', icon: '▤' },
  ]},
  { title: 'MONITORING', items: [
    { key: 'monitoring-metrics', label: 'Metrics', icon: '◈' },
    { key: 'monitoring-alerts', label: 'Alerts', icon: '▲' },
    { key: 'monitoring-services', label: 'Services', icon: '◎' },
    { key: 'monitoring-processes', label: 'Processes', icon: '◇' },
  ]},
  { title: 'REMOTE', items: [
    { key: 'remote-terminal', label: 'Terminal', icon: '>' },
    { key: 'remote-desktop', label: 'Desktop', icon: '▢' },
    { key: 'remote-files', label: 'File Manager', icon: '/' },
  ]},
  { title: 'SOFTWARE', items: [
    { key: 'software-inventory', label: 'Inventory', icon: '◻' },
    { key: 'software-catalog', label: 'Catalog', icon: '❐' },
    { key: 'software-deployment', label: 'Deployment', icon: '▸' },
  ]},
  { title: 'PATCH MGMT', items: [{ key: 'patches', label: 'Patches', icon: '⊞' }] },
  { title: 'AUTOMATION', items: [
    { key: 'automation-scripts', label: 'Scripts', icon: '//' },
    { key: 'automation-tasks', label: 'Tasks', icon: '☑' },
    { key: 'automation-scheduler', label: 'Scheduler', icon: '◷' },
    { key: 'automation-workflows', label: 'Workflows', icon: ' ~> ' },
  ]},
  { title: 'POLICIES', items: [
    { key: 'policies-security', label: 'Security', icon: '△' },
    { key: 'policies-device', label: 'Device', icon: '◇' },
    { key: 'policies-software', label: 'Software', icon: '◻' },
    { key: 'policies-compliance', label: 'Compliance', icon: '✓' },
  ]},
  { title: 'SECURITY', items: [
    { key: 'security-threats', label: 'Threats', icon: '▲' },
    { key: 'security-vulns', label: 'Vulnerabilities', icon: '⚠' },
    { key: 'security-antivirus', label: 'Antivirus', icon: '⛊' },
    { key: 'security-firewall', label: 'Firewall', icon: '⊟' },
    { key: 'security-events', label: 'Security Events', icon: '◈' },
  ]},
  { title: 'NETWORK', items: [
    { key: 'network-discovery', label: 'Discovery', icon: '◎' },
    { key: 'network-snmp', label: 'SNMP', icon: '◇' },
    { key: 'network-devices', label: 'Network Devices', icon: '▣' },
  ]},
  { title: '', items: [
    { key: 'reports', label: 'Reports', icon: '▤' },
    { key: 'audit', label: 'Audit Logs', icon: '▧' },
  ]},
  { title: 'ADMINISTRATION', items: [
    { key: 'admin-orgs', label: 'Organizations', icon: '◇' },
    { key: 'admin-sites', label: 'Sites', icon: '◎' },
    { key: 'admin-users', label: 'Users', icon: '●' },
    { key: 'admin-roles', label: 'Roles', icon: '◀' },
    { key: 'admin-api', label: 'API', icon: '{ }' },
    { key: 'admin-integrations', label: 'Integrations', icon: '⟡' },
    { key: 'admin-settings', label: 'System Settings', icon: '⚙' },
  ]},
]

function App() {
  const [view, setView] = useState<View>('dashboard')
  const [devices, setDevices] = useState<Device[]>([])
  const [metrics, setMetrics] = useState<Metric[]>([])
  const [selected, setSelected] = useState<Device | null>(null)
  const [inventory, setInventory] = useState<Inventory | null>(null)
  const [deviceMetrics, setDeviceMetrics] = useState<Metric[]>([])
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const [authenticated, setAuthenticated] = useState(!!localStorage.getItem('token'))
  const [sidebarOpen, setSidebarOpen] = useState<Record<string, boolean>>({})

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
  useEffect(() => { if (!authenticated) return; const t = window.setInterval(() => { void loadDevices(); void loadMetrics() }, 30000); return () => window.clearInterval(t) }, [authenticated])

  const createToken = async () => {
    setError('')
    const response = await fetch(`${API}/api/v1/enrollment-tokens`, { method: 'POST', headers: authHeaders() })
    const body = await response.json()
    if (response.ok) { setToken(body.token); setView('devices-all') } else setError(body.error?.message ?? 'Unable to create enrollment token')
  }
  const showDevice = async (device: Device) => {
    setSelected(device); setInventory(null); setDeviceMetrics([]); setView('devices-all')
    const r1 = await fetch(`${API}/api/v1/devices/${device.id}/inventory`, { headers: authHeaders() })
    if (r1.ok) setInventory((await r1.json()).data)
    const r2 = await fetch(`${API}/api/v1/devices/${device.id}/metrics`, { headers: authHeaders() })
    if (r2.ok) setDeviceMetrics((await r2.json()).data)
  }

  const toggleGroup = (title: string) => setSidebarOpen(prev => ({ ...prev, [title]: !prev[title] }))

  if (!authenticated) return <Login onSuccess={() => setAuthenticated(true)} />
  const online = devices.filter(d => d.status === 'ONLINE').length
  const pageTitle = view.split('-').map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' ')
  const parentPage = view.split('-')[0]

  return (
    <main>
      <aside>
        <div className="brand"><span className="mark">C</span><span>CYVERRA <b>NEXUS</b></span></div>
        <nav>
          {NAV_GROUPS.map((group, gi) => (
            <div key={gi} className="nav-group">
              {group.title && <button className="nav-group-title" onClick={() => toggleGroup(group.title)}>{group.title}<span>{sidebarOpen[group.title] ? '▾' : '▸'}</span></button>}
              {(!group.title || sidebarOpen[group.title]) && group.items.map(item => (
                <button key={item.key} className={view === item.key ? 'active' : ''} onClick={() => { setView(item.key); setError('') }}>
                  <span className="nav-icon">{item.icon}</span>{item.label}
                  {item.key === 'devices-all' && <small>{devices.length}</small>}
                </button>
              ))}
            </div>
          ))}
        </nav>
        <div className="tenant"><span className="avatar">CN</span><div><strong>Control room</strong><small>Organization workspace</small></div></div>
      </aside>
      <section className="content">
        <header>
          <div>
            <p className="eyebrow">CYVERRA NEXUS / {parentPage.toUpperCase()}</p>
            <h1>{pageTitle}</h1>
            <p className="subhead">{getPageDescription(view)}</p>
          </div>
          {(view === 'dashboard' || view === 'devices-all') && <button onClick={createToken}>+ Add device</button>}
        </header>
        {error && <div className="notice">{error}</div>}
        {renderContent(view, { devices, metrics, online, selected, inventory, deviceMetrics, token, showDevice, createToken, setError })}
      </section>
    </main>
  )
}

function getPageDescription(view: View): string {
  const descriptions: Record<string, string> = {
    'dashboard': 'Fleet command center — overview of all endpoints and systems.',
    'devices-all': 'Manage all enrolled devices across your organization.',
    'devices-windows': 'Windows endpoints and servers.',
    'devices-linux': 'Linux endpoints and servers.',
    'devices-servers': 'Server infrastructure overview.',
    'devices-groups': 'Organize devices into logical groups.',
    'monitoring-metrics': 'Live CPU, memory, and disk metrics from agents.',
    'monitoring-alerts': 'Alert rules and triggered notifications.',
    'monitoring-services': 'Service health monitoring across endpoints.',
    'monitoring-processes': 'Process monitoring and resource usage.',
    'remote-terminal': 'Execute commands via PowerShell, CMD, or Bash.',
    'remote-desktop': 'WebRTC remote desktop sessions.',
    'remote-files': 'Browse, upload, and manage files on endpoints.',
    'software-inventory': 'Installed software across all devices.',
    'software-catalog': 'Software catalog and repository.',
    'software-deployment': 'Push software to devices.',
    'patches': 'Patch management and update tracking.',
    'automation-scripts': 'Reusable script library.',
    'automation-tasks': 'Task definitions and execution history.',
    'automation-scheduler': 'Schedule recurring operations.',
    'automation-workflows': 'Multi-step automation workflows.',
    'policies-security': 'Security policies and enforcement.',
    'policies-device': 'Device configuration policies.',
    'policies-software': 'Software installation policies.',
    'policies-compliance': 'Compliance status and reporting.',
    'security-threats': 'Detected threats and mitigation.',
    'security-vulns': 'Vulnerability scanning results.',
    'security-antivirus': 'Antivirus status and management.',
    'security-firewall': 'Firewall rules and status.',
    'security-events': 'Security event log.',
    'network-discovery': 'Network device discovery.',
    'network-snmp': 'SNMP monitoring configuration.',
    'network-devices': 'Network infrastructure inventory.',
    'reports': 'Generate and view reports.',
    'audit': 'Complete audit trail of all operations.',
    'admin-orgs': 'Organization management.',
    'admin-sites': 'Site and location management.',
    'admin-users': 'User account management.',
    'admin-roles': 'Role-based access control.',
    'admin-api': 'API keys and access management.',
    'admin-integrations': 'Third-party integrations.',
    'admin-settings': 'System configuration and settings.',
  }
  return descriptions[view] || ''
}

function renderContent(view: View, ctx: { devices: Device[]; metrics: Metric[]; online: number; selected: Device | null; inventory: Inventory | null; deviceMetrics: Metric[]; token: string; showDevice: (d: Device) => void; createToken: () => void; setError: (e: string) => void }) {
  switch (view) {
    case 'dashboard': return <DashboardPage {...ctx} />
    case 'devices-all': return <DevicesAllPage {...ctx} />
    case 'devices-windows': return <DevicesFilterPage {...ctx} filter="windows" />
    case 'devices-linux': return <DevicesFilterPage {...ctx} filter="linux" />
    case 'devices-servers': return <DevicesFilterPage {...ctx} filter="server" />
    case 'devices-groups': return <ComingSoon module="Device Groups" />
    case 'monitoring-metrics': return <MonitoringMetricsPage {...ctx} />
    case 'monitoring-alerts': return <ComingSoon module="Alerts" />
    case 'monitoring-services': return <MonitoringServicesPage {...ctx} />
    case 'monitoring-processes': return <ComingSoon module="Processes" />
    case 'remote-terminal': return <RemoteTerminalPage {...ctx} />
    case 'remote-desktop': return <RemoteDesktopPage {...ctx} />
    case 'remote-files': return <RemoteFilesPage {...ctx} />
    case 'software-inventory': return <ComingSoon module="Software Inventory" />
    case 'software-catalog': return <ComingSoon module="Software Catalog" />
    case 'software-deployment': return <ComingSoon module="Software Deployment" />
    case 'patches': return <ComingSoon module="Patch Management" />
    case 'automation-scripts': return <ComingSoon module="Scripts" />
    case 'automation-tasks': return <ComingSoon module="Tasks" />
    case 'automation-scheduler': return <ComingSoon module="Scheduler" />
    case 'automation-workflows': return <ComingSoon module="Workflows" />
    case 'policies-security': return <ComingSoon module="Security Policies" />
    case 'policies-device': return <ComingSoon module="Device Policies" />
    case 'policies-software': return <ComingSoon module="Software Policies" />
    case 'policies-compliance': return <ComingSoon module="Compliance" />
    case 'security-threats': return <ComingSoon module="Threats" />
    case 'security-vulns': return <ComingSoon module="Vulnerabilities" />
    case 'security-antivirus': return <ComingSoon module="Antivirus" />
    case 'security-firewall': return <ComingSoon module="Firewall" />
    case 'security-events': return <ComingSoon module="Security Events" />
    case 'network-discovery': return <ComingSoon module="Network Discovery" />
    case 'network-snmp': return <ComingSoon module="SNMP" />
    case 'network-devices': return <ComingSoon module="Network Devices" />
    case 'reports': return <ComingSoon module="Reports" />
    case 'audit': return <AuditPage />
    case 'admin-orgs': return <ComingSoon module="Organizations" />
    case 'admin-sites': return <ComingSoon module="Sites" />
    case 'admin-users': return <ComingSoon module="Users" />
    case 'admin-roles': return <ComingSoon module="Roles" />
    case 'admin-api': return <ComingSoon module="API Management" />
    case 'admin-integrations': return <ComingSoon module="Integrations" />
    case 'admin-settings': return <ComingSoon module="System Settings" />
    default: return <ComingSoon module="Module" />
  }
}

// ── Dashboard ──
function DashboardPage({ devices, online, metrics, showDevice, createToken }: { devices: Device[]; online: number; metrics: Metric[]; showDevice: (d: Device) => void; createToken: () => void }) {
  return <><div className="stats">
    <Stat label="Total devices" value={devices.length} tone="ink" />
    <Stat label="Online now" value={online} tone="teal" />
    <Stat label="Offline" value={devices.filter(d => d.status === 'OFFLINE').length} tone="coral" />
    <Stat label="Metric streams" value={metrics.filter(m => m.collected_at).length} tone="gold" />
  </div>
  <div className="workspace">
    <DeviceTable devices={devices} onSelect={showDevice} onAdd={createToken} />
    <div className="panel activity">
      <div className="panel-head"><div><p className="eyebrow">SYSTEM PULSE</p><h2>Live signal</h2></div></div>
      <div className="pulse">
        <Pulse label="Heartbeat" text={`${online} endpoint${online === 1 ? '' : 's'} online`} tone="online" />
        <Pulse label="Metrics" text={`${metrics.filter(m => m.collected_at).length} reporting`} tone="gold" />
        <Pulse label="Alerts" text="No rules configured" tone="coral" />
      </div>
    </div>
  </div></>
}

// ── Devices ──
function DevicesAllPage({ devices, token, selected, inventory, deviceMetrics, showDevice, createToken }: any) {
  return <DevicesLayout devices={devices} token={token} selected={selected} inventory={inventory} deviceMetrics={deviceMetrics} onAdd={createToken} onSelect={showDevice} />
}
function DevicesFilterPage({ devices, showDevice, createToken, filter }: any) {
  const filtered = devices.filter((d: Device) => {
    if (filter === 'windows') return d.platform === 'windows'
    if (filter === 'linux') return d.platform === 'linux'
    return true
  })
  return <div className="panel"><div className="panel-head"><div><p className="eyebrow">FILTERED</p><h2>{filter.charAt(0).toUpperCase() + filter.slice(1)} devices ({filtered.length})</h2></div></div>{filtered.length ? <DeviceTable devices={filtered} onSelect={showDevice} onAdd={createToken} /> : <p className="muted">No {filter} devices found.</p>}</div>
}
function DevicesLayout({ devices, token, selected, inventory, deviceMetrics, onAdd, onSelect }: any) {
  return <div className="device-layout">
    <div className="panel">
      <div className="panel-head"><div><p className="eyebrow">INVENTORY</p><h2>Managed devices</h2></div><button className="quiet" onClick={onAdd}>+ Token</button></div>
      <AgentDownloads />
      {token && <Enrollment token={token} />}
      {devices.length ? <DeviceTable devices={devices} onSelect={onSelect} onAdd={onAdd} /> : <Empty onAdd={onAdd} />}
    </div>
    {selected && <div className="panel detail">
      <div className="panel-head"><div><p className="eyebrow">DEVICE DETAIL</p><h2>{selected.hostname}</h2></div><span className="status"><i className={`dot ${selected.status.toLowerCase()}`} />{selected.status}</span></div>
      <div className="detail-grid">
        <Info label="Platform" value={`${selected.os_name || selected.platform} ${selected.os_version || ''}`} />
        <Info label="Agent" value={selected.agent_version} />
        <Info label="Last seen" value={selected.last_seen ? new Date(selected.last_seen).toLocaleString() : 'Never'} />
        <Info label="Architecture" value={inventory?.architecture || 'Collecting'} />
      </div>
      <MetricHistory metrics={deviceMetrics} />
      {inventory && <div className="inventory">
        <h3>Hardware</h3><p>{inventory.cpu_cores ?? '--'} CPU cores · {formatBytes(inventory.memory_used_bytes)} / {formatBytes(inventory.memory_total_bytes)} RAM</p>
        <h3>Disks</h3>{inventory.disks?.map((d: any) => <div className="network" key={d.name}><strong>{d.name}</strong><span>{formatBytes(d.used_bytes)} / {formatBytes(d.size_bytes)}</span></div>)}
        <h3>Services</h3>{inventory.services?.slice(0, 20).map((s: any) => <div className="network" key={s.name}><strong>{s.name}</strong><span>{s.status}</span></div>)}
        <h3>Software</h3>{inventory.software?.length ? <div className="software-list">{inventory.software.slice(0, 30).map((s: string) => <span key={s}>{s}</span>)}</div> : <p className="muted">No data</p>}
      </div>}
    </div>}
  </div>
}

// ── Monitoring ──
function MonitoringMetricsPage({ devices, metrics, showDevice }: { devices: Device[]; metrics: Metric[]; showDevice: (d: Device) => void }) {
  return <div className="panel"><div className="panel-head"><div><p className="eyebrow">LIVE TELEMETRY</p><h2>Endpoint metrics</h2></div><span className="muted">Auto-refresh 30s</span></div>
    <div className="table"><div className="row heading"><span>Device</span><span>CPU</span><span>Memory</span><span>Collected</span></div>
      {devices.map(d => {
        const m = metrics.find(i => i.device_id === d.id)
        return <button className="row clickable" key={d.id} onClick={() => showDevice(d)}>
          <span><strong>{d.hostname}</strong><small>{d.status}</small></span>
          <span>{m?.cpu_percent != null ? `${m.cpu_percent.toFixed(1)}%` : '--'}</span>
          <span>{m?.memory_total_bytes ? `${Math.round((m.memory_used_bytes ?? 0) / m.memory_total_bytes * 100)}%` : '--'}</span>
          <span>{m?.collected_at ? new Date(m.collected_at).toLocaleTimeString() : 'Waiting'}</span>
        </button>
      })}
    </div></div>
}
function MonitoringServicesPage({ devices }: { devices: Device[] }) {
  return <div className="panel"><div className="panel-head"><div><p className="eyebrow">SERVICE STATUS</p><h2>Service health</h2></div></div>
    <p className="muted" style={{ padding: 22 }}>Service data is collected from agent inventory. Select a device to view services.</p>
    <div className="table"><div className="row heading"><span>Device</span><span>Platform</span><span>Status</span></div>
      {devices.filter(d => d.status === 'ONLINE').map(d => <div className="row" key={d.id}>
        <span><strong>{d.hostname}</strong></span><span>{d.platform}</span><span><i className="dot online" />Online</span>
      </div>)}
    </div></div>
}

// ── Remote ──
function RemoteTerminalPage({ devices, selected, showDevice }: any) {
  const [deviceId, setDeviceId] = useState(selected?.id ?? '')
  const [shell, setShell] = useState<'powershell' | 'cmd' | 'bash'>('powershell')
  const [command, setCommand] = useState('')
  const [tasks, setTasks] = useState<RMMTask[]>([])
  const [message, setMessage] = useState('')
  const loadTasks = async (id: string) => { if (!id) return; const r = await fetch(`${API}/api/v1/devices/${id}/rmm/tasks`, { headers: authHeaders() }); if (r.ok) setTasks((await r.json()).data) }
  useEffect(() => { if (deviceId) void loadTasks(deviceId) }, [deviceId])
  const run = async (e: FormEvent) => { e.preventDefault(); if (!command.trim()) return; const r = await fetch(`${API}/api/v1/devices/${deviceId}/rmm/execute`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ command, command_type: shell }) }); const b = await r.json(); if (r.ok) { setMessage(`Task ${b.task_id} queued`); setCommand(''); loadTasks(deviceId) } else setMessage(b.error?.message ?? 'Failed') }
  return <div className="device-layout">
    <div className="panel">
      <div className="panel-head"><div><p className="eyebrow">REMOTE TERMINAL</p><h2>Execute commands</h2></div></div>
      <label className="rmm-device-select"><span>Target device</span>
        <select value={deviceId} onChange={e => setDeviceId(e.target.value)}><option value="">-- Select --</option>{devices.filter((d: Device) => d.status === 'ONLINE').map((d: Device) => <option key={d.id} value={d.id}>{d.hostname}</option>)}</select>
      </label>
      <div className="rmm-action">
        <div className="rmm-shell-select">{(['powershell', 'cmd', 'bash'] as const).map(s => <button key={s} className={shell === s ? 'active' : ''} onClick={() => setShell(s)}>{s}</button>)}</div>
        <form onSubmit={run}><input value={command} onChange={e => setCommand(e.target.value)} placeholder={shell === 'powershell' ? 'Get-Service' : shell === 'cmd' ? 'dir' : 'ls -la'} maxLength={4096} /><button>Execute</button></form>
        {message && <small>{message}</small>}
      </div>
    </div>
    <div className="panel"><div className="panel-head"><div><p className="eyebrow">HISTORY</p><h2>Recent commands</h2></div></div>
      {tasks.length === 0 ? <p className="muted" style={{ padding: 22 }}>No tasks yet.</p> : <div className="table"><div className="row heading"><span>Type</span><span>Status</span><span>Output</span><span>Time</span></div>
        {tasks.map(t => <div className="row" key={t.id}><span><strong>{t.command_type}</strong></span><span><i className={`dot ${t.status === 'SUCCESS' ? 'online' : t.status === 'FAILED' ? 'coral' : 'gold'}`} />{t.status}</span><span className="rmm-output">{t.stdout?.slice(0, 60) || t.stderr?.slice(0, 60) || '--'}</span><span>{new Date(t.created_at).toLocaleTimeString()}</span></div>)}
      </div>}
    </div>
  </div>
}
function RemoteDesktopPage({ devices, selected }: any) {
  const [tool, setTool] = useState<'remote' | 'chat' | 'sysinfo' | 'wol' | 'shutdown' | 'sysmgr' | 'announce'>('remote')
  const [tab, setTab] = useState<'computers' | 'history' | 'unattended' | 'settings' | 'recording' | 'performance' | 'confirmation'>('computers')
  const [viewer, setViewer] = useState<'html5' | 'activex'>('html5')
  const [protocol, setProtocol] = useState<'WEBRTC' | 'VNC' | 'RDP'>('WEBRTC')
  const [filterPlatform, setFilterPlatform] = useState('all')
  const [filterStatus, setFilterStatus] = useState('all')
  const [search, setSearch] = useState('')
  const [sessions, setSessions] = useState<any[]>([])
  const [session, setSession] = useState<any>(null)
  const [msg, setMsg] = useState('')
  const [page, setPage] = useState(1)
  const [connectDevice, setConnectDevice] = useState<string | null>(null)
  const [connectMode, setConnectMode] = useState<'manual' | 'unattended'>('manual')
  const [unattendedToken, setUnattendedToken] = useState('')
  const [rdpUser, setRdpUser] = useState('')
  const [rdpPass, setRdpPass] = useState('')
  const [vncPass, setVncPass] = useState('')
  const [unattendedDevices, setUnattendedDevices] = useState<any[]>([])
  const [generatedToken, setGeneratedToken] = useState('')
  const [tokenDialog, setTokenDialog] = useState<string | null>(null)
  const [viewerWs, setViewerWs] = useState<WebSocket | null>(null)
  const [viewerFrame, setViewerFrame] = useState('')
  const [viewerActive, setViewerActive] = useState(false)
  const [viewerScale, setViewerScale] = useState(1)
  const viewerCanvasRef = useRef<HTMLCanvasElement>(null)
  const perPage = 25

  const onlineDevices = devices.filter((d: Device) => d.status === 'ONLINE')
  const filtered = onlineDevices.filter((d: Device) => {
    if (filterPlatform !== 'all' && d.platform !== filterPlatform) return false
    if (search && !d.hostname.toLowerCase().includes(search.toLowerCase())) return false
    return true
  })
  const totalPages = Math.ceil(filtered.length / perPage)
  const paged = filtered.slice((page - 1) * perPage, page * perPage)

  useEffect(() => {
    if (tab === 'history') {
      (async () => {
        const all: any[] = []
        for (const d of devices) {
          const r = await fetch(`${API}/api/v1/devices/${d.id}/remote-sessions`, { headers: authHeaders() })
          if (r.ok) { const b = await r.json(); (b.data || []).forEach((s: any) => all.push({ ...s, hostname: d.hostname, device_id: d.id })) }
        }
        setSessions(all.sort((a: any, b: any) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()))
      })()
    }
    if (tab === 'unattended') { loadUnattendedDevices() }
  }, [tab, devices])

  const loadUnattendedDevices = async () => {
    const r = await fetch(`${API}/api/v1/devices/unattended`, { headers: authHeaders() })
    if (r.ok) setUnattendedDevices((await r.json()).data)
  }

  const generateToken = async (deviceId: string) => {
    const r = await fetch(`${API}/api/v1/devices/${deviceId}/unattended/token`, { method: 'POST', headers: authHeaders() })
    const b = await r.json()
    if (r.ok) { setGeneratedToken(b.token); setTokenDialog(deviceId); setMsg('Token generated. Copy and save it securely.') }
    else setMsg(b.error?.message ?? 'Failed')
  }

  const revokeToken = async (deviceId: string) => {
    if (!confirm('Revoke unattended access token? The device will lose unattended access.')) return
    const r = await fetch(`${API}/api/v1/devices/${deviceId}/unattended/token`, { method: 'DELETE', headers: authHeaders() })
    if (r.ok) { setMsg('Token revoked'); loadUnattendedDevices() } else setMsg('Failed to revoke token')
  }

  const toggleUnattended = async (deviceId: string, enabled: boolean, port: number, proto: string) => {
    const r = await fetch(`${API}/api/v1/devices/${deviceId}/unattended`, { method: 'PUT', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ enabled, port, protocol: proto }) })
    if (r.ok) { setMsg(`Unattended access ${enabled ? 'enabled' : 'disabled'}`); loadUnattendedDevices() }
  }

  const connectWithToken = async () => {
    if (!unattendedToken || !connectDevice) return
    const r = await fetch(`${API}/api/v1/unattended/validate`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ token: unattendedToken, device_id: connectDevice }) })
    const b = await r.json()
    if (r.ok) {
      const body: Record<string, unknown> = { protocol: b.protocol }
      if (b.protocol === 'RDP') { body.rdp_username = rdpUser; body.rdp_password = rdpPass; body.rdp_port = b.port }
      if (b.protocol === 'VNC') { body.vnc_password = vncPass; body.vnc_port = b.port }
      const sr = await fetch(`${API}/api/v1/devices/${b.device_id}/remote-sessions`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      const sb = await sr.json()
      if (sr.ok) {
        setSession({ ...sb, device_id: b.device_id })
        setMsg(`Unattended: connected to ${b.hostname} via ${b.protocol} (${b.ip}:${b.port})`)
        setConnectDevice(null); setUnattendedToken('')
      } else setMsg(sb.error?.message ?? 'Session failed')
    } else setMsg(b.error?.message ?? 'Invalid token')
  }

  const openConnectDialog = (deviceId: string, mode: 'manual' | 'unattended' = 'manual') => {
    setConnectMode(mode)
    if (mode === 'unattended') {
      setConnectDevice(deviceId); setUnattendedToken(''); setRdpUser(''); setRdpPass(''); setVncPass('')
      return
    }
    if (protocol === 'WEBRTC') { connect(deviceId); return }
    setConnectDevice(deviceId); setRdpUser(''); setRdpPass(''); setVncPass('')
  }

  const connect = async (deviceId: string) => {
    const body: Record<string, unknown> = { protocol }
    if (protocol === 'RDP') { body.rdp_username = rdpUser; body.rdp_password = rdpPass; body.rdp_port = 3389 }
    if (protocol === 'VNC') { body.vnc_password = vncPass; body.vnc_port = 5900 }
    const r = await fetch(`${API}/api/v1/devices/${deviceId}/remote-sessions`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    const b = await r.json()
    if (r.ok) {
      const host = devices.find((d: Device) => d.id === deviceId)?.hostname
      setConnectDevice(null)
      openViewer(deviceId, b.session_id, host || deviceId)
    } else setMsg(b.error?.message ?? 'Connection failed')
  }

  const openViewer = (deviceId: string, sessionId: string, hostname: string) => {
    const wsBase = API.replace('https://', 'wss://').replace('http://', 'ws://')
    const wsUrl = `${wsBase}/api/v1/remote-sessions/${sessionId}/ws`
    const token = localStorage.getItem('token') || ''
    const ws = new WebSocket(wsUrl)
    ws.onopen = () => {
      setSession({ session_id: sessionId, device_id: deviceId, protocol, hostname })
      setViewerActive(true)
      setMsg(`Connected to ${hostname}`)
    }
    ws.onmessage = (ev) => {
      if (ev.data instanceof Blob) {
        const reader = new FileReader()
        reader.onload = () => setViewerFrame(reader.result as string)
        reader.readAsDataURL(ev.data)
      } else {
        try {
          const msg = JSON.parse(ev.data)
          if (msg.type === 'error') setMsg(msg.message)
        } catch {}
      }
    }
    ws.onclose = () => { setViewerActive(false); setViewerFrame('') }
    ws.onerror = () => { setMsg('WebSocket connection failed'); setViewerActive(false) }
    setViewerWs(ws)
  }
  const closeSession = async () => {
    if (viewerWs) { viewerWs.close(); setViewerWs(null) }
    if (session) {
      await fetch(`${API}/api/v1/remote-sessions/${session.session_id}/close`, { method: 'POST', headers: { ...authHeaders(), 'Content-Type': 'application/json' }, body: JSON.stringify({ reason: 'operator closed' }) })
    }
    setSession(null); setViewerActive(false); setViewerFrame(''); setMsg('Session closed')
  }

  const toolItems = [
    { key: 'remote' as const, label: 'Remote Control', icon: '⊞' },
    { key: 'chat' as const, label: 'Chat', icon: '○' },
    { key: 'sysinfo' as const, label: 'System Tools', icon: '◇' },
    { key: 'wol' as const, label: 'Wake on LAN', icon: '◈' },
    { key: 'shutdown' as const, label: 'Remote Shutdown', icon: '⊠' },
    { key: 'sysmgr' as const, label: 'System Manager', icon: '▣' },
    { key: 'announce' as const, label: 'Announcement', icon: '◎' },
  ]

  const tabs = [
    { key: 'computers' as const, label: 'Computers' },
    { key: 'history' as const, label: 'History' },
    { key: 'unattended' as const, label: 'Unattended Access' },
    { key: 'settings' as const, label: 'Settings' },
    { key: 'recording' as const, label: 'Screen Recording' },
    { key: 'performance' as const, label: 'Performance' },
    { key: 'confirmation' as const, label: 'User Confirmation' },
  ]

  const connectDeviceName = connectDevice ? devices.find((d: Device) => d.id === connectDevice)?.hostname : ''

  return (
    <div className="rdp-layout">
      <div className="rdp-sidebar">
        <div className="rdp-sidebar-head"><p className="eyebrow">TOOLS</p></div>
        {toolItems.map(t => (
          <button key={t.key} className={tool === t.key ? 'active' : ''} onClick={() => setTool(t.key)}>
            <span className="rdp-tool-icon">{t.icon}</span>{t.label}
          </button>
        ))}
        <div className="rdp-sidebar-foot"><span>Need Additional Tools?</span></div>
      </div>
      <div className="rdp-main">
        {viewerActive && (
          <div className="rdp-viewer-container">
            <div className="rdp-viewer-toolbar">
              <span className="rdp-viewer-title">{session?.hostname || 'Remote Desktop'}</span>
              <div className="rdp-viewer-controls">
                <button className="rdp-btn rdp-btn-secondary" onClick={() => setViewerScale(s => Math.max(0.25, s - 0.25))}>-</button>
                <span className="rdp-viewer-scale">{Math.round(viewerScale * 100)}%</span>
                <button className="rdp-btn rdp-btn-secondary" onClick={() => setViewerScale(s => Math.min(2, s + 0.25))}>+</button>
                <button className="rdp-btn rdp-btn-secondary" onClick={() => setViewerScale(1)}>Fit</button>
                <button className="rdp-btn rdp-btn-danger-sm" onClick={closeSession}>Disconnect</button>
              </div>
            </div>
            <div className="rdp-viewer-screen">
              {viewerFrame ? (
                <img
                  src={viewerFrame}
                  className="rdp-viewer-canvas"
                  style={{ transform: `scale(${viewerScale})`, transformOrigin: 'top left' }}
                  draggable={false}
                />
              ) : (
                <div className="rdp-viewer-waiting">
                  <div className="rdp-viewer-spinner"></div>
                  <p>Waiting for agent screen...</p>
                </div>
              )}
            </div>
          </div>
        )}
        {tool === 'remote' && (
          <>
            <div className="rdp-info-bar">
              <p>Remote Control helps you to gain access to remote computer. This requires Cyverra Agent to be installed in client systems. Supports WebRTC, VNC, and RDP protocols.</p>
              <a href="#">Prerequisites</a> | <a href="#">Need more Features?</a>
            </div>
            <div className="rdp-tabs">
              {tabs.map(t => <button key={t.key} className={tab === t.key ? 'active' : ''} onClick={() => setTab(t.key)}>{t.label}</button>)}
            </div>
            {tab === 'computers' && (
              <div className="rdp-content">
                <div className="rdp-toolbar">
                  <div className="rdp-viewer-select">
                    <label><input type="radio" name="viewer" checked={viewer === 'html5'} onChange={() => setViewer('html5')} /> HTML5 Viewer</label>
                    <label><input type="radio" name="viewer" checked={viewer === 'activex'} onChange={() => setViewer('activex')} /> WebRTC</label>
                  </div>
                  <div className="rdp-protocol-select">
                    <span className="rdp-proto-label">Protocol:</span>
                    {(['WEBRTC', 'VNC', 'RDP'] as const).map(p => (
                      <button key={p} className={`rdp-proto-btn ${protocol === p ? 'active' : ''}`} onClick={() => setProtocol(p)}>{p}</button>
                    ))}
                  </div>
                  <div className="rdp-filters">
                    <select value={filterStatus} onChange={e => { setFilterStatus(e.target.value); setPage(1) }}>
                      <option value="all">Agent Live Status</option>
                      <option value="online">Online</option>
                      <option value="offline">Offline</option>
                    </select>
                    <select value={filterPlatform} onChange={e => { setFilterPlatform(e.target.value); setPage(1) }}>
                      <option value="all">Platform</option>
                      <option value="windows">Windows</option>
                      <option value="linux">Linux</option>
                    </select>
                    <input type="text" className="rdp-search" placeholder="Search hostname..." value={search} onChange={e => { setSearch(e.target.value); setPage(1) }} />
                    <button className="rdp-refresh" onClick={() => setPage(1)}>⟳</button>
                  </div>
                  <span className="rdp-total">Total: {filtered.length}</span>
                </div>
                <div className="rdp-table">
                  <div className="rdp-row rdp-heading">
                    <span className="rdp-col-icon"></span>
                    <span className="rdp-col-status"></span>
                    <span className="rdp-col-name">Computer Name</span>
                    <span className="rdp-col-user">User Name</span>
                    <span className="rdp-col-ip">IP Address</span>
                    <span className="rdp-col-action">Action</span>
                    <span className="rdp-col-remarks">Remarks</span>
                    <span className="rdp-col-platform">OS Platform</span>
                  </div>
                  {paged.length === 0 ? (
                    <div className="rdp-empty">No devices available for remote control.</div>
                  ) : paged.map((d: Device) => (
                    <div className="rdp-row" key={d.id}>
                      <span className="rdp-col-icon"><span className="rdp-pc-icon">⊞</span></span>
                      <span className="rdp-col-status"><i className={`dot ${d.status.toLowerCase()}`} /></span>
                      <span className="rdp-col-name"><strong>{d.hostname}</strong></span>
                      <span className="rdp-col-user">--</span>
                      <span className="rdp-col-ip">--</span>
                      <span className="rdp-col-action">
                        {session?.device_id === d.id ? (
                          <button className="rdp-btn rdp-btn-disconnect" onClick={closeSession}>Disconnect</button>
                        ) : (
                          <button className="rdp-btn rdp-btn-connect" onClick={() => openConnectDialog(d.id)}>Connect</button>
                        )}
                      </span>
                      <span className="rdp-col-remarks">--</span>
                      <span className="rdp-col-platform"><span className="rdp-platform-icon">{d.platform === 'windows' ? '⊞' : '◉'}</span></span>
                    </div>
                  ))}
                </div>
                {totalPages > 1 && (
                  <div className="rdp-pagination">
                    <span>{(page - 1) * perPage + 1} - {Math.min(page * perPage, filtered.length)} of {filtered.length}</span>
                    <select value={perPage} disabled><option>25</option></select>
                    <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>‹</button>
                    <button disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>›</button>
                  </div>
                )}
              </div>
            )}
            {tab === 'history' && (
              <div className="rdp-content">
                <div className="rdp-table">
                  <div className="rdp-row rdp-heading">
                    <span className="rdp-col-name">Device</span>
                    <span className="rdp-col-status">Status</span>
                    <span className="rdp-col-ip">Protocol</span>
                    <span className="rdp-col-action">Started</span>
                    <span className="rdp-col-remarks">Closed</span>
                    <span className="rdp-col-platform">Reason</span>
                  </div>
                  {sessions.length === 0 ? <div className="rdp-empty">No remote sessions yet.</div> : sessions.map((s: any) => (
                    <div className="rdp-row" key={s.id}>
                      <span className="rdp-col-name"><strong>{s.hostname}</strong></span>
                      <span className="rdp-col-status"><i className={`dot ${s.status === 'CLOSED' ? 'online' : s.status === 'ACTIVE' ? 'teal' : 'gold'}`} />{s.status}</span>
                      <span className="rdp-col-ip">{s.protocol}</span>
                      <span className="rdp-col-action">{s.started_at ? new Date(s.started_at).toLocaleString() : '--'}</span>
                      <span className="rdp-col-remarks">{s.closed_at ? new Date(s.closed_at).toLocaleString() : '--'}</span>
                      <span className="rdp-col-platform">{s.close_reason || '--'}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
            {tab === 'unattended' && (
              <div className="rdp-content">
                <div className="rdp-unattended-info">
                  <p>Unattended access allows secure remote connections using API tokens instead of passwords. Generate a token per device, then use it to connect without user interaction.</p>
                </div>
                <div className="rdp-table">
                  <div className="rdp-row rdp-heading">
                    <span className="rdp-col-icon"></span>
                    <span className="rdp-col-status"></span>
                    <span className="rdp-col-name">Device</span>
                    <span className="rdp-col-user">Protocol</span>
                    <span className="rdp-col-ip">Port</span>
                    <span className="rdp-col-action">Actions</span>
                    <span className="rdp-col-remarks">Last Used</span>
                    <span className="rdp-col-platform">Status</span>
                  </div>
                  {devices.filter((d: Device) => d.status === 'ONLINE').map((d: Device) => {
                    const ua = unattendedDevices.find((u: any) => u.id === d.id)
                    return (
                      <div className="rdp-row" key={d.id}>
                        <span className="rdp-col-icon"><span className="rdp-pc-icon">⊞</span></span>
                        <span className="rdp-col-status"><i className={`dot ${d.status.toLowerCase()}`} /></span>
                        <span className="rdp-col-name"><strong>{d.hostname}</strong></span>
                        <span className="rdp-col-user">{ua?.protocol || 'VNC'}</span>
                        <span className="rdp-col-ip">{ua?.port || '--'}</span>
                        <span className="rdp-col-action">
                          {ua?.enabled ? (
                            <div className="rdp-ua-actions">
                              <button className="rdp-btn rdp-btn-connect" onClick={() => openConnectDialog(d.id, 'unattended')}>Connect</button>
                              <button className="rdp-btn rdp-btn-secondary" onClick={() => generateToken(d.id)}>Rotate</button>
                              <button className="rdp-btn rdp-btn-danger-sm" onClick={() => revokeToken(d.id)}>Revoke</button>
                            </div>
                          ) : (
                            <button className="rdp-btn rdp-btn-connect" onClick={() => generateToken(d.id)}>Enable</button>
                          )}
                        </span>
                        <span className="rdp-col-remarks">{ua?.last_used ? new Date(ua.last_used).toLocaleString() : 'Never'}</span>
                        <span className="rdp-col-platform">
                          {ua?.enabled ? <span className="rdp-badge rdp-badge-active">Active</span> : <span className="rdp-badge rdp-badge-inactive">Off</span>}
                        </span>
                      </div>
                    )
                  })}
                </div>
                {devices.filter((d: Device) => d.status === 'ONLINE').length === 0 && <div className="rdp-empty">No online devices available.</div>}
              </div>
            )}
            {tab === 'settings' && <div className="rdp-content"><div className="rdp-placeholder"><h3>Connection Settings</h3><p>Configure WebRTC STUN/TURN servers, connection timeout, and quality settings.</p></div></div>}
            {tab === 'recording' && <div className="rdp-content"><div className="rdp-placeholder"><h3>Screen Recording</h3><p>View and download recorded remote sessions.</p></div></div>}
            {tab === 'performance' && <div className="rdp-content"><div className="rdp-placeholder"><h3>Performance Metrics</h3><p>Real-time performance data during remote sessions.</p></div></div>}
            {tab === 'confirmation' && <div className="rdp-content"><div className="rdp-placeholder"><h3>User Confirmation</h3><p>Configure user consent prompts before remote connection.</p></div></div>}
          </>
        )}
        {tool === 'chat' && <div className="rdp-content"><div className="rdp-placeholder"><h3>Chat</h3><p>Send messages to remote users before or during a session.</p></div></div>}
        {tool === 'sysinfo' && <div className="rdp-content"><div className="rdp-placeholder"><h3>System Tools</h3><p>Remote system diagnostics and troubleshooting tools.</p></div></div>}
        {tool === 'wol' && <div className="rdp-content"><div className="rdp-placeholder"><h3>Wake on LAN</h3><p>Send magic packet to wake up offline devices.</p></div></div>}
        {tool === 'shutdown' && <div className="rdp-content"><div className="rdp-placeholder"><h3>Remote Shutdown</h3><p>Shutdown or restart multiple devices simultaneously.</p></div></div>}
        {tool === 'sysmgr' && <div className="rdp-content"><div className="rdp-placeholder"><h3>System Manager</h3><p>Process manager, service manager, and event viewer.</p></div></div>}
        {tool === 'announce' && <div className="rdp-content"><div className="rdp-placeholder"><h3>Announcement</h3><p>Broadcast messages to all or selected endpoints.</p></div></div>}
        {msg && <div className="rdp-toast">{msg}<button onClick={() => setMsg('')}>×</button></div>}
        {connectDevice && (
          <div className="rdp-modal-overlay" onClick={() => setConnectDevice(null)}>
            <div className="rdp-modal" onClick={e => e.stopPropagation()}>
              <div className="rdp-modal-head">
                <h3>{connectMode === 'unattended' ? 'Unattended Access' : `Connect via ${protocol}`}</h3>
                <button onClick={() => setConnectDevice(null)}>×</button>
              </div>
              <div className="rdp-modal-body">
                <p className="rdp-modal-info">Device: <strong>{connectDeviceName}</strong></p>
                {connectMode === 'unattended' ? (
                  <>
                    <div className="rdp-modal-ua-badge"><span className="rdp-badge rdp-badge-active">Unattended Access</span><span>Enter the device API token to connect without user interaction.</span></div>
                    <label className="rdp-modal-field"><span>Access Token</span><input value={unattendedToken} onChange={e => setUnattendedToken(e.target.value)} placeholder="ua_..." className="rdp-token-input" /></label>
                    <p className="rdp-modal-note">The token was generated from the Unattended Access tab. It provides secure access without exposing passwords.</p>
                  </>
                ) : (
                  <>
                    <p className="rdp-modal-info">Protocol: <strong>{protocol}</strong></p>
                    {protocol === 'RDP' && (
                      <>
                        <label className="rdp-modal-field"><span>Username</span><input value={rdpUser} onChange={e => setRdpUser(e.target.value)} placeholder="Administrator" /></label>
                        <label className="rdp-modal-field"><span>Password</span><input type="password" value={rdpPass} onChange={e => setRdpPass(e.target.value)} placeholder="Password" /></label>
                      </>
                    )}
                    {protocol === 'VNC' && (
                      <label className="rdp-modal-field"><span>VNC Password</span><input type="password" value={vncPass} onChange={e => setVncPass(e.target.value)} placeholder="VNC password" /></label>
                    )}
                    {protocol === 'WEBRTC' && <p className="rdp-modal-note">WebRTC requires the capture agent to be installed on the target device.</p>}
                  </>
                )}
              </div>
              <div className="rdp-modal-foot">
                <button className="rdp-btn rdp-btn-cancel" onClick={() => setConnectDevice(null)}>Cancel</button>
                {connectMode === 'unattended' ? (
                  <button className="rdp-btn rdp-btn-connect" onClick={connectWithToken} disabled={!unattendedToken}>Connect with Token</button>
                ) : (
                  <button className="rdp-btn rdp-btn-connect" onClick={() => connect(connectDevice)}>Start Session</button>
                )}
              </div>
            </div>
          </div>
        )}
        {tokenDialog && (
          <div className="rdp-modal-overlay" onClick={() => setTokenDialog(null)}>
            <div className="rdp-modal" onClick={e => e.stopPropagation()}>
              <div className="rdp-modal-head"><h3>Unattended Access Token</h3><button onClick={() => setTokenDialog(null)}>×</button></div>
              <div className="rdp-modal-body">
                <div className="rdp-token-display">
                  <p className="rdp-modal-info">Copy this token and store it securely. <strong>It will not be shown again.</strong></p>
                  <div className="rdp-token-box">
                    <code>{generatedToken}</code>
                    <button className="rdp-btn rdp-btn-copy" onClick={() => { navigator.clipboard?.writeText(generatedToken); setMsg('Token copied to clipboard') }}>Copy</button>
                  </div>
                  <p className="rdp-modal-note">Use this token in the Unattended Access tab to connect to this device without passwords.</p>
                </div>
              </div>
              <div className="rdp-modal-foot">
                <button className="rdp-btn rdp-btn-connect" onClick={() => setTokenDialog(null)}>Done</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
function RemoteFilesPage({ devices, selected }: any) {
  const [deviceId, setDeviceId] = useState(selected?.id ?? '')
  const [action, setAction] = useState<'browse' | 'retrieve' | 'upload' | 'delete' | 'rename'>('browse')
  const [path, setPath] = useState('/')
  const [newPath, setNewPath] = useState('')
  const [content, setContent] = useState('')
  const [msg, setMsg] = useState('')
  const run = async (e: FormEvent) => {
    e.preventDefault(); if (!deviceId || !path.trim()) return
    const params: Record<string, unknown> = { path }
    const cmdType: string = action
    let command: string = action
    if (action === 'rename') { params.old_path = path; params.new_path = newPath }
    if (action === 'upload') { params.content = content }
    const body = JSON.stringify({ command, command_type: cmdType, params })
    const resp = await fetch(`${API}/api/v1/devices/${deviceId}/rmm/execute`, {
      method: 'POST',
      headers: { ...authHeaders(), 'Content-Type': 'application/json' },
      body
    })
    const data = await resp.json()
    if (resp.ok) {
      setMsg(`Task ${data.task_id} queued`)
    } else {
      setMsg(data.error?.message ?? 'Failed')
    }
  }
  return <div className="panel"><div className="panel-head"><div><p className="eyebrow">FILE MANAGER</p><h2>Remote file operations</h2></div></div>
    <label className="rmm-device-select"><span>Target device</span>
      <select value={deviceId} onChange={e => setDeviceId(e.target.value)}><option value="">-- Select --</option>{devices.filter((d: Device) => d.status === 'ONLINE').map((d: Device) => <option key={d.id} value={d.id}>{d.hostname}</option>)}</select>
    </label>
    <div className="rmm-action">
      <div className="rmm-shell-select">{(['browse', 'retrieve', 'upload', 'delete', 'rename'] as const).map(a => <button key={a} className={action === a ? 'active' : ''} onClick={() => setAction(a)}>{a.charAt(0).toUpperCase() + a.slice(1)}</button>)}</div>
      <form onSubmit={run}>
        <input value={path} onChange={e => setPath(e.target.value)} placeholder="Path" />
        {action === 'rename' && <input value={newPath} onChange={e => setNewPath(e.target.value)} placeholder="New path" />}
        {action === 'upload' && <textarea value={content} onChange={e => setContent(e.target.value)} placeholder="Content" rows={4} />}
        <button>Execute</button>
      </form>
      {msg && <small>{msg}</small>}
    </div>
  </div>
}

// ── Audit ──
function AuditPage() {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [loading, setLoading] = useState(true)
  useEffect(() => { (async () => { const r = await fetch(`${API}/api/v1/audit-logs`, { headers: authHeaders() }); if (r.ok) { const d = await r.json(); setLogs(d.data || []) } setLoading(false) })() }, [])
  return <div className="panel"><div className="panel-head"><div><p className="eyebrow">AUDIT TRAIL</p><h2>Audit logs</h2></div></div>
    {loading ? <p className="muted" style={{ padding: 22 }}>Loading...</p> : logs.length === 0 ? <p className="muted" style={{ padding: 22 }}>No audit logs yet.</p> : <div className="table"><div className="row heading"><span>Action</span><span>Resource</span><span>Details</span><span>Time</span></div>
      {logs.map(l => <div className="row" key={l.id}><span><strong>{l.action}</strong></span><span>{l.resource}</span><span className="rmm-output">{JSON.stringify(l.metadata || {}).slice(0, 80)}</span><span>{new Date(l.created_at).toLocaleString()}</span></div>)}
    </div>}
  </div>
}

// ── Coming Soon ──
function ComingSoon({ module }: { module: string }) {
  return <div className="panel empty page-empty"><strong>{module}</strong><span>This module is planned for the next delivery. No placeholder data is shown.</span></div>
}

// ── Shared Components ──
function DeviceTable({ devices, onSelect, onAdd }: { devices: Device[]; onSelect: (d: Device) => void; onAdd: () => void }) {
  return devices.length ? <div className="table"><div className="row heading"><span>Device</span><span>Platform</span><span>Agent</span><span>State</span></div>
    {devices.map(d => <button className="row clickable" key={d.id} onClick={() => onSelect(d)}>
      <span><strong>{d.hostname}</strong><small>{d.os_name || d.platform}</small></span><span>{d.platform}</span><span>{d.agent_version}</span><span><i className={`dot ${d.status.toLowerCase()}`} />{d.status}</span>
    </button>)}
  </div> : <Empty onAdd={onAdd} />
}
function Enrollment({ token }: { token: string }) {
  const wc = `.\\cyverra-agent-windows-amd64.exe --api ${API} --enrollment-token ${token}`
  const lc = `./cyverra-agent-linux-amd64 --api ${API} --enrollment-token ${token}`
  const copy = (c: string) => navigator.clipboard?.writeText(c)
  return <div className="enrollment"><strong>Enrollment token</strong><span>Install the agent and run the command within 24 hours.</span><code>{token}</code>
    <label>Windows<code>{wc}</code><button onClick={() => copy(wc)}>Copy</button></label>
    <label>Linux<code>{lc}</code><button onClick={() => copy(lc)}>Copy</button></label>
  </div>
}
function AgentDownloads() {
  return <div className="agent-downloads"><span><strong>Agent downloads</strong></span>
    <a href="/downloads/cyverra-agent-windows-amd64.exe" download>Windows</a>
    <a href="/downloads/cyverra-agent-linux-amd64" download>Linux</a>
  </div>
}
function MetricHistory({ metrics }: { metrics: Metric[] }) {
  const recent = [...metrics].reverse(); const latest = recent[recent.length - 1]; const maxCpu = Math.max(100, ...recent.map(m => m.cpu_percent ?? 0))
  return <div className="metric-history"><div className="section-title"><h3>Metrics history</h3><span>{recent.length} samples</span></div>
    {recent.length ? <><div className="metric-chart">{recent.map((m, i) => <div className="metric-bar" key={i}><i style={{ height: `${Math.min(100, ((m.cpu_percent ?? 0) / maxCpu) * 100)}%` }} /></div>)}</div>
    <div className="metric-latest"><span>CPU <strong>{latest?.cpu_percent?.toFixed(1) ?? '--'}%</strong></span><span>RAM <strong>{latest?.memory_total_bytes ? `${Math.round((latest.memory_used_bytes ?? 0) / latest.memory_total_bytes * 100)}%` : '--'}</strong></span></div></> : <p className="muted">Waiting for metrics.</p>}
  </div>
}
function Pulse({ label, text, tone }: { label: string; text: string; tone: string }) { return <div className="pulse-line"><i className={`dot ${tone}`} /><span><strong>{label}</strong><small>{text}</small></span><b>Live</b></div> }
function Info({ label, value }: { label: string; value: string }) { return <div><span>{label}</span><strong>{value}</strong></div> }
function Stat({ label, value, tone }: { label: string; value: string | number; tone: string }) { return <div className={`stat ${tone}`}><span>{label}</span><strong>{value}</strong><i /></div> }
function Empty({ onAdd }: { onAdd: () => void }) { return <div className="empty"><strong>No devices</strong><span>Create an enrollment token and install the agent.</span><button onClick={onAdd}>+ Add device</button></div> }
function formatBytes(v?: number) { if (!v) return '--'; const u = ['B', 'KB', 'MB', 'GB', 'TB']; let s = v; let i = 0; while (s >= 1024 && i < u.length - 1) { s /= 1024; i++ } return `${s.toFixed(i ? 1 : 0)} ${u[i]}` }
function Login({ onSuccess }: { onSuccess: () => void }) {
  const [email, setEmail] = useState('admin@cyverra.local'); const [password, setPassword] = useState(''); const [error, setError] = useState('')
  const submit = async (e: FormEvent) => { e.preventDefault(); const r = await fetch(`${API}/api/v1/auth/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email, password }) }); const b = await r.json(); if (!r.ok) { setError(b.error?.message ?? 'Failed'); return } localStorage.setItem('token', b.token); onSuccess() }
  return <div className="login"><form onSubmit={submit}><span className="mark">C</span><p className="eyebrow">CYVERRA NEXUS</p><h1>Sign in</h1><p className="subhead">Access your workspace.</p>
    <label>Email<input value={email} onChange={e => setEmail(e.target.value)} type="email" required /></label>
    <label>Password<input value={password} onChange={e => setPassword(e.target.value)} type="password" required /></label>
    {error && <div className="notice">{error}</div>}<button>Sign in</button></form></div>
}

createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>)
