import { useState, useEffect, useMemo } from 'react'
import { api } from '../api'
import type { AppDetail, AppInput, ConfigDefaultsResponse, InstallRequest, DevicePassthrough, Install } from '../types'
import { Badge, SectionTitle, FormRow, FormInput } from '../components/ui'
import { DirectoryBrowser } from '../components/DirectoryBrowser'

export function InstallWizard({ app, onClose, replaceExisting, keepVolumes, existingInstall }: { app: AppDetail; onClose: () => void; replaceExisting?: boolean; keepVolumes?: string[]; existingInstall?: Install | null }) {
  const prev = existingInstall // shorthand for previous install values
  const [inputs, setInputs] = useState<Record<string, string>>(() => {
    if (prev?.inputs) return { ...prev.inputs }
    const d: Record<string, string> = {}
    app.inputs?.forEach(i => { if (i.default !== undefined) d[i.key] = String(i.default) })
    return d
  })
  const [cores, setCores] = useState(prev ? String(prev.cores) : String(app.lxc.defaults.cores))
  const [memory, setMemory] = useState(prev ? String(prev.memory_mb) : String(app.lxc.defaults.memory_mb))
  const [disk, setDisk] = useState(prev ? String(prev.disk_gb) : String(app.lxc.defaults.disk_gb))
  const [storage, setStorage] = useState(prev?.storage || '')
  const [bridge, setBridge] = useState(prev?.bridge || '')
  const [hostname, setHostname] = useState(prev?.hostname || '')
  const [ipAddress, setIpAddress] = useState(prev?.ip_address || '')
  const [macAddress, setMacAddress] = useState(prev?.mac_address || '')
  const [onboot, setOnboot] = useState(prev?.onboot ?? app.lxc.defaults.onboot ?? true)
  const [unprivileged, setUnprivileged] = useState(prev?.unprivileged ?? app.lxc.defaults.unprivileged ?? true)
  const [installing, setInstalling] = useState(false)
  const [error, setError] = useState('')
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [defaults, setDefaults] = useState<ConfigDefaultsResponse | null>(null)
  const [bindMounts, setBindMounts] = useState<Record<string, string>>(() => {
    // Pre-fill bind mount host paths from existing install's mount points
    if (prev?.mount_points) {
      const d: Record<string, string> = {}
      for (const mp of prev.mount_points) {
        if (mp.type === 'bind' && mp.host_path) d[mp.name] = mp.host_path
      }
      if (Object.keys(d).length > 0) return d
    }
    const d: Record<string, string> = {}
    app.volumes?.filter(v => v.type === 'bind' && v.default_host_path).forEach(v => { d[v.name] = v.default_host_path! })
    return d
  })
  const [extraMounts, setExtraMounts] = useState<{ host_path: string; mount_path: string; read_only: boolean }[]>([])
  const [storageInputMounts, setStorageInputMounts] = useState<Record<string, string>>({})
  const [volumeStorages, setVolumeStorages] = useState<Record<string, string>>(() => {
    // Pre-fill per-volume storage from existing install
    if (prev?.mount_points) {
      const d: Record<string, string> = {}
      for (const mp of prev.mount_points) {
        if (mp.type === 'volume' && mp.storage) d[mp.name] = mp.storage
      }
      return d
    }
    return {}
  })
  const [volumeBindOverrides, setVolumeBindOverrides] = useState<Record<string, string>>({})
  const [customVars, setCustomVars] = useState<{key: string; value: string}[]>([])
  const [devices, setDevices] = useState<DevicePassthrough[]>(prev?.devices || [])
  const [envVars] = useState<Record<string, string>>(prev?.env_vars || {})
  const [envVarList, setEnvVarList] = useState<{key: string; value: string}[]>([])
  const [browseTarget, setBrowseTarget] = useState<string | null>(null)
  const [browseInitPath, setBrowseInitPath] = useState('/')

  useEffect(() => {
    api.configDefaults().then(d => {
      setDefaults(d)
      // Only set defaults if no previous install values
      if (!prev?.storage) setStorage(s => s || d.storages[0] || '')
      if (!prev?.bridge) setBridge(b => b || d.bridges[0] || '')
    }).catch(() => {})
  }, [])

  const volumeVolumes = (app.volumes || []).filter(v => (v.type || 'volume') === 'volume')
  const bindVolumes = (app.volumes || []).filter(v => v.type === 'bind')
  const hasMounts = volumeVolumes.length > 0 || bindVolumes.length > 0

  // Build a lookup map for storage details (capacity info)
  const storageDetailMap = useMemo(() => {
    const m = new Map<string, { type: string; total_gb: number; available_gb: number }>()
    if (defaults?.storage_details) {
      for (const sd of defaults.storage_details) {
        m.set(sd.id, { type: sd.type, total_gb: sd.total_gb || 0, available_gb: sd.available_gb || 0 })
      }
    }
    return m
  }, [defaults])

  const formatSize = (gb: number) => gb >= 1024 ? `${(gb / 1024).toFixed(1)} TB` : `${gb} GB`

  const storageLabel = (id: string) => {
    const sd = storageDetailMap.get(id)
    if (!sd || !sd.total_gb) return id
    return `${id}  (${sd.type})  ${formatSize(sd.available_gb)} free / ${formatSize(sd.total_gb)}`
  }

  // Build a lookup map for bridge details (CIDR, comment, VLANs)
  const bridgeDetailMap = useMemo(() => {
    const m = new Map<string, { cidr?: string; gateway?: string; comment?: string; vlan_aware?: boolean; vlans?: string }>()
    if (defaults?.bridge_details) {
      for (const bd of defaults.bridge_details) {
        m.set(bd.name, { cidr: bd.cidr, gateway: bd.gateway, comment: bd.comment, vlan_aware: bd.vlan_aware, vlans: bd.vlans })
      }
    }
    return m
  }, [defaults])

  const bridgeLabel = (name: string) => {
    const bd = bridgeDetailMap.get(name)
    if (!bd) return name
    const parts = [name]
    if (bd.cidr) {
      const slashIdx = bd.cidr.indexOf('/')
      if (slashIdx > 0) {
        const prefix = bd.cidr.substring(slashIdx)
        const octets = bd.cidr.substring(0, slashIdx).split('.')
        if (octets.length === 4) {
          const mask = parseInt(prefix.substring(1))
          if (mask <= 8) octets[1] = octets[2] = octets[3] = '0'
          else if (mask <= 16) octets[2] = octets[3] = '0'
          else if (mask <= 24) octets[3] = '0'
          parts.push(`${octets.join('.')}${prefix}`)
        } else {
          parts.push(bd.cidr)
        }
      }
    } else {
      parts.push('(no IP)')
    }
    if (bd.vlan_aware && bd.vlans) parts.push(`VLAN ${bd.vlans}`)
    else if (bd.vlan_aware) parts.push('VLAN-aware')
    if (bd.comment) parts.push(bd.comment)
    return parts.join('    ')
  }

  // Input validation
  const inputErrors = useMemo(() => {
    const errors: Record<string, string> = {}
    for (const inp of (app.inputs || [])) {
      const val = inputs[inp.key] || ''
      if (inp.type === 'boolean') continue
      if (inp.required && !val) { errors[inp.key] = 'Required'; continue }
      if (!val) continue
      const v = inp.validation
      if (!v) continue
      if (inp.type === 'number') {
        const num = parseFloat(val)
        if (isNaN(num)) { errors[inp.key] = 'Must be a number'; continue }
        if (v.min !== undefined && num < v.min) { errors[inp.key] = `Minimum ${v.min}`; continue }
        if (v.max !== undefined && num > v.max) { errors[inp.key] = `Maximum ${v.max}`; continue }
      }
      if (inp.type === 'string' || inp.type === 'secret') {
        if (v.min_length !== undefined && val.length < v.min_length) { errors[inp.key] = `At least ${v.min_length} characters`; continue }
        if (v.max_length !== undefined && val.length > v.max_length) { errors[inp.key] = `At most ${v.max_length} characters`; continue }
        if (v.regex) { try { if (!new RegExp(v.regex).test(val)) errors[inp.key] = `Does not match required pattern` } catch {} }
      }
      if (v.enum && v.enum.length > 0 && !v.enum.includes(val)) { errors[inp.key] = 'Invalid selection' }
    }
    return errors
  }, [inputs, app.inputs])
  const hasInputErrors = Object.keys(inputErrors).length > 0

  const mountErrors = useMemo(() => {
    const errors: Record<string, string> = {}
    const pathMap: Record<string, string[]> = {}
    for (const vol of bindVolumes) {
      const hp = bindMounts[vol.name]
      if (hp) {
        if (!pathMap[hp]) pathMap[hp] = []
        pathMap[hp].push(vol.name)
      }
    }
    for (const vol of volumeVolumes) {
      const hp = volumeBindOverrides[vol.name]
      if (hp) {
        if (!pathMap[hp]) pathMap[hp] = []
        pathMap[hp].push(vol.name)
      }
    }
    for (const [hp, names] of Object.entries(pathMap)) {
      if (names.length > 1) {
        for (const name of names) {
          errors[name] = `Duplicate host path "${hp}" — each volume must use a unique directory`
        }
      }
    }
    return errors
  }, [bindMounts, volumeBindOverrides, bindVolumes, volumeVolumes])
  const hasMountErrors = Object.keys(mountErrors).length > 0

  const openBrowser = (target: string, currentPath?: string) => {
    setBrowseTarget(target)
    setBrowseInitPath(currentPath || '')
  }

  const handleBrowseSelect = (path: string) => {
    if (!browseTarget) return
    if (browseTarget.startsWith('extra-')) {
      const idx = parseInt(browseTarget.replace('extra-', ''))
      setExtraMounts(p => p.map((em, i) => i === idx ? { ...em, host_path: path } : em))
    } else if (browseTarget.startsWith('storage-')) {
      const key = browseTarget.replace('storage-', '')
      setStorageInputMounts(p => ({ ...p, [key]: path }))
    } else if (browseTarget.startsWith('volbind-')) {
      const name = browseTarget.replace('volbind-', '')
      setVolumeBindOverrides(p => ({ ...p, [name]: path }))
    } else {
      setBindMounts(p => ({ ...p, [browseTarget]: path }))
    }
    setBrowseTarget(null)
  }

  const handleInstall = async () => {
    setInstalling(true); setError('')
    try {
      // Merge custom variables into inputs
      const allInputs = { ...inputs }
      for (const cv of customVars) {
        if (cv.key.trim()) allInputs[cv.key.trim()] = cv.value
      }
      const req: Record<string, unknown> = {
        cores: parseInt(cores) || undefined,
        memory_mb: parseInt(memory) || undefined,
        disk_gb: parseInt(disk) || undefined,
        storage: storage || undefined,
        bridge: bridge || undefined,
        hostname: hostname || undefined,
        ip_address: ipAddress || undefined,
        mac_address: macAddress || undefined,
        onboot,
        unprivileged,
        inputs: allInputs,
      }
      // Merge bind mounts: manifest bind volumes + volume-to-bind overrides
      const allBindMounts = { ...bindMounts }
      for (const [name, hp] of Object.entries(volumeBindOverrides)) {
        if (hp) allBindMounts[name] = hp
      }
      if (Object.keys(allBindMounts).length > 0) req.bind_mounts = allBindMounts
      // Per-volume storage overrides (only for volumes NOT overridden to bind)
      const vs: Record<string, string> = {}
      for (const [name, st] of Object.entries(volumeStorages)) {
        if (st && st !== storage && !volumeBindOverrides[name]) vs[name] = st
      }
      if (Object.keys(vs).length > 0) req.volume_storages = vs
      // Merge storage input mounts into extra mounts
      const allExtras = [...extraMounts.filter(em => em.host_path && em.mount_path)]
      for (const [key, hostPath] of Object.entries(storageInputMounts)) {
        if (hostPath) {
          const inp = app.inputs?.find(i => i.key === key)
          allExtras.push({
            host_path: hostPath,
            mount_path: inputs[key] || String(inp?.default || ''),
            read_only: false,
          })
        }
      }
      if (allExtras.length > 0) req.extra_mounts = allExtras
      // Device passthrough
      const validDevices = devices.filter(d => d.path.trim())
      if (validDevices.length > 0) req.devices = validDevices
      // Environment variables (merge overrides + custom)
      const allEnv: Record<string, string> = { ...envVars }
      for (const ev of envVarList) {
        if (ev.key.trim()) allEnv[ev.key.trim()] = ev.value
      }
      if (Object.keys(allEnv).length > 0) req.env_vars = allEnv
      if (replaceExisting) req.replace_existing = true
      if (keepVolumes && keepVolumes.length > 0) req.keep_volumes = keepVolumes
      const job = await api.installApp(app.id, req as InstallRequest)
      window.location.hash = `#/job/${job.id}`
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Install failed')
      setInstalling(false)
    }
  }

  // Split inputs into storage-group (with path defaults) and the rest
  // Fix 2: Skip storage inputs whose default path is under a managed volume
  const managedMountPaths = volumeVolumes.map(v => v.mount_path)
  const storagePathInputs: AppInput[] = []
  const otherInputGroups: Record<string, AppInput[]> = {}
  if (app.inputs) {
    for (const inp of app.inputs) {
      if (inp.group === 'Storage' && typeof inp.default === 'string' && inp.default.startsWith('/')) {
        const isUnderManagedVolume = managedMountPaths.some(mp =>
          inp.default === mp || (inp.default as string).startsWith(mp + '/')
        )
        if (!isUnderManagedVolume) {
          storagePathInputs.push(inp)
        }
      } else {
        const g = inp.group || 'General'
        if (!otherInputGroups[g]) otherInputGroups[g] = []
        otherInputGroups[g].push(inp)
      }
    }
  }
  const hasOtherInputs = Object.keys(otherInputGroups).length > 0

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]">
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[700px] max-h-[90vh] overflow-auto">
        <h2 className="text-xl font-bold text-text-primary mb-5 font-mono">Install {app.name}</h2>

        <SectionTitle>Resources</SectionTitle>
        <FormRow label="CPU Cores"><FormInput value={cores} onChange={setCores} type="number" /></FormRow>
        <FormRow label="Memory (MB)"><FormInput value={memory} onChange={setMemory} type="number" /></FormRow>
        <FormRow label="Disk (GB)" help="Root filesystem only — app data lives on separate volumes"><FormInput value={disk} onChange={setDisk} type="number" /></FormRow>

        <SectionTitle>Networking & Storage</SectionTitle>
        <FormRow label="Storage Pool" description="Proxmox storage where the container's virtual disk will be created." help={`Disk size: ${disk} GB`}>
          {defaults && defaults.storages.length > 1 ? (
            <select value={storage} onChange={e => setStorage(e.target.value)} className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
              {defaults.storages.map(s => <option key={s} value={s}>{storageLabel(s)}</option>)}
            </select>
          ) : (
            <span className="block px-3 py-2 bg-bg-primary border border-border rounded-md text-text-secondary text-sm font-mono">{storageLabel(storage)}</span>
          )}
        </FormRow>
        <FormRow label="Network Bridge" description="Proxmox virtual bridge that connects the container to your network." help="Container gets its own IP via DHCP on this bridge">
          {defaults && defaults.bridges.length > 1 ? (
            <select value={bridge} onChange={e => setBridge(e.target.value)} className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
              {defaults.bridges.map(b => <option key={b} value={b}>{bridgeLabel(b)}</option>)}
            </select>
          ) : (
            <span className="block px-3 py-2 bg-bg-primary border border-border rounded-md text-text-secondary text-sm font-mono">{bridgeLabel(bridge)}</span>
          )}
        </FormRow>

        {/* Unified Mounts section */}
        {(hasMounts || true) && (
          <>
            <SectionTitle>Mounts</SectionTitle>
            <div className="text-[11px] text-text-muted font-mono mb-2 border-l-2 border-primary/30 pl-2">
              Managed volumes survive container recreation. Bind mounts reference existing host paths.
            </div>

            {/* Managed volumes with toggle: PVE Volume vs Host Path */}
            {volumeVolumes.map(vol => {
              const isKept = keepVolumes && keepVolumes.includes(vol.name)
              const isBind = !isKept && volumeBindOverrides[vol.name] !== undefined
              if (isKept) {
                return (
                  <div key={vol.name} className="bg-bg-secondary rounded-lg p-3 mb-1.5 border border-primary/20">
                    <div className="flex justify-between items-center">
                      <div className="flex items-center gap-2">
                        <span className="text-sm text-text-primary">{vol.label || vol.name}</span>
                        <span className="text-xs text-text-muted font-mono">{vol.mount_path}</span>
                      </div>
                      <Badge className="bg-primary/10 text-primary">keeping data</Badge>
                    </div>
                    <div className="text-xs text-text-muted mt-1">Existing volume will be reattached to the new container.</div>
                  </div>
                )
              }
              return (
                <div key={vol.name} className="bg-bg-secondary rounded-lg p-3 mb-1.5">
                  <div className="flex justify-between items-center">
                    <div className="flex items-center gap-2">
                      <span className="text-sm text-text-primary">{vol.label || vol.name}</span>
                      <span className="text-xs text-text-muted font-mono">{vol.mount_path}</span>
                    </div>
                    {!isBind && <span className="text-sm text-text-muted font-mono">{vol.size_gb} GB</span>}
                  </div>
                  <div className="flex items-center gap-2 mt-1.5">
                    <button type="button" onClick={() => {
                      if (isBind) {
                        setVolumeBindOverrides(p => { const n = { ...p }; delete n[vol.name]; return n })
                      } else {
                        setVolumeBindOverrides(p => ({ ...p, [vol.name]: '' }))
                      }
                    }} className="text-[11px] text-primary bg-transparent border-none cursor-pointer p-0 font-mono hover:underline whitespace-nowrap">
                      {isBind ? 'use pve volume' : 'use host path'}
                    </button>
                    {isBind ? (
                      <>
                        <Badge className="bg-status-warning/10 text-status-warning">host path</Badge>
                      </>
                    ) : (
                      <>
                        {defaults && defaults.storages.length > 1 ? (
                          <select value={volumeStorages[vol.name] || storage}
                            onChange={e => setVolumeStorages(p => ({ ...p, [vol.name]: e.target.value }))}
                            className="px-2 py-1 text-xs bg-bg-primary border border-border rounded text-text-primary font-mono">
                            {defaults.storages.map(s => <option key={s} value={s}>{storageLabel(s)}</option>)}
                          </select>
                        ) : (
                          <span className="text-xs text-text-secondary font-mono">{storageLabel(storage)}</span>
                        )}
                        <Badge className="bg-primary/10 text-primary">pve volume</Badge>
                      </>
                    )}
                  </div>
                  {isBind && (
                    <div className="flex gap-2 mt-1.5">
                      <FormInput value={volumeBindOverrides[vol.name]} onChange={v => setVolumeBindOverrides(p => ({ ...p, [vol.name]: v }))} placeholder="/host/path" />
                      <button type="button" onClick={() => openBrowser(`volbind-${vol.name}`, volumeBindOverrides[vol.name] || '')}
                        className="px-3 py-2 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-primary hover:border-primary hover:text-primary transition-colors cursor-pointer whitespace-nowrap">
                        Browse
                      </button>
                    </div>
                  )}
                  {mountErrors[vol.name] && <div className="text-status-stopped text-xs mt-1 font-mono">{mountErrors[vol.name]}</div>}
                </div>
              )
            })}

            {/* Host path bind mounts from manifest */}
            {bindVolumes.map(vol => (
              <div key={vol.name} className="bg-bg-secondary rounded-lg p-3 mb-2">
                <div className="flex items-center justify-between mb-1.5">
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-text-primary">{vol.label || vol.name}</span>
                    <Badge className="bg-status-warning/10 text-status-warning">bind</Badge>
                    {!vol.required && <Badge className="bg-bg-primary text-text-muted">optional</Badge>}
                  </div>
                </div>
                <div className="flex gap-2 mb-1">
                  <FormInput value={bindMounts[vol.name] || ''} onChange={v => setBindMounts(p => ({ ...p, [vol.name]: v }))} placeholder="/host/path" />
                  <button type="button" onClick={() => openBrowser(vol.name, bindMounts[vol.name] || vol.default_host_path || '')}
                    className="px-3 py-2 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-primary hover:border-primary hover:text-primary transition-colors cursor-pointer whitespace-nowrap">
                    Browse
                  </button>
                </div>
                <div className="text-[11px] text-text-muted font-mono">Container path: {vol.mount_path}</div>
                {mountErrors[vol.name] && <div className="text-status-stopped text-xs mt-1 font-mono">{mountErrors[vol.name]}</div>}
              </div>
            ))}

            {/* Extra user-added mounts (stacked card layout) */}
            {extraMounts.map((em, i) => (
              <div key={i} className="bg-bg-secondary rounded-lg p-3 mb-2">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-text-muted font-mono">Extra Path #{i + 1}</span>
                  <button type="button" onClick={() => setExtraMounts(p => p.filter((_, j) => j !== i))}
                    className="text-status-stopped text-sm bg-transparent border-none cursor-pointer hover:text-status-stopped/80 leading-none px-1">&times;</button>
                </div>
                <div className="flex gap-2 mb-1.5">
                  <input type="text" value={em.host_path} onChange={e => setExtraMounts(p => p.map((x, j) => j === i ? { ...x, host_path: e.target.value } : x))} placeholder="/host/path"
                    className="flex-1 px-3 py-2 bg-bg-primary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
                  <button type="button" onClick={() => openBrowser(`extra-${i}`, em.host_path || '')}
                    className="px-3 py-2 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-primary hover:border-primary hover:text-primary transition-colors cursor-pointer">
                    Browse
                  </button>
                </div>
                <div className="flex gap-2 items-center">
                  <input type="text" value={em.mount_path} onChange={e => setExtraMounts(p => p.map((x, j) => j === i ? { ...x, mount_path: e.target.value } : x))} placeholder="/container/path"
                    className="flex-1 px-3 py-2 bg-bg-primary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
                  <label className="flex items-center gap-1.5 text-xs text-text-muted whitespace-nowrap cursor-pointer">
                    <input type="checkbox" checked={em.read_only} onChange={e => setExtraMounts(p => p.map((x, j) => j === i ? { ...x, read_only: e.target.checked } : x))} className="w-3.5 h-3.5 accent-primary" />
                    Read-only
                  </label>
                </div>
              </div>
            ))}

            <button type="button" onClick={() => setExtraMounts(p => [...p, { host_path: '', mount_path: '', read_only: false }])}
              className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0">
              + Add Path
            </button>
          </>
        )}

        {/* App inputs (non-storage-path groups) */}
        {hasOtherInputs && (
          <div className="mt-4 mb-1 text-[11px] text-text-muted font-mono border-l-2 border-primary/30 pl-2">
            All app settings below apply inside the LXC container, not on the Proxmox host.
          </div>
        )}
        {Object.entries(otherInputGroups).map(([group, groupInps]) => {
          const visibleInps = groupInps.filter(inp => {
            if (!inp.show_when) return true
            const depVal = inputs[inp.show_when.input] || ''
            return inp.show_when.values.includes(depVal)
          })
          if (visibleInps.length === 0) return null
          return (
          <div key={group}>
            <SectionTitle>{group}</SectionTitle>
            {visibleInps.map(inp => (
              <FormRow key={inp.key} label={inp.label} help={inp.help} description={inp.description}>
                {inp.type === 'select' && inp.validation?.enum ? (
                  <select value={inputs[inp.key] || ''} onChange={e => setInputs(p => ({ ...p, [inp.key]: e.target.value }))} className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
                    {inp.validation.enum.map(v => <option key={v} value={v}>{v}</option>)}
                  </select>
                ) : inp.type === 'boolean' ? (
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input type="checkbox" checked={inputs[inp.key] === 'true'} onChange={e => setInputs(p => ({ ...p, [inp.key]: e.target.checked ? 'true' : 'false' }))}
                      className="w-4 h-4 accent-primary" />
                    <span className="text-sm text-text-secondary">Enable</span>
                  </label>
                ) : (
                  <FormInput value={inputs[inp.key] || ''} onChange={v => setInputs(p => ({ ...p, [inp.key]: v }))}
                    type={inp.type === 'secret' ? 'password' : inp.type === 'number' ? 'number' : 'text'} />
                )}
                {inputErrors[inp.key] && <div className="text-status-stopped text-xs mt-0.5 font-mono">{inputErrors[inp.key]}</div>}
              </FormRow>
            ))}
          </div>
          )
        })}

        {/* Storage-path inputs with optional "mount externally" toggle */}
        {storagePathInputs.length > 0 && (
          <>
            <SectionTitle>Storage</SectionTitle>
            {storagePathInputs.map(inp => (
              <div key={inp.key} className="mb-3">
                <label className="text-sm text-text-secondary block mb-1">{inp.label}</label>
                {inp.description && <div className="text-xs text-text-muted mb-1.5 leading-relaxed">{inp.description}</div>}
                <FormInput value={inputs[inp.key] || ''} onChange={v => setInputs(p => ({ ...p, [inp.key]: v }))} />
                {inp.help && <div className="text-[11px] text-text-muted mt-0.5 italic">{inp.help}</div>}
                <div className="mt-1.5">
                  <label className="flex items-center gap-2 text-xs text-text-muted cursor-pointer">
                    <input type="checkbox" checked={!!storageInputMounts[inp.key]}
                      onChange={e => {
                        if (e.target.checked) {
                          setStorageInputMounts(p => ({ ...p, [inp.key]: '' }))
                        } else {
                          setStorageInputMounts(p => { const n = { ...p }; delete n[inp.key]; return n })
                        }
                      }}
                      className="w-3.5 h-3.5 accent-primary" />
                    Mount from host path
                  </label>
                  {storageInputMounts[inp.key] !== undefined && (
                    <div className="flex gap-2 mt-1.5">
                      <FormInput value={storageInputMounts[inp.key]} onChange={v => setStorageInputMounts(p => ({ ...p, [inp.key]: v }))} placeholder="/host/path" />
                      <button type="button" onClick={() => openBrowser(`storage-${inp.key}`, storageInputMounts[inp.key] || '')}
                        className="px-3 py-2 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-secondary hover:border-primary hover:text-primary transition-colors cursor-pointer whitespace-nowrap">
                        Browse
                      </button>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </>
        )}

        {/* Custom variables */}
        <SectionTitle>Custom Config</SectionTitle>
        {customVars.map((v, i) => (
          <div key={i} className="flex gap-2 mb-1.5 items-center">
            <input type="text" value={v.key} onChange={e => setCustomVars(p => p.map((x, j) => j === i ? { ...x, key: e.target.value } : x))} placeholder="KEY"
              className="w-1/3 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted uppercase" />
            <input type="text" value={v.value} onChange={e => setCustomVars(p => p.map((x, j) => j === i ? { ...x, value: e.target.value } : x))} placeholder="value"
              className="flex-1 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
            <button type="button" onClick={() => setCustomVars(p => p.filter((_, j) => j !== i))}
              className="text-status-stopped text-sm bg-transparent border-none cursor-pointer hover:text-status-stopped/80 leading-none px-1">&times;</button>
          </div>
        ))}
        <button type="button" onClick={() => setCustomVars(p => [...p, { key: '', value: '' }])}
          className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0">
          + Add Variable
        </button>
        <div className="text-[11px] text-text-muted mt-2">
          Ports: LXC containers get their own IP — all ports are directly accessible.
        </div>

        {/* Device Passthrough */}
        {(app.gpu.supported && app.gpu.supported.length > 0) && (
          <>
            <SectionTitle>Device Passthrough</SectionTitle>
            {app.gpu.profiles && app.gpu.profiles.length > 0 && (
              <div className="text-xs text-text-muted mb-2 font-mono">GPU profiles: {app.gpu.profiles.join(', ')}</div>
            )}
            <label className="flex items-center gap-2 text-xs text-text-muted cursor-pointer mb-2">
              <input type="checkbox" checked={devices.length > 0}
                onChange={e => {
                  if (e.target.checked) {
                    // Auto-populate from GPU profiles
                    const profileDevs: DevicePassthrough[] = []
                    for (const profile of (app.gpu.profiles || [])) {
                      if (profile === 'dri-render') profileDevs.push({ path: '/dev/dri/renderD128', gid: 44, mode: '0666' })
                      else if (profile === 'nvidia-basic') {
                        profileDevs.push({ path: '/dev/nvidia0' }, { path: '/dev/nvidiactl' }, { path: '/dev/nvidia-uvm' })
                      }
                    }
                    setDevices(profileDevs.length > 0 ? profileDevs : [{ path: '' }])
                  } else setDevices([])
                }}
                className="w-3.5 h-3.5 accent-primary" />
              Enable GPU/device passthrough
            </label>
            {devices.map((dev, i) => (
              <div key={i} className="flex gap-2 mb-1.5 items-center">
                <input type="text" value={dev.path} onChange={e => setDevices(p => p.map((x, j) => j === i ? { ...x, path: e.target.value } : x))} placeholder="/dev/dri/renderD128"
                  className="flex-1 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
                <button type="button" onClick={() => setDevices(p => p.filter((_, j) => j !== i))}
                  className="text-status-stopped text-sm bg-transparent border-none cursor-pointer hover:text-status-stopped/80 leading-none px-1">&times;</button>
              </div>
            ))}
            {devices.length > 0 && (
              <button type="button" onClick={() => setDevices(p => [...p, { path: '' }])}
                className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0">
                + Add Device
              </button>
            )}
          </>
        )}

        {/* Environment Variables */}
        {(app.provisioning as { env?: Record<string, string> }).env && Object.keys((app.provisioning as { env?: Record<string, string> }).env || {}).length > 0 && (
          <>
            <SectionTitle>Environment Variables</SectionTitle>
            <div className="text-[11px] text-text-muted font-mono mb-2 border-l-2 border-primary/30 pl-2">
              Manifest defaults (passed to provision script):
            </div>
            {Object.entries((app.provisioning as { env?: Record<string, string> }).env || {}).map(([k, v]) => (
              <div key={k} className="flex gap-2 mb-1 text-xs font-mono">
                <span className="text-text-muted w-1/3 truncate">{k}</span>
                <span className="text-text-secondary flex-1 truncate">{envVars[k] ?? v}</span>
              </div>
            ))}
          </>
        )}
        {envVarList.length > 0 && (
          <>
            {!((app.provisioning as { env?: Record<string, string> }).env && Object.keys((app.provisioning as { env?: Record<string, string> }).env || {}).length > 0) && <SectionTitle>Environment Variables</SectionTitle>}
            {envVarList.map((ev, i) => (
              <div key={i} className="flex gap-2 mb-1.5 items-center">
                <input type="text" value={ev.key} onChange={e => setEnvVarList(p => p.map((x, j) => j === i ? { ...x, key: e.target.value } : x))} placeholder="ENV_KEY"
                  className="w-1/3 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted uppercase" />
                <input type="text" value={ev.value} onChange={e => setEnvVarList(p => p.map((x, j) => j === i ? { ...x, value: e.target.value } : x))} placeholder="value"
                  className="flex-1 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
                <button type="button" onClick={() => setEnvVarList(p => p.filter((_, j) => j !== i))}
                  className="text-status-stopped text-sm bg-transparent border-none cursor-pointer hover:text-status-stopped/80 leading-none px-1">&times;</button>
              </div>
            ))}
          </>
        )}
        <button type="button" onClick={() => setEnvVarList(p => [...p, { key: '', value: '' }])}
          className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0 mt-1">
          + Add Env Var
        </button>

        {/* Advanced settings */}
        <div className="mt-5">
          <button onClick={() => setShowAdvanced(!showAdvanced)} className="bg-transparent border-none text-primary text-sm cursor-pointer p-0 font-mono hover:underline">
            {showAdvanced ? '- hide' : '+ show'} advanced
          </button>
          {showAdvanced && (
            <div className="mt-3 space-y-3">
              <FormRow label="Hostname" description="Container hostname on the network" help={`Defaults to: ${app.id}`}>
                <FormInput value={hostname} onChange={setHostname} placeholder={app.id} />
              </FormRow>
              <FormRow label="Static IP" description="Fixed IP address for this container" help="Leave blank for DHCP">
                <FormInput value={ipAddress} onChange={setIpAddress} placeholder="e.g. 192.168.1.100" />
              </FormRow>
              <FormRow label="MAC Address" description="Fixed MAC for DHCP reservations" help="Leave blank for auto-assign">
                <FormInput value={macAddress} onChange={setMacAddress} placeholder="e.g. BC:24:11:AB:CD:EF" />
              </FormRow>
              <FormRow label="Start on Boot">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={onboot} onChange={e => setOnboot(e.target.checked)} className="w-4 h-4 accent-primary" />
                  <span className="text-sm text-text-secondary">Auto-start when Proxmox host boots</span>
                </label>
              </FormRow>
              <FormRow label="Unprivileged">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={unprivileged} onChange={e => setUnprivileged(e.target.checked)} className="w-4 h-4 accent-primary" />
                  <span className="text-sm text-text-secondary">Run as unprivileged container (recommended)</span>
                </label>
              </FormRow>
              <div className="text-[11px] text-text-muted font-mono mt-2">
                OS: {app.lxc.ostemplate} | Features: {(app.lxc.defaults.features || []).join(', ') || 'none'}
              </div>
            </div>
          )}
        </div>

        {error && <div className="text-status-stopped text-sm mt-3 font-mono">{error}</div>}

        <div className="flex gap-3 mt-6 justify-end">
          <button onClick={onClose} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={handleInstall} disabled={installing || hasInputErrors || hasMountErrors} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 disabled:cursor-not-allowed font-mono">
            {installing ? 'Installing...' : 'Install'}
          </button>
        </div>
      </div>
      {browseTarget !== null && (
        <DirectoryBrowser initialPath={browseInitPath} onSelect={handleBrowseSelect} onClose={() => setBrowseTarget(null)} />
      )}
    </div>
  )
}
